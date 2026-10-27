package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/internal/audit"
	"pidex/internal/identity"
	"pidex/internal/store"
	"pidex/internal/store/dbq"
)

type libraryService struct{ *Server }

const maxAssignBatch = 200

func (s *libraryService) ListPhotos(ctx context.Context, req *connect.Request[pidexv1.ListPhotosRequest]) (*connect.Response[pidexv1.ListPhotosResponse], error) {
	pr := principalFrom(ctx)
	m := req.Msg
	subjectID, err := parseOptionalID("subject_id", m.SubjectId)
	if err != nil {
		return nil, err
	}
	before, beforeID, err := decodePageToken(m.PageToken)
	if err != nil {
		return nil, err
	}
	params := dbq.ListPhotosParams{
		TeamID: pr.TeamID, SubjectID: subjectID, TagKeys: m.TagKeys, Assignments: []string{},
		BeforeCreated: before, BeforeID: beforeID, PageLimit: pageSize(m.PageSize) + 1,
	}
	if params.TagKeys == nil {
		params.TagKeys = []string{}
	}
	for _, a := range m.Assignments {
		if v, ok := assignmentFromProto[a]; ok {
			params.Assignments = append(params.Assignments, v)
		}
	}
	if r, ok := roleFromProto[m.Role]; ok {
		params.Role = &r
	}
	rows, err := s.Q.ListPhotos(ctx, params)
	if err != nil {
		return nil, internal(err)
	}
	out := &pidexv1.ListPhotosResponse{}
	if limit := int(pageSize(m.PageSize)); len(rows) > limit {
		last := rows[limit-1]
		out.NextPageToken = encodePageToken(last.CreatedAt, last.ID)
		rows = rows[:limit]
	}
	if out.Photos, err = s.photosProto(ctx, s.Q, rows); err != nil {
		return nil, err
	}
	return connect.NewResponse(out), nil
}

func (s *libraryService) GetPhoto(ctx context.Context, req *connect.Request[pidexv1.GetPhotoRequest]) (*connect.Response[pidexv1.GetPhotoResponse], error) {
	pr := principalFrom(ctx)
	id, err := parseID("id", req.Msg.Id)
	if err != nil {
		return nil, err
	}
	ph, err := s.Q.GetPhoto(ctx, dbq.GetPhotoParams{TeamID: pr.TeamID, ID: id})
	if err != nil {
		return nil, notFoundOr(err, "photo")
	}
	photos, err := s.photosProto(ctx, s.Q, []dbq.Photo{ph})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pidexv1.GetPhotoResponse{Photo: photos[0]}), nil
}

