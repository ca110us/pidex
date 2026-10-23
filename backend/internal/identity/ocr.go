package identity

import (
	"regexp"
	"strings"
	"time"
	"unicode"

	"pidex/internal/teamconfig"
)

// ReadFromOCR pulls identifier and date fields out of free on-device OCR
// text, without any model. Only unambiguous values are returned: if two
// different tokens fit a field, the field is left out. Names cannot be told
// apart from other words this way; callers check them against known
// subjects instead.
func ReadFromOCR(st *teamconfig.SubjectType, ocr, dateOrder string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(ocr) == "" {
		return out
	}
	for _, f := range st.IdentityFields {
		var found map[string]bool
		switch f.Kind {
		case teamconfig.KindID:
			found = idTokens(f, ocr)
		case teamconfig.KindDate:
			found = map[string]bool{}
			for _, m := range append(numericDate.FindAllString(ocr, -1), textDate.FindAllString(ocr, -1)...) {
				if d := NormalizeDate(m, dateOrder, time.Now()); d != "" {
					found[d] = true
				}
			}
		default:
			continue
		}
		if len(found) == 1 {
			for v := range found {
				out[f.Key] = v
			}
		}
	}
	return out
}

func idTokens(f teamconfig.Field, ocr string) map[string]bool {
	var re *regexp.Regexp
	if f.Pattern != "" {
		re, _ = regexp.Compile(f.Pattern)
	}
	label := normalizeID(f.Name)
	found := map[string]bool{}
	for _, w := range strings.FieldsFunc(ocr, unicode.IsSpace) {
		// "07/14/1990" would otherwise collapse to "07141990" and pass for
		// an identifier.
		if numericDate.MatchString(w) {
			continue
		}
		n := normalizeID(w)
		if label != "" && strings.HasPrefix(n, label) && len(n) > len(label) {
			n = strings.TrimPrefix(n, label)
		}
		// A token without digits is a word ("PATIENT"), not an identifier.
		if !strings.ContainsFunc(n, unicode.IsDigit) {
			continue
		}
		if re != nil && !re.MatchString(n) {
			continue
		}
		found[n] = true
	}
	return found
}

// Tokens are the words of OCR text that could be identifiers, normalized
// as identifiers are: those with a digit, dates aside. Matching them against
// known subjects finds a serial number among the other text on a screen.
func Tokens(ocr string) []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range strings.FieldsFunc(ocr, unicode.IsSpace) {
		if numericDate.MatchString(w) {
			continue
		}
		n := normalizeID(w)
		if len(n) < 4 || !strings.ContainsFunc(n, unicode.IsDigit) || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}
