package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/internal/ai"
	"pidex/internal/audit"
	"pidex/internal/identity"
	"pidex/internal/pipeline"
	"pidex/internal/store"
	"pidex/internal/store/dbq"
	"pidex/internal/teamconfig"
)

type captureService struct{ *Server }

var uploadTypes = map[string]bool{"image/jpeg": true, "image/png": true}

func (s *captureService) CreateUpload(ctx context.Context, req *connect.Request[pidexv1.CreateUploadRequest]) (*connect.Response[pidexv1.CreateUploadResponse], error) {
	pr := principalFrom(ctx)
	if err := requireWrite(pr); err != nil {
		return nil, err
	}
	m := req.Msg
	switch {
	case m.ClientPhotoId == "" || len(m.ClientPhotoId) > 128:
		return nil, invalid("client_photo_id must be 1 to 128 characters")
	case !uploadTypes[m.ContentType]:
		return nil, invalid("content_type must be image/jpeg or image/png")
	case m.SizeBytes <= 0 || m.SizeBytes > s.MaxUploadBytes:
		return nil, invalid("size_bytes must be between 1 and %d", s.MaxUploadBytes)
	}

	id := uuid.New()
	// Object keys carry no identifying information.
	key := fmt.Sprintf("teams/%s/photos/%s/%s", pr.TeamID, time.Now().UTC().Format("2006/01"), id)
	row, err := s.Q.CreatePhotoUpload(ctx, dbq.CreatePhotoUploadParams{
		ID: id, TeamID: pr.TeamID, UploadedBy: pr.UserID, ClientPhotoID: m.ClientPhotoId,
		ObjectKey: key, ContentType: m.ContentType, SizeBytes: m.SizeBytes, Sha256Hex: strings.ToLower(m.Sha256Hex),
	})
	if err != nil {
		return nil, internal(err)
	}
	resp := &pidexv1.CreateUploadResponse{PhotoId: row.ID.String()}
	if row.Status != "awaiting_upload" {
		resp.AlreadyUploaded = true
		return connect.NewResponse(resp), nil
	}
	url, headers, err := s.Storage.PresignPut(ctx, row.ObjectKey, row.ContentType, row.SizeBytes, s.UploadURLTTL)
	if err != nil {
		return nil, internal(err)
	}
	resp.UploadUrl, resp.UploadHeaders = url, headers
	resp.ExpiresAt = timestamppb.New(time.Now().Add(s.UploadURLTTL))
	return connect.NewResponse(resp), nil
}