// AssignPhotos is a person's decision. It is logged with what the system
// had decided before, which is the feedback that tunes thresholds, and the
// affected sessions are re-filed: confirming an identity photo files the
// photos that followed it.
func (s *libraryService) AssignPhotos(ctx context.Context, req *connect.Request[pidexv1.AssignPhotosRequest]) (*connect.Response[pidexv1.AssignPhotosResponse], error) {
	pr := principalFrom(ctx)
	if err := requireWrite(pr); err != nil {
		return nil, err
	}
	m := req.Msg
	if len(m.PhotoIds) == 0 || len(m.PhotoIds) > maxAssignBatch {
		return nil, invalid("photo_ids must have 1 to %d entries", maxAssignBatch)
	}
	subjectID, err := parseID("subject_id", m.SubjectId)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(m.PhotoIds))
	for i, raw := range m.PhotoIds {
		if ids[i], err = parseID("photo_ids", raw); err != nil {
			return nil, err
		}
	}
	if _, err := s.Q.GetSubject(ctx, dbq.GetSubjectParams{TeamID: pr.TeamID, ID: subjectID}); err != nil {
		return nil, notFoundOr(err, "subject")
	}

	var updated []dbq.Photo
	err = store.WithTx(ctx, s.Pool, func(tx pgx.Tx) error {
		q := s.Q.WithTx(tx)
		before := map[uuid.UUID]dbq.Photo{}
		for _, id := range ids {
			ph, err := q.GetPhoto(ctx, dbq.GetPhotoParams{TeamID: pr.TeamID, ID: id})
			if err != nil {
				return err
			}
			before[id] = ph
		}
		var err error
		if updated, err = q.ManualAssignPhotos(ctx, dbq.ManualAssignPhotosParams{TeamID: pr.TeamID, Ids: ids, SubjectID: &subjectID}); err != nil {
			return err
		}
		for _, ph := range updated {
			prev := before[ph.ID]
			if err := audit.Log(ctx, q, audit.Entry{
				TeamID: pr.TeamID, ActorID: &pr.UserID, Action: "photo.manually_filed", TargetType: "photo", TargetID: &ph.ID,
				Details: map[string]any{
					"subject": subjectID, "previous_assignment": prev.Assignment,
					"previous_subject": prev.SubjectID, "previous_suggested": prev.SuggestedSubjectID,
					"previous_confidence": prev.AssignmentConfidence,
				},
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, notFoundOr(err, "photo")
	}

	sessions := map[uuid.UUID]bool{}
	for _, ph := range updated {
		if ph.SessionID != nil {
			sessions[*ph.SessionID] = true
		}
	}
	for sid := range sessions {
		if err := s.Processor.ResegmentSession(ctx, pr.TeamID, sid); err != nil {
			s.Log.Error("re-file session after manual assignment", "session", sid, "err", err)
		}
	}
	photos, err := s.photosProto(ctx, s.Q, updated)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pidexv1.AssignPhotosResponse{Photos: photos}), nil
}

func (s *libraryService) UpdatePhotoTags(ctx context.Context, req *connect.Request[pidexv1.UpdatePhotoTagsRequest]) (*connect.Response[pidexv1.UpdatePhotoTagsResponse], error) {
	pr := principalFrom(ctx)
	if err := requireWrite(pr); err != nil {
		return nil, err
	}
	m := req.Msg
	id, err := parseID("photo_id", m.PhotoId)
	if err != nil {
		return nil, err
	}
	cfg, _, err := s.Configs.Active(ctx, pr.TeamID)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	for _, k := range m.Add {
		if !cfg.HasTag(k) {
			return nil, invalid("unknown tag %q", k)
		}
	}
	var ph dbq.Photo
	err = store.WithTx(ctx, s.Pool, func(tx pgx.Tx) error {
		q := s.Q.WithTx(tx)
		var err error
		if ph, err = q.GetPhoto(ctx, dbq.GetPhotoParams{TeamID: pr.TeamID, ID: id}); err != nil {
			return err
		}
		for _, k := range m.Add {
			if err := q.AddUserTag(ctx, dbq.AddUserTagParams{PhotoID: id, TagKey: k}); err != nil {
				return err
			}
		}
		for _, k := range m.Remove {
			if err := q.RejectTag(ctx, dbq.RejectTagParams{PhotoID: id, TagKey: k}); err != nil {
				return err
			}
		}
		if err := s.Jobs.EnqueueIndexPhoto(ctx, tx, id); err != nil {
			return err
		}
		return audit.Log(ctx, q, audit.Entry{
			TeamID: pr.TeamID, ActorID: &pr.UserID, Action: "photo.tags_edited", TargetType: "photo", TargetID: &id,
			Details: map[string]any{"added": m.Add, "removed": m.Remove},
		})
	})
	if err != nil {
		return nil, notFoundOr(err, "photo")
	}
	photos, err := s.photosProto(ctx, s.Q, []dbq.Photo{ph})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pidexv1.UpdatePhotoTagsResponse{Photo: photos[0]}), nil
}

func (s *libraryService) ListSubjects(ctx context.Context, req *connect.Request[pidexv1.ListSubjectsRequest]) (*connect.Response[pidexv1.ListSubjectsResponse], error) {
	pr := principalFrom(ctx)
	m := req.Msg
	before, beforeID, err := decodePageToken(m.PageToken)
	if err != nil {
		return nil, err
	}
	params := dbq.ListSubjectsParams{TeamID: pr.TeamID, BeforeCreated: before, BeforeID: beforeID, PageLimit: pageSize(m.PageSize) + 1}
	if m.TypeKey != "" {
		params.TypeKey = &m.TypeKey
	}
	if m.Query != "" {
		params.Query = &m.Query
	}
	rows, err := s.Q.ListSubjects(ctx, params)
	if err != nil {
		return nil, internal(err)
	}
	out := &pidexv1.ListSubjectsResponse{}
	if limit := int(pageSize(m.PageSize)); len(rows) > limit {
		last := rows[limit-1]
		out.NextPageToken = encodePageToken(last.CreatedAt, last.ID)
		rows = rows[:limit]
	}
	counts, err := s.photoCounts(ctx, pr.TeamID, rows)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out.Subjects = append(out.Subjects, subjectProto(r, counts[r.ID]))
	}
	if err := s.addCovers(ctx, pr.TeamID, out.Subjects); err != nil {
		return nil, err
	}
	return connect.NewResponse(out), nil
}

