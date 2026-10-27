// Package pipeline turns an uploaded photo into a filed photo. Cheap tiers
// run first and expensive ones only when needed:
//
//	tier 0  on-device barcode, or a near-duplicate already analyzed: no call
//	tier 1  one call to a cheap vision model for role, identity, tags
//	tier 2  a stronger model re-reads identity that OCR could not confirm
//	tier 3  a person confirms, after the shoot, in one place
//
// Filing never waits on a model: a session is re-filed from its anchors
// whenever any of its photos changes.
package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/bits"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/internal/ai"
	"pidex/internal/analysis"
	"pidex/internal/audit"
	"pidex/internal/configstore"
	"pidex/internal/identity"
	"pidex/internal/imaging"
	"pidex/internal/looks"
	"pidex/internal/storage"
	"pidex/internal/store"
	"pidex/internal/store/dbq"
	"pidex/internal/teamconfig"
	"pidex/internal/tracing"
)

type Processor struct {
	Pool    *pgxpool.Pool
	Configs *configstore.Store
	Storage *storage.S3
	// Vision is required for tier 1; when empty, photos are filed by
	// barcode anchors only.
	Vision *ai.Router
	// Strong is optional; it re-reads identity in strict teams when OCR
	// could not confirm the cheap model's read.
	Strong *ai.Router
	// Match is optional; it compares photos of subjects told apart by
	// appearance. Vision does it when unset.
	Match *ai.Router
	// Embedder is optional; without it similarity search is unavailable.
	Embedder ai.ImageEmbedder
	// TextEmbedder is optional; it finds the closest-looking subjects
	// before they are compared.
	TextEmbedder ai.TextEmbedder
	// Looks is optional; it tells apart subjects that have no code by how
	// their photos look (see look.go). Without it, words and Match do.
	Looks *looks.Client
	// LookAutoSimilarity and LookMargin override DefaultLook*.
	LookAutoSimilarity, LookMargin float64
	Recorder                       ai.Recorder
	Log                            *slog.Logger

	MaxImageEdge   int
	MaxObjectBytes int64
}

const (
	duplicateWindow  = 10 * time.Second
	duplicateMaxBits = 6
	// Model-only reads are capped below "strict" auto-filing; agreement
	// with on-device OCR is what earns full confidence.
	modelOnlyCap     = 0.85
	ocrConfirmedConf = 0.98
	escalatedAgree   = 0.95
)

