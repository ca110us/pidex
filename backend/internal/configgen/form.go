package configgen

import (
	"slices"

	"pidex/internal/teamconfig"
)

// form is what the model fills in. It mirrors teamconfig.Config with every
// field required and no optional objects, which strict structured output
// needs: empty strings and a disabled barcode stand for "not set".
type form struct {
	Summary  string     `json:"summary"`
	TestTips []string   `json:"test_tips"`
	Config   formConfig `json:"config"`
}

type formConfig struct {
	Language     string                  `json:"language"`
	Name         string                  `json:"name"`
	Industry     string                  `json:"industry"`
	Description  string                  `json:"description"`
	Strictness   string                  `json:"strictness"`
	Privacy      string                  `json:"privacy"`
	LivePhotos   bool                    `json:"live_photos"`
	DateOrder    string                  `json:"date_order"`
	SubjectTypes []formType              `json:"subject_types"`
	Tags         []formTag               `json:"tags"`
	Session      teamconfig.SessionRules `json:"session"`
}

type formType struct {
	Key            string      `json:"key"`
	Name           string      `json:"name"`
	Parent         string      `json:"parent"`
	IdentifyBy     string      `json:"identify_by"`
	Carriers       string      `json:"carriers"`
	IdentityFields []formField `json:"identity_fields"`
	DisplayFields  []string    `json:"display_fields"`
	MinFields      int         `json:"min_fields"`
	Barcode        formBarcode `json:"barcode"`
}

type formField struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Pattern     string `json:"pattern"`
	// Example is a value as printed on the carrier. It is not stored; it
	// lets checkExamples catch a pattern that no real value can match.
	Example string `json:"example"`
}

type formBarcode struct {
	Enabled bool   `json:"enabled"`
	Field   string `json:"field"`
	Pattern string `json:"pattern"`
	Strong  bool   `json:"strong"`
}

type formTag struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Group       string `json:"group"`
	Description string `json:"description"`
}

func (f formConfig) toConfig() *teamconfig.Config {
	c := &teamconfig.Config{
		FormatVersion: teamconfig.FormatVersion,
		Language:      f.Language,
		Name:          f.Name,
		Industry:      f.Industry,
		Description:   f.Description,
		Strictness:    teamconfig.Strictness(f.Strictness),
		Privacy:       teamconfig.Privacy(f.Privacy),
		LivePhotos:    f.LivePhotos,
		DateOrder:     f.DateOrder,
		Session:       f.Session,
	}
	for _, t := range f.SubjectTypes {
		st := teamconfig.SubjectType{
			Key: t.Key, Name: t.Name, Parent: t.Parent, IdentifyBy: teamconfig.IdentifyBy(t.IdentifyBy), Carriers: t.Carriers,
			DisplayFields: slices.Clone(t.DisplayFields),
			Match:         teamconfig.MatchRule{MinFields: t.MinFields},
		}
		for _, fl := range t.IdentityFields {
			st.IdentityFields = append(st.IdentityFields, teamconfig.Field{
				Key: fl.Key, Name: fl.Name, Kind: teamconfig.FieldKind(fl.Kind), Description: fl.Description, Pattern: fl.Pattern,
			})
		}
		if t.Barcode.Enabled {
			st.Barcode = &teamconfig.BarcodeRule{Field: t.Barcode.Field, Pattern: t.Barcode.Pattern, Strong: t.Barcode.Strong}
		}
		c.SubjectTypes = append(c.SubjectTypes, st)
	}
	for _, t := range f.Tags {
		c.Tags = append(c.Tags, teamconfig.Tag{Key: t.Key, Name: t.Name, Group: t.Group, Description: t.Description})
	}
	return c
}

// fromConfig is the form for an existing config. Barcode symbologies are
// not in the form; they keep their defaults.
func fromConfig(c *teamconfig.Config) formConfig {
	f := formConfig{
		Language: c.Language, Name: c.Name, Industry: c.Industry, Description: c.Description,
		Strictness: string(c.Strictness), Privacy: string(c.Privacy), LivePhotos: c.LivePhotos, DateOrder: c.DateOrder,
		Session: c.Session, SubjectTypes: []formType{}, Tags: []formTag{},
	}
	for _, st := range c.SubjectTypes {
		t := formType{
			Key: st.Key, Name: st.Name, Parent: st.Parent, IdentifyBy: string(st.IdentifyBy), Carriers: st.Carriers,
			DisplayFields: append([]string{}, st.DisplayFields...), MinFields: st.Match.MinFields,
		}
		for _, fl := range st.IdentityFields {
			t.IdentityFields = append(t.IdentityFields, formField{
				Key: fl.Key, Name: fl.Name, Kind: string(fl.Kind), Description: fl.Description, Pattern: fl.Pattern,
			})
		}
		if b := st.Barcode; b != nil {
			t.Barcode = formBarcode{Enabled: true, Field: b.Field, Pattern: b.Pattern, Strong: b.Strong}
		}
		f.SubjectTypes = append(f.SubjectTypes, t)
	}
	for _, t := range c.Tags {
		f.Tags = append(f.Tags, formTag{Key: t.Key, Name: t.Name, Group: t.Group, Description: t.Description})
	}
	return f
}

// schema is the JSON Schema of form, in the subset every provider's strict
// mode accepts: closed objects, all properties required.
func schema() map[string]any {
	str := map[string]any{"type": "string"}
	integer := map[string]any{"type": "integer"}
	boolean := map[string]any{"type": "boolean"}
	enum := func(vs ...string) map[string]any { return map[string]any{"type": "string", "enum": vs} }
	arr := func(items map[string]any) map[string]any { return map[string]any{"type": "array", "items": items} }
	obj := func(props map[string]any) map[string]any {
		keys := make([]string, 0, len(props))
		for k := range props {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		return map[string]any{"type": "object", "properties": props, "required": keys, "additionalProperties": false}
	}

	field := obj(map[string]any{
		"key": str, "name": str, "description": str, "pattern": str, "example": str,
		"kind": enum("person_name", "date", "id", "address", "text"),
	})
	subjectType := obj(map[string]any{
		"key": str, "name": str, "parent": str, "identify_by": enum("code", "appearance"), "carriers": str,
		"identity_fields": arr(field), "display_fields": arr(str), "min_fields": integer,
		"barcode": obj(map[string]any{"enabled": boolean, "field": str, "pattern": str, "strong": boolean}),
	})
	tag := obj(map[string]any{"key": str, "name": str, "group": str, "description": str})
	config := obj(map[string]any{
		"language": str, "name": str, "industry": str, "description": str,
		"strictness":    enum("strict", "standard", "lenient"),
		"privacy":       enum("high", "standard"),
		"live_photos":   boolean,
		"date_order":    enum("mdy", "dmy"),
		"subject_types": arr(subjectType),
		"tags":          arr(tag),
		"session":       obj(map[string]any{"gap_minutes": integer, "anchor_valid_minutes": integer, "max_jump_meters": integer}),
	})
	return obj(map[string]any{"summary": str, "test_tips": arr(str), "config": config})
}