// addCovers gives each subject a photo to be shown by.
func (s *Server) addCovers(ctx context.Context, teamID uuid.UUID, subjects []*pidexv1.Subject) error {
	ids := make([]uuid.UUID, 0, len(subjects))
	for _, sub := range subjects {
		if id, err := uuid.Parse(sub.Id); err == nil {
			ids = append(ids, id)
		}
	}
	rows, err := s.Q.SubjectCovers(ctx, dbq.SubjectCoversParams{TeamID: teamID, SubjectIds: ids})
	if err != nil {
		return internal(err)
	}
	photoIDs := make([]uuid.UUID, len(rows))
	bySubject := map[string]int{}
	for i, r := range rows {
		photoIDs[i] = r.PhotoID
		bySubject[r.SubjectID.String()] = i
	}
	photos, err := s.photosInOrder(ctx, teamID, photoIDs)
	if err != nil {
		return err
	}
	covers, err := s.photosProto(ctx, s.Q, photos)
	if err != nil {
		return err
	}
	byPhoto := map[string]*pidexv1.Photo{}
	for _, c := range covers {
		byPhoto[c.Id] = c
	}
	for _, sub := range subjects {
		if i, ok := bySubject[sub.Id]; ok {
			sub.Cover = byPhoto[photoIDs[i].String()]
		}
	}
	return nil
}

func (s *Server) photoCounts(ctx context.Context, teamID uuid.UUID, subs []dbq.Subject) (map[uuid.UUID]int32, error) {
	ids := make([]uuid.UUID, len(subs))
	for i, sub := range subs {
		ids[i] = sub.ID
	}
	rows, err := s.Q.CountPhotosBySubject(ctx, dbq.CountPhotosBySubjectParams{TeamID: teamID, SubjectIds: ids})
	if err != nil {
		return nil, internal(err)
	}
	out := map[uuid.UUID]int32{}
	for _, r := range rows {
		out[r.SubjectID] = r.PhotoCount
	}
	return out, nil
}

func (s *libraryService) GetSubject(ctx context.Context, req *connect.Request[pidexv1.GetSubjectRequest]) (*connect.Response[pidexv1.GetSubjectResponse], error) {
	pr := principalFrom(ctx)
	id, err := parseID("id", req.Msg.Id)
	if err != nil {
		return nil, err
	}
	sub, err := s.Q.GetSubject(ctx, dbq.GetSubjectParams{TeamID: pr.TeamID, ID: id})
	if err != nil {
		return nil, notFoundOr(err, "subject")
	}
	rows, err := s.Q.ListSubjectPhotos(ctx, dbq.ListSubjectPhotosParams{TeamID: pr.TeamID, SubjectID: &id})
	if err != nil {
		return nil, internal(err)
	}
	photos, err := s.photosProto(ctx, s.Q, rows)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pidexv1.GetSubjectResponse{Subject: subjectProto(sub, int32(len(rows))), Photos: photos}), nil
}

