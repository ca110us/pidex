// Package analysis is the contract between a team's config and the vision
// model: it writes the instructions and schema from the config, and checks
// the answer against the same config. One model call per photo answers every
// question at once: role, identity, document fields, description and tags.
package analysis

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"pidex/internal/teamconfig"
)

type Role string

const (
	RoleIdentity Role = "identity"
	RoleDocument Role = "document"
	RoleContent  Role = "content"
	RoleJunk     Role = "junk"
)

var roles = []Role{RoleIdentity, RoleDocument, RoleContent, RoleJunk}

const noSubjectType = "none"

type IdentityValue struct {
	Field      string  `json:"field"`
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
}

// Appearance describes the one subject a photo is mainly of, for subject
// types told apart by how they look.
type Appearance struct {
	// Label is a short name from its look ("Orange tabby", "MongoDB mug").
	Label string `json:"label"`
	// Features are what would tell it from others of its kind.
	Features string `json:"features"`
	// NamedDescription is the description with this one written as
	// SubjectPlaceholder, for its name once it is known.
	NamedDescription string `json:"named_description,omitempty"`
}

// SubjectPlaceholder stands for the subject's name in a NamedDescription.
const SubjectPlaceholder = "{subject}"

// spaceByName is a space a model put between the placeholder and Chinese
// or Japanese text, which is written without one, as in "{subject} <Han text>".
var spaceByName = regexp.MustCompile(`(\{subject\})\s+([\p{Han}\p{Hiragana}\p{Katakana}])|([\p{Han}\p{Hiragana}\p{Katakana}])\s+(\{subject\})`)

// tidyNamed keeps a named description only if it names the subject.
func tidyNamed(s string) string {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, SubjectPlaceholder) {
		return ""
	}
	return spaceByName.ReplaceAllString(s, "$1$2$3$4")
}

type DocumentField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type TagScore struct {
	Key        string  `json:"key"`
	Confidence float64 `json:"confidence"`
}

// Result is the model's answer after validation against the config.
type Result struct {
	Role           Role            `json:"role"`
	RoleConfidence float64         `json:"role_confidence"`
	SubjectType    string          `json:"subject_type"`
	Identity       []IdentityValue `json:"identity"`
	// Appearance is set when SubjectType is told apart by appearance.
	Appearance     *Appearance     `json:"appearance,omitempty"`
	DocumentFields []DocumentField `json:"document_fields"`
	Description    string          `json:"description"`
	Tags           []TagScore      `json:"tags"`
}

// IdentityFields returns field -> value, keeping the most confident value
// when a field was reported twice, and the lowest per-field confidence.
func (r *Result) IdentityFields() (map[string]string, float64) {
	if len(r.Identity) == 0 {
		return nil, 0
	}
	best := map[string]IdentityValue{}
	for _, v := range r.Identity {
		if cur, ok := best[v.Field]; !ok || v.Confidence > cur.Confidence {
			best[v.Field] = v
		}
	}
	fields := map[string]string{}
	minConf := 1.0
	for k, v := range best {
		fields[k] = v.Value
		minConf = min(minConf, v.Confidence)
	}
	return fields, minConf
}

