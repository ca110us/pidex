// Package teamconfig defines a team's vocabulary: what photos are filed
// under, how those subjects are recognized, and which tags exist. The rest of
// the system reads this instead of hardcoding any industry.
package teamconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// FormatVersion is the version of the config format itself, not of a team's
// config. Bump it when the shape changes and add an upgrade step in Parse.
const FormatVersion = 1

const (
	maxSubjectTypes  = 6
	maxIdentityField = 5
	maxTags          = 150
)

type Strictness string

const (
	StrictnessStrict   Strictness = "strict"
	StrictnessStandard Strictness = "standard"
	StrictnessLenient  Strictness = "lenient"
)

type Privacy string

const (
	// PrivacyHigh keeps identifying details out of descriptions and logs.
	PrivacyHigh     Privacy = "high"
	PrivacyStandard Privacy = "standard"
)

type FieldKind string

const (
	KindText       FieldKind = "text"
	KindPersonName FieldKind = "person_name"
	KindDate       FieldKind = "date"
	// KindID is an identifier that must match exactly after normalization.
	KindID      FieldKind = "id"
	KindAddress FieldKind = "address"
)

var (
	keyPattern      = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
	languagePattern = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)
)

type Config struct {
	FormatVersion int    `json:"format_version"`
	Name          string `json:"name"`
	Industry      string `json:"industry"`
	Description   string `json:"description,omitempty"`
	// Language is the BCP 47 tag of the language names, tags and photo
	// descriptions are written in ("en", "zh-Hans"). Empty means English.
	Language   string     `json:"language,omitempty"`
	Strictness Strictness `json:"strictness"`
	Privacy    Privacy    `json:"privacy"`
	// LivePhotos keeps the seconds of motion around each shot taken in the
	// app (no sound), beside the photo. Off unless the team asks for it.
	LivePhotos bool `json:"live_photos,omitempty"`
	// DateOrder resolves ambiguous dates like 03/04/1985: "mdy" or "dmy".
	DateOrder    string        `json:"date_order"`
	SubjectTypes []SubjectType `json:"subject_types"`
	Tags         []Tag         `json:"tags"`
	Session      SessionRules  `json:"session"`
}

// MarshalJSON writes no subject types (a collection kept by place and
// time) as an empty list, not null: clients read a list.
func (c Config) MarshalJSON() ([]byte, error) {
	type plain Config
	p := plain(c)
	if p.SubjectTypes == nil {
		p.SubjectTypes = []SubjectType{}
	}
	if p.Tags == nil {
		p.Tags = []Tag{}
	}
	return json.Marshal(p)
}

// IdentifyBy is how one subject is told from another.
type IdentifyBy string

const (
	// IdentifyByCode reads an identifier off a label, wristband, serial
	// plate or barcode: exact, and the default.
	IdentifyByCode IdentifyBy = "code"
	// IdentifyByAppearance compares how subjects look, for things nobody
	// labels: pets, mugs, plants. A photo is matched against photos already
	// filed, and a person confirms when the match is unsure.
	IdentifyByAppearance IdentifyBy = "appearance"
)

type SubjectType struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Parent string `json:"parent,omitempty"`
	// IdentifyBy is code or appearance; empty means code.
	IdentifyBy IdentifyBy `json:"identify_by,omitempty"`
	// Carriers says in plain language where identity appears in photos, or,
	// for appearance, which features tell one subject from another. It is
	// given to the vision model verbatim.
	Carriers       string       `json:"carriers"`
	IdentityFields []Field      `json:"identity_fields"`
	DisplayFields  []string     `json:"display_fields"`
	Match          MatchRule    `json:"match"`
	Barcode        *BarcodeRule `json:"barcode,omitempty"`
}

type Field struct {
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Kind        FieldKind `json:"kind"`
	Description string    `json:"description,omitempty"`
	// Pattern, when set, is a regexp a normalized value must match.
	Pattern string `json:"pattern,omitempty"`
}

type MatchRule struct {
	// MinFields is how many identity fields must agree before a photo can be
	// filed under an existing subject or a new subject can be created.
	MinFields int `json:"min_fields"`
}