func (s *libraryService) CreateSubject(ctx context.Context, req *connect.Request[pidexv1.CreateSubjectRequest]) (*connect.Response[pidexv1.CreateSubjectResponse], error) {
	pr := principalFrom(ctx)
	if err := requireWrite(pr); err != nil {
		return nil, err
	}
	m := req.Msg
	cfg, _, err := s.Configs.Active(ctx, pr.TeamID)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	st, ok := cfg.SubjectType(m.TypeKey)
	if !ok {
		return nil, invalid("unknown subject type %q", m.TypeKey)
	}
	parentID, err := parseOptionalID("parent_id", m.ParentId)
	if err != nil {
		return nil, err
	}
	raw, norm := identity.Clean(st, m.Identity, cfg.DateOrder)
	name := st.DisplayName(raw)
	switch {
	case st.ByAppearance():
		// Nothing to read: it is known by its look, and by what it is called.
		if name = strings.TrimSpace(m.DisplayName); name == "" || utf8.RuneCountInString(name) > maxSubjectName {
			return nil, invalid("display_name must have 1 to %d characters", maxSubjectName)
		}
	case len(norm) == 0:
		return nil, invalid("identity has no usable %s fields", st.Name)
	}
	rawJSON, _ := json.Marshal(raw)
	normJSON, _ := json.Marshal(norm)
	var sub dbq.Subject
	err = store.WithTx(ctx, s.Pool, func(tx pgx.Tx) error {
		q := s.Q.WithTx(tx)
		if parentID != nil {
			if _, err := q.GetSubject(ctx, dbq.GetSubjectParams{TeamID: pr.TeamID, ID: *parentID}); err != nil {
				return err
			}
		}
		var err error
		sub, err = q.CreateSubject(ctx, dbq.CreateSubjectParams{
			TeamID: pr.TeamID, TypeKey: st.Key, ParentID: parentID, DisplayName: name,
			Identity: rawJSON, IdentityNorm: normJSON, CreatedBy: &pr.UserID,
		})
		if err != nil {
			return err
		}
		return audit.Log(ctx, q, audit.Entry{
			TeamID: pr.TeamID, ActorID: &pr.UserID, Action: "subject.created", TargetType: "subject", TargetID: &sub.ID,
		})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("parent subject not found"))
	}
	if err != nil {
		return nil, internal(err)
	}
	return connect.NewResponse(&pidexv1.CreateSubjectResponse{Subject: subjectProto(sub, 0)}), nil
}

// maxSubjectName bounds a name given by hand.
const maxSubjectName = 80

func (s *libraryService) RenameSubject(ctx context.Context, req *connect.Request[pidexv1.RenameSubjectRequest]) (*connect.Response[pidexv1.RenameSubjectResponse], error) {
	pr := principalFrom(ctx)
	if err := requireWrite(pr); err != nil {
		return nil, err
	}
	id, err := parseID("subject_id", req.Msg.SubjectId)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Msg.DisplayName)
	if name == "" || utf8.RuneCountInString(name) > maxSubjectName {
		return nil, invalid("display_name must have 1 to %d characters", maxSubjectName)
	}
	var sub dbq.Subject
	err = store.WithTx(ctx, s.Pool, func(tx pgx.Tx) error {
		q := s.Q.WithTx(tx)
		before, err := q.GetSubject(ctx, dbq.GetSubjectParams{TeamID: pr.TeamID, ID: id})
		if err != nil {
			return err
		}
		if sub, err = q.RenameSubject(ctx, dbq.RenameSubjectParams{TeamID: pr.TeamID, ID: id, DisplayName: name}); err != nil {
			return err
		}
		return audit.Log(ctx, q, audit.Entry{
			TeamID: pr.TeamID, ActorID: &pr.UserID, Action: "subject.renamed", TargetType: "subject", TargetID: &id,
			Details: map[string]any{"from": before.DisplayName, "to": name},
		})
	})
	if err != nil {
		return nil, notFoundOr(err, "subject")
	}
	n, err := s.photoCount(ctx, pr.TeamID, sub.ID)
	if err != nil {
		return nil, internal(err)
	}
	return connect.NewResponse(&pidexv1.RenameSubjectResponse{Subject: subjectProto(sub, n)}), nil
}