func (p *Processor) ProcessPhoto(ctx context.Context, photoID uuid.UUID) error {
	q := dbq.New(p.Pool)
	photo, err := q.GetPhotoByID(ctx, photoID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Deleted with its collection while waiting: nothing to do.
		return nil
	}
	if err != nil {
		return fmt.Errorf("load photo: %w", err)
	}
	if photo.Status == "awaiting_upload" {
		return fmt.Errorf("photo %s has not been uploaded", photoID)
	}
	if photo.ConfigVersion == nil {
		return fmt.Errorf("photo %s has no config version", photoID)
	}
	cfg, err := p.Configs.Version(ctx, photo.TeamID, *photo.ConfigVersion)
	if err != nil {
		return err
	}
	traceTeam(ctx, photo.TeamID, cfg)
	if photo, err = q.MarkPhotoProcessing(ctx, photoID); err != nil {
		return fmt.Errorf("mark processing: %w", err)
	}

	od := DecodeOnDevice(photo.OnDevice)
	ocr := ocrText(od)
	var (
		ex          Extracted
		role        = analysis.RoleContent
		description string
		typeKey     string
		read        *identity.Read
		seen        *analysis.Appearance
		imageForEmb []byte
	)

	switch {
	case p.barcodeRead(cfg, od, &typeKey, &read):
		role, ex.Source, ex.SubjectType = analysis.RoleIdentity, SourceBarcode, typeKey
		if res, model := p.enrichNewSubject(ctx, photo, cfg, typeKey, read); res != nil {
			ex.Model, ex.Tags, description = model, res.Tags, res.Description
		}
	case p.reuseDuplicate(ctx, q, photo, od, &ex, &role, &description):
		if ex.IdentityRead != nil && ex.SubjectType != "" {
			typeKey = ex.SubjectType
			read = &identity.Read{Fields: ex.IdentityRead.Fields, Confidence: ex.IdentityRead.Confidence, BarcodeField: ex.IdentityRead.BarcodeField}
		}
		// Identity is resolved again below; a match by appearance is
		// carried over, as the photo shows the same one.
		if ex.Appearance == nil {
			ex.Anchor = nil
		}
		ex.Resolution, ex.CreatedSubject = nil, false
	default:
		res, img, model, err := p.analyze(ctx, photo, cfg)
		switch {
		case errors.Is(err, ai.ErrNoModel):
			ex.Source = SourceNone
		case err != nil:
			return err
		default:
			imageForEmb = img
			role, description = res.Role, res.Description
			ex.Source, ex.Model = SourceModel, model
			ex.RoleConfidence, ex.DocumentFields, ex.Tags = res.RoleConfidence, res.DocumentFields, res.Tags
			if st, ok := cfg.SubjectType(res.SubjectType); ok && st.ByAppearance() {
				typeKey, seen = st.Key, res.Appearance
				if seen != nil {
					ex.NamedDescription = seen.NamedDescription
				}
			} else if res.SubjectType != "" {
				typeKey = res.SubjectType
				read = p.identityRead(ctx, photo, cfg, res, ocr, img, &ex)
			}
		}
	}

	if read != nil {
		if ex.IdentityRead == nil {
			ex.IdentityRead = &IdentityRead{Fields: read.Fields, Confidence: read.Confidence, BarcodeField: read.BarcodeField}
		}
		ex.SubjectType = typeKey
		if err := p.resolve(ctx, photo, cfg, typeKey, *read, &ex); err != nil {
			return err
		}
	} else if seen != nil {
		st, _ := cfg.SubjectType(typeKey)
		if photo.Assignment == "manual" && photo.SubjectID != nil {
			// A person said who this is on the camera: nothing to match, but
			// it shows what that one looks like. What the camera's match
			// said of it, if it asked, is kept: the person's answer checks it.
			ex.Appearance, ex.SubjectType = &AppearanceMatch{Label: seen.Label, Features: seen.Features}, st.Key
			if raw, err := dbq.New(p.Pool).GetShotVerdict(ctx, dbq.GetShotVerdictParams{TeamID: photo.TeamID, ClientPhotoID: photo.ClientPhotoID}); err == nil {
				var am AppearanceMatch
				if json.Unmarshal(raw, &am) == nil {
					am.Label, am.Features = seen.Label, seen.Features
					ex.Appearance = &am
				}
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if err := p.learnAppearance(ctx, photo, seen); err != nil {
				return err
			}
			// Filed by hand, it is one more photo to compare others with.
			if imageForEmb != nil {
				if err := p.saveLook(ctx, photo.ID, p.lookOf(ctx, SubjectOnly(imageForEmb, od, refEdge*2))); err != nil {
					return err
				}
			}
		} else if err := p.matchAppearance(ctx, photo, cfg, st, seen, imageForEmb, &ex); err != nil {
			return err
		}
	}

	var emb *pgvector.Vector
	if p.Embedder != nil && imageForEmb != nil && (role == analysis.RoleContent || role == analysis.RoleDocument) {
		emb = p.embed(ctx, photo, imageForEmb)
	}

	exJSON, err := json.Marshal(ex)
	if err != nil {
		return err
	}
	if role == analysis.RoleJunk && photo.Assignment == "manual" {
		// A person said who this shows: it is not to be hidden as junk.
		role = analysis.RoleContent
	}
	th := cfg.Thresholds()
	err = store.WithTx(ctx, p.Pool, func(tx pgx.Tx) error {
		qtx := q.WithTx(tx)
		if err := qtx.SavePhotoAnalysis(ctx, dbq.SavePhotoAnalysisParams{
			ID: photo.ID, Role: ptr(string(role)), Extracted: exJSON, Description: description, Embedding: emb,
		}); err != nil {
			return err
		}
		keep := []string{}
		for _, t := range ex.Tags {
			if t.Confidence < th.TagKeep {
				continue
			}
			keep = append(keep, t.Key)
			if err := qtx.UpsertAITag(ctx, dbq.UpsertAITagParams{PhotoID: photo.ID, TagKey: t.Key, Confidence: float32(t.Confidence)}); err != nil {
				return err
			}
		}
		return qtx.DeleteStaleAITags(ctx, dbq.DeleteStaleAITagsParams{PhotoID: photo.ID, Keep: keep})
	})
	if err != nil {
		return fmt.Errorf("save analysis: %w", err)
	}

	if photo.SessionID != nil {
		return p.ResegmentSession(ctx, photo.TeamID, *photo.SessionID)
	}
	return p.fileAlone(ctx, q, photo, cfg, ex)
}

// barcodeRead is tier 0: a barcode the phone already decoded that fits a
// subject type's barcode rule identifies the subject without any model.
func (p *Processor) barcodeRead(cfg *teamconfig.Config, od *pidexv1.OnDeviceAnalysis, typeKey *string, read **identity.Read) bool {
	for i := range cfg.SubjectTypes {
		st := &cfg.SubjectTypes[i]
		for _, bc := range od.GetBarcodes() {
			field, value, ok := identity.FromBarcode(st, bc.GetSymbology(), bc.GetPayload())
			if !ok {
				continue
			}
			if _, norm := identity.Clean(st, map[string]string{field: value}, cfg.DateOrder); len(norm) == 0 {
				continue
			}
			*typeKey = st.Key
			*read = &identity.Read{Fields: map[string]string{field: value}, Confidence: 1, BarcodeField: field}
			return true
		}
	}
	return false
}

// enrichNewSubject handles a barcode that identifies nobody yet. Barcodes
// usually carry only an ID, so a record created from one alone would be a
// bare number. One model call reads the remaining identity fields; they are
// used only if the model read the same ID the barcode encodes, which shows
// it looked at the same label. Known subjects never pay for this call.
func (p *Processor) enrichNewSubject(ctx context.Context, photo dbq.Photo, cfg *teamconfig.Config, typeKey string, read *identity.Read) (*analysis.Result, string) {
	st, _ := cfg.SubjectType(typeKey)
	if p.Vision.Empty() || len(read.Fields) >= len(st.IdentityFields) {
		return nil, ""
	}
	cands, err := p.candidates(ctx, dbq.New(p.Pool), photo.TeamID, st, *read, cfg.DateOrder)
	if err != nil || identity.Resolve(st, *read, cfg.DateOrder, cands).Outcome != identity.OutcomeNewSubject {
		return nil, ""
	}
	res, _, model, err := p.analyze(ctx, photo, cfg)
	if err != nil {
		p.Log.Warn("could not read identity details for new subject", "photo", photo.ID, "err", err)
		return nil, ""
	}
	fields, _ := res.IdentityFields()
	if res.SubjectType != typeKey {
		return res, model
	}
	f, _ := st.Field(read.BarcodeField)
	if v, ok := fields[read.BarcodeField]; !ok ||
		identity.Normalize(f.Kind, v, cfg.DateOrder) != identity.Normalize(f.Kind, read.Fields[read.BarcodeField], cfg.DateOrder) {
		return res, model
	}
	for k, v := range fields {
		if _, set := read.Fields[k]; !set {
			read.Fields[k] = v
		}
	}
	return res, model
}

// reuseDuplicate copies the analysis of a near-identical photo taken
// seconds earlier in the same session, instead of paying for another call.
func (p *Processor) reuseDuplicate(ctx context.Context, q *dbq.Queries, photo dbq.Photo, od *pidexv1.OnDeviceAnalysis, ex *Extracted, role *analysis.Role, description *string) bool {
	hash, ok := parseHash(od.GetPerceptualHash())
	if !ok || photo.SessionID == nil || photo.CapturedAt == nil {
		return false
	}
	siblings, err := q.ListSessionPhotos(ctx, dbq.ListSessionPhotosParams{TeamID: photo.TeamID, SessionID: photo.SessionID})
	if err != nil {
		return false
	}
	for _, s := range siblings {
		if s.ID == photo.ID || s.Status != "processed" || s.Role == nil || s.CapturedAt == nil {
			continue
		}
		if photo.CapturedAt.Sub(*s.CapturedAt).Abs() > duplicateWindow {
			continue
		}
		other, ok := parseHash(DecodeOnDevice(s.OnDevice).GetPerceptualHash())
		if !ok || bits.OnesCount64(hash^other) > duplicateMaxBits {
			continue
		}
		prev := DecodeExtracted(s.Extracted)
		if prev.Source == SourceNone || prev.Source == SourceReused {
			continue
		}
		*ex = prev
		ex.Source, ex.ReusedFrom = SourceReused, &s.ID
		*role, *description = analysis.Role(*s.Role), s.Description
		return true
	}
	return false
}

func parseHash(s string) (uint64, bool) {
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 16, 64)
	return v, err == nil
}