type BarcodeRule struct {
	// Field receives the barcode payload.
	Field string `json:"field"`
	// Pattern must match the payload; capture group 1 is used when present.
	Pattern string `json:"pattern,omitempty"`
	// Strong means a valid barcode alone satisfies the match rule, as with a
	// hospital wristband scan.
	Strong bool `json:"strong"`
	// Symbologies the identity barcode may use, lowercase ("code128",
	// "qr"). Defaults exclude retail EAN/UPC codes, which appear on product
	// packaging in photos and must never be read as identity.
	Symbologies []string `json:"symbologies,omitempty"`
}

// DefaultIdentitySymbologies are the codes used on labels, wristbands and
// work orders, as opposed to products.
var DefaultIdentitySymbologies = []string{"code128", "code39", "code93", "pdf417", "datamatrix", "qr", "aztec", "itf"}

// Accepts reports whether a barcode of this symbology may carry identity.
func (b *BarcodeRule) Accepts(symbology string) bool {
	return slices.Contains(b.Symbologies, NormalizeSymbology(symbology))
}

// Extract applies the pattern to a raw barcode payload and returns the
// identity value: capture group 1 when present, else the whole match.
func (b *BarcodeRule) Extract(payload string) (string, bool) {
	payload = strings.TrimSpace(payload)
	if b.Pattern == "" {
		return payload, payload != ""
	}
	re, err := regexp.Compile(b.Pattern)
	if err != nil {
		return "", false
	}
	m := re.FindStringSubmatch(strings.ToUpper(payload))
	switch {
	case m == nil:
		return "", false
	case len(m) > 1 && m[1] != "":
		return m[1], true
	default:
		return m[0], true
	}
}

// NormalizeSymbology maps names like "VNBarcodeSymbologyCode128",
// "CODE_128" or "QR" to "code128" and "qr".
func NormalizeSymbology(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimPrefix(s, "vnbarcodesymbology")
	s = strings.TrimPrefix(s, "org.iso.")
	s = strings.TrimPrefix(s, "org.gs1.")
	return strings.NewReplacer("_", "", "-", "", " ", "", ".", "").Replace(s)
}

type Tag struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Group       string `json:"group,omitempty"`
	Description string `json:"description,omitempty"`
}

type SessionRules struct {
	// GapMinutes of no photos ends a run; later photos need a new anchor.
	GapMinutes int `json:"gap_minutes"`
	// AnchorValidMinutes bounds how long an identity photo keeps filing
	// later photos. Zero means no limit beyond GapMinutes.
	AnchorValidMinutes int `json:"anchor_valid_minutes"`
	// MaxJumpMeters between consecutive photos ends a run.
	MaxJumpMeters int `json:"max_jump_meters"`
}

// Thresholds turn confidences into actions.
type Thresholds struct {
	// AutoAssign at or above: file without asking.
	AutoAssign float64
	// Suggest at or above: propose and wait for one tap.
	Suggest float64
	// TagKeep at or above: store an AI tag.
	TagKeep float64
}

func (c *Config) Thresholds() Thresholds {
	switch c.Strictness {
	case StrictnessStrict:
		return Thresholds{AutoAssign: 0.97, Suggest: 0.6, TagKeep: 0.6}
	case StrictnessLenient:
		return Thresholds{AutoAssign: 0.75, Suggest: 0.4, TagKeep: 0.5}
	default:
		return Thresholds{AutoAssign: 0.88, Suggest: 0.5, TagKeep: 0.55}
	}
}

func (c *Config) SubjectType(key string) (*SubjectType, bool) {
	for i := range c.SubjectTypes {
		if c.SubjectTypes[i].Key == key {
			return &c.SubjectTypes[i], true
		}
	}
	return nil, false
}

func (c *Config) HasTag(key string) bool {
	return slices.ContainsFunc(c.Tags, func(t Tag) bool { return t.Key == key })
}

// ByAppearance reports whether subjects of this type are told apart by how
// they look rather than by a code.
func (st *SubjectType) ByAppearance() bool { return st.IdentifyBy == IdentifyByAppearance }