// Instructions are identical for every photo of a team and config version.
func Instructions(cfg *teamconfig.Config) string {
	var b strings.Builder
	b.WriteString(`You file work photos for a team. You see one photo at a time and fill in a JSON form about it.

Roles:
- identity: the photo's main purpose is to show who or what the work is about, on one of the identity carriers listed for a subject type below.
- document: paperwork whose content matters (report, form, invoice, screen showing text).
- content: the work itself, as this team does it.
- junk: blurred, black, accidental, or showing nothing useful, and anything unrelated to this team's work (personal photos, other places or events, artwork, advertisements, other trades).

Identity rules:
- Identity comes only from the carriers listed for each subject type. Text on anything else (tickets, packaging, artwork, posters, unrelated signs) is not identity.
- Report identity values only if you can actually read them in this photo, character by character. Never guess, infer, or complete a partly visible value, and never use an example or placeholder value.
- Copy values exactly as printed, including letters, digits and punctuation.
- Any role may carry identity, for example a content photo with a visible label.
- Confidence is your probability that the value is read correctly, from 0 to 1.

Tag rules:
- Use only the tag keys listed below. Include a tag only when the photo clearly shows it.
- Confidence is your probability that the tag applies, from 0 to 1.

`)
	fmt.Fprintf(&b, "Team: %s (%s).\n\n", cfg.Name, cfg.Industry)
	b.WriteString("Subject types (set subject_type to the type whose identity is visible, or \"none\"):\n")
	for _, st := range cfg.SubjectTypes {
		if st.ByAppearance() {
			fmt.Fprintf(&b, "- %s (%s). Told apart by appearance, not by any label: set subject_type to %s when one %s is the main subject of the photo and clearly visible. What tells them apart: %s\n",
				st.Key, st.Name, st.Key, st.Name, st.Carriers)
			continue
		}
		fmt.Fprintf(&b, "- %s (%s). Identity usually appears on: %s\n", st.Key, st.Name, st.Carriers)
		for _, f := range st.IdentityFields {
			fmt.Fprintf(&b, "    field %s: %s", fieldKey(st.Key, f.Key), f.Name)
			if f.Description != "" {
				fmt.Fprintf(&b, " (%s)", f.Description)
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\nTags:\n")
	for _, t := range cfg.Tags {
		fmt.Fprintf(&b, "- %s: %s", t.Key, t.Name)
		if t.Description != "" {
			fmt.Fprintf(&b, ". %s", t.Description)
		}
		b.WriteString("\n")
	}
	if hasAppearance(cfg) {
		b.WriteString(`
Appearance (only when subject_type is told apart by appearance; otherwise leave all empty):
- label: a short name for this one from its look, 2 to 4 words, such as its colour and kind.
- features: the visible features that would tell this one from others of its kind: colours, markings, patterns, prints or logos, wear or damage, accessories. Not its pose, the background or the light.
`)
		if cfg.Privacy == teamconfig.PrivacyHigh {
			b.WriteString("- named_description: leave empty.\n")
		} else {
			b.WriteString(`- named_description: the description below again, word for word, except that this one is written as {subject} wherever it is mentioned, without any article or words for its kind or look (its name goes there later), e.g. "{subject} crouches by the kitchen sink.". Empty if the description does not mention it.
`)
		}
		if cfg.Language != "" && cfg.Language != "en" {
			fmt.Fprintf(&b, "Write the label and features in the language with BCP 47 tag %s.\n", cfg.Language)
		}
	}
	b.WriteString("\nDescription: one short factual sentence about what the photo shows")
	if cfg.Language != "" && cfg.Language != "en" {
		fmt.Fprintf(&b, ", written in the language with BCP 47 tag %s", cfg.Language)
	}
	b.WriteString(".")
	if cfg.Privacy == teamconfig.PrivacyHigh {
		b.WriteString(" It must not contain names, dates of birth, record numbers, addresses or any other identifying detail; identity goes only in the identity list.")
	}
	b.WriteString("\n")
	return b.String()
}

const Prompt = "Fill in the form for this photo."

func hasAppearance(cfg *teamconfig.Config) bool {
	return slices.ContainsFunc(cfg.SubjectTypes, func(st teamconfig.SubjectType) bool { return st.ByAppearance() })
}

// fieldKey qualifies a field with its subject type so fields with the same
// key in different types stay distinct in the schema.
func fieldKey(typeKey, field string) string { return typeKey + "." + field }

// Schema is the JSON Schema the model must answer with.
func Schema(cfg *teamconfig.Config) map[string]any {
	typeKeys := []string{noSubjectType}
	var fieldKeys []string
	for _, st := range cfg.SubjectTypes {
		typeKeys = append(typeKeys, st.Key)
		for _, f := range st.IdentityFields {
			fieldKeys = append(fieldKeys, fieldKey(st.Key, f.Key))
		}
	}
	tagKeys := make([]string, 0, len(cfg.Tags))
	for _, t := range cfg.Tags {
		tagKeys = append(tagKeys, t.Key)
	}
	roleNames := make([]string, len(roles))
	for i, r := range roles {
		roleNames[i] = string(r)
	}
	// The schema sticks to the subset every provider's strict mode accepts:
	// closed objects, all properties required, no numeric bounds (Parse
	// clamps confidences instead).
	confidence := map[string]any{"type": "number", "description": "Probability from 0 to 1."}
	object := func(props map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
	}
	tagKey := map[string]any{"type": "string", "enum": tagKeys}
	if len(tagKeys) == 0 {
		// An empty enum is invalid; Parse drops any tag the config lacks.
		tagKey = map[string]any{"type": "string"}
	}
	props := map[string]any{
		"role":            map[string]any{"type": "string", "enum": roleNames},
		"role_confidence": confidence,
		"subject_type":    map[string]any{"type": "string", "enum": typeKeys},
		"identity": map[string]any{
			"type": "array",
			"items": object(map[string]any{
				"field":      map[string]any{"type": "string", "enum": fieldKeys},
				"value":      map[string]any{"type": "string"},
				"confidence": confidence,
			}, "field", "value", "confidence"),
		},
		"document_fields": map[string]any{
			"type": "array",
			"items": object(map[string]any{
				"name":  map[string]any{"type": "string"},
				"value": map[string]any{"type": "string"},
			}, "name", "value"),
		},
		"description": map[string]any{"type": "string"},
		"tags": map[string]any{
			"type": "array",
			"items": object(map[string]any{
				"key":        tagKey,
				"confidence": confidence,
			}, "key", "confidence"),
		},
	}
	required := []string{"role", "role_confidence", "subject_type", "identity", "document_fields", "description", "tags"}
	// Only teams with subjects told apart by appearance pay for these.
	if hasAppearance(cfg) {
		props["appearance"] = object(map[string]any{
			"label":             map[string]any{"type": "string"},
			"features":          map[string]any{"type": "string"},
			"named_description": map[string]any{"type": "string"},
		}, "label", "features", "named_description")
		required = append(required, "appearance")
	}
	return object(props, required...)
}

// Parse decodes a model answer and drops anything the config does not
// allow, so a model that ignores the schema cannot invent subject types,
// fields or tags.
func Parse(raw []byte, cfg *teamconfig.Config) (*Result, error) {
	var r Result
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("decode analysis: %w", err)
	}
	if !slices.Contains(roles, r.Role) {
		return nil, fmt.Errorf("analysis has unknown role %q", r.Role)
	}
	r.RoleConfidence = clamp(r.RoleConfidence)
	r.Description = strings.TrimSpace(r.Description)

	st, ok := cfg.SubjectType(r.SubjectType)
	if !ok {
		r.SubjectType = ""
	}
	var ident []IdentityValue
	for _, v := range r.Identity {
		typeKey, field, found := strings.Cut(v.Field, ".")
		if !ok || !found || typeKey != st.Key {
			continue
		}
		if _, known := st.Field(field); !known || strings.TrimSpace(v.Value) == "" {
			continue
		}
		ident = append(ident, IdentityValue{Field: field, Value: strings.TrimSpace(v.Value), Confidence: clamp(v.Confidence)})
	}
	r.Identity = ident
	switch {
	case ok && st.ByAppearance():
		// Seen, not read: what it looks like is all there is to match.
		r.Identity = nil
		if a := r.Appearance; a != nil && strings.TrimSpace(a.Features) != "" {
			r.Appearance = &Appearance{Label: strings.TrimSpace(a.Label), Features: strings.TrimSpace(a.Features), NamedDescription: tidyNamed(a.NamedDescription)}
		} else {
			r.SubjectType, r.Appearance = "", nil
		}
	case len(ident) == 0:
		r.SubjectType = ""
		r.Appearance = nil
	default:
		r.Appearance = nil
	}

	var tags []TagScore
	seen := map[string]bool{}
	for _, t := range r.Tags {
		if !cfg.HasTag(t.Key) || seen[t.Key] {
			continue
		}
		seen[t.Key] = true
		tags = append(tags, TagScore{Key: t.Key, Confidence: clamp(t.Confidence)})
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].Confidence > tags[j].Confidence })
	r.Tags = tags

	var docs []DocumentField
	for _, d := range r.DocumentFields {
		if strings.TrimSpace(d.Name) != "" && strings.TrimSpace(d.Value) != "" {
			docs = append(docs, DocumentField{Name: strings.TrimSpace(d.Name), Value: strings.TrimSpace(d.Value)})
		}
	}
	r.DocumentFields = docs
	return &r, nil
}

func clamp(f float64) float64 { return max(0, min(1, f)) }