// analyze is tier 1: one cheap call answering every question about the
// photo. It returns the downscaled image for reuse.
func (p *Processor) analyze(ctx context.Context, photo dbq.Photo, cfg *teamconfig.Config) (*analysis.Result, []byte, string, error) {
	if p.Vision.Empty() {
		return nil, nil, "", ai.ErrNoModel
	}
	data, err := p.Storage.Get(ctx, photo.ObjectKey, p.MaxObjectBytes)
	if err != nil {
		return nil, nil, "", err
	}
	img, err := imaging.Fit(data, p.MaxImageEdge)
	if err != nil {
		return nil, nil, "", err
	}
	res, err := p.callVision(ctx, p.Vision, ai.CallMeta{TeamID: photo.TeamID, PhotoID: &photo.ID, Purpose: "analyze"}, cfg, img)
	if err != nil {
		return nil, nil, "", err
	}
	return res, img, p.Vision.Primary(), nil
}

func (p *Processor) callVision(ctx context.Context, r *ai.Router, meta ai.CallMeta, cfg *teamconfig.Config, img []byte) (*analysis.Result, error) {
	raw, _, err := r.Structured(ctx, meta, ai.StructuredRequest{
		Instructions:    analysis.Instructions(cfg),
		Prompt:          analysis.Prompt,
		Images:          []ai.Image{{Data: img, MIMEType: "image/jpeg"}},
		Schema:          analysis.Schema(cfg),
		MaxOutputTokens: 1024,
	})
	if err != nil {
		return nil, fmt.Errorf("vision %s: %w", meta.Purpose, err)
	}
	return analysis.Parse(raw, cfg)
}