func (s *captureService) CompleteUpload(ctx context.Context, req *connect.Request[pidexv1.CompleteUploadRequest]) (*connect.Response[pidexv1.CompleteUploadResponse], error) {
	pr := principalFrom(ctx)
	if err := requireWrite(pr); err != nil {
		return nil, err
	}
	m := req.Msg
	id, err := parseID("photo_id", m.PhotoId)
	if err != nil {
		return nil, err
	}
	sessionID, err := parseOptionalID("session_id", m.SessionId)
	if err != nil {
		return nil, err
	}
	lockedID, err := parseOptionalID("locked_subject_id", m.LockedSubjectId)
	if err != nil {
		return nil, err
	}
	photo, err := s.Q.GetPhoto(ctx, dbq.GetPhotoParams{TeamID: pr.TeamID, ID: id})
	if err != nil {
		return nil, notFoundOr(err, "photo")
	}
	if photo.Status != "awaiting_upload" {
		// Already completed: answer with the current state, enqueue nothing.
		return connect.NewResponse(&pidexv1.CompleteUploadResponse{Photo: s.photoProto(ctx, photo, nil, nil)}), nil
	}
	size, exists, err := s.Storage.Head(ctx, photo.ObjectKey)
	if err != nil {
		return nil, internal(err)
	}
	if !exists {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("upload not received"))
	}
	if lockedID != nil {
		if _, err := s.Q.GetSubject(ctx, dbq.GetSubjectParams{TeamID: pr.TeamID, ID: *lockedID}); err != nil {
			return nil, notFoundOr(err, "locked subject")
		}
	} else if err := s.checkIdentityShot(ctx, pr.TeamID, photo, sessionID, m.IdentityClientPhotoId); err != nil {
		return nil, err
	}
	_, version, err := s.Configs.Active(ctx, pr.TeamID)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	onDevice := []byte("{}")
	if m.OnDevice != nil {
		if onDevice, err = protojson.Marshal(m.OnDevice); err != nil {
			return nil, invalid("on_device: %v", err)
		}
	}
	captured := time.Now()
	if t := m.CapturedAt; t != nil && t.IsValid() && t.AsTime().Before(captured.Add(5*time.Minute)) {
		captured = t.AsTime()
	}
	params := dbq.CompletePhotoUploadParams{
		TeamID: pr.TeamID, ID: id, SessionID: sessionID, CapturedAt: &captured,
		OnDevice: onDevice, LockedSubjectID: lockedID, ConfigVersion: &version, SizeBytes: size,
	}
	if loc := m.Location; loc != nil {
		params.Latitude, params.Longitude = &loc.Latitude, &loc.Longitude
		params.LocationAccuracyM = &loc.AccuracyMeters
	}

	var done dbq.Photo
	err = store.WithTx(ctx, s.Pool, func(tx pgx.Tx) error {
		var err error
		q := s.Q.WithTx(tx)
		if done, err = q.CompletePhotoUpload(ctx, params); err != nil {
			return err
		}
		if m.SubjectChosen && lockedID != nil {
			filed, err := q.ManualAssignPhotos(ctx, dbq.ManualAssignPhotosParams{TeamID: pr.TeamID, Ids: []uuid.UUID{id}, SubjectID: lockedID})
			if err != nil || len(filed) == 0 {
				return errors.Join(err, pgx.ErrNoRows)
			}
			done = filed[0]
			if err := audit.Log(ctx, q, audit.Entry{
				TeamID: pr.TeamID, ActorID: &pr.UserID, Action: "photo.manually_filed", TargetType: "photo", TargetID: &id,
				Details: map[string]any{"subject": *lockedID, "chosen_on": "camera"},
			}); err != nil {
				return err
			}
		}
		return s.Jobs.EnqueueProcessPhoto(ctx, tx, id)
	})
	if err != nil {
		return nil, notFoundOr(err, "photo")
	}
	return connect.NewResponse(&pidexv1.CompleteUploadResponse{Photo: s.photoProto(ctx, done, nil, nil)}), nil
}

// checkIdentityShot refuses a photo that says neither who it shows nor
// which shot does: nothing unidentified gets in to be filed wrongly or lost.
// The identifying shot is this one, or one already uploaded in its session.
func (s *captureService) checkIdentityShot(ctx context.Context, teamID uuid.UUID, photo dbq.Photo, sessionID *uuid.UUID, ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return invalid("the photo says nothing of who it shows: set locked_subject_id, or identity_client_photo_id to the shot that identifies it")
	}
	if ref == photo.ClientPhotoID {
		return nil
	}
	shot, err := s.Q.GetPhotoByClientID(ctx, dbq.GetPhotoByClientIDParams{TeamID: teamID, ClientPhotoID: ref})
	if errors.Is(err, pgx.ErrNoRows) || err == nil && (shot.Status == "awaiting_upload" || sessionID == nil || shot.SessionID == nil || *shot.SessionID != *sessionID) {
		return invalid("identity_client_photo_id is not a shot uploaded in this session")
	}
	if err != nil {
		return internal(err)
	}
	return nil
}

// onDeviceReadConfidence is lower than a model read confirmed by OCR: the
// banner it drives is provisional until the pipeline confirms.
const onDeviceReadConfidence = 0.8

