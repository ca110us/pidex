package search

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"pidex/internal/ai"
	"pidex/internal/teamconfig"
)

func renovation(t *testing.T) *teamconfig.Config {
	t.Helper()
	c, err := teamconfig.LoadTemplate("construction_renovation")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestParsePlanTrustsOnlyTheConfig(t *testing.T) {
	cfg := renovation(t)
	ny, _ := time.LoadLocation("America/New_York")
	f, err := parsePlan([]byte(`{"mode":"missing","subject_query":" unit 203 ","subject_type":"spaceship",
		"tags":["waterproofing","knee","waterproofing","tiling"],"date_from":"2026-10-12","date_to":"2026-10-12",
		"text":" leak "}`), cfg, ny)
	if err != nil {
		t.Fatal(err)
	}
	if f.Mode != ModeMissing || f.SubjectQuery != "unit 203" || f.SubjectType != "" || f.Text != "leak" {
		t.Errorf("filters %+v", f)
	}
	if strings.Join(f.Tags, ",") != "waterproofing,tiling" {
		t.Errorf("tags %v: unknown or repeated values must be dropped", f.Tags)
	}
	// One day given as both ends covers that whole day, in the user's zone.
	if f.From.Format(time.RFC3339) != "2026-10-12T00:00:00-04:00" || f.To.Format(time.RFC3339) != "2026-10-13T00:00:00-04:00" {
		t.Errorf("day range %v - %v", f.From, f.To)
	}
}

func TestPlanSchemaListsOnlyTeamVocabulary(t *testing.T) {
	cfg := renovation(t)
	s := planSchema(cfg)
	props := s["properties"].(map[string]any)
	tags := props["tags"].(map[string]any)["items"].(map[string]any)["enum"].([]string)
	if len(tags) != len(cfg.Tags) || tags[0] != cfg.Tags[0].Key {
		t.Errorf("tag enum %v", tags)
	}
	types := props["subject_type"].(map[string]any)["enum"].([]string)
	if strings.Join(types, ",") != ",project,unit" {
		t.Errorf("type enum %v", types)
	}
	ins := planInstructions(cfg, time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC))
	if !strings.Contains(ins, "Today is Thursday 2026-10-22") || !strings.Contains(ins, "waterproofing: Waterproofing") {
		t.Errorf("instructions lack the date or the tags:\n%s", ins)
	}
}

func TestDocumentLeavesOutIdentity(t *testing.T) {
	cfg := renovation(t)
	doc := Document(cfg, DocInput{Description: "Leak under the sink.", TagKeys: []string{"plumbing", "unknown"}, DocumentFields: map[string]string{"Invoice": "INV-7"}})
	if doc != "Leak under the sink.\nPlumbing (trade)\nInvoice: INV-7" {
		t.Errorf("document %q", doc)
	}
	cfg.Privacy = teamconfig.PrivacyHigh
	if doc := Document(cfg, DocInput{Description: "Wound.", DocumentFields: map[string]string{"MRN": "123"}}); strings.Contains(doc, "123") {
		t.Errorf("high privacy leaked document fields: %q", doc)
	}
}

type cannedModel string

func (c cannedModel) Structured(context.Context, ai.CallMeta, ai.StructuredRequest) (json.RawMessage, ai.Usage, error) {
	return json.RawMessage(c), ai.Usage{}, nil
}

// TestPlanRanksByTheWholeRequest: the request minus the subject's words is
// what is matched by meaning, not only the leftover text.
func TestPlanRanksByTheWholeRequest(t *testing.T) {
	m := cannedModel(`{"mode":"photos","subject_query":"unit 203","subject_type":"","tags":["hvac"],"date_from":"","date_to":"","text":"outside"}`)
	f, err := Plan(context.Background(), m, uuid.New(), renovation(t), "the HVAC unit outside unit 203", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if f.Request != "the HVAC unit outside" || f.Text != "outside" || f.SubjectQuery != "unit 203" {
		t.Errorf("filters %+v", f)
	}
}

// TestPlaceInDocument: where a photo was taken is searchable by its names,
// widest first; high-privacy teams keep no place names in what is indexed.
func TestPlaceInDocument(t *testing.T) {
	words := PlaceWords([]byte(`{"country":"China","region":"Liaoning","city":"Dalian","name":"Xinghai Square"}`))
	if words != "China · Liaoning · Dalian · Xinghai Square" {
		t.Errorf("place words: %q", words)
	}
	if PlaceWords(nil) != "" || PlaceWords([]byte(`{"city":""}`)) != "" {
		t.Error("no place, no words")
	}
	cfg := &teamconfig.Config{Privacy: teamconfig.PrivacyStandard}
	if doc := Document(cfg, DocInput{Description: "A square by the sea.", Place: words}); !strings.Contains(doc, "Xinghai Square") {
		t.Errorf("the place is not indexed: %q", doc)
	}
	cfg.Privacy = teamconfig.PrivacyHigh
	if doc := Document(cfg, DocInput{Description: "A wound.", Place: words}); strings.Contains(doc, "Dalian") {
		t.Errorf("a high-privacy team's place is indexed: %q", doc)
	}
}