// ShotRead is who a shot still on the phone shows: an identity read off a
// label, or, for subjects told apart by appearance, the closest match.
type ShotRead struct {
	Type *teamconfig.SubjectType
	// Read is set for subjects identified by a code.
	Read *identity.Read
	// Seen, Match and Verdict are set for subjects told apart by how they
	// look; Match is nil for one not on file.
	Seen     *analysis.Appearance
	Match    *AppearanceCandidate
	Verdict  AppearanceVerdict
	Compared []uuid.UUID
}

// Appearance is what the look match said, as kept with a photo.
func (s *ShotRead) Appearance() *AppearanceMatch {
	v := s.Verdict
	am := &AppearanceMatch{Label: s.Seen.Label, Features: s.Seen.Features, Compared: s.Compared,
		Confidence: v.Confidence, ModelConfidence: v.ModelConfidence, Reason: v.Reason, Unseen: v.Unseen}
	if s.Match != nil {
		am.Verdict = &s.Match.ID
	}
	return am
}

// ReadShot reads who a shot still on the phone shows, as the pipeline will
// once it is uploaded, so the camera can show who a group of shots is for.
// Nothing is stored. It returns nil when the shot shows nobody.
func (p *Processor) ReadShot(ctx context.Context, teamID uuid.UUID, cfg *teamconfig.Config, jpeg []byte, od *pidexv1.OnDeviceAnalysis) (*ShotRead, error) {
	if p.Vision.Empty() {
		return nil, ai.ErrNoModel
	}
	img, err := imaging.Fit(jpeg, p.MaxImageEdge)
	if err != nil {
		return nil, err
	}
	res, err := p.callVision(ctx, p.Vision, ai.CallMeta{TeamID: teamID, Purpose: "identify_shot"}, cfg, img)
	if err != nil {
		return nil, err
	}
	st, ok := cfg.SubjectType(res.SubjectType)
	if !ok {
		return nil, nil
	}
	if st.ByAppearance() {
		if res.Appearance == nil {
			return nil, nil
		}
		probe := p.appearanceVector(ctx, teamID, nil, res.Appearance.Features)
		subject := SubjectOnly(img, od, refEdge*2)
		cands, v, err := p.appearanceVerdict(ctx, p.Pool, teamID, nil, cfg, st, subject, res.Appearance, probe, p.lookOf(ctx, subject))
		if err != nil {
			return nil, err
		}
		out := &ShotRead{Type: st, Seen: res.Appearance, Verdict: v}
		for _, c := range cands {
			out.Compared = append(out.Compared, c.ID)
		}
		if v.Match >= 0 {
			out.Match = &cands[v.Match]
		}
		return out, nil
	}
	fields, conf := res.IdentityFields()
	read := &identity.Read{Fields: fields, Confidence: min(conf, modelOnlyCap)}
	if ocrConfirms(st, fields, ocrText(od), cfg.DateOrder) {
		read.Confidence = ocrConfirmedConf
	}
	return &ShotRead{Type: st, Read: read}, nil
}

