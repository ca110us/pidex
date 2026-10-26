package pipeline

import (
	"testing"

	"github.com/google/uuid"

	"pidex/internal/teamconfig"
)

// TestGuardLookalikes: a sure match to one of two subjects described alike
// is asked about, not filed.
func TestGuardLookalikes(t *testing.T) {
	th := (&teamconfig.Config{Strictness: teamconfig.StrictnessStandard}).Thresholds()
	tuxedo := []float32{1, 0.1, 0}
	cands := []AppearanceCandidate{
		{DisplayName: "Socks", Vec: tuxedo},
		{DisplayName: "Palmerston", Vec: []float32{1, 0.12, 0.01}},
		{DisplayName: "Larry", Vec: []float32{0, 1, 0}},
	}
	v := GuardLookalikes(AppearanceVerdict{Match: 0, Confidence: 0.97}, cands, th)
	if v.Confidence >= th.AutoAssign || v.Confidence < th.Suggest {
		t.Errorf("a look-alike match is a suggestion: %.2f", v.Confidence)
	}
	if v := GuardLookalikes(AppearanceVerdict{Match: 2, Confidence: 0.97}, cands, th); v.Confidence != 0.97 {
		t.Errorf("one with no look-alike is filed: %.2f", v.Confidence)
	}
	if v := GuardLookalikes(AppearanceVerdict{Match: -1, Confidence: 0.97}, cands, th); v.Confidence != 0.97 {
		t.Errorf("a new one is not guarded: %.2f", v.Confidence)
	}
}

// TestAppearanceRules: only a near-certain match by look is filed without
// asking; strict teams ask unless it is all but certain.
func TestAppearanceRules(t *testing.T) {
	cands := []AppearanceCandidate{{DisplayName: "Socks", Vec: []float32{1, 0}}, {DisplayName: "Larry", Vec: []float32{0, 1}}}
	standard := &teamconfig.Config{Strictness: teamconfig.StrictnessStandard}
	th := standard.Thresholds()
	if v := AppearanceRules(AppearanceVerdict{Match: 0, Confidence: 0.92}, cands, standard, nil); v.Confidence >= th.AutoAssign {
		t.Errorf("0.92 by look is filed without asking: %.2f", v.Confidence)
	}
	if v := AppearanceRules(AppearanceVerdict{Match: 0, Confidence: 0.98}, cands, standard, nil); v.Confidence != 0.98 {
		t.Errorf("0.98 by look is filed: %.2f", v.Confidence)
	}
	strict := &teamconfig.Config{Strictness: teamconfig.StrictnessStrict}
	if v := AppearanceRules(AppearanceVerdict{Match: 0, Confidence: 0.98}, cands, strict, nil); v.Confidence >= strict.Thresholds().AutoAssign {
		t.Errorf("strict files 0.98 by look: %.2f", v.Confidence)
	}
	if v := AppearanceRules(AppearanceVerdict{Match: -1, Confidence: 0.9}, cands, standard, nil); v.Confidence != 0.9 {
		t.Errorf("a new one is not held back: %.2f", v.Confidence)
	}
}