func (s *captureService) ResolveAnchor(ctx context.Context, req *connect.Request[pidexv1.ResolveAnchorRequest]) (*connect.Response[pidexv1.ResolveAnchorResponse], error) {
	pr := principalFrom(ctx)
	cfg, _, err := s.Configs.Active(ctx, pr.TeamID)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	od := req.Msg.OnDevice
	ocr := ""
	for _, t := range od.GetText() {
		ocr += t.GetText() + "\n"
	}

	for i := range cfg.SubjectTypes {
		st := &cfg.SubjectTypes[i]
		read := identity.Read{Fields: map[string]string{}, Confidence: onDeviceReadConfidence}
		for _, bc := range od.GetBarcodes() {
			if field, value, ok := identity.FromBarcode(st, bc.GetSymbology(), bc.GetPayload()); ok {
				read.Fields[field], read.BarcodeField, read.Confidence = value, field, 1
				break
			}
		}
		for k, v := range identity.ReadFromOCR(st, ocr, cfg.DateOrder) {
			if _, set := read.Fields[k]; !set {
				read.Fields[k] = v
			}
		}
		if len(read.Fields) == 0 {
			// Nothing reads as this type's identifier by itself, but a
			// known one may be among the text: a serial on a settings
			// screen full of other numbers.
			res, err := s.knownInText(ctx, pr.TeamID, st, ocr)
			if err != nil {
				return nil, internal(err)
			}
			if res != nil {
				return connect.NewResponse(s.anchorResponse(ctx, pr.TeamID, st, *res)), nil
			}
			continue
		}
		cands, err := s.anchorCandidates(ctx, pr.TeamID, st, &read, ocr, cfg.DateOrder)
		if err != nil {
			return nil, internal(err)
		}
		res := identity.Resolve(st, read, cfg.DateOrder, cands)
		if res.Outcome == identity.OutcomeNotAnchor {
			continue
		}
		return connect.NewResponse(s.anchorResponse(ctx, pr.TeamID, st, res)), nil
	}
	return connect.NewResponse(&pidexv1.ResolveAnchorResponse{State: pidexv1.AnchorState_ANCHOR_STATE_NOT_ANCHOR}), nil
}

// anchorCandidates finds subjects sharing a read value, then fills in a
// name field when exactly one candidate's name appears in the OCR text.
// Picking from known subjects is far more reliable than reading a name.
// knownTextConfidence files a shot whose text contains a known subject's
// identifier exactly: strong, but read by the phone alone.
const knownTextConfidence = 0.9

// maxTextProbes bounds the identifier lookups one screenful of text makes.
const maxTextProbes = 200

// knownInText finds subjects whose identifier appears among the words of
// the text. One subject is a match; several are for a person to settle.
func (s *captureService) knownInText(ctx context.Context, teamID uuid.UUID, st *teamconfig.SubjectType, ocr string) (*identity.Resolution, error) {
	var probes [][]byte
	for _, f := range st.IdentityFields {
		if f.Kind != teamconfig.KindID {
			continue
		}
		for _, t := range identity.Tokens(ocr) {
			if len(probes) == maxTextProbes {
				break
			}
			b, _ := json.Marshal(map[string]string{f.Key: t})
			probes = append(probes, b)
		}
	}
	if len(probes) == 0 {
		return nil, nil
	}
	rows, err := s.Q.FindSubjectCandidates(ctx, dbq.FindSubjectCandidatesParams{TeamID: teamID, TypeKey: st.Key, Probes: probes})
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	if len(rows) == 1 {
		var raw map[string]string
		_ = json.Unmarshal(rows[0].Identity, &raw)
		return &identity.Resolution{Outcome: identity.OutcomeMatched, SubjectID: rows[0].ID, Confidence: knownTextConfidence, Fields: raw}, nil
	}
	res := &identity.Resolution{Outcome: identity.OutcomeAmbiguous, Confidence: knownTextConfidence / 2}
	for _, r := range rows {
		res.Candidates = append(res.Candidates, r.ID)
	}
	return res, nil
}