// maxProfile bounds what a person writes about how a subject looks.
const maxProfile = 2000

// SetSubjectProfile keeps what a person says a subject looks like ("black
// nose"), searched from then on and no longer rewritten from photos. Empty
// hands it back to the photos.
func (s *libraryService) SetSubjectProfile(ctx context.Context, req *connect.Request[pidexv1.SetSubjectProfileRequest]) (*connect.Response[pidexv1.SetSubjectProfileResponse], error) {
	pr := principalFrom(ctx)
	if err := requireWrite(pr); err != nil {
		return nil, err
	}
	id, err := parseID("subject_id", req.Msg.SubjectId)
	if err != nil {
		return nil, err
	}
	profile := strings.TrimSpace(req.Msg.Profile)
	if utf8.RuneCountInString(profile) > maxProfile {
		return nil, invalid("profile must have at most %d characters", maxProfile)
	}
	if _, err := s.Q.GetSubject(ctx, dbq.GetSubjectParams{TeamID: pr.TeamID, ID: id}); err != nil {
		return nil, notFoundOr(err, "subject")
	}
	if profile == "" {
		err = s.Q.ResetSubjectProfile(ctx, dbq.ResetSubjectProfileParams{TeamID: pr.TeamID, ID: id})
	} else {
		err = s.Processor.SaveProfile(ctx, pr.TeamID, id, profile, 0, true)
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}
	if err := audit.Log(ctx, s.Q, audit.Entry{
		TeamID: pr.TeamID, ActorID: &pr.UserID, Action: "subject.profile_set", TargetType: "subject", TargetID: &id,
		Details: map[string]any{"cleared": profile == ""},
	}); err != nil {
		return nil, internal(err)
	}
	sub, err := s.Q.GetSubject(ctx, dbq.GetSubjectParams{TeamID: pr.TeamID, ID: id})
	if err != nil {
		return nil, internal(err)
	}
	n, err := s.photoCount(ctx, pr.TeamID, sub.ID)
	if err != nil {
		return nil, internal(err)
	}
	return connect.NewResponse(&pidexv1.SetSubjectProfileResponse{Subject: subjectProto(sub, n)}), nil
}

