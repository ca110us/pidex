package pipeline

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"

	"pidex/internal/imaging"
	"pidex/internal/looks"
	"pidex/internal/store/dbq"
	"pidex/internal/teamconfig"
)

// Filing by look: the look service turns the subject of a photo into a
// vector (an image embedding), and the photo is compared with the photos
// already filed. Words lose what tells two black cats or two black sockets
// apart; the vector keeps it. Measured on cats, dog faces, mugs and products
// (eval/visual), photos are filed by look alone only when the closest
// subject is both very similar and clearly closer than the next; the rest
// go to the vision model, with the closest subjects by look as candidates.
const (
	// DefaultLookAutoSimilarity and DefaultLookMargin keep filing by look
	// alone at least 98% right on every set measured for dinov2_b@336,
	// photos shot in order into an empty collection.
	DefaultLookAutoSimilarity = 0.85
	DefaultLookMargin         = 0.08
)

// LookMatch is a subject and how similar its closest filed photo looks.
type LookMatch struct {
	SubjectID  uuid.UUID
	Similarity float64
}

// SureByLook reports whether the closest subject, first of matches (closest
// first), is surely the one: similar enough, and clearly more than the next.
// Strict teams always have a vision model look as well.
func SureByLook(matches []LookMatch, cfg *teamconfig.Config, auto, margin float64) bool {
	if len(matches) == 0 || cfg.Strictness == teamconfig.StrictnessStrict {
		return false
	}
	next := -1.0
	if len(matches) > 1 {
		next = matches[1].Similarity
	}
	return matches[0].Similarity >= auto && matches[0].Similarity-next >= margin
}

func (p *Processor) lookThresholds() (auto, margin float64) {
	auto, margin = p.LookAutoSimilarity, p.LookMargin
	if auto == 0 {
		auto = DefaultLookAutoSimilarity
	}
	if margin == 0 {
		margin = DefaultLookMargin
	}
	return auto, margin
}

// lookOf is the look of a subject already cut out of its photo, or nil
// without a look service or when it fails: filing then goes on as before.
func (p *Processor) lookOf(ctx context.Context, subject []byte) *looks.Look {
	if p.Looks == nil || len(subject) == 0 {
		return nil
	}
	l, err := p.Looks.Embed(ctx, subject)
	if err != nil {
		p.Log.Warn("look service unavailable; matching without it", "err", err)
		return nil
	}
	return &l
}

// saveLook keeps a photo's look, so later photos are compared with it.
func (p *Processor) saveLook(ctx context.Context, photoID uuid.UUID, l *looks.Look) error {
	if l == nil {
		return nil
	}
	_, err := p.Pool.Exec(ctx, `UPDATE photos SET look_embedding = $2, look_model = $3 WHERE id = $1`,
		photoID, pgvector.NewVector(l.Vector), l.Model)
	return err
}

// lookMatches are the subjects of a type whose filed photos look most like
// l, closest first: each by its closest photo of the same model.
func lookMatches(ctx context.Context, q queryer, teamID uuid.UUID, photoID *uuid.UUID, typeKey string, l *looks.Look) ([]LookMatch, error) {
	if l == nil {
		return nil, nil
	}
	exclude := uuid.Nil
	if photoID != nil {
		exclude = *photoID
	}
	rows, err := q.Query(ctx, `
SELECT p.subject_id, max(1 - (p.look_embedding <=> $3::vector)) AS similarity
FROM photos p JOIN subjects s ON s.id = p.subject_id
WHERE p.team_id = $1 AND p.id <> $2 AND p.look_model = $4 AND p.look_embedding IS NOT NULL
  AND p.assignment IN ('auto', 'manual') AND s.type_key = $5 AND s.merged_into_id IS NULL
GROUP BY p.subject_id
ORDER BY similarity DESC
LIMIT $6`, teamID, exclude, pgvector.NewVector(l.Vector), l.Model, typeKey, maxCompared)
	if err != nil {
		return nil, fmt.Errorf("look matches: %w", err)
	}
	var out []LookMatch
	for rows.Next() {
		var m LookMatch
		if err := rows.Scan(&m.SubjectID, &m.Similarity); err != nil {
			rows.Close()
			return nil, fmt.Errorf("look matches: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// EmbedLook gives a photo filed before there was a look service its look.
func (p *Processor) EmbedLook(ctx context.Context, photoID uuid.UUID) error {
	if p.Looks == nil {
		return nil
	}
	photo, err := dbq.New(p.Pool).GetPhotoByID(ctx, photoID)
	if err != nil || photo.Status != "processed" {
		return nil
	}
	data, err := p.Storage.Get(ctx, photo.ObjectKey, p.MaxObjectBytes)
	if err != nil {
		return err
	}
	img, err := imaging.Fit(data, p.MaxImageEdge)
	if err != nil {
		return nil
	}
	l, err := p.Looks.Embed(ctx, SubjectOnly(img, DecodeOnDevice(photo.OnDevice), refEdge*2))
	if err != nil {
		return err
	}
	return p.saveLook(ctx, photo.ID, &l)
}
