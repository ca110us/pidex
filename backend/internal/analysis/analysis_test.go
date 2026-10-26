package analysis

import (
	"encoding/json"
	"strings"
	"testing"

	"pidex/internal/teamconfig"
)

func hospital(t *testing.T) *teamconfig.Config {
	t.Helper()
	cfg, err := teamconfig.LoadTemplate("hospital_orthopedics")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestInstructionsComeFromConfig(t *testing.T) {
	cfg := hospital(t)
	ins := Instructions(cfg)
	for _, want := range []string{"patient.mrn", "wristband", "surgical_wound", "must not contain names"} {
		if !strings.Contains(ins, want) {
			t.Errorf("instructions lack %q", want)
		}
	}
	construction, _ := teamconfig.LoadTemplate("construction_renovation")
	ins = Instructions(construction)
	if strings.Contains(ins, "patient") || !strings.Contains(ins, "unit.unit_number") {
		t.Error("instructions must follow the team's vocabulary, not a hardcoded one")
	}
}

func TestDescriptionsFollowTheTeamLanguage(t *testing.T) {
	cfg := hospital(t)
	if strings.Contains(Instructions(cfg), "BCP 47") {
		t.Error("an English team needs no language instruction")
	}
	cfg.Language = "zh-Hans"
	if !strings.Contains(Instructions(cfg), "written in the language with BCP 47 tag zh-Hans") {
		t.Error("a Chinese team's descriptions must be asked for in Chinese")
	}
}

// TestNamedDescription: a pet's photo is also described with the pet as a
// placeholder, for its name once it is filed; teams that keep names out of
// descriptions are not asked.
func TestNamedDescription(t *testing.T) {
	pets, err := teamconfig.LoadTemplate("household_pets")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(Instructions(pets), "- named_description: the description below again") {
		t.Error("pets are not described by name")
	}
	raw, _ := json.Marshal(Schema(pets))
	if !strings.Contains(string(raw), `"named_description"`) {
		t.Error("the schema lacks named_description")
	}
	pets.Privacy = teamconfig.PrivacyHigh
	if ins := Instructions(pets); !strings.Contains(ins, "- named_description: leave empty.") {
		t.Error("a high-privacy team is asked to name subjects in descriptions")
	}
	if strings.Contains(Instructions(hospital(t)), "named_description") {
		t.Error("teams without subjects told apart by look are asked for it")
	}
	for in, want := range map[string]string{
		"{subject} 站在笔记本电脑旁。":             "{subject}站在笔记本电脑旁。",
		"笔记本电脑旁是 {subject}。":              "笔记本电脑旁是{subject}。",
		"{subject} crouches by the sink.": "{subject} crouches by the sink.",
		"A cat crouches by the sink.":     "",
	} {
		if got := tidyNamed(in); got != want {
			t.Errorf("tidyNamed(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSchemaIsValidJSONWithEnums(t *testing.T) {
	cfg := hospital(t)
	raw, err := json.Marshal(Schema(cfg))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"patient.dob"`, `"knee"`, `"none"`, `"junk"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("schema lacks %s", want)
		}
	}
}

func TestParseDropsWhatTheConfigDoesNotAllow(t *testing.T) {
	cfg := hospital(t)
	raw := `{
	  "role": "identity", "role_confidence": 1.4, "subject_type": "patient",
	  "identity": [
	    {"field": "patient.name", "value": " John Smith ", "confidence": 0.9},
	    {"field": "patient.name", "value": "Jon Smith", "confidence": 0.5},
	    {"field": "patient.ssn", "value": "123-45-6789", "confidence": 0.9},
	    {"field": "vehicle.plate", "value": "ABC123", "confidence": 0.9}
	  ],
	  "document_fields": [{"name": "", "value": "x"}],
	  "description": "A wristband.",
	  "tags": [{"key": "knee", "confidence": 0.4}, {"key": "invented", "confidence": 0.9}, {"key": "knee", "confidence": 0.9}]
	}`
	res, err := Parse([]byte(raw), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res.RoleConfidence != 1 {
		t.Errorf("confidence not clamped: %v", res.RoleConfidence)
	}
	fields, conf := res.IdentityFields()
	if len(fields) != 1 || fields["name"] != "John Smith" || conf != 0.9 {
		t.Errorf("identity %v conf %v", fields, conf)
	}
	if len(res.Tags) != 1 || res.Tags[0].Key != "knee" {
		t.Errorf("tags %v", res.Tags)
	}
	if len(res.DocumentFields) != 0 {
		t.Errorf("empty document field kept")
	}
}

func TestParseRejectsUnknownRole(t *testing.T) {
	if _, err := Parse([]byte(`{"role":"selfie"}`), hospital(t)); err == nil {
		t.Fatal("want error")
	}
}

// TestAlbumRoles: in an album kept by place and time (no subject types),
// whatever was photographed belongs; only shots showing nothing are junk.
func TestAlbumRoles(t *testing.T) {
	trips, err := teamconfig.LoadTemplate("trips")
	if err != nil {
		t.Fatal(err)
	}
	ins := Instructions(trips)
	if strings.Contains(ins, "unrelated to this team's work") || !strings.Contains(ins, "Never junk for being ordinary") {
		t.Errorf("an album's photos are judged as work:\n%s", ins)
	}
	if !strings.Contains(Instructions(hospital(t)), "unrelated to this team's work") {
		t.Error("work photos lost their rule for what is unrelated")
	}
}