// MergeSubjects joins subjects that are one: the same cat filed twice
// before anyone noticed. Photos, suggestions and children move to into.
func (s *libraryService) MergeSubjects(ctx context.Context, req *connect.Request[pidexv1.MergeSubjectsRequest]) (*connect.Response[pidexv1.MergeSubjectsResponse], error) {
	pr := principalFrom(ctx)
	if err := requireWrite(pr); err != nil {
		return nil, err
	}
	into, err := parseID("into_id", req.Msg.IntoId)
	if err != nil {
		return nil, err
	}
	if len(req.Msg.SubjectIds) == 0 || len(req.Msg.SubjectIds) > maxAssignBatch {
		return nil, invalid("subject_ids must have 1 to %d entries", maxAssignBatch)
	}
	var ids []uuid.UUID
	for _, raw := range req.Msg.SubjectIds {
		id, err := parseID("subject_ids", raw)
		if err != nil {
			return nil, err
		}
		if id != into {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, invalid("subject_ids must name subjects other than into_id")
	}
	var target dbq.Subject
	err = store.WithTx(ctx, s.Pool, func(tx pgx.Tx) error {
		q := s.Q.WithTx(tx)
		var err error
		if target, err = q.GetSubject(ctx, dbq.GetSubjectParams{TeamID: pr.TeamID, ID: into}); err != nil {
			return err
		}
		if target.MergedIntoID != nil {
			return errMergedTarget
		}
		others, err := q.GetSubjectsByIDs(ctx, dbq.GetSubjectsByIDsParams{TeamID: pr.TeamID, Ids: ids})
		if err != nil {
			return err
		}
		if len(others) != len(ids) {
			return pgx.ErrNoRows
		}
		for _, o := range others {
			if o.TypeKey != target.TypeKey {
				return errMergeTypes
			}
		}
		if err := q.MergeSubjectPhotos(ctx, dbq.MergeSubjectPhotosParams{TeamID: pr.TeamID, Ids: ids, IntoID: into}); err != nil {
			return err
		}
		if err := q.MergeSubjectChildren(ctx, dbq.MergeSubjectChildrenParams{TeamID: pr.TeamID, Ids: ids, IntoID: into}); err != nil {
			return err
		}
		if _, err := q.MarkSubjectsMerged(ctx, dbq.MarkSubjectsMergedParams{TeamID: pr.TeamID, Ids: ids, IntoID: into}); err != nil {
			return err
		}
		return audit.Log(ctx, q, audit.Entry{
			TeamID: pr.TeamID, ActorID: &pr.UserID, Action: "subject.merged", TargetType: "subject", TargetID: &into,
			Details: map[string]any{"merged": ids},
		})
	})
	switch {
	case errors.Is(err, errMergeTypes), errors.Is(err, errMergedTarget):
		return nil, invalid("%v", err)
	case err != nil:
		return nil, notFoundOr(err, "subject")
	}
	n, err := s.photoCount(ctx, pr.TeamID, into)
	if err != nil {
		return nil, internal(err)
	}
	return connect.NewResponse(&pidexv1.MergeSubjectsResponse{Subject: subjectProto(target, n)}), nil
}

var (
	errMergeTypes   = errors.New("only subjects of the same type can be merged")
	errMergedTarget = errors.New("into_id was itself merged into another subject")
)

func (s *libraryService) photoCount(ctx context.Context, teamID, subjectID uuid.UUID) (int32, error) {
	rows, err := s.Q.CountPhotosBySubject(ctx, dbq.CountPhotosBySubjectParams{TeamID: teamID, SubjectIds: []uuid.UUID{subjectID}})
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return rows[0].PhotoCount, nil
}

// DeletePhotos deletes photos for good; owners and admins any, members
// those they took.
func (s *libraryService) DeletePhotos(ctx context.Context, req *connect.Request[pidexv1.DeletePhotosRequest]) (*connect.Response[pidexv1.DeletePhotosResponse], error) {
	pr := principalFrom(ctx)
	if err := requireWrite(pr); err != nil {
		return nil, err
	}
	m := req.Msg
	if len(m.PhotoIds) == 0 || len(m.PhotoIds) > maxAssignBatch {
		return nil, invalid("photo_ids must have 1 to %d entries", maxAssignBatch)
	}
	ids := make([]uuid.UUID, len(m.PhotoIds))
	for i, raw := range m.PhotoIds {
		var err error
		if ids[i], err = parseID("photo_ids", raw); err != nil {
			return nil, err
		}
	}
	owners, err := s.Q.PhotoUploaders(ctx, dbq.PhotoUploadersParams{TeamID: pr.TeamID, Ids: ids})
	if err != nil {
		return nil, internal(err)
	}
	if len(owners) != len(ids) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("photo not found"))
	}
	if !pr.CanAdmin() {
		for _, o := range owners {
			if o.UploadedBy != pr.UserID {
				return nil, connect.NewError(connect.CodePermissionDenied, errors.New("members can delete only the photos they took"))
			}
		}
	}

	n, gone, err := s.deletePhotos(ctx, pr, ids)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pidexv1.DeletePhotosResponse{DeletedPhotos: int32(n), DeletedSubjects: int32(len(gone))}), nil
}

