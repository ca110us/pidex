package identity

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"pidex/internal/teamconfig"
)

func patientType(t *testing.T) *teamconfig.SubjectType {
	t.Helper()
	cfg, err := teamconfig.LoadTemplate("hospital_orthopedics")
	if err != nil {
		t.Fatal(err)
	}
	st, _ := cfg.SubjectType("patient")
	return st
}

func TestNormalizeName(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"SMITH, John A.", "john smith"},
		{"Mr. John Smith", "john smith"},
		{"José Álvarez", "alvarez jose"},
		{"  ", ""},
	} {
		if got := Normalize(teamconfig.KindPersonName, tc.in, "mdy"); got != tc.want {
			t.Errorf("%q: got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeDate(t *testing.T) {
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ in, order, want string }{
		{"03/02/1985", "mdy", "1985-03-02"},
		{"03/02/1985", "dmy", "1985-02-03"},
		{"25/04/1985", "mdy", "1985-04-25"}, // unambiguous either way
		{"DOB: 1985-03-02", "mdy", "1985-03-02"},
		{"2 Mar 1985", "mdy", "1985-03-02"},
		{"March 2, 1985", "dmy", "1985-03-02"},
		{"03/02/85", "mdy", "1985-03-02"},
		{"03/02/24", "mdy", "2024-03-02"},
		{"02/30/1985", "mdy", ""},
		{"no date here", "mdy", ""},
	} {
		if got := NormalizeDate(tc.in, tc.order, now); got != tc.want {
			t.Errorf("%q (%s): got %q want %q", tc.in, tc.order, got, tc.want)
		}
	}
}

func TestSimilar(t *testing.T) {
	if !Similar(teamconfig.KindPersonName, "john smith", "john michael smith") {
		t.Error("middle name should not break a name match")
	}
	if !Similar(teamconfig.KindPersonName, "jonathan smith", "jonathon smith") {
		t.Error("one OCR slip should be tolerated in names")
	}
	if Similar(teamconfig.KindID, "12345678", "12345679") {
		t.Error("identifiers must match exactly")
	}
	if Similar(teamconfig.KindPersonName, "john smith", "jane smith") {
		t.Error("different first names must not match")
	}
}

func TestContainsValue(t *testing.T) {
	ocr := "SMITH, JOHN\nDOB 03/02/1985\nMRN:12345678 ACCT 99"
	if !ContainsValue(teamconfig.KindID, "12345678", ocr, "mdy") {
		t.Error("label glued to the id should still confirm it")
	}
	if ContainsValue(teamconfig.KindID, "1234", "12 34", "mdy") {
		t.Error("ids must not be assembled across tokens")
	}
	if !ContainsValue(teamconfig.KindDate, "1985-03-02", ocr, "mdy") {
		t.Error("date should be found")
	}
	if !ContainsValue(teamconfig.KindPersonName, "john smith", ocr, "mdy") {
		t.Error("name should be found")
	}
}

func TestFromBarcode(t *testing.T) {
	st := patientType(t)
	if f, v, ok := FromBarcode(st, "code128", "MRN-12345678"); !ok || f != "mrn" || v != "12345678" {
		t.Errorf("got %q %q %v", f, v, ok)
	}
	if _, _, ok := FromBarcode(st, "upca", "012345678905"); ok {
		t.Error("a product barcode must never identify a patient")
	}
}

func TestReadFromOCR(t *testing.T) {
	st := patientType(t)
	got := ReadFromOCR(st, "PATIENT\nDOE, JANE\nDOB 07/14/1990\nMRN 87654321", "mdy")
	if got["mrn"] != "87654321" || got["dob"] != "1990-07-14" {
		t.Errorf("got %v", got)
	}
	if _, ok := got["name"]; ok {
		t.Error("names are not read from free OCR")
	}
	if got := ReadFromOCR(st, "MRN 87654321 VISIT 55554444", "mdy"); got["mrn"] != "" {
		t.Errorf("two id-like tokens must leave the field out, got %v", got)
	}
}

func TestResolve(t *testing.T) {
	st := patientType(t)
	a, b := uuid.New(), uuid.New()
	known := []Candidate{
		{ID: a, Norm: map[string]string{"name": "john smith", "dob": "1985-03-02", "mrn": "12345678"}},
		{ID: b, Norm: map[string]string{"name": "jane doe", "dob": "1990-07-14", "mrn": "87654321"}},
	}
	read := func(conf float64, kv ...string) Read {
		r := Read{Fields: map[string]string{}, Confidence: conf}
		for i := 0; i < len(kv); i += 2 {
			r.Fields[kv[i]] = kv[i+1]
		}
		return r
	}

	t.Run("two fields match one patient", func(t *testing.T) {
		res := Resolve(st, read(0.98, "name", "SMITH, JOHN", "dob", "03/02/1985"), "mdy", known)
		if res.Outcome != OutcomeMatched || res.SubjectID != a || res.Confidence < 0.97 {
			t.Fatalf("%+v", res)
		}
	})
	t.Run("one field is not enough for strict patients", func(t *testing.T) {
		res := Resolve(st, read(0.98, "name", "John Smith"), "mdy", known)
		if res.Outcome != OutcomeAmbiguous || len(res.Candidates) != 1 || res.Candidates[0] != a {
			t.Fatalf("%+v", res)
		}
	})
	t.Run("a conflicting field rules a candidate out", func(t *testing.T) {
		res := Resolve(st, read(0.98, "name", "John Smith", "dob", "03/02/1985", "mrn", "11112222"), "mdy", known)
		if res.Outcome == OutcomeMatched {
			t.Fatalf("must not match with a conflicting MRN: %+v", res)
		}
		if res.Outcome != OutcomeNewSubject || res.Confidence >= 0.97*0.98 {
			t.Fatalf("near-duplicate should need confirmation: %+v", res)
		}
	})
	t.Run("strong barcode alone matches", func(t *testing.T) {
		r := read(1, "mrn", "87654321")
		r.BarcodeField = "mrn"
		res := Resolve(st, r, "mdy", known)
		if res.Outcome != OutcomeMatched || res.SubjectID != b || res.Confidence < 0.99 {
			t.Fatalf("%+v", res)
		}
	})
	t.Run("unknown patient becomes new subject", func(t *testing.T) {
		res := Resolve(st, read(0.98, "name", "Bob Lee", "dob", "01/01/1970"), "mdy", known)
		if res.Outcome != OutcomeNewSubject || res.Confidence != 0.98 {
			t.Fatalf("%+v", res)
		}
	})
	t.Run("nothing usable is not an anchor", func(t *testing.T) {
		res := Resolve(st, read(0.9, "mrn", "??"), "mdy", known)
		if res.Outcome != OutcomeNotAnchor {
			t.Fatalf("%+v", res)
		}
	})
}
