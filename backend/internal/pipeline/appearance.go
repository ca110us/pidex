package pipeline

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/internal/ai"
	"pidex/internal/analysis"
	"pidex/internal/audit"
	"pidex/internal/imaging"
	"pidex/internal/looks"
	"pidex/internal/store"
	"pidex/internal/store/dbq"
	"pidex/internal/teamconfig"
)

// Matching by appearance: subjects nobody labels (pets, mugs, plants) are
// told apart by how they look. A photo's subject is compared with the
// closest-looking subjects already filed, through photos of each, and the
// vision model says which one it is or that it is a new one. Unsure answers
// become suggestions a person confirms with one tap.
const (
	// maxCompared bounds the subjects shown to the model in one call; the
	// rest are found first by how their descriptions read.
	maxCompared = 6
	// refsPerSubject are the photos of each subject the model compares:
	// more angles, fewer mistakes, at a little more per call. One with a
	// look-alike among those compared shows more, where the difference
	// may be one patch only some angles show.
	refsPerSubject   = 3
	refsPerLookalike = 5
	refEdge          = 512
	// subjectMargin keeps a little of what is around a subject when it is
	// cut out of its photo, so its outline is not lost.
	subjectMargin = 0.15
	// lookalikeSimilarity is how alike two subjects' descriptions must be
	// for neither to be filed without asking: two tuxedo cats, two mugs of
	// one make.
	lookalikeSimilarity = 0.85
	// sureByLook is how sure a match by appearance must be to be filed
	// without asking. Measured on photos of look-alike cats: every match
	// this sure was right, while wrong ones scored up to 0.96, among them a
	// new cat taken for a known one before the new one was on file.
	sureByLook       = 0.97
	sureByLookStrict = 0.99
	// aloneConfidence files the first subject of its type: there is
	// nothing it could be confused with.
	aloneConfidence = 0.99
	// byLookConfidence is a photo filed by its look alone; a person
	// correcting it counts as the system wrong at its surest.
	byLookConfidence = 0.98
	// A match is trusted, and filed without asking below sureByLook or
	// between look-alikes, once people checking the model's answers have
	// found it right trustAfter times running, in separate shoots, at
	// least as sure as it is now, among the last trustWindow checks of that
	// subject and its look-alikes. One mistake takes the trust back. Never
	// below trustFloor, and never for strict teams.
	trustAfter  = 8
	trustWindow = 30
	trustFloor  = 0.8
)

// AppearanceCandidate is a subject a photo may show, with what it looks
// like.
type AppearanceCandidate struct {
	ID          uuid.UUID
	DisplayName string
	Appearance  string
	Refs        [][]byte
	// Vec is the description's vector, when it has one.
	Vec []float32
}

// AppearanceVerdict is the model's answer: which candidate, or none.
type AppearanceVerdict struct {
	// Match indexes the candidates; -1 is a new subject.
	Match      int
	Confidence float64
	// ModelConfidence is the model's own; Confidence is after the rules.
	ModelConfidence float64
	Reason          string
	// Unseen: the photo did not show one clearly enough to tell.
	Unseen bool
	// ByLook: decided by the look service alone, without the model.
	ByLook bool
	// Look is how similar the closest subject's photos look (0 without a
	// look service).
	Look float64
}

// LookCheck is one answer of the model a person checked: the subject it
// named and the one the person filed the photo under.
type LookCheck struct {
	Verdict, Final uuid.UUID
	// Confidence is the model's own.
	Confidence float64
	// Session is the shoot the photo was taken in; checks from one shoot
	// count once.
	Session uuid.UUID
}

