package pipeline

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"pidex/internal/audit"
	"pidex/internal/identity"
	"pidex/internal/segment"
	"pidex/internal/store"
	"pidex/internal/store/dbq"
	"pidex/internal/teamconfig"
)

type decision struct {
	subject    *uuid.UUID
	suggested  *uuid.UUID
	assignment string
	confidence float64
}

func decide(th teamconfig.Thresholds, subject *uuid.UUID, conf float64) decision {
	switch {
	case subject != nil && conf >= th.AutoAssign:
		return decision{subject: subject, assignment: "auto", confidence: conf}
	case subject != nil && conf >= th.Suggest:
		return decision{suggested: subject, assignment: "suggested", confidence: conf}
	default:
		return decision{assignment: "unassigned", confidence: conf}
	}
}

// ResegmentSession re-files every photo in a session from its anchors. It
// runs after any photo in the session changes and holds a per-session lock,
// so whichever run finishes last saw the latest state.
func (p *Processor) ResegmentSession(ctx context.Context, teamID, sessionID uuid.UUID) error {
	cfg, _, err := p.Configs.Active(ctx, teamID)
	if err != nil {
		return err
	}
	th := cfg.Thresholds()
	return store.WithTx(ctx, p.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", "session:"+sessionID.String()); err != nil {
			return err
		}
		q := dbq.New(tx)
		photos, err := q.ListSessionPhotos(ctx, dbq.ListSessionPhotosParams{TeamID: teamID, SessionID: &sessionID})
		if err != nil {
			return err
		}
		// Anchors read before a subject was deleted or merged: a deleted one
		// is someone no longer on file, a merged one its new self.
		var anchored []uuid.UUID
		for _, ph := range photos {
			if a := DecodeExtracted(ph.Extracted).Anchor; a != nil {
				anchored = append(anchored, a.SubjectID)
			}
		}
		current := map[uuid.UUID]uuid.UUID{}
		if len(anchored) > 0 {
			subs, err := q.GetSubjectsByIDs(ctx, dbq.GetSubjectsByIDsParams{TeamID: teamID, Ids: anchored})
			if err != nil {
				return err
			}
			for _, sub := range subs {
				current[sub.ID] = sub.ID
				if sub.MergedIntoID != nil {
					current[sub.ID] = *sub.MergedIntoID
				}
			}
		}
		// Photos shot together of one subject told apart by appearance are
		// one subject: each match to it counts as the surest of them, and
		// one a person confirmed makes all of them certain. One question,
		// not one per photo.
		surest := map[uuid.UUID]float64{}
		for _, ph := range photos {
			ex := DecodeExtracted(ph.Extracted)
			switch {
			case ex.Appearance == nil:
			case ph.Assignment == "manual" && ph.SubjectID != nil:
				surest[*ph.SubjectID] = 1
			case ex.Anchor != nil:
				if id, ok := current[ex.Anchor.SubjectID]; ok {
					surest[id] = max(surest[id], ex.Anchor.Confidence)
				}
			}
		}
		byID := map[uuid.UUID]dbq.Photo{}
		items := make([]segment.Item, 0, len(photos))
		for _, ph := range photos {
			byID[ph.ID] = ph
			it := segment.Item{
				ID:     ph.ID,
				Lat:    ph.Latitude,
				Lng:    ph.Longitude,
				Manual: ph.Assignment == "manual",
				Locked: ph.LockedSubjectID,
			}
			switch {
			case ph.CapturedAt != nil:
				it.CapturedAt = *ph.CapturedAt
			case ph.ReceivedAt != nil:
				it.CapturedAt = *ph.ReceivedAt
			default:
				it.CapturedAt = ph.CreatedAt
			}
			ex := DecodeExtracted(ph.Extracted)
			if a := ex.Anchor; a != nil {
				if now, ok := current[a.SubjectID]; ok {
					a.SubjectID = now
				} else {
					ex.Anchor = nil
				}
			}
			if it.Manual && (ex.IdentityRead != nil || ex.Appearance != nil) && ph.SubjectID != nil {
				// A person confirmed who this identity photo shows.
				it.Anchor = &segment.Anchor{SubjectID: *ph.SubjectID, Confidence: 1}
			} else if a := ex.Anchor; a != nil {
				conf := a.Confidence
				if ex.Appearance != nil {
					conf = surest[a.SubjectID]
				}
				it.Anchor = &segment.Anchor{SubjectID: a.SubjectID, Confidence: conf}
			} else if ex.IdentityRead != nil || ex.Appearance != nil {
				// Someone was seen but not recognized: earlier photos'
				// subject must not carry over to them.
				it.Barrier = true
			}
			items = append(items, it)
		}

		for _, dec := range segment.Assign(items, cfg.Session) {
			ph := byID[dec.PhotoID]
			ex := DecodeExtracted(ph.Extracted)
			var d decision
			switch {
			case ph.Role != nil && *ph.Role == "junk":
				d = decision{assignment: "unassigned"}
			case dec.SubjectID != nil:
				d = decide(th, dec.SubjectID, dec.Confidence)
			default:
				d = suggestFromResolution(th, ex.Resolution)
			}
			if sameAssignment(ph, d) {
				continue
			}
			if _, err := q.UpdateAutoAssignment(ctx, dbq.UpdateAutoAssignmentParams{
				ID: ph.ID, SubjectID: d.subject, SuggestedSubjectID: d.suggested,
				Assignment: d.assignment, AssignmentConfidence: float32(d.confidence),
			}); err != nil {
				return fmt.Errorf("update assignment: %w", err)
			}
			target := d.subject
			if target == nil {
				target = d.suggested
			}
			if err := audit.Log(ctx, q, audit.Entry{
				TeamID: teamID, Action: "photo.auto_filed", TargetType: "photo", TargetID: &ph.ID,
				Details: map[string]any{"assignment": d.assignment, "subject": target, "confidence": d.confidence, "source": dec.Source},
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// suggestFromResolution proposes the best candidate of an ambiguous read,
// so the one-tap review has something to offer.
func suggestFromResolution(th teamconfig.Thresholds, res *identity.Resolution) decision {
	if res == nil || res.Outcome != identity.OutcomeAmbiguous || len(res.Candidates) == 0 || res.Confidence < th.Suggest {
		return decision{assignment: "unassigned"}
	}
	id := res.Candidates[0]
	return decision{suggested: &id, assignment: "suggested", confidence: res.Confidence}
}

func sameAssignment(ph dbq.Photo, d decision) bool {
	return ph.Assignment == d.assignment &&
		uuidEq(ph.SubjectID, d.subject) &&
		uuidEq(ph.SuggestedSubjectID, d.suggested) &&
		float64(ph.AssignmentConfidence) == float64(float32(d.confidence))
}

func uuidEq(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