// TestEarnedTrust: between two look-alikes the model's answers are asked
// about until people have found it right often enough, in separate shoots,
// at least as sure as it is now; then they are filed. A mistake takes the
// trust back.
func TestEarnedTrust(t *testing.T) {
	tutu, xiuxiu, other := uuid.New(), uuid.New(), uuid.New()
	cands := []AppearanceCandidate{
		{ID: tutu, DisplayName: "Tutu", Vec: []float32{1, 0.1, 0}},
		{ID: xiuxiu, DisplayName: "Xiuxiu", Vec: []float32{1, 0.12, 0.01}},
		{ID: other, DisplayName: "Tiger", Vec: []float32{0, 1, 0}},
	}
	standard := &teamconfig.Config{Strictness: teamconfig.StrictnessStandard}
	th := standard.Thresholds()
	check := func(verdict, final uuid.UUID, conf float64) LookCheck {
		return LookCheck{Verdict: verdict, Final: final, Confidence: conf, Session: uuid.New()}
	}
	var checks []LookCheck
	for i := range trustAfter {
		// Both look-alikes' checks count for either.
		who := map[bool]uuid.UUID{true: tutu, false: xiuxiu}[i%2 == 0]
		checks = append(checks, check(who, who, 0.85))
	}
	filed := func(conf float64, checks []LookCheck) bool {
		return AppearanceRules(AppearanceVerdict{Match: 0, Confidence: conf}, cands, standard, checks).Confidence >= th.AutoAssign
	}

	if filed(0.9, checks[:trustAfter-1]) {
		t.Error("filed before enough checks")
	}
	if !filed(0.9, checks) {
		t.Error("not filed after enough right answers")
	}
	if !filed(0.85, checks) || filed(0.82, checks) {
		t.Error("trusted below the confidence the checks were at")
	}
	if filed(0.78, checks) {
		t.Error("trusted below the floor")
	}

	// One shoot counts once.
	shoot := uuid.New()
	same := make([]LookCheck, trustAfter)
	for i := range same {
		same[i] = LookCheck{Verdict: tutu, Final: tutu, Confidence: 0.9, Session: shoot}
	}
	if filed(0.9, same) {
		t.Error("one shoot earned trust")
	}

	// A mistake as sure as now, the newest, takes it back; above it, the
	// right answers still count.
	wrong := append([]LookCheck{check(tutu, xiuxiu, 0.9)}, checks...)
	if filed(0.9, wrong) {
		t.Error("filed after a mistake at that confidence")
	}
	sure := append([]LookCheck{check(tutu, xiuxiu, 0.86)}, checks...)
	for range trustAfter {
		sure = append([]LookCheck{check(tutu, tutu, 0.95)}, sure...)
	}
	if !filed(0.95, sure) || filed(0.9, sure) {
		t.Error("the bar is not above the last mistake")
	}

	// Checks of an unrelated subject do not count, nor do strict teams.
	unrelated := make([]LookCheck, trustAfter)
	for i := range unrelated {
		unrelated[i] = check(other, other, 0.95)
	}
	if filed(0.9, unrelated) {
		t.Error("another subject's checks earned trust")
	}
	strict := &teamconfig.Config{Strictness: teamconfig.StrictnessStrict}
	if v := AppearanceRules(AppearanceVerdict{Match: 0, Confidence: 0.95}, cands, strict, checks); v.Confidence >= strict.Thresholds().AutoAssign {
		t.Error("strict teams trust the look")
	}
}

// TestSureByLook: a photo is filed by its look alone only when the closest
// subject is similar enough and clearly closer than the next; never for
// strict teams.
func TestSureByLook(t *testing.T) {
	standard := &teamconfig.Config{Strictness: teamconfig.StrictnessStandard}
	m := func(s ...float64) []LookMatch {
		out := make([]LookMatch, len(s))
		for i, v := range s {
			out[i] = LookMatch{SubjectID: uuid.New(), Similarity: v}
		}
		return out
	}
	cases := []struct {
		name    string
		matches []LookMatch
		cfg     *teamconfig.Config
		want    bool
	}{
		{"alone and close", m(0.9), standard, true},
		{"close, clearly closer than the next", m(0.92, 0.80), standard, true},
		{"close, but a look-alike is as close", m(0.92, 0.88), standard, false},
		{"not close enough", m(0.84, 0.2), standard, false},
		{"nothing filed", nil, standard, false},
		{"strict teams always ask the model", m(0.99), &teamconfig.Config{Strictness: teamconfig.StrictnessStrict}, false},
	}
	for _, c := range cases {
		if got := SureByLook(c.matches, c.cfg, DefaultLookAutoSimilarity, DefaultLookMargin); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