// matchAppearance files a photo whose subject is told apart by appearance:
// as an existing subject, as a new one, or not at all when unsure.
func (p *Processor) matchAppearance(ctx context.Context, photo dbq.Photo, cfg *teamconfig.Config, st *teamconfig.SubjectType, seen *analysis.Appearance, img []byte, ex *Extracted) error {
	am := &AppearanceMatch{Label: seen.Label, Features: seen.Features}
	ex.Appearance, ex.SubjectType = am, st.Key
	probe := p.appearanceVector(ctx, photo.TeamID, &photo.ID, seen.Features)
	subject := SubjectOnly(img, DecodeOnDevice(photo.OnDevice), refEdge*2)
	look := p.lookOf(ctx, subject)
	if err := p.saveLook(ctx, photo.ID, look); err != nil {
		return err
	}
	th := cfg.Thresholds()
	return store.WithTx(ctx, p.Pool, func(tx pgx.Tx) error {
		// One photo at a time per type, so two photos of a new subject
		// cannot each create it.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", "subject:"+photo.TeamID.String()+":"+st.Key); err != nil {
			return err
		}
		cands, v, err := p.appearanceVerdict(ctx, tx, photo.TeamID, &photo.ID, cfg, st, subject, seen, probe, look)
		if err != nil {
			return err
		}
		for _, c := range cands {
			am.Compared = append(am.Compared, c.ID)
		}
		am.Confidence, am.ModelConfidence, am.Reason, am.Unseen = v.Confidence, v.ModelConfidence, v.Reason, v.Unseen
		am.ByLook, am.Look = v.ByLook, v.Look
		switch {
		case v.Match >= 0:
			id := cands[v.Match].ID
			am.Verdict = &id
			// A low confidence still anchors: the photo and its neighbours
			// become suggestions to confirm, not silent filings.
			ex.Anchor = &Anchor{SubjectID: id, Confidence: v.Confidence}
		case v.Confidence >= th.Suggest:
			subj, err := p.createAppearanceSubject(ctx, tx, photo, st, seen, probe)
			if err != nil {
				return err
			}
			ex.Anchor = &Anchor{SubjectID: subj, Confidence: v.Confidence}
			ex.CreatedSubject = true
		}
		return nil
	})
}

// appearanceVerdict compares a subject, cut out of its photo, with the
// closest-looking subjects on file. A match to one of two look-alikes is
// never certain enough to file without asking.
//
// With a look, the subjects whose photos look most like it come first, and
// one that surely is it is taken without asking the model (see look.go).
func (p *Processor) appearanceVerdict(ctx context.Context, q queryer, teamID uuid.UUID, photoID *uuid.UUID, cfg *teamconfig.Config, st *teamconfig.SubjectType,
	subject []byte, seen *analysis.Appearance, probe *pgvector.Vector, look *looks.Look) ([]AppearanceCandidate, AppearanceVerdict, error) {
	matches, err := lookMatches(ctx, q, teamID, photoID, st.Key, look)
	if err != nil {
		return nil, AppearanceVerdict{}, err
	}
	closest := 0.0
	if len(matches) > 0 {
		closest = matches[0].Similarity
	}
	if auto, margin := p.lookThresholds(); SureByLook(matches, cfg, auto, margin) {
		m := matches[0]
		var name string
		if err := q.QueryRow(ctx, `SELECT display_name FROM subjects WHERE id = $1`, m.SubjectID).Scan(&name); err != nil {
			return nil, AppearanceVerdict{}, fmt.Errorf("look match: %w", err)
		}
		next := "no other subject"
		if len(matches) > 1 {
			next = fmt.Sprintf("the next %.2f", matches[1].Similarity)
		}
		return []AppearanceCandidate{{ID: m.SubjectID, DisplayName: name}}, AppearanceVerdict{
			Match: 0, Confidence: byLookConfidence, ModelConfidence: byLookConfidence, ByLook: true, Look: m.Similarity,
			Reason: fmt.Sprintf("Looks like the photos of %s (similarity %.2f, %s).", name, m.Similarity, next),
		}, nil
	}
	first := make([]uuid.UUID, len(matches))
	for i, m := range matches {
		first[i] = m.SubjectID
	}
	cands, err := p.appearanceCandidates(ctx, q, teamID, photoID, st.Key, probe, first)
	if err != nil || len(cands) == 0 {
		return cands, AppearanceVerdict{Match: -1, Confidence: aloneConfidence, Look: closest}, err
	}
	v, err := CompareAppearance(ctx, p.matcher(), ai.CallMeta{TeamID: teamID, PhotoID: photoID, Purpose: "appearance_match"}, st, subject, seen, cands)
	if err != nil {
		return cands, v, err
	}
	checks, err := lookChecks(ctx, q, teamID, st.Key)
	if err != nil {
		return cands, v, err
	}
	v = AppearanceRules(v, cands, cfg, checks)
	v.Look = closest
	return cands, v, nil
}

