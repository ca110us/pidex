package configgen

import (
	"fmt"

	"pidex/internal/teamconfig"
)

type ChangeKind int

// The values match pidex.v1.ConfigChangeKind.
const (
	SubjectTypeRemoved ChangeKind = iota + 1
	FieldRemoved
	FieldKindChanged
	FieldPatternChanged
	MinFieldsRaised
	BarcodeRemoved
	ParentChanged
	TagRemoved
	StrictnessChanged
	PrivacyLowered
	IdentifyByChanged
)

// Change is one difference between two configs that affects subjects and
// photos filed under the old one. Names are the config's display names, so
// an app can word the change in its user's language.
type Change struct {
	Kind        ChangeKind
	SubjectType string
	Field       string
	Tag         string
	From, To    string
	// Message is the English wording.
	Message string
}

// Compare lists the changes from old to new that affect subjects and photos
// filed under old: anything removed, and identity rules that changed
// meaning. Additions and renamed labels are harmless and not listed.
func Compare(old, new *teamconfig.Config) []Change {
	var changes []Change
	add := func(c Change, format string, args ...any) {
		c.Message = fmt.Sprintf(format, args...)
		changes = append(changes, c)
	}

	for _, ot := range old.SubjectTypes {
		nt, ok := new.SubjectType(ot.Key)
		if !ok {
			add(Change{Kind: SubjectTypeRemoved, SubjectType: ot.Name},
				"Subject type %q (%s) is removed. Existing %s subjects keep their photos, but no new photos will be filed under them.", ot.Name, ot.Key, ot.Name)
			continue
		}
		for _, of := range ot.IdentityFields {
			nf, ok := nt.Field(of.Key)
			switch {
			case !ok:
				add(Change{Kind: FieldRemoved, SubjectType: ot.Name, Field: of.Name},
					"Identity field %q of %s is removed. Existing subjects keep the value, but it is no longer read or matched.", of.Name, ot.Name)
			case nf.Kind != of.Kind:
				add(Change{Kind: FieldKindChanged, SubjectType: ot.Name, Field: of.Name, From: string(of.Kind), To: string(nf.Kind)},
					"Identity field %q of %s changes from %s to %s, which changes how values are matched.", of.Name, ot.Name, of.Kind, nf.Kind)
			case nf.Pattern != of.Pattern && nf.Pattern != "":
				add(Change{Kind: FieldPatternChanged, SubjectType: ot.Name, Field: of.Name},
					"Identity field %q of %s gets a new format rule; values that do not fit it will be ignored.", of.Name, ot.Name)
			}
		}
		if nt.Match.MinFields > ot.Match.MinFields {
			add(Change{Kind: MinFieldsRaised, SubjectType: ot.Name, From: fmt.Sprint(ot.Match.MinFields), To: fmt.Sprint(nt.Match.MinFields)},
				"%s now needs %d matching identity fields instead of %d, so more photos will ask for confirmation.", ot.Name, nt.Match.MinFields, ot.Match.MinFields)
		}
		if ot.Barcode != nil && (nt.Barcode == nil || nt.Barcode.Field != ot.Barcode.Field) {
			field := ot.Barcode.Field
			if f, ok := ot.Field(field); ok {
				field = f.Name
			}
			add(Change{Kind: BarcodeRemoved, SubjectType: ot.Name, Field: field},
				"Barcodes no longer identify %s by %q.", ot.Name, field)
		}
		if nt.IdentifyBy != ot.IdentifyBy {
			add(Change{Kind: IdentifyByChanged, SubjectType: ot.Name, From: string(ot.IdentifyBy), To: string(nt.IdentifyBy)},
				"%s will be recognized by %s instead of %s. Photos already filed stay where they are.", ot.Name, nt.IdentifyBy, ot.IdentifyBy)
		}
		if nt.Parent != ot.Parent {
			add(Change{Kind: ParentChanged, SubjectType: ot.Name, From: typeName(old, ot.Parent), To: typeName(new, nt.Parent)},
				"%s moves from parent %q to %q; existing subjects keep their old parent.", ot.Name, ot.Parent, nt.Parent)
		}
	}
	for _, t := range old.Tags {
		if !new.HasTag(t.Key) {
			add(Change{Kind: TagRemoved, Tag: t.Name},
				"Tag %q is removed. Photos that have it keep it, but it is no longer suggested.", t.Name)
		}
	}
	if old.Strictness != new.Strictness {
		add(Change{Kind: StrictnessChanged, From: string(old.Strictness), To: string(new.Strictness)},
			"Strictness changes from %s to %s.", old.Strictness, new.Strictness)
	}
	if old.Privacy == teamconfig.PrivacyHigh && new.Privacy != teamconfig.PrivacyHigh {
		add(Change{Kind: PrivacyLowered}, "Privacy is lowered from high: descriptions may now contain identifying details.")
	}
	return changes
}

func typeName(c *teamconfig.Config, key string) string {
	if st, ok := c.SubjectType(key); ok {
		return st.Name
	}
	return key
}

// keepUnformed copies settings the form does not carry from base, so that
// changing a config through the form never resets them.
func keepUnformed(base, cfg *teamconfig.Config) {
	for i := range cfg.SubjectTypes {
		nt := &cfg.SubjectTypes[i]
		ot, ok := base.SubjectType(nt.Key)
		if ok && nt.Barcode != nil && ot.Barcode != nil && nt.Barcode.Field == ot.Barcode.Field {
			nt.Barcode.Symbologies = append([]string(nil), ot.Barcode.Symbologies...)
		}
	}
}
