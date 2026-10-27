package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"

	"pidex/internal/ai"
	"pidex/internal/store/dbq"
	"pidex/internal/tracing"
)

// A subject told apart by look gets a profile: what it looks like in full,
// written from several of its photos at once. A black nose belongs to the
// cat, not to any one photo, and many photos do not show it; searching
// subjects by their profile finds "the cat with the black nose".
const (
	// profilePhotos are the photos a profile is written from: different
	// shoots and angles, the clearest first.
	profilePhotos = 4
	profileEdge   = 768
	// profileBatch bounds the subjects profiled in one sweep.
	profileBatch = 10
)

const profileInstructions = `You describe one individual (a pet, a mug, a plant, a tool) from several photos of it, so that people can later find it by how it looks.

Rules:
- Give short facts, each about one feature: colours and pattern of the coat or surface; head and face; nose; eyes and their colour; ears; paws, feet or base; tail; size and build; distinctive marks, damage or wear; collar, tag, print or other accessories.
- Only what is clearly visible in at least one photo. Leave out any feature no photo shows clearly; never guess.
- Not what it is doing, where it is, the light or the background.`

// ProfileSubjects writes the profile of subjects that have none yet, or
// have twice the photos they had when it was written. Run now and then:
// failures are logged and left for the next run.
func (p *Processor) ProfileSubjects(ctx context.Context) error {
	if p.Vision.Empty() {
		return nil
	}
	rows, err := dbq.New(p.Pool).SubjectsToProfile(ctx, profileBatch)
	if err != nil {
		return fmt.Errorf("subjects to profile: %w", err)
	}
	for _, s := range rows {
		if err := p.profileSubject(ctx, s); err != nil {
			p.Log.Warn("subject profile failed", "subject", s.ID, "err", err)
		}
	}
	return nil
}

// Each subject profiled is a trace of its own: subjects of many teams are
// profiled in one run.
func (p *Processor) profileSubject(ctx context.Context, s dbq.SubjectsToProfileRow) (err error) {
	ctx, sp := tracing.Start(ctx, "subject profile", tracing.Task)
	defer func() { sp.End(err) }()
	sp.Tag("subject profile")
	sp.Set("subject_id", s.ID)
	cfg, _, err := p.Configs.Active(ctx, s.TeamID)
	if err != nil {
		return err
	}
	traceTeam(ctx, s.TeamID, cfg)
	st, ok := cfg.SubjectType(s.TypeKey)
	if !ok || !st.ByAppearance() {
		return nil
	}
	images, err := p.subjectPhotos(ctx, p.Pool, s.TeamID, uuid.Nil, s.ID, s.AppearancePhotoID, profilePhotos, profileEdge)
	if err != nil || len(images) == 0 {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Kind: %s. What tells them apart: %s\n", st.Name, st.Carriers)
	fmt.Fprintf(&b, "The %d photos all show the same one. Describe it.", len(images))
	if cfg.Language != "" && cfg.Language != "en" {
		fmt.Fprintf(&b, "\nWrite in the language with BCP 47 tag %s.", cfg.Language)
	}
	parts := make([]ai.Image, len(images))
	for i, img := range images {
		parts[i] = ai.Image{Data: img, MIMEType: "image/jpeg"}
	}
	raw, _, err := p.Vision.Structured(ctx, ai.CallMeta{TeamID: s.TeamID, Purpose: "subject_profile"}, ai.StructuredRequest{
		Instructions: profileInstructions,
		Prompt:       b.String(),
		Images:       parts,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"facts": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required":             []string{"facts"},
			"additionalProperties": false,
		},
		MaxOutputTokens: 800,
	})
	if err != nil {
		return err
	}
	var ans struct {
		Facts []string `json:"facts"`
	}
	if err := json.Unmarshal(raw, &ans); err != nil {
		return fmt.Errorf("subject profile: %w", err)
	}
	var facts []string
	for _, f := range ans.Facts {
		if f = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(f), "-")); f != "" {
			facts = append(facts, f)
		}
	}
	if len(facts) == 0 {
		return nil
	}
	return p.SaveProfile(ctx, s.TeamID, s.ID, strings.Join(facts, "\n"), int32(s.Photos), false)
}

// SaveProfile keeps a subject's profile with its vector, for finding the
// subject by it. One a person wrote is theirs: written again only by a
// person.
func (p *Processor) SaveProfile(ctx context.Context, teamID, subjectID uuid.UUID, profile string, photos int32, byPerson bool) error {
	var vec *pgvector.Vector
	if p.TextEmbedder != nil && strings.TrimSpace(profile) != "" {
		vecs, usage, err := p.TextEmbedder.EmbedTexts(ctx, []string{profile})
		if p.Recorder != nil {
			p.Recorder.Record(ctx, ai.CallRecord{CallMeta: ai.CallMeta{TeamID: teamID, Purpose: "profile_embed"}, Usage: usage, Err: err})
		}
		if err != nil {
			return fmt.Errorf("profile embedding: %w", err)
		}
		v := pgvector.NewVector(vecs[0])
		vec = &v
	}
	return dbq.New(p.Pool).SetSubjectProfile(ctx, dbq.SetSubjectProfileParams{
		TeamID: teamID, ID: subjectID, Profile: profile, ProfileEmbedding: vec, ProfilePhotos: photos, ByPerson: byPerson,
	})
}