// lookChecks are the model's answers people have checked in a team, newest
// first: photos it compared by look that a person then filed. Subjects
// merged since count as the one they became.
func lookChecks(ctx context.Context, q queryer, teamID uuid.UUID, typeKey string) ([]LookCheck, error) {
	rows, err := q.Query(ctx, `
SELECT coalesce(v.merged_into_id, v.id), coalesce(f.merged_into_id, f.id),
    coalesce((p.extracted->'appearance'->>'model_confidence')::float8, (p.extracted->'appearance'->>'confidence')::float8, 0),
    coalesce(p.session_id, p.id)
FROM photos p
JOIN subjects v ON v.team_id = p.team_id AND v.id::text = p.extracted->'appearance'->>'verdict'
JOIN subjects f ON f.team_id = p.team_id AND f.id = p.subject_id
WHERE p.team_id = $1 AND p.assignment = 'manual' AND v.type_key = $2
ORDER BY p.updated_at DESC
LIMIT 500`, teamID, typeKey)
	if err != nil {
		return nil, fmt.Errorf("look checks: %w", err)
	}
	checks, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (LookCheck, error) {
		var c LookCheck
		return c, r.Scan(&c.Verdict, &c.Final, &c.Confidence, &c.Session)
	})
	if err != nil {
		return nil, fmt.Errorf("look checks: %w", err)
	}
	return checks, nil
}

// AppearanceRules keeps a match by appearance from being filed without
// asking unless the model is all but certain and no other subject reads
// alike. Unsure matches stay suggestions: one tap to confirm.
func AppearanceRules(v AppearanceVerdict, cands []AppearanceCandidate, cfg *teamconfig.Config, checks []LookCheck) AppearanceVerdict {
	th := cfg.Thresholds()
	if v.Match >= 0 && cfg.Strictness != teamconfig.StrictnessStrict && v.Confidence >= trustFloor {
		if bar, right := trustBar(checks, lookalikeGroup(cands, v.Match)); right >= trustAfter && v.Confidence >= bar {
			v.Confidence = max(v.Confidence, th.AutoAssign)
			v.Reason = strings.TrimSpace(fmt.Sprintf("%s Right the last %d times a person checked.", v.Reason, right))
			return v
		}
	}
	v = GuardLookalikes(v, cands, th)
	sure := sureByLook
	if cfg.Strictness == teamconfig.StrictnessStrict {
		sure = sureByLookStrict
	}
	if v.Match >= 0 && v.Confidence >= th.AutoAssign && v.Confidence < sure {
		v.Confidence = th.AutoAssign - 0.01
	}
	return v
}

// RefCount is how many photos of the i-th of the subjects compared are
// shown: more for one described like another of them.
func RefCount(vecs [][]float32, i int) int {
	for j, v := range vecs {
		if j != i && cosine(vecs[i], v) >= lookalikeSimilarity {
			return refsPerLookalike
		}
	}
	return refsPerSubject
}

// lookalikeGroup is the subject picked and those described like it.
func lookalikeGroup(cands []AppearanceCandidate, pick int) map[uuid.UUID]bool {
	group := map[uuid.UUID]bool{cands[pick].ID: true}
	for i, c := range cands {
		if i != pick && cosine(cands[pick].Vec, c.Vec) >= lookalikeSimilarity {
			group[c.ID] = true
		}
	}
	return group
}

