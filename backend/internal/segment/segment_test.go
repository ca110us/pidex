package segment

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"pidex/internal/teamconfig"
)

var t0 = time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)

func at(min int) time.Time { return t0.Add(time.Duration(min) * time.Minute) }

func rules() teamconfig.SessionRules {
	return teamconfig.SessionRules{GapMinutes: 15, AnchorValidMinutes: 45, MaxJumpMeters: 150}
}

func byPhoto(ds []Decision) map[uuid.UUID]Decision {
	out := map[uuid.UUID]Decision{}
	for _, d := range ds {
		out[d.PhotoID] = d
	}
	return out
}

func subjectOf(d Decision) uuid.UUID {
	if d.SubjectID == nil {
		return uuid.Nil
	}
	return *d.SubjectID
}

func TestAnchorsFileTheirRuns(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	items := []Item{
		{ID: uuid.New(), CapturedAt: at(0), Anchor: &Anchor{SubjectID: a, Confidence: 1}},
		{ID: uuid.New(), CapturedAt: at(1)},
		{ID: uuid.New(), CapturedAt: at(2)},
		{ID: uuid.New(), CapturedAt: at(5), Anchor: &Anchor{SubjectID: b, Confidence: 0.98}},
		{ID: uuid.New(), CapturedAt: at(6)},
	}
	d := byPhoto(Assign(items, rules()))
	for i, want := range []uuid.UUID{a, a, a, b, b} {
		if got := subjectOf(d[items[i].ID]); got != want {
			t.Errorf("photo %d filed under %v, want %v", i, got, want)
		}
	}
	if s := d[items[1].ID].Source; s != SourceAnchorAhead {
		t.Errorf("source %s", s)
	}
}

func TestAnchorShotLast(t *testing.T) {
	a := uuid.New()
	items := []Item{
		{ID: uuid.New(), CapturedAt: at(0)},
		{ID: uuid.New(), CapturedAt: at(1), Anchor: &Anchor{SubjectID: a, Confidence: 1}},
	}
	d := byPhoto(Assign(items, rules()))
	got := d[items[0].ID]
	if subjectOf(got) != a || got.Source != SourceAnchorAfter || got.Confidence >= 1 {
		t.Fatalf("%+v", got)
	}
}

func TestGapEndsRun(t *testing.T) {
	a := uuid.New()
	items := []Item{
		{ID: uuid.New(), CapturedAt: at(0), Anchor: &Anchor{SubjectID: a, Confidence: 1}},
		{ID: uuid.New(), CapturedAt: at(30)},
	}
	if d := byPhoto(Assign(items, rules()))[items[1].ID]; d.SubjectID != nil {
		t.Fatalf("anchor must not reach across a 30 minute gap: %+v", d)
	}
}

func TestMovingAwayEndsRun(t *testing.T) {
	a := uuid.New()
	lat1, lng1 := 40.7128, -74.0060
	lat2, lng2 := 40.7228, -74.0060 // about 1.1 km north
	items := []Item{
		{ID: uuid.New(), CapturedAt: at(0), Lat: &lat1, Lng: &lng1, Anchor: &Anchor{SubjectID: a, Confidence: 1}},
		{ID: uuid.New(), CapturedAt: at(1), Lat: &lat2, Lng: &lng2},
	}
	if d := byPhoto(Assign(items, rules()))[items[1].ID]; d.SubjectID != nil {
		t.Fatalf("anchor must not follow the photographer to another site: %+v", d)
	}
}

func TestBarrierStopsPreviousSubject(t *testing.T) {
	a := uuid.New()
	items := []Item{
		{ID: uuid.New(), CapturedAt: at(0), Anchor: &Anchor{SubjectID: a, Confidence: 1}},
		{ID: uuid.New(), CapturedAt: at(1)},
		{ID: uuid.New(), CapturedAt: at(2), Barrier: true}, // next patient's unreadable wristband
		{ID: uuid.New(), CapturedAt: at(3)},
	}
	d := byPhoto(Assign(items, rules()))
	if subjectOf(d[items[1].ID]) != a {
		t.Error("photo before the barrier keeps its anchor")
	}
	if d[items[3].ID].SubjectID != nil {
		t.Errorf("photo after an unresolved identity must not inherit the previous patient: %+v", d[items[3].ID])
	}
}

func TestExpiredAnchorFallsBackToBanner(t *testing.T) {
	a, locked := uuid.New(), uuid.New()
	items := []Item{
		{ID: uuid.New(), CapturedAt: at(0), Anchor: &Anchor{SubjectID: a, Confidence: 1}},
	}
	for m := 10; m <= 60; m += 10 {
		items = append(items, Item{ID: uuid.New(), CapturedAt: at(m), Locked: &locked})
	}
	last := byPhoto(Assign(items, rules()))[items[len(items)-1].ID]
	if subjectOf(last) != locked || last.Source != SourceLocked {
		t.Fatalf("after 60 minutes the anchor is stale: %+v", last)
	}
}

func TestManualPhotosAreLeftAlone(t *testing.T) {
	a := uuid.New()
	items := []Item{
		{ID: uuid.New(), CapturedAt: at(0), Anchor: &Anchor{SubjectID: a, Confidence: 1}},
		{ID: uuid.New(), CapturedAt: at(1), Manual: true},
	}
	for _, d := range Assign(items, rules()) {
		if d.PhotoID == items[1].ID {
			t.Fatal("manual photo must not get a decision")
		}
	}
}