// deletePhotos deletes photos and what exists only because of them. The
// rows go in one transaction; then their files, and the sessions they were
// in are filed again, as a deleted identity photo filed the ones after it.
// It returns how many photos went, and the subjects left empty that went.
func (s *libraryService) deletePhotos(ctx context.Context, pr Principal, ids []uuid.UUID) (int, []uuid.UUID, error) {
	var (
		keys     []string
		sessions = map[uuid.UUID]bool{}
		touched  = map[uuid.UUID]bool{}
		deleted  []dbq.DeletePhotosRow
		gone     []uuid.UUID
	)
	err := store.WithTx(ctx, s.Pool, func(tx pgx.Tx) error {
		q := s.Q.WithTx(tx)
		var err error
		if deleted, err = q.DeletePhotos(ctx, dbq.DeletePhotosParams{TeamID: pr.TeamID, Ids: ids}); err != nil {
			return err
		}
		for _, d := range deleted {
			keys = append(keys, d.ObjectKey)
			for _, k := range []*string{d.ThumbKey, d.OriginalKey, d.LiveKey} {
				if k != nil {
					keys = append(keys, *k)
				}
			}
			if d.SessionID != nil {
				sessions[*d.SessionID] = true
			}
			for _, sub := range []*uuid.UUID{d.SubjectID, d.SuggestedSubjectID, d.LockedSubjectID} {
				if sub != nil {
					touched[*sub] = true
				}
			}
		}
		if err := q.ForgetPhotosInMessages(ctx, dbq.ForgetPhotosInMessagesParams{TeamID: pr.TeamID, Ids: ids}); err != nil {
			return err
		}
		return audit.Log(ctx, q, audit.Entry{
			TeamID: pr.TeamID, ActorID: &pr.UserID, Action: "photo.deleted", TargetType: "photo",
			Details: map[string]any{"photos": ids},
		})
	})
	if err != nil {
		return 0, nil, internal(err)
	}
	bg := context.WithoutCancel(ctx)
	if err := s.Storage.Delete(bg, keys); err != nil {
		s.Log.Error("files of deleted photos remain", "team", pr.TeamID, "files", len(keys), "err", err)
	}
	for sid := range sessions {
		if err := s.Processor.ResegmentSession(bg, pr.TeamID, sid); err != nil {
			s.Log.Warn("re-filing a session after deleting photos", "session", sid, "err", err)
		}
	}
	// Only now, with the sessions filed again, is it clear which subjects
	// nothing is filed under any more.
	if gone, err = s.removeEmptySubjects(bg, pr, touched); err != nil {
		s.Log.Warn("removing subjects left empty", "team", pr.TeamID, "err", err)
	}
	return len(deleted), gone, nil
}