// identityRead scores a model's identity read. Agreement with the phone's
// own OCR is an independent second read; without it the read is capped,
// and strict teams escalate to a stronger model.
func (p *Processor) identityRead(ctx context.Context, photo dbq.Photo, cfg *teamconfig.Config, res *analysis.Result, ocr string, img []byte, ex *Extracted) *identity.Read {
	st, _ := cfg.SubjectType(res.SubjectType)
	fields, modelConf := res.IdentityFields()
	ir := &IdentityRead{Fields: fields}
	switch {
	case ocrConfirms(st, fields, ocr, cfg.DateOrder):
		ir.Confidence, ir.ConfirmedByOCR = ocrConfirmedConf, true
	case p.Strong != nil && !p.Strong.Empty() && cfg.Strictness == teamconfig.StrictnessStrict:
		ir.Escalated = true
		ir.Confidence = min(modelConf, modelOnlyCap)
		strong, err := p.callVision(ctx, p.Strong, ai.CallMeta{TeamID: photo.TeamID, PhotoID: &photo.ID, Purpose: "identity_escalation"}, cfg, img)
		if err != nil {
			p.Log.Warn("identity escalation failed", "photo", photo.ID, "err", err)
			break
		}
		if strong.SubjectType != res.SubjectType {
			break
		}
		strongFields, strongConf := strong.IdentityFields()
		_, a := identity.Clean(st, fields, cfg.DateOrder)
		_, b := identity.Clean(st, strongFields, cfg.DateOrder)
		if len(a) > 0 && mapsEqual(a, b) {
			ir.Confidence = escalatedAgree
		} else {
			ir.Fields, ir.Confidence = strongFields, min(strongConf, modelOnlyCap)
		}
	default:
		ir.Confidence = min(modelConf, modelOnlyCap)
	}
	ex.IdentityRead = ir
	return &identity.Read{Fields: ir.Fields, Confidence: ir.Confidence}
}