// trustBar is how sure the model must be for its answers naming one of a
// group to be trusted, and how many checks in a row back that up: the
// lowest confidence at or above which every recent check was right. A shoot
// counts once, by its worst answer.
func trustBar(checks []LookCheck, group map[uuid.UUID]bool) (bar float64, right int) {
	var recent []LookCheck
	bySession := map[uuid.UUID]int{}
	for _, c := range checks {
		if !group[c.Verdict] {
			continue
		}
		if i, ok := bySession[c.Session]; ok {
			if worse(c, recent[i]) {
				recent[i] = c
			}
			continue
		}
		if len(recent) == trustWindow {
			break
		}
		bySession[c.Session] = len(recent)
		recent = append(recent, c)
	}
	// Surest first; at one confidence, a mistake before a right answer.
	rightness := func(c LookCheck) int {
		if c.Verdict != c.Final {
			return 0
		}
		return 1
	}
	slices.SortStableFunc(recent, func(a, b LookCheck) int {
		return cmp.Or(cmp.Compare(b.Confidence, a.Confidence), cmp.Compare(rightness(a), rightness(b)))
	})
	bar = 1
	for _, c := range recent {
		if c.Verdict != c.Final {
			break
		}
		right++
		bar = c.Confidence
	}
	return bar, right
}

// worse is a mistake over a right answer, else the less sure.
func worse(a, b LookCheck) bool {
	if wa, wb := a.Verdict != a.Final, b.Verdict != b.Final; wa != wb {
		return wa
	}
	return a.Confidence < b.Confidence
}

// GuardLookalikes keeps a match to one of two subjects that read alike
// below filing without asking, however sure the model was.
func GuardLookalikes(v AppearanceVerdict, cands []AppearanceCandidate, th teamconfig.Thresholds) AppearanceVerdict {
	if v.Match < 0 || v.Confidence < th.AutoAssign {
		return v
	}
	pick := cands[v.Match]
	for i, c := range cands {
		if i != v.Match && cosine(pick.Vec, c.Vec) >= lookalikeSimilarity {
			v.Confidence = min(v.Confidence, th.AutoAssign-0.01)
			v.Reason = strings.TrimSpace(v.Reason + " Looks like " + c.DisplayName + " too: confirm.")
			break
		}
	}
	return v
}

func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// SubjectOnly cuts a photo's main subject out of it, where the phone found
// one, so a comparison sees the cat and not the sofa.
func SubjectOnly(img []byte, od *pidexv1.OnDeviceAnalysis, edge int) []byte {
	b := od.GetSubjectBox()
	if b == nil {
		return img
	}
	cut, err := imaging.Crop(img, imaging.Box{X: float64(b.X), Y: float64(b.Y), W: float64(b.Width), H: float64(b.Height)}, subjectMargin, edge)
	if err != nil {
		return img
	}
	return cut
}

// queryer is a pool or a transaction.
type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (p *Processor) matcher() *ai.Router {
	if !p.Match.Empty() {
		return p.Match
	}
	return p.Vision
}

// appearanceVector embeds what tells a subject apart, for finding the
// closest-looking subjects first. Without a text embedder the most recent
// subjects are compared instead.
func (p *Processor) appearanceVector(ctx context.Context, teamID uuid.UUID, photoID *uuid.UUID, features string) *pgvector.Vector {
	if p.TextEmbedder == nil || strings.TrimSpace(features) == "" {
		return nil
	}
	vecs, usage, err := p.TextEmbedder.EmbedTexts(ctx, []string{features})
	if p.Recorder != nil {
		p.Recorder.Record(ctx, ai.CallRecord{CallMeta: ai.CallMeta{TeamID: teamID, PhotoID: photoID, Purpose: "appearance_embed"}, Usage: usage, Err: err})
	}
	if err != nil {
		p.Log.Warn("appearance embedding failed; comparing the latest subjects", "team", teamID, "err", err)
		return nil
	}
	v := pgvector.NewVector(vecs[0])
	return &v
}