func (st *SubjectType) Field(key string) (*Field, bool) {
	for i := range st.IdentityFields {
		if st.IdentityFields[i].Key == key {
			return &st.IdentityFields[i], true
		}
	}
	return nil, false
}

// DisplayName joins display fields in order, falling back to any identity
// value so a subject is never nameless.
func (st *SubjectType) DisplayName(identity map[string]string) string {
	var parts []string
	for _, k := range st.DisplayFields {
		if v := strings.TrimSpace(identity[k]); v != "" {
			parts = append(parts, v)
		}
	}
	if len(parts) == 0 {
		for _, f := range st.IdentityFields {
			if v := strings.TrimSpace(identity[f.Key]); v != "" {
				parts = append(parts, v)
			}
		}
	}
	if len(parts) == 0 {
		return st.Name
	}
	return strings.Join(parts, " · ")
}

// Parse decodes and validates a config, filling defaults.
func Parse(data []byte) (*Config, error) {
	var c Config
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("decode team config: %w", err)
	}
	c.applyDefaults()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.FormatVersion == 0 {
		c.FormatVersion = FormatVersion
	}
	if c.Strictness == "" {
		c.Strictness = StrictnessStandard
	}
	if c.Privacy == "" {
		c.Privacy = PrivacyStandard
	}
	if c.DateOrder == "" {
		c.DateOrder = "mdy"
	}
	if c.Session.GapMinutes == 0 {
		c.Session.GapMinutes = 20
	}
	if c.Session.MaxJumpMeters == 0 {
		c.Session.MaxJumpMeters = 300
	}
	for i := range c.SubjectTypes {
		st := &c.SubjectTypes[i]
		if st.IdentifyBy == "" {
			st.IdentifyBy = IdentifyByCode
		}
		if st.Match.MinFields == 0 && !st.ByAppearance() {
			st.Match.MinFields = 1
		}
		for j := range st.IdentityFields {
			if st.IdentityFields[j].Kind == "" {
				st.IdentityFields[j].Kind = KindText
			}
		}
		if st.Barcode != nil {
			if len(st.Barcode.Symbologies) == 0 {
				st.Barcode.Symbologies = slices.Clone(DefaultIdentitySymbologies)
			}
			for k, s := range st.Barcode.Symbologies {
				st.Barcode.Symbologies[k] = NormalizeSymbology(s)
			}
		}
	}
}

// Validate reports every problem at once so a generated config can be fixed
// in a single round.
func (c *Config) Validate() error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if c.FormatVersion != FormatVersion {
		add("format_version %d is not supported (want %d)", c.FormatVersion, FormatVersion)
	}
	if strings.TrimSpace(c.Name) == "" {
		add("name is required")
	}
	if !slices.Contains([]Strictness{StrictnessStrict, StrictnessStandard, StrictnessLenient}, c.Strictness) {
		add("strictness %q must be strict, standard or lenient", c.Strictness)
	}
	if !slices.Contains([]Privacy{PrivacyHigh, PrivacyStandard}, c.Privacy) {
		add("privacy %q must be high or standard", c.Privacy)
	}
	if c.Language != "" && !languagePattern.MatchString(c.Language) {
		add("language %q must be a BCP 47 tag such as en or zh-Hans", c.Language)
	}
	if c.DateOrder != "mdy" && c.DateOrder != "dmy" {
		add("date_order %q must be mdy or dmy", c.DateOrder)
	}
	if c.Session.GapMinutes < 1 || c.Session.AnchorValidMinutes < 0 || c.Session.MaxJumpMeters < 0 {
		add("session rules must be positive")
	}

	// No subject types: photos are kept by where and when they were taken
	// (a trip, an event), grouped by place and day.
	if len(c.SubjectTypes) > maxSubjectTypes {
		add("at most %d subject types are allowed", maxSubjectTypes)
	}
	typeKeys := map[string]bool{}
	for _, st := range c.SubjectTypes {
		if !keyPattern.MatchString(st.Key) {
			add("subject type key %q must be lowercase letters, digits or underscores", st.Key)
		}
		if typeKeys[st.Key] {
			add("duplicate subject type %q", st.Key)
		}
		typeKeys[st.Key] = true
		errs = append(errs, st.validate()...)
	}
	for _, st := range c.SubjectTypes {
		if st.Parent != "" && !typeKeys[st.Parent] {
			add("subject type %q has unknown parent %q", st.Key, st.Parent)
		}
	}
	if hasParentCycle(c.SubjectTypes) {
		add("subject type parents form a cycle")
	}

	if len(c.Tags) > maxTags {
		add("at most %d tags are allowed", maxTags)
	}
	tagKeys := map[string]bool{}
	for _, t := range c.Tags {
		if !keyPattern.MatchString(t.Key) {
			add("tag key %q must be lowercase letters, digits or underscores", t.Key)
		}
		if tagKeys[t.Key] {
			add("duplicate tag %q", t.Key)
		}
		tagKeys[t.Key] = true
		if strings.TrimSpace(t.Name) == "" {
			add("tag %q needs a name", t.Key)
		}
	}
	return errors.Join(errs...)
}

