package teamconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTemplatesAreValid(t *testing.T) {
	tpls, err := Templates()
	if err != nil {
		t.Fatal(err)
	}
	if len(tpls) < 2 {
		t.Fatalf("want at least 2 templates, got %d", len(tpls))
	}
	for _, tpl := range tpls {
		if err := tpl.Config.Validate(); err != nil {
			t.Errorf("%s: %v", tpl.Key, err)
		}
	}
}

func TestHospitalTemplateIsStrictAndPrivate(t *testing.T) {
	cfg, err := LoadTemplate("hospital_orthopedics")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Strictness != StrictnessStrict || cfg.Privacy != PrivacyHigh {
		t.Fatalf("strictness=%s privacy=%s", cfg.Strictness, cfg.Privacy)
	}
	st, _ := cfg.SubjectType("patient")
	if st.Match.MinFields != 2 {
		t.Fatalf("patients need two identifiers, got %d", st.Match.MinFields)
	}
	if st.Barcode.Accepts("ean13") || st.Barcode.Accepts("upca") {
		t.Fatal("retail barcodes must not identify patients")
	}
	if !st.Barcode.Accepts("VNBarcodeSymbologyCode128") {
		t.Fatal("code128 wristbands must be accepted")
	}
}

func TestValidateReportsAllProblems(t *testing.T) {
	raw := `{
	  "name": "x", "strictness": "extreme", "language": "Chinese",
	  "subject_types": [
	    {"key": "Bad Key", "name": "", "carriers": "", "identity_fields": [], "match": {"min_fields": 3}},
	    {"key": "child", "name": "Child", "parent": "missing", "carriers": "c",
	     "identity_fields": [{"key": "id", "name": "ID", "kind": "id", "pattern": "("}],
	     "display_fields": ["nope"]}
	  ],
	  "tags": [{"key": "a", "name": "A"}, {"key": "a", "name": "A again"}]
	}`
	_, err := Parse([]byte(raw))
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{
		"strictness", "lowercase", "carriers", "identity fields", "unknown parent",
		"pattern", "display field", "duplicate tag", "BCP 47",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q:\n%v", want, err)
		}
	}
}

func TestParseRejectsUnknownFields(t *testing.T) {
	_, err := Parse([]byte(`{"name":"x","surprise":true}`))
	if err == nil || !strings.Contains(err.Error(), "surprise") {
		t.Fatalf("want unknown field error, got %v", err)
	}
}

func TestParentCycle(t *testing.T) {
	cfg, _ := LoadTemplate("construction_renovation")
	cfg.SubjectTypes[0].Parent = "unit"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("want cycle error, got %v", err)
	}
}

func TestDisplayNameFallsBack(t *testing.T) {
	cfg, _ := LoadTemplate("hospital_orthopedics")
	st, _ := cfg.SubjectType("patient")
	if got := st.DisplayName(map[string]string{"name": "John Smith", "dob": "03/02/1985"}); got != "John Smith · 03/02/1985" {
		t.Errorf("got %q", got)
	}
	if got := st.DisplayName(map[string]string{"mrn": "12345678"}); got != "12345678" {
		t.Errorf("got %q", got)
	}
}

// TestNoSubjectTypesIsAList: a collection kept by place and time has no
// subject types, written as an empty list (clients read a list), and is
// valid.
func TestNoSubjectTypesIsAList(t *testing.T) {
	cfg, err := LoadTemplate("trips")
	if err != nil {
		t.Fatal(err)
	}
	cfg.SubjectTypes = nil
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"subject_types":[]`) {
		t.Errorf("no subject types written as %s", raw)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("a collection without subject types is refused: %v", err)
	}
}