// appearanceCandidates are the subjects of a type that look most like the
// photo's, each with a few photos of it, cut down to the subject: clear
// ones from different shoots, those a person filed first, then the one
// that introduced it, then the latest.
//
// Subjects in first (the closest by look) come first, in its order; words
// fill the rest.
func (p *Processor) appearanceCandidates(ctx context.Context, q queryer, teamID uuid.UUID, photoID *uuid.UUID, typeKey string, probe *pgvector.Vector, first []uuid.UUID) ([]AppearanceCandidate, error) {
	if first == nil {
		first = []uuid.UUID{}
	}
	rows, err := q.Query(ctx, `
SELECT id, display_name, appearance, appearance_photo_id, appearance_embedding FROM subjects
WHERE team_id = $1 AND type_key = $2 AND merged_into_id IS NULL
ORDER BY array_position($5::uuid[], id) NULLS LAST,
    CASE WHEN $3::vector IS NULL OR appearance_embedding IS NULL THEN 1 ELSE 0 END,
    appearance_embedding <=> $3::vector, created_at DESC
LIMIT $4`, teamID, typeKey, probe, maxCompared, first)
	if err != nil {
		return nil, fmt.Errorf("appearance candidates: %w", err)
	}
	type row struct {
		c     AppearanceCandidate
		intro *uuid.UUID
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
		var x row
		var vec *pgvector.Vector
		err := r.Scan(&x.c.ID, &x.c.DisplayName, &x.c.Appearance, &x.intro, &vec)
		if vec != nil {
			x.c.Vec = vec.Slice()
		}
		return x, err
	})
	if err != nil {
		return nil, fmt.Errorf("appearance candidates: %w", err)
	}
	exclude := uuid.Nil
	if photoID != nil {
		exclude = *photoID
	}
	out := make([]AppearanceCandidate, 0, len(list))
	vecs := make([][]float32, len(list))
	for i, x := range list {
		vecs[i] = x.c.Vec
	}
	for i, x := range list {
		x.c.Refs, err = p.subjectPhotos(ctx, q, teamID, exclude, x.c.ID, x.intro, RefCount(vecs, i), refEdge)
		if err != nil {
			return nil, err
		}
		// A subject with neither photos nor words cannot be compared.
		if len(x.c.Refs) > 0 || x.c.Appearance != "" {
			out = append(out, x.c)
		}
	}
	return out, nil
}

// subjectPhotos are up to n photos of a subject, cut down to it, the best
// to tell it by first. Photos where the subject itself was seen clearly
// come first: one that did not show it clearly, or one that only followed
// its neighbours (the bowl, not the cat), gives nothing to go by. Then one
// from each shoot, for different days and angles; then those a person
// filed, the one that introduced it, and the latest.
func (p *Processor) subjectPhotos(ctx context.Context, q queryer, teamID, exclude, subjectID uuid.UUID, intro *uuid.UUID, n, edge int) ([][]byte, error) {
	rows, err := q.Query(ctx, `
SELECT object_key, on_device FROM (
    SELECT object_key, on_device, id, assignment, coalesce(captured_at, created_at) AS at,
        extracted->'appearance' IS NOT NULL AND NOT coalesce((extracted->'appearance'->>'unseen')::bool, false) AS clear,
        row_number() OVER (PARTITION BY session_id ORDER BY (assignment = 'manual') DESC, coalesce(captured_at, created_at) DESC) AS nth
    FROM photos
    WHERE team_id = $1 AND id <> $2 AND status IN ('uploaded', 'processing', 'processed')
      AND ((subject_id = $3 AND assignment IN ('auto', 'manual')) OR id = $4)
) r
ORDER BY clear DESC, nth = 1 DESC, (assignment = 'manual') DESC, (id = $4) DESC, at DESC
LIMIT $5`, teamID, exclude, subjectID, intro, n)
	if err != nil {
		return nil, fmt.Errorf("subject photos: %w", err)
	}
	type ref struct {
		key string
		od  []byte
	}
	found, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ref, error) {
		var f ref
		return f, r.Scan(&f.key, &f.od)
	})
	if err != nil {
		return nil, fmt.Errorf("subject photos: %w", err)
	}
	var out [][]byte
	for _, f := range found {
		data, err := p.Storage.Get(ctx, f.key, p.MaxObjectBytes)
		if err != nil {
			p.Log.Warn("subject photo unreadable", "subject", subjectID, "key", f.key, "err", err)
			continue
		}
		if small, err := imaging.Fit(data, edge*2); err == nil {
			out = append(out, SubjectOnly(small, DecodeOnDevice(f.od), edge))
		}
	}
	return out, nil
}

