package configgen

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"pidex/internal/ai"
	"pidex/internal/teamconfig"
)

type fakeModel struct {
	answers []string
	prompts []string
}

func (m *fakeModel) Structured(_ context.Context, _ ai.CallMeta, req ai.StructuredRequest) (json.RawMessage, ai.Usage, error) {
	m.prompts = append(m.prompts, req.Prompt)
	if len(m.answers) == 0 {
		return nil, ai.Usage{}, errors.New("no more answers")
	}
	a := m.answers[0]
	m.answers = m.answers[1:]
	return json.RawMessage(a), ai.Usage{}, nil
}

func answer(t *testing.T, c *teamconfig.Config, mutate func(*formConfig)) string {
	t.Helper()
	f := form{Summary: "A config.", TestTips: []string{"Photograph a label."}, Config: fromConfig(c)}
	if mutate != nil {
		mutate(&f.Config)
	}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func template(t *testing.T, key string) *teamconfig.Config {
	t.Helper()
	c, err := teamconfig.LoadTemplate(key)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestFormRoundTrip guarantees that changing a config through the form
// loses nothing the user did not ask to change.
func TestFormRoundTrip(t *testing.T) {
	for _, key := range []string{"hospital_orthopedics", "construction_renovation"} {
		c := template(t, key)
		m := &fakeModel{answers: []string{answer(t, c, nil)}}
		d, err := Generate(context.Background(), m, Request{Description: "No change.", Base: c})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(d.Config, c) {
			got, _ := json.Marshal(d.Config)
			want, _ := json.Marshal(c)
			t.Errorf("%s changed in the round trip:\n got %s\nwant %s", key, got, want)
		}
	}
}

func TestRepairsInvalidAnswer(t *testing.T) {
	c := template(t, "construction_renovation")
	bad := answer(t, c, func(f *formConfig) { f.Tags = append(f.Tags, f.Tags[0]) })
	m := &fakeModel{answers: []string{bad, answer(t, c, nil)}}
	d, err := Generate(context.Background(), m, Request{Description: "We renovate apartments."})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.prompts) != 2 || !strings.Contains(m.prompts[1], `duplicate tag "plumbing"`) {
		t.Errorf("the validation error must go back to the model; prompts: %q", m.prompts)
	}
	if d.Summary != "A config." || len(d.TestTips) != 1 {
		t.Errorf("draft %+v", d)
	}
}

func TestGivesUpAfterRepairRounds(t *testing.T) {
	bad := `{"summary":"","test_tips":[],"config":{"name":"","subject_types":[],"tags":[]}}`
	m := &fakeModel{answers: []string{bad, bad, bad, bad}}
	_, err := Generate(context.Background(), m, Request{Description: "Plants."})
	if !errors.Is(err, ErrInvalid) || len(m.prompts) != repairRounds+1 {
		t.Fatalf("err=%v after %d calls", err, len(m.prompts))
	}
}

func TestRejectsEmptyOrHugeDescription(t *testing.T) {
	for _, d := range []string{"  ", strings.Repeat("x", MaxDescription+1)} {
		if _, err := Generate(context.Background(), &fakeModel{}, Request{Description: d}); err == nil {
			t.Errorf("description of %d bytes accepted", len(d))
		}
	}
}

func TestKeepsBarcodeSymbologies(t *testing.T) {
	c := template(t, "hospital_orthopedics")
	c.SubjectTypes[0].Barcode.Symbologies = []string{"code128"}
	m := &fakeModel{answers: []string{answer(t, c, nil)}}
	d, err := Generate(context.Background(), m, Request{Description: "Add a tag.", Base: c})
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Config.SubjectTypes[0].Barcode.Symbologies; !slices.Equal(got, []string{"code128"}) {
		t.Errorf("symbologies reset to %v", got)
	}
}

func TestCompare(t *testing.T) {
	old := template(t, "hospital_orthopedics")
	m := &fakeModel{answers: []string{answer(t, old, func(f *formConfig) {
		f.Tags = f.Tags[1:]
		f.SubjectTypes[0].IdentityFields = f.SubjectTypes[0].IdentityFields[:2]
		f.SubjectTypes[0].Barcode = formBarcode{}
		f.Strictness = "standard"
		f.Tags = append(f.Tags, formTag{Key: "new_tag", Name: "New"})
	})}}
	d, err := Generate(context.Background(), m, Request{Description: "Simplify.", Base: old})
	if err != nil {
		t.Fatal(err)
	}
	var msgs []string
	kinds := map[ChangeKind]bool{}
	for _, c := range Compare(old, d.Config) {
		msgs = append(msgs, c.Message)
		kinds[c.Kind] = true
	}
	got := strings.Join(msgs, "\n")
	for _, k := range []ChangeKind{TagRemoved, FieldRemoved, BarcodeRemoved, StrictnessChanged} {
		if !kinds[k] {
			t.Errorf("missing change kind %d", k)
		}
	}
	for _, want := range []string{`Tag "Left" is removed`, `Identity field "MRN" of Patient is removed`, `Barcodes no longer identify Patient by "MRN"`, "Strictness changes from strict to standard"} {
		if !strings.Contains(got, want) {
			t.Errorf("warnings miss %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"New"`) {
		t.Errorf("an added tag must not warn:\n%s", got)
	}
	if w := Compare(old, old); len(w) != 0 {
		t.Errorf("no change must give no warnings, got %v", w)
	}
}

// TestSchemaMatchesForm keeps the schema and the Go form in step: every
// object in a filled form has exactly the properties the schema requires.
func TestSchemaMatchesForm(t *testing.T) {
	b, err := json.Marshal(form{TestTips: []string{}, Config: fromConfig(template(t, "construction_renovation"))})
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	var walk func(path string, s map[string]any, v any)
	walk = func(path string, s map[string]any, v any) {
		switch s["type"] {
		case "object":
			obj, ok := v.(map[string]any)
			if !ok {
				t.Fatalf("%s: want object, got %T", path, v)
			}
			req := s["required"].([]string)
			var keys []string
			for k := range obj {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			if !slices.Equal(keys, req) {
				t.Errorf("%s: form has %v, schema requires %v", path, keys, req)
			}
			props := s["properties"].(map[string]any)
			for k, sub := range props {
				walk(path+"."+k, sub.(map[string]any), obj[k])
			}
		case "array":
			for _, item := range v.([]any) {
				walk(path+"[]", s["items"].(map[string]any), item)
			}
		}
	}
	walk("$", schema(), doc)
}

func assetForm(fieldPattern, barcodePattern, example string) string {
	f := form{Summary: "Devices.", TestTips: []string{}, Config: formConfig{
		Name: "IT assets", Industry: "it", Strictness: "standard", Privacy: "standard", DateOrder: "dmy",
		Session: teamconfig.SessionRules{GapMinutes: 15},
		SubjectTypes: []formType{{
			Key: "device", Name: "Device", Carriers: "Asset label with a barcode.",
			IdentityFields: []formField{{Key: "asset_tag", Name: "Asset tag", Kind: "id", Pattern: fieldPattern, Example: example}},
			DisplayFields:  []string{"asset_tag"}, MinFields: 1,
			Barcode: formBarcode{Enabled: true, Field: "asset_tag", Pattern: barcodePattern, Strong: true},
		}},
		Tags: []formTag{{Key: "screen_crack", Name: "Screen crack"}},
	}}
	b, _ := json.Marshal(f)
	return string(b)
}

// TestExamplesCatchPatternsThatNeverMatch: ids are normalized to letters and
// digits before the pattern runs, so a pattern with the printed hyphen
// would drop every asset tag without any error.
func TestExamplesCatchPatternsThatNeverMatch(t *testing.T) {
	m := &fakeModel{answers: []string{
		assetForm(`^IT-[0-9]+$`, "", "IT-004512"),
		assetForm(`^IT[0-9]{6}$`, "", "IT-004512"),
	}}
	d, err := Generate(context.Background(), m, Request{Description: "IT asset inventory."})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.prompts[1], `example "IT-004512" of field "asset_tag" normalizes to "IT004512", which the pattern "^IT-[0-9]+$" rejects`) {
		t.Errorf("repair prompt does not explain the pattern problem:\n%s", m.prompts[1])
	}
	if d.Config.SubjectTypes[0].IdentityFields[0].Pattern != `^IT[0-9]{6}$` {
		t.Errorf("kept the broken pattern")
	}

	bad := assetForm("", `^ASSET:(IT-[0-9]+)$`, "IT-004512")
	m = &fakeModel{answers: []string{bad, bad, bad}}
	_, err = Generate(context.Background(), m, Request{Description: "IT asset inventory."})
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "does not match the barcode pattern") {
		t.Errorf("a barcode pattern the example does not fit must be rejected, got %v", err)
	}
}

// TestLanguageRule: a new config follows the description's language; a
// change keeps the config's language whatever language the request is in.
func TestLanguageRule(t *testing.T) {
	c := template(t, "construction_renovation")
	m := &fakeModel{answers: []string{answer(t, c, nil), answer(t, c, nil)}}
	if _, err := Generate(context.Background(), m, Request{Description: "我们做装修。"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(context.Background(), m, Request{Description: "Add a tag.", Base: c}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.prompts[0], "in the language of the description above") {
		t.Errorf("new config prompt: %q", m.prompts[0])
	}
	if !strings.Contains(m.prompts[1], "Keep the language of the current config") {
		t.Errorf("change prompt: %q", m.prompts[1])
	}
}

// TestFollowsChosenIdentification sends a draft that ignores the user's
// choice back to the model, and keeps the corrected one.
func TestFollowsChosenIdentification(t *testing.T) {
	pets := template(t, "household_pets")
	coded := answer(t, template(t, "construction_renovation"), nil)
	m := &fakeModel{answers: []string{coded, answer(t, pets, nil)}}
	d, err := Generate(context.Background(), m, Request{Description: "My mugs.", IdentifyBy: teamconfig.IdentifyByAppearance})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Config.SubjectTypes[0].ByAppearance() || len(m.prompts) != 2 {
		t.Fatalf("draft %+v after %d calls", d.Config.SubjectTypes[0], len(m.prompts))
	}
	if !strings.Contains(m.prompts[0], "by how they look") || !strings.Contains(m.prompts[1], "but the user chose appearance") {
		t.Errorf("prompts did not carry the choice:\n%s\n---\n%s", m.prompts[0], m.prompts[1])
	}
	if _, err := Generate(context.Background(), m, Request{Description: "x", IdentifyBy: "face"}); err == nil {
		t.Error("an unknown choice is rejected")
	}
}