// maxShotBytes bounds a shot sent to be identified; the phone downscales.
const maxShotBytes = 2 << 20

// IdentifyShot has the vision model read a shot the phone could not, and
// resolves what it read against known subjects, as the pipeline will once
// the shot is uploaded.
func (s *captureService) IdentifyShot(ctx context.Context, req *connect.Request[pidexv1.IdentifyShotRequest]) (*connect.Response[pidexv1.IdentifyShotResponse], error) {
	pr := principalFrom(ctx)
	if err := requireWrite(pr); err != nil {
		return nil, err
	}
	if n := len(req.Msg.Jpeg); n == 0 || n > maxShotBytes {
		return nil, invalid("jpeg must have 1 to %d bytes", maxShotBytes)
	}
	cfg, _, err := s.Configs.Active(ctx, pr.TeamID)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	ocr := ""
	for _, t := range req.Msg.OnDevice.GetText() {
		ocr += t.GetText() + "\n"
	}
	none := &pidexv1.IdentifyShotResponse{Anchor: &pidexv1.ResolveAnchorResponse{State: pidexv1.AnchorState_ANCHOR_STATE_NOT_ANCHOR}}
	shot, err := s.Processor.ReadShot(ctx, pr.TeamID, cfg, req.Msg.Jpeg, req.Msg.OnDevice)
	switch {
	case errors.Is(err, ai.ErrNoModel):
		return connect.NewResponse(none), nil
	case err != nil:
		return nil, connect.NewError(connect.CodeUnavailable, err)
	case shot == nil:
		return connect.NewResponse(none), nil
	case shot.Seen != nil:
		if err := s.keepShotVerdict(ctx, pr.TeamID, req.Msg.ClientPhotoId, shot); err != nil {
			return nil, internal(err)
		}
		return connect.NewResponse(&pidexv1.IdentifyShotResponse{Anchor: s.appearanceAnchor(ctx, pr.TeamID, cfg, shot)}), nil
	case shot.Read == nil || len(shot.Read.Fields) == 0:
		return connect.NewResponse(none), nil
	}
	st, read := shot.Type, shot.Read
	cands, err := s.anchorCandidates(ctx, pr.TeamID, st, read, ocr, cfg.DateOrder)
	if err != nil {
		return nil, internal(err)
	}
	res := identity.Resolve(st, *read, cfg.DateOrder, cands)
	if res.Outcome == identity.OutcomeNotAnchor {
		return connect.NewResponse(none), nil
	}
	return connect.NewResponse(&pidexv1.IdentifyShotResponse{Anchor: s.anchorResponse(ctx, pr.TeamID, st, res)}), nil
}

// keepShotVerdict keeps what the look match said of a shot until it is
// uploaded, where the person's answer on the camera checks it.
func (s *captureService) keepShotVerdict(ctx context.Context, teamID uuid.UUID, clientPhotoID string, shot *pipeline.ShotRead) error {
	if clientPhotoID == "" || len(clientPhotoID) > 128 {
		return nil
	}
	raw, err := json.Marshal(shot.Appearance())
	if err != nil {
		return err
	}
	if err := s.Q.ForgetOldShotVerdicts(ctx, teamID); err != nil {
		return err
	}
	return s.Q.SaveShotVerdict(ctx, dbq.SaveShotVerdictParams{TeamID: teamID, ClientPhotoID: clientPhotoID, Appearance: raw})
}