func ocrConfirms(st *teamconfig.SubjectType, fields map[string]string, ocr, dateOrder string) bool {
	_, norm := identity.Clean(st, fields, dateOrder)
	if len(norm) == 0 || ocr == "" {
		return false
	}
	for k, v := range norm {
		f, _ := st.Field(k)
		if !identity.ContainsValue(f.Kind, v, ocr, dateOrder) {
			return false
		}
	}
	return true
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// resolve matches a read to a subject, creating one when the read alone is
// strong enough. Matching and creation hold a per-type lock so two photos
// of the same new subject cannot create it twice.
func (p *Processor) resolve(ctx context.Context, photo dbq.Photo, cfg *teamconfig.Config, typeKey string, read identity.Read, ex *Extracted) error {
	st, ok := cfg.SubjectType(typeKey)
	if !ok {
		return nil
	}
	th := cfg.Thresholds()
	return store.WithTx(ctx, p.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", "subject:"+photo.TeamID.String()+":"+typeKey); err != nil {
			return err
		}
		q := dbq.New(tx)
		cands, err := p.candidates(ctx, q, photo.TeamID, st, read, cfg.DateOrder)
		if err != nil {
			return err
		}
		res := identity.Resolve(st, read, cfg.DateOrder, cands)
		ex.Resolution = &res
		switch res.Outcome {
		case identity.OutcomeMatched:
			ex.Anchor = &Anchor{SubjectID: res.SubjectID, Confidence: res.Confidence}
		case identity.OutcomeNewSubject:
			if res.Confidence < th.AutoAssign {
				return nil
			}
			parent, err := p.parentFromSession(ctx, q, photo, st)
			if err != nil {
				return err
			}
			rawJSON, _ := json.Marshal(res.Fields)
			normJSON, _ := json.Marshal(res.Norm)
			subj, err := q.CreateSubject(ctx, dbq.CreateSubjectParams{
				TeamID: photo.TeamID, TypeKey: typeKey, ParentID: parent,
				DisplayName: st.DisplayName(res.Fields), Identity: rawJSON, IdentityNorm: normJSON,
			})
			if err != nil {
				return fmt.Errorf("create subject: %w", err)
			}
			ex.Anchor = &Anchor{SubjectID: subj.ID, Confidence: res.Confidence}
			ex.CreatedSubject = true
			return audit.Log(ctx, q, audit.Entry{
				TeamID: photo.TeamID, Action: "subject.auto_created", TargetType: "subject", TargetID: &subj.ID,
				Details: map[string]any{"from_photo": photo.ID, "confidence": res.Confidence, "type": typeKey},
			})
		}
		return nil
	})
}

func (p *Processor) candidates(ctx context.Context, q *dbq.Queries, teamID uuid.UUID, st *teamconfig.SubjectType, read identity.Read, dateOrder string) ([]identity.Candidate, error) {
	raw, norm := identity.Clean(st, read.Fields, dateOrder)
	probes := make([][]byte, 0, len(norm))
	for k, v := range norm {
		b, _ := json.Marshal(map[string]string{k: v})
		probes = append(probes, b)
	}
	nameProbe := ""
	for _, f := range st.IdentityFields {
		if f.Kind == teamconfig.KindPersonName && raw[f.Key] != "" {
			nameProbe = raw[f.Key]
			break
		}
	}
	rows, err := q.FindSubjectCandidates(ctx, dbq.FindSubjectCandidatesParams{
		TeamID: teamID, TypeKey: st.Key, Probes: probes, NameProbe: nameProbe,
	})
	if err != nil {
		return nil, fmt.Errorf("find candidates: %w", err)
	}
	out := make([]identity.Candidate, 0, len(rows))
	for _, r := range rows {
		var n map[string]string
		_ = json.Unmarshal(r.IdentityNorm, &n)
		out = append(out, identity.Candidate{ID: r.ID, Norm: n})
	}
	return out, nil
}