func (st *SubjectType) validate() []error {
	var errs []error
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("subject type %q: "+format, append([]any{st.Key}, args...)...))
	}
	if strings.TrimSpace(st.Name) == "" {
		add("name is required")
	}
	if st.IdentifyBy != IdentifyByCode && st.IdentifyBy != IdentifyByAppearance {
		add("identify_by %q must be code or appearance", st.IdentifyBy)
	}
	if strings.TrimSpace(st.Carriers) == "" {
		add("carriers must describe where identity appears in photos, or what tells subjects apart")
	}
	// Subjects told apart by appearance need no identity fields: they are
	// named from how they look until someone names them.
	minFields := 1
	if st.ByAppearance() {
		minFields = 0
	}
	if len(st.IdentityFields) < minFields || len(st.IdentityFields) > maxIdentityField {
		add("needs %d to %d identity fields", minFields, maxIdentityField)
	}
	fieldKeys := map[string]bool{}
	for _, f := range st.IdentityFields {
		if !keyPattern.MatchString(f.Key) {
			add("field key %q must be lowercase letters, digits or underscores", f.Key)
		}
		if fieldKeys[f.Key] {
			add("duplicate field %q", f.Key)
		}
		fieldKeys[f.Key] = true
		if !slices.Contains([]FieldKind{KindText, KindPersonName, KindDate, KindID, KindAddress}, f.Kind) {
			add("field %q has unknown kind %q", f.Key, f.Kind)
		}
		if f.Pattern != "" {
			if _, err := regexp.Compile(f.Pattern); err != nil {
				add("field %q pattern: %v", f.Key, err)
			}
		}
	}
	for _, k := range st.DisplayFields {
		if !fieldKeys[k] {
			add("display field %q is not an identity field", k)
		}
	}
	if !st.ByAppearance() && (st.Match.MinFields < 1 || st.Match.MinFields > len(st.IdentityFields)) {
		add("match.min_fields must be between 1 and the number of identity fields")
	}
	if st.ByAppearance() && st.Barcode != nil {
		add("a barcode is a code: set identify_by to code to read one")
	}
	if b := st.Barcode; b != nil {
		if !fieldKeys[b.Field] {
			add("barcode field %q is not an identity field", b.Field)
		}
		if b.Pattern != "" {
			if _, err := regexp.Compile(b.Pattern); err != nil {
				add("barcode pattern: %v", err)
			}
		}
	}
	return errs
}

func hasParentCycle(types []SubjectType) bool {
	parent := map[string]string{}
	for _, st := range types {
		parent[st.Key] = st.Parent
	}
	for _, st := range types {
		seen := map[string]bool{}
		for k := st.Key; k != ""; k = parent[k] {
			if seen[k] {
				return true
			}
			seen[k] = true
		}
	}
	return false
}

// Marshal encodes a config for storage.
func (c *Config) Marshal() ([]byte, error) {
	return json.Marshal(c)
}