// appearanceAnchor words a match by appearance as the camera shows anchors:
// the one it is, a new one named from its look, or one to confirm later.
func (s *captureService) appearanceAnchor(ctx context.Context, teamID uuid.UUID, cfg *teamconfig.Config, shot *pipeline.ShotRead) *pidexv1.ResolveAnchorResponse {
	th := cfg.Thresholds()
	v := shot.Verdict
	out := &pidexv1.ResolveAnchorResponse{Confidence: float32(v.Confidence), SubjectTypeKey: shot.Type.Key}
	switch {
	case shot.Match != nil && v.Confidence >= th.Suggest:
		sub, err := s.Q.GetSubject(ctx, dbq.GetSubjectParams{TeamID: teamID, ID: shot.Match.ID})
		if err != nil {
			out.State = pidexv1.AnchorState_ANCHOR_STATE_AMBIGUOUS
			return out
		}
		if v.Confidence < th.AutoAssign {
			// Looks like this one, not surely: the camera asks.
			out.State = pidexv1.AnchorState_ANCHOR_STATE_AMBIGUOUS
			out.Candidates = []*pidexv1.Subject{subjectProto(sub, 0)}
			return out
		}
		out.State = pidexv1.AnchorState_ANCHOR_STATE_MATCHED
		out.Subject = subjectProto(sub, 0)
	case shot.Match == nil && v.Confidence >= th.Suggest:
		out.State = pidexv1.AnchorState_ANCHOR_STATE_NEW_SUBJECT
		out.ExtractedIdentity = map[string]string{"name": shot.Seen.Label}
	default:
		out.State = pidexv1.AnchorState_ANCHOR_STATE_AMBIGUOUS
	}
	return out
}

func (s *captureService) anchorCandidates(ctx context.Context, teamID uuid.UUID, st *teamconfig.SubjectType, read *identity.Read, ocr, dateOrder string) ([]identity.Candidate, error) {
	_, norm := identity.Clean(st, read.Fields, dateOrder)
	probes := make([][]byte, 0, len(norm))
	for k, v := range norm {
		b, _ := json.Marshal(map[string]string{k: v})
		probes = append(probes, b)
	}
	if len(probes) == 0 {
		return nil, nil
	}
	rows, err := s.Q.FindSubjectCandidates(ctx, dbq.FindSubjectCandidatesParams{TeamID: teamID, TypeKey: st.Key, Probes: probes})
	if err != nil {
		return nil, err
	}
	cands := make([]identity.Candidate, 0, len(rows))
	for _, f := range st.IdentityFields {
		if f.Kind != teamconfig.KindPersonName || read.Fields[f.Key] != "" {
			continue
		}
		var hit string
		hits := 0
		for _, r := range rows {
			var raw, n map[string]string
			_ = json.Unmarshal(r.Identity, &raw)
			_ = json.Unmarshal(r.IdentityNorm, &n)
			if n[f.Key] != "" && identity.ContainsValue(f.Kind, n[f.Key], ocr, dateOrder) {
				hit = raw[f.Key]
				hits++
			}
		}
		if hits == 1 {
			read.Fields[f.Key] = hit
		}
	}
	for _, r := range rows {
		var n map[string]string
		_ = json.Unmarshal(r.IdentityNorm, &n)
		cands = append(cands, identity.Candidate{ID: r.ID, Norm: n})
	}
	return cands, nil
}

