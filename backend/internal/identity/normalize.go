// Package identity turns identity text read from photos into canonical values
// and decides which subject, if any, a read belongs to.
package identity

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"pidex/internal/teamconfig"
)

var foldDiacritics = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

func fold(s string) string {
	out, _, err := transform.String(foldDiacritics, s)
	if err != nil {
		return s
	}
	return out
}

// words lowercases, folds diacritics and splits on anything that is not a
// letter or digit.
func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(fold(s)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

var honorifics = map[string]bool{"mr": true, "mrs": true, "ms": true, "miss": true, "dr": true, "prof": true}

// Normalize returns the canonical form of a value for matching, or "" when
// the value is unusable for its kind.
func Normalize(kind teamconfig.FieldKind, value, dateOrder string) string {
	switch kind {
	case teamconfig.KindPersonName:
		return normalizeName(value)
	case teamconfig.KindDate:
		return NormalizeDate(value, dateOrder, time.Now())
	case teamconfig.KindID:
		return normalizeID(value)
	default:
		return strings.Join(words(value), " ")
	}
}

// normalizeName sorts tokens so "SMITH, John A." and "john smith" agree.
// Initials and honorifics are dropped.
func normalizeName(s string) string {
	var kept []string
	for _, w := range words(s) {
		if len([]rune(w)) < 2 || honorifics[w] {
			continue
		}
		kept = append(kept, w)
	}
	sort.Strings(kept)
	return strings.Join(kept, " ")
}

func normalizeID(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(fold(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var (
	numericDate = regexp.MustCompile(`(\d{1,4})[./-](\d{1,2})[./-](\d{1,4})`)
	textDate    = regexp.MustCompile(`(?i)\d{1,2}[\s-]+[a-z]{3,9}\.?[\s-]+\d{4}|[a-z]{3,9}\.?\s+\d{1,2},?\s+\d{4}`)
	textLayouts = []string{"2 Jan 2006", "2 January 2006", "Jan 2 2006", "January 2 2006"}
)

// NormalizeDate finds a date anywhere in s and returns it as YYYY-MM-DD.
// dateOrder ("mdy" or "dmy") resolves 03/04/1985; an unambiguous value such
// as 25/04/1985 is read correctly either way.
func NormalizeDate(s, dateOrder string, now time.Time) string {
	if m := numericDate.FindStringSubmatch(s); m != nil {
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[2])
		c, _ := strconv.Atoi(m[3])
		var y, mo, d int
		switch {
		case len(m[1]) == 4:
			y, mo, d = a, b, c
		case dateOrder == "dmy" && b <= 12, a > 12 && b <= 12:
			d, mo, y = a, b, c
		default:
			mo, d, y = a, b, c
		}
		if len(m[3]) == 2 && len(m[1]) != 4 {
			y = expandYear(y, now)
		}
		return formatDate(y, mo, d)
	}
	if m := textDate.FindString(s); m != "" {
		cleaned := strings.Join(strings.FieldsFunc(m, func(r rune) bool {
			return r == ' ' || r == '-' || r == ',' || r == '.'
		}), " ")
		for _, layout := range textLayouts {
			if t, err := time.Parse(layout, titleMonth(cleaned)); err == nil {
				return t.Format("2006-01-02")
			}
		}
	}
	return ""
}

// expandYear reads a two-digit year as the most recent past year it could be.
func expandYear(yy int, now time.Time) int {
	century := now.Year() / 100 * 100
	if century+yy > now.Year() {
		return century - 100 + yy
	}
	return century + yy
}

func formatDate(y, mo, d int) string {
	if mo < 1 || mo > 12 || d < 1 || d > 31 || y < 1850 || y > 2200 {
		return ""
	}
	t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
	if t.Day() != d {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", y, mo, d)
}

func titleMonth(s string) string {
	parts := strings.Fields(s)
	for i, p := range parts {
		if p != "" && unicode.IsLetter(rune(p[0])) {
			parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		}
	}
	return strings.Join(parts, " ")
}

// Similar reports whether two normalized values denote the same thing.
// Identifiers and dates must be identical; names and free text tolerate the
// small slips OCR makes.
func Similar(kind teamconfig.FieldKind, a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	switch kind {
	case teamconfig.KindID, teamconfig.KindDate:
		return false
	case teamconfig.KindPersonName:
		if tokenSubset(a, b) {
			return true
		}
	}
	return similarity(a, b) >= 0.9
}

// tokenSubset accepts "john smith" against "john michael smith": every token
// of the shorter name appears in the longer one and at least two match.
func tokenSubset(a, b string) bool {
	ta, tb := strings.Fields(a), strings.Fields(b)
	if len(ta) > len(tb) {
		ta, tb = tb, ta
	}
	if len(ta) < 2 {
		return false
	}
	set := map[string]bool{}
	for _, t := range tb {
		set[t] = true
	}
	for _, t := range ta {
		if !set[t] {
			return false
		}
	}
	return true
}

// similarity is 1 - edit distance / longer length.
func similarity(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	longer := max(len(ra), len(rb))
	if longer == 0 {
		return 1
	}
	return 1 - float64(levenshtein(ra, rb))/float64(longer)
}

func levenshtein(a, b []rune) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// ContainsValue reports whether normalized value v can be found in free OCR
// text. It is the second, independent read used to confirm a model's answer.
func ContainsValue(kind teamconfig.FieldKind, v, ocrText, dateOrder string) bool {
	if v == "" || ocrText == "" {
		return false
	}
	switch kind {
	case teamconfig.KindDate:
		for _, m := range numericDate.FindAllString(ocrText, -1) {
			if NormalizeDate(m, dateOrder, time.Now()) == v {
				return true
			}
		}
		for _, m := range textDate.FindAllString(ocrText, -1) {
			if NormalizeDate(m, dateOrder, time.Now()) == v {
				return true
			}
		}
		return false
	case teamconfig.KindID:
		// Whole tokens only, allowing a label glued on the front ("MRN:123").
		// Matching across token boundaries would let digits from unrelated
		// numbers combine into a false confirmation.
		for _, w := range strings.FieldsFunc(ocrText, unicode.IsSpace) {
			n := normalizeID(w)
			if n == v || (len(v) >= 5 && strings.HasSuffix(n, v)) {
				return true
			}
		}
		return false
	case teamconfig.KindPersonName:
		have := map[string]bool{}
		for _, w := range words(ocrText) {
			have[w] = true
		}
		for _, t := range strings.Fields(v) {
			if !have[t] {
				return false
			}
		}
		return true
	default:
		return strings.Contains(" "+strings.Join(words(ocrText), " ")+" ", " "+v+" ")
	}
}
