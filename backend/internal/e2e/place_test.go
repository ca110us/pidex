package e2e

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/gen/pidex/v1/pidexv1connect"
)

// TestTripsByPlace: a collection kept by place and day has no subjects, so
// photos need not say who they show and nothing waits for a person; each
// keeps where it was taken, and is found by it.
func TestTripsByPlace(t *testing.T) {
	e := setup(t, "trips")
	hour := time.Hour
	dalian := &pidexv1.Place{Country: "China", Region: "Liaoning", City: "Dalian", District: "Shahekou", Name: "Xinghai Square"}
	shenyang := &pidexv1.Place{Country: "China", Region: "Liaoning", City: "Shenyang", Name: "Imperial Palace"}
	d1 := e.take(shot{scene: "tiling", session: "trip", at: 0, place: dalian})
	d2 := e.take(shot{scene: "knee", session: "trip", at: hour, place: dalian})
	s1 := e.take(shot{scene: "ankle", session: "trip", at: 30 * hour, place: shenyang})
	plain := e.take(shot{scene: "tiling", session: "home", at: 60 * hour})

	if p := e.photo(d1); p.SubjectID != nil || len(p.Place) == 0 {
		t.Errorf("a trip photo: subject %v, place %s", p.SubjectID, p.Place)
	}
	summary, err := e.capture.GetSessionSummary(e.ctx, connect.NewRequest(&pidexv1.GetSessionSummaryRequest{SessionId: e.sessions["trip"].String()}))
	if err != nil || len(summary.Msg.NeedsReview) != 0 {
		t.Errorf("trip photos wait for a person: %v %v", len(summary.Msg.GetNeedsReview()), err)
	}

	client := pidexv1connect.NewSearchServiceClient(e.server.Client(), e.server.URL, withToken(e.token))
	find := func(place string) []string {
		t.Helper()
		plan, _ := json.Marshal(map[string]any{"mode": "photos", "subject_query": "", "subject_type": "", "tags": []string{},
			"date_from": "", "date_to": "", "place": place, "text": ""})
		e.searchAnswers.push(string(plan))
		res, err := client.SearchPhotos(e.ctx, connect.NewRequest(&pidexv1.SearchPhotosRequest{Query: "photos from " + place, TimeZone: "UTC"}))
		if err != nil {
			t.Fatal(err)
		}
		got := ids(res.Msg.Photos)
		slices.Sort(got)
		return got
	}
	sorted := func(xs ...uuid.UUID) []string {
		var out []string
		for _, x := range xs {
			out = append(out, x.String())
		}
		slices.Sort(out)
		return out
	}
	if got := find("Dalian"); !slices.Equal(got, sorted(d1, d2)) {
		t.Errorf("Dalian: %v", got)
	}
	if got := find("Xinghai Square"); !slices.Equal(got, sorted(d1, d2)) {
		t.Errorf("a landmark: %v", got)
	}
	if got := find("Liaoning"); !slices.Equal(got, sorted(d1, d2, s1)) {
		t.Errorf("a province: %v", got)
	}

	// The collection page counts photos by place, and each shows its place.
	all, err := client.SearchPhotos(e.ctx, connect.NewRequest(&pidexv1.SearchPhotosRequest{Filters: &pidexv1.SearchFilters{}, TagCounts: true}))
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int32{}
	for _, c := range all.Msg.PlaceCounts {
		counts[c.Name] = c.Count
	}
	if counts["Dalian"] != 2 || counts["Shenyang"] != 1 || len(counts) != 2 {
		t.Errorf("place counts: %v", counts)
	}
	for _, p := range all.Msg.Photos {
		if p.Id == d1.String() && (p.Place.GetName() != "Xinghai Square" || p.Location.GetLatitude() == 0) {
			t.Errorf("a photo's place: %v %v", p.Place, p.Location)
		}
		if p.Id == plain.String() && p.Place != nil {
			t.Errorf("a photo without a place shows one: %v", p.Place)
		}
	}
}