// DeleteSubject deletes a subject, with its photos or leaving them unfiled.
// Its children stand alone, names merged into it go, and the sessions its
// photos were in are filed again without it.
func (s *libraryService) DeleteSubject(ctx context.Context, req *connect.Request[pidexv1.DeleteSubjectRequest]) (*connect.Response[pidexv1.DeleteSubjectResponse], error) {
	pr := principalFrom(ctx)
	if !pr.CanAdmin() {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only owners and admins can delete a subject"))
	}
	id, err := parseID("subject_id", req.Msg.SubjectId)
	if err != nil {
		return nil, err
	}
	if _, err := s.Q.GetSubject(ctx, dbq.GetSubjectParams{TeamID: pr.TeamID, ID: id}); err != nil {
		return nil, notFoundOr(err, "subject")
	}
	out := &pidexv1.DeleteSubjectResponse{}
	if req.Msg.WithPhotos {
		photos, err := s.Q.SubjectPhotoIDs(ctx, dbq.SubjectPhotoIDsParams{TeamID: pr.TeamID, SubjectID: &id})
		if err != nil {
			return nil, internal(err)
		}
		if len(photos) > 0 {
			n, _, err := s.deletePhotos(ctx, pr, photos)
			if err != nil {
				return nil, err
			}
			out.DeletedPhotos = int32(n)
		}
	}
	sessions := map[uuid.UUID]bool{}
	err = store.WithTx(ctx, s.Pool, func(tx pgx.Tx) error {
		q := s.Q.WithTx(tx)
		unfiled, err := q.UnfileSubjectPhotos(ctx, dbq.UnfileSubjectPhotosParams{TeamID: pr.TeamID, SubjectID: &id})
		if err != nil {
			return err
		}
		for _, u := range unfiled {
			if u.SessionID != nil {
				sessions[*u.SessionID] = true
			}
		}
		out.UnfiledPhotos = int32(len(unfiled))
		if err := q.OrphanSubjectChildren(ctx, dbq.OrphanSubjectChildrenParams{TeamID: pr.TeamID, SubjectID: &id}); err != nil {
			return err
		}
		if err := q.DeleteSubjectAliases(ctx, dbq.DeleteSubjectAliasesParams{TeamID: pr.TeamID, SubjectID: &id}); err != nil {
			return err
		}
		if err := q.ForgetSubjectsInMessages(ctx, dbq.ForgetSubjectsInMessagesParams{TeamID: pr.TeamID, Ids: []uuid.UUID{id}}); err != nil {
			return err
		}
		// Already gone when it was made from the deleted photos alone.
		if _, err := q.DeleteSubject(ctx, dbq.DeleteSubjectParams{TeamID: pr.TeamID, SubjectID: id}); err != nil {
			return err
		}
		return audit.Log(ctx, q, audit.Entry{
			TeamID: pr.TeamID, ActorID: &pr.UserID, Action: "subject.deleted", TargetType: "subject", TargetID: &id,
			Details: map[string]any{"with_photos": req.Msg.WithPhotos, "deleted_photos": out.DeletedPhotos, "unfiled_photos": out.UnfiledPhotos},
		})
	})
	if err != nil {
		return nil, internal(err)
	}
	for sid := range sessions {
		if err := s.Processor.ResegmentSession(context.WithoutCancel(ctx), pr.TeamID, sid); err != nil {
			s.Log.Warn("re-filing a session after deleting a subject", "session", sid, "err", err)
		}
	}
	return connect.NewResponse(out), nil
}

// removeEmptySubjects deletes subjects the system made that deleted photos
// left with nothing filed under them.
func (s *libraryService) removeEmptySubjects(ctx context.Context, pr Principal, touched map[uuid.UUID]bool) ([]uuid.UUID, error) {
	if len(touched) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, 0, len(touched))
	for id := range touched {
		ids = append(ids, id)
	}
	var gone []uuid.UUID
	err := store.WithTx(ctx, s.Pool, func(tx pgx.Tx) error {
		q := s.Q.WithTx(tx)
		var err error
		if gone, err = q.DeleteEmptyAutoSubjects(ctx, dbq.DeleteEmptyAutoSubjectsParams{TeamID: pr.TeamID, Ids: ids}); err != nil || len(gone) == 0 {
			return err
		}
		if err := q.ForgetSubjectsInMessages(ctx, dbq.ForgetSubjectsInMessagesParams{TeamID: pr.TeamID, Ids: gone}); err != nil {
			return err
		}
		return audit.Log(ctx, q, audit.Entry{
			TeamID: pr.TeamID, ActorID: &pr.UserID, Action: "subject.removed_empty", TargetType: "subject",
			Details: map[string]any{"subjects": gone},
		})
	})
	return gone, err
}
