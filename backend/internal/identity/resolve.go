package identity

import (
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"

	"pidex/internal/teamconfig"
)

// Read is identity text extracted from one photo for one subject type.
type Read struct {
	Fields map[string]string
	// Confidence that Fields were read correctly, before any matching.
	Confidence float64
	// BarcodeField is set when that field came from a barcode that passed
	// the type's barcode pattern.
	BarcodeField string
}

type Candidate struct {
	ID   uuid.UUID
	Norm map[string]string
}

type Outcome string

const (
	OutcomeNotAnchor  Outcome = "not_anchor"
	OutcomeMatched    Outcome = "matched"
	OutcomeNewSubject Outcome = "new_subject"
	OutcomeAmbiguous  Outcome = "ambiguous"
)

type Resolution struct {
	Outcome    Outcome     `json:"outcome"`
	SubjectID  uuid.UUID   `json:"subject_id,omitzero"`
	Confidence float64     `json:"confidence"`
	Candidates []uuid.UUID `json:"candidates,omitempty"`
	// Fields holds the usable raw values; Norm their normalized forms.
	Fields map[string]string `json:"fields,omitempty"`
	Norm   map[string]string `json:"norm,omitempty"`
}

// Clean keeps only fields that belong to the type, normalize to something
// usable and satisfy the field pattern.
func Clean(st *teamconfig.SubjectType, fields map[string]string, dateOrder string) (raw, normalized map[string]string) {
	raw, normalized = map[string]string{}, map[string]string{}
	for _, f := range st.IdentityFields {
		v := strings.TrimSpace(fields[f.Key])
		if v == "" {
			continue
		}
		n := Normalize(f.Kind, v, dateOrder)
		if n == "" {
			continue
		}
		if f.Pattern != "" {
			if re, err := regexp.Compile(f.Pattern); err == nil && !re.MatchString(n) {
				continue
			}
		}
		raw[f.Key], normalized[f.Key] = v, n
	}
	return raw, normalized
}

// FromBarcode applies the type's barcode rule to a payload.
func FromBarcode(st *teamconfig.SubjectType, symbology, payload string) (field, value string, ok bool) {
	b := st.Barcode
	if b == nil || !b.Accepts(symbology) {
		return "", "", false
	}
	value, ok = b.Extract(payload)
	return b.Field, value, ok
}

type scored struct {
	id       uuid.UUID
	agree    int
	conflict int
	barcode  bool
}

// Resolve decides which candidate a read belongs to. It is conservative:
// any disagreeing field rules a candidate out, and a new subject is only
// proposed when the read alone satisfies the type's match rule.
func Resolve(st *teamconfig.SubjectType, read Read, dateOrder string, candidates []Candidate) Resolution {
	raw, normalized := Clean(st, read.Fields, dateOrder)
	res := Resolution{Fields: raw, Norm: normalized}
	if len(normalized) == 0 {
		res.Outcome = OutcomeNotAnchor
		return res
	}

	strongBarcode := st.Barcode != nil && st.Barcode.Strong &&
		read.BarcodeField == st.Barcode.Field && normalized[read.BarcodeField] != ""

	var eligible, partial []scored
	sawConflict := false
	for _, c := range candidates {
		s := scored{id: c.ID}
		for _, f := range st.IdentityFields {
			mine, theirs := normalized[f.Key], c.Norm[f.Key]
			if mine == "" || theirs == "" {
				continue
			}
			if Similar(f.Kind, mine, theirs) {
				s.agree++
				if f.Key == read.BarcodeField {
					s.barcode = true
				}
			} else {
				s.conflict++
			}
		}
		switch {
		case s.conflict > 0:
			if s.agree > 0 {
				sawConflict = true
			}
		case s.agree >= st.Match.MinFields || (strongBarcode && s.barcode):
			eligible = append(eligible, s)
		case s.agree > 0:
			partial = append(partial, s)
		}
	}

	sort.SliceStable(eligible, func(i, j int) bool { return eligible[i].agree > eligible[j].agree })
	if len(eligible) == 1 || (len(eligible) > 1 && eligible[0].agree > eligible[1].agree) {
		best := eligible[0]
		res.Outcome = OutcomeMatched
		res.SubjectID = best.id
		switch {
		case strongBarcode && best.barcode:
			res.Confidence = 0.995 * read.Confidence
		default:
			// Landing exactly on an existing subject's values is evidence in
			// itself: a misread rarely reproduces someone else's record.
			c := 1 - (1-read.Confidence)*0.5
			coverage := float64(best.agree) / float64(len(normalized))
			res.Confidence = c * (0.9 + 0.1*coverage)
		}
		return res
	}
	if len(eligible) > 1 {
		res.Outcome = OutcomeAmbiguous
		for _, s := range eligible {
			res.Candidates = append(res.Candidates, s.id)
		}
		res.Confidence = 0.5 * read.Confidence
		return res
	}
	if len(partial) > 0 {
		sort.SliceStable(partial, func(i, j int) bool { return partial[i].agree > partial[j].agree })
		res.Outcome = OutcomeAmbiguous
		for _, s := range partial {
			res.Candidates = append(res.Candidates, s.id)
		}
		res.Confidence = 0.6 * read.Confidence
		return res
	}
	if len(normalized) >= st.Match.MinFields || strongBarcode {
		res.Outcome = OutcomeNewSubject
		res.Confidence = read.Confidence
		if sawConflict {
			// Someone shares part of this identity; a misread is plausible,
			// so a person should confirm before a near-duplicate is created.
			res.Confidence *= 0.8
		}
		return res
	}
	res.Outcome = OutcomeAmbiguous
	return res
}