func (s *captureService) anchorResponse(ctx context.Context, teamID uuid.UUID, st *teamconfig.SubjectType, res identity.Resolution) *pidexv1.ResolveAnchorResponse {
	out := &pidexv1.ResolveAnchorResponse{
		Confidence:        float32(res.Confidence),
		SubjectTypeKey:    st.Key,
		ExtractedIdentity: res.Fields,
	}
	ids := res.Candidates
	switch res.Outcome {
	case identity.OutcomeMatched:
		out.State = pidexv1.AnchorState_ANCHOR_STATE_MATCHED
		ids = []uuid.UUID{res.SubjectID}
	case identity.OutcomeNewSubject:
		out.State = pidexv1.AnchorState_ANCHOR_STATE_NEW_SUBJECT
	default:
		out.State = pidexv1.AnchorState_ANCHOR_STATE_AMBIGUOUS
	}
	if len(ids) == 0 {
		return out
	}
	subs, err := s.Q.GetSubjectsByIDs(ctx, dbq.GetSubjectsByIDsParams{TeamID: teamID, Ids: ids})
	if err != nil {
		s.Log.Warn("load anchor subjects", "err", err)
		return out
	}
	byID := map[uuid.UUID]dbq.Subject{}
	for _, sub := range subs {
		byID[sub.ID] = sub
	}
	for _, id := range ids {
		sub, ok := byID[id]
		if !ok {
			continue
		}
		if res.Outcome == identity.OutcomeMatched {
			out.Subject = subjectProto(sub, 0)
		} else {
			out.Candidates = append(out.Candidates, subjectProto(sub, 0))
		}
	}
	return out
}

func (s *captureService) GetSessionSummary(ctx context.Context, req *connect.Request[pidexv1.GetSessionSummaryRequest]) (*connect.Response[pidexv1.GetSessionSummaryResponse], error) {
	pr := principalFrom(ctx)
	sessionID, err := parseID("session_id", req.Msg.SessionId)
	if err != nil {
		return nil, err
	}
	photos, err := s.Q.ListSessionPhotos(ctx, dbq.ListSessionPhotosParams{TeamID: pr.TeamID, SessionID: &sessionID})
	if err != nil {
		return nil, internal(err)
	}
	out := &pidexv1.GetSessionSummaryResponse{TotalCount: int32(len(photos))}
	type group struct{ count, auto int32 }
	groups := map[uuid.UUID]*group{}
	var order []uuid.UUID
	var review []dbq.Photo
	for _, ph := range photos {
		switch {
		case ph.Status == "awaiting_upload" || ph.Status == "uploaded" || ph.Status == "processing":
			out.ProcessingCount++
		case ph.Role != nil && *ph.Role == "junk":
			out.JunkCount++
			continue
		}
		if ph.SubjectID != nil {
			g, ok := groups[*ph.SubjectID]
			if !ok {
				g = &group{}
				groups[*ph.SubjectID] = g
				order = append(order, *ph.SubjectID)
			}
			g.count++
			if ph.Assignment == "auto" {
				g.auto++
			}
			continue
		}
		if ph.Status == "processed" || ph.Status == "failed" {
			review = append(review, ph)
		}
	}
	if len(order) > 0 {
		subs, err := s.Q.GetSubjectsByIDs(ctx, dbq.GetSubjectsByIDsParams{TeamID: pr.TeamID, Ids: order})
		if err != nil {
			return nil, internal(err)
		}
		byID := map[uuid.UUID]dbq.Subject{}
		for _, sub := range subs {
			byID[sub.ID] = sub
		}
		for _, id := range order {
			g := groups[id]
			out.Groups = append(out.Groups, &pidexv1.SubjectGroup{
				Subject: subjectProto(byID[id], g.count), PhotoCount: g.count, AutoCount: g.auto,
			})
		}
	}
	if out.NeedsReview, err = s.photosProto(ctx, s.Q, review); err != nil {
		return nil, err
	}
	var suggested []uuid.UUID
	seen := map[uuid.UUID]bool{}
	for _, ph := range review {
		if id := ph.SuggestedSubjectID; id != nil && !seen[*id] {
			seen[*id] = true
			suggested = append(suggested, *id)
		}
	}
	if len(suggested) > 0 {
		subs, err := s.Q.GetSubjectsByIDs(ctx, dbq.GetSubjectsByIDsParams{TeamID: pr.TeamID, Ids: suggested})
		if err != nil {
			return nil, internal(err)
		}
		for _, sub := range subs {
			out.SuggestedSubjects = append(out.SuggestedSubjects, subjectProto(sub, 0))
		}
	}
	return connect.NewResponse(out), nil
}