// parentFromSession links a new child subject (a unit) to the latest parent
// subject (its project) identified earlier in the same session.
func (p *Processor) parentFromSession(ctx context.Context, q *dbq.Queries, photo dbq.Photo, st *teamconfig.SubjectType) (*uuid.UUID, error) {
	if st.Parent == "" || photo.SessionID == nil {
		return nil, nil
	}
	photos, err := q.ListSessionPhotos(ctx, dbq.ListSessionPhotosParams{TeamID: photo.TeamID, SessionID: photo.SessionID})
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	anchorAt := map[uuid.UUID]time.Time{}
	for _, s := range photos {
		if s.ID == photo.ID || s.CapturedAt == nil || photo.CapturedAt == nil || s.CapturedAt.After(*photo.CapturedAt) {
			continue
		}
		if a := DecodeExtracted(s.Extracted).Anchor; a != nil {
			ids = append(ids, a.SubjectID)
			if t, ok := anchorAt[a.SubjectID]; !ok || s.CapturedAt.After(t) {
				anchorAt[a.SubjectID] = *s.CapturedAt
			}
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	subjects, err := q.GetSubjectsByIDs(ctx, dbq.GetSubjectsByIDsParams{TeamID: photo.TeamID, Ids: ids})
	if err != nil {
		return nil, err
	}
	var best *uuid.UUID
	var bestAt time.Time
	for _, s := range subjects {
		if s.TypeKey != st.Parent {
			continue
		}
		if t := anchorAt[s.ID]; best == nil || t.After(bestAt) {
			id := s.ID
			best, bestAt = &id, t
		}
	}
	return best, nil
}

func (p *Processor) embed(ctx context.Context, photo dbq.Photo, img []byte) *pgvector.Vector {
	vec, usage, err := p.Embedder.EmbedImage(ctx, ai.Image{Data: img, MIMEType: "image/jpeg"})
	if p.Recorder != nil {
		p.Recorder.Record(ctx, ai.CallRecord{CallMeta: ai.CallMeta{TeamID: photo.TeamID, PhotoID: &photo.ID, Purpose: "embed"}, Usage: usage, Err: err})
	}
	if err != nil {
		p.Log.Warn("embedding failed; photo stays searchable by filters", "photo", photo.ID, "err", err)
		return nil
	}
	v := pgvector.NewVector(vec)
	return &v
}

// fileAlone handles a photo taken outside any session: it can only be
// filed by its own identity.
func (p *Processor) fileAlone(ctx context.Context, q *dbq.Queries, photo dbq.Photo, cfg *teamconfig.Config, ex Extracted) error {
	d := decision{assignment: "unassigned"}
	if ex.Anchor != nil {
		d = decide(cfg.Thresholds(), &ex.Anchor.SubjectID, ex.Anchor.Confidence)
	}
	_, err := q.UpdateAutoAssignment(ctx, dbq.UpdateAutoAssignmentParams{
		ID: photo.ID, SubjectID: d.subject, SuggestedSubjectID: d.suggested,
		Assignment: d.assignment, AssignmentConfidence: float32(d.confidence),
	})
	return err
}

// MarkFailed records a photo whose processing gave up. It stays filed by
// its session's anchors and can be reprocessed.
func (p *Processor) MarkFailed(ctx context.Context, photoID uuid.UUID, cause error) error {
	return dbq.New(p.Pool).MarkPhotoFailed(ctx, dbq.MarkPhotoFailedParams{ID: photoID, ProcessingError: cause.Error()})
}

func ptr[T any](v T) *T { return &v }

// NameDescription gives a photo filed before descriptions named their
// subject the named one, by describing it again; nothing else changes.
func (p *Processor) NameDescription(ctx context.Context, photoID uuid.UUID) error {
	photo, err := dbq.New(p.Pool).GetPhotoByID(ctx, photoID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load photo: %w", err)
	}
	ex := DecodeExtracted(photo.Extracted)
	if photo.Status != "processed" || photo.ConfigVersion == nil || ex.Appearance == nil || ex.NamedDescription != "" {
		return nil
	}
	cfg, err := p.Configs.Version(ctx, photo.TeamID, *photo.ConfigVersion)
	if err != nil {
		return err
	}
	traceTeam(ctx, photo.TeamID, cfg)
	res, _, _, err := p.analyze(ctx, photo, cfg)
	if err != nil {
		return err
	}
	if res.Appearance == nil || res.Appearance.NamedDescription == "" {
		return nil
	}
	_, err = p.Pool.Exec(ctx, `UPDATE photos SET extracted = jsonb_set(extracted, '{named_description}', to_jsonb($2::text)) WHERE id = $1`,
		photo.ID, res.Appearance.NamedDescription)
	return err
}

// traceTeam notes the team in the trace of the job at hand, and lets the
// trace carry what models were told and said unless the team's privacy is
// high.
func traceTeam(ctx context.Context, teamID uuid.UUID, cfg *teamconfig.Config) {
	tracing.From(ctx).Set("team_id", teamID)
	tracing.Allow(ctx, cfg.Privacy != teamconfig.PrivacyHigh)
}