// learnAppearance gives a subject named by hand, which has no description
// yet, the one seen in a photo of it, so later photos can be matched to it.
func (p *Processor) learnAppearance(ctx context.Context, photo dbq.Photo, seen *analysis.Appearance) error {
	probe := p.appearanceVector(ctx, photo.TeamID, &photo.ID, seen.Features)
	_, err := p.Pool.Exec(ctx, `UPDATE subjects SET appearance = $3, appearance_embedding = $4, appearance_photo_id = $5
WHERE team_id = $1 AND id = $2 AND appearance = ''`, photo.TeamID, *photo.SubjectID, seen.Features, probe, photo.ID)
	return err
}

func (p *Processor) createAppearanceSubject(ctx context.Context, tx pgx.Tx, photo dbq.Photo, st *teamconfig.SubjectType, seen *analysis.Appearance, probe *pgvector.Vector) (uuid.UUID, error) {
	q := dbq.New(tx)
	parent, err := p.parentFromSession(ctx, q, photo, st)
	if err != nil {
		return uuid.Nil, err
	}
	name, err := uniqueName(ctx, tx, photo.TeamID, st, seen.Label)
	if err != nil {
		return uuid.Nil, err
	}
	subj, err := q.CreateSubject(ctx, dbq.CreateSubjectParams{
		TeamID: photo.TeamID, TypeKey: st.Key, ParentID: parent,
		DisplayName: name, Identity: []byte("{}"), IdentityNorm: []byte("{}"),
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("create subject: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE subjects SET appearance = $2, appearance_embedding = $3, appearance_photo_id = $4 WHERE id = $1`,
		subj.ID, seen.Features, probe, photo.ID); err != nil {
		return uuid.Nil, fmt.Errorf("save appearance: %w", err)
	}
	return subj.ID, audit.Log(ctx, q, audit.Entry{
		TeamID: photo.TeamID, Action: "subject.auto_created", TargetType: "subject", TargetID: &subj.ID,
		Details: map[string]any{"from_photo": photo.ID, "type": st.Key, "by": "appearance"},
	})
}

// uniqueName names a new subject from its look, numbered when another of
// its type already has that name ("Orange tabby 2"), until someone names it.
func uniqueName(ctx context.Context, tx pgx.Tx, teamID uuid.UUID, st *teamconfig.SubjectType, label string) (string, error) {
	base := strings.TrimSpace(label)
	if base == "" {
		base = st.Name
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM subjects WHERE team_id = $1 AND type_key = $2 AND (display_name = $3 OR display_name LIKE $4)`,
		teamID, st.Key, base, escapeLike(base)+" %").Scan(&n); err != nil {
		return "", err
	}
	if n == 0 {
		return base, nil
	}
	return fmt.Sprintf("%s %d", base, n+1), nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s)
}

const compareInstructions = `You tell individual things apart by how they look: which known one a new photo shows, or that it shows a new one.

Rules:
- Compare the features that tell individuals of this kind apart: colours, markings, patterns, prints or logos, wear or damage, accessories.
- Ignore pose, angle, distance, background, light and image quality: the same one looks different in different photos.
- Several known ones may share a kind and colour. Pick one only when its distinguishing features match; if two are equally likely, give a low confidence.
- Things made identically (two of the same mug, same model and colour) may be impossible to tell apart. Then say so in the reason and give a low confidence rather than guess.
- Answer "new" when the photo clearly shows one that is none of the known ones.
- Answer "unseen" when image 1 does not show one clearly enough to tell which it is: too small, too dark, hidden, or not there.
- Confidence is your probability that your answer is right, from 0 to 1. Calibrate it:
  - 0.9 or more only when a feature specific to that one individual (a marking's shape and place, a notch, a scar, a print, damage) is clearly visible in the new photo and in its photos, and nothing contradicts it.
  - At most 0.7 when the match rests on kind, colour, breed or pattern that other individuals could share, or when the subject is small, blurred, partly hidden or seen from an angle that hides its distinguishing features.
  - The same applies to "new": 0.9 or more only when a distinguishing feature clearly differs from every known one.`

// CompareAppearance asks which candidate a photo shows. The new photo comes
// first, then each candidate's photos in order.
func CompareAppearance(ctx context.Context, model *ai.Router, meta ai.CallMeta, st *teamconfig.SubjectType, img []byte, seen *analysis.Appearance, cands []AppearanceCandidate) (AppearanceVerdict, error) {
	if model.Empty() {
		return AppearanceVerdict{}, ai.ErrNoModel
	}
	images := []ai.Image{{Data: img, MIMEType: "image/jpeg"}}
	letters := make([]string, len(cands))
	var b strings.Builder
	fmt.Fprintf(&b, "Kind: %s. What tells them apart: %s\n\n", st.Name, st.Carriers)
	fmt.Fprintf(&b, "Image 1 is the new photo. Its subject was described as: %s\n\nKnown ones:\n", seen.Features)
	for i, c := range cands {
		letters[i] = string(rune('A' + i))
		fmt.Fprintf(&b, "%s: %q.", letters[i], c.DisplayName)
		if c.Appearance != "" {
			fmt.Fprintf(&b, " Described as: %s.", c.Appearance)
		}
		if len(c.Refs) > 0 {
			first := len(images) + 1
			for _, r := range c.Refs {
				images = append(images, ai.Image{Data: r, MIMEType: "image/jpeg"})
			}
			if len(c.Refs) == 1 {
				fmt.Fprintf(&b, " Image %d.", first)
			} else {
				fmt.Fprintf(&b, " Images %d to %d.", first, len(images))
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("\nWhich known one does image 1 show? Answer with its letter, \"new\" or \"unseen\".")

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"reason":     map[string]any{"type": "string", "description": "The features compared, in one or two sentences."},
			"match":      map[string]any{"type": "string", "enum": append(letters, "new", "unseen")},
			"confidence": map[string]any{"type": "number", "description": "Probability from 0 to 1."},
		},
		"required":             []string{"reason", "match", "confidence"},
		"additionalProperties": false,
	}
	raw, _, err := model.Structured(ctx, meta, ai.StructuredRequest{
		Instructions: compareInstructions, Prompt: b.String(), Images: images, Schema: schema, MaxOutputTokens: 2000,
	})
	if err != nil {
		return AppearanceVerdict{}, fmt.Errorf("appearance compare: %w", err)
	}
	var ans struct {
		Reason     string  `json:"reason"`
		Match      string  `json:"match"`
		Confidence float64 `json:"confidence"`
	}
	if err := json.Unmarshal(raw, &ans); err != nil {
		return AppearanceVerdict{}, fmt.Errorf("appearance compare: %w", err)
	}
	v := AppearanceVerdict{Match: -1, Confidence: max(0, min(1, ans.Confidence)), Reason: strings.TrimSpace(ans.Reason)}
	v.ModelConfidence = v.Confidence
	if strings.EqualFold(strings.TrimSpace(ans.Match), "unseen") {
		// Neither a known one nor a new one: nothing is filed or made by it.
		v.Confidence, v.Unseen = 0, true
		return v, nil
	}
	for i, l := range letters {
		if strings.EqualFold(strings.TrimSpace(ans.Match), l) {
			v.Match = i
		}
	}
	if v.Match < 0 && !strings.EqualFold(strings.TrimSpace(ans.Match), "new") {
		return AppearanceVerdict{}, fmt.Errorf("appearance compare: unknown answer %q", ans.Match)
	}
	return v, nil
}
