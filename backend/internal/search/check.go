package search

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"pidex/internal/ai"
	"pidex/internal/teamconfig"
)

// Found is a photo a search found, as words: what checking it reads.
type Found struct {
	// Description is as the photo was described, its look included ("a
	// golden shaded cat"), not with a name in its place.
	Description string
	// Subject is the name of the one it is filed under, if any, and Looks
	// what that one looks like in full (its profile): a black nose may be
	// in no photo's description.
	Subject string
	Looks   string
	Tags    []string
}

// Checks reports whether a request asked for anything a photo's words can
// confirm: what it shows, where, doing what. Only a subject, a time or the
// whole collection is not checked; those filters are exact.
func Checks(f Filters) bool {
	return f.Mode == ModePhotos && (len(f.Tags) > 0 || strings.TrimSpace(f.Text) != "")
}

const checkInstructions = `You check photo search results against what a person asked for. Each photo is given by its description and tags.

Rules:
- The subject the person named and any time were already matched exactly; judge only what the photo shows: things, places, activities, states, and how it looks (colour, markings, size).
- A photo's subject, when it has one, is named in brackets before its description, and what it looks like may follow. Features of how it looks (a black nose, yellow eyes) hold for every photo of it, even one that does not show them.
- A photo matches when its description or tags say it shows what was asked, or say something that clearly implies it.
- A photo does not match when they say it shows something else instead (lying on the floor when a sofa was asked for), or say nothing about what was asked.`

// Check keeps the photos whose words show what was asked: matching by
// meaning ranks the closest photos first even when none is close, and a
// cat on the floor is no answer to "on the sofa". It returns, for each
// photo, whether it matches.
func Check(ctx context.Context, m Model, teamID uuid.UUID, cfg *teamconfig.Config, request string, f Filters, found []Found) ([]bool, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Asked for: %s\n", request)
	var tags []string
	for _, k := range f.Tags {
		for _, t := range cfg.Tags {
			if t.Key == k {
				tags = append(tags, t.Name)
			}
		}
	}
	if len(tags) > 0 || f.Text != "" {
		fmt.Fprintf(&b, "Understood as: %s\n", strings.Trim(strings.Join(append(tags, f.Text), "; "), "; "))
	}
	b.WriteString("\nPhotos:\n")
	for i, p := range found {
		fmt.Fprintf(&b, "%d. ", i+1)
		if p.Subject != "" {
			fmt.Fprintf(&b, "[%s] ", p.Subject)
		}
		b.WriteString(p.Description)
		if p.Looks != "" {
			fmt.Fprintf(&b, " %s looks like this: %s.", p.Subject, strings.ReplaceAll(p.Looks, "\n", "; "))
		}
		if len(p.Tags) > 0 {
			fmt.Fprintf(&b, " Tags: %s.", strings.Join(p.Tags, ", "))
		}
		b.WriteString("\n")
	}
	b.WriteString("\nWhich photos show what was asked for? Give their numbers.")
	raw, _, err := m.Structured(ctx, ai.CallMeta{TeamID: teamID, Purpose: "search_check"}, ai.StructuredRequest{
		Instructions: checkInstructions,
		Prompt:       b.String(),
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"matching": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
			},
			"required":             []string{"matching"},
			"additionalProperties": false,
		},
		MaxOutputTokens: 400,
	})
	if err != nil {
		return nil, err
	}
	var ans struct {
		Matching []int `json:"matching"`
	}
	if err := json.Unmarshal(raw, &ans); err != nil {
		return nil, fmt.Errorf("search check: %w", err)
	}
	keep := make([]bool, len(found))
	for _, n := range ans.Matching {
		if n >= 1 && n <= len(found) {
			keep[n-1] = true
		}
	}
	return keep, nil
}
