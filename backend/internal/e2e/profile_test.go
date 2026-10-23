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

// TestSubjectProfiles: a pet's profile, written from its photos, holds what
// no single photo's description says (a freckled nose), and searching for it
// finds that pet's photos only. A profile a person writes is theirs: photos
// do not rewrite it, and it is searched the same way.
func TestSubjectProfiles(t *testing.T) {
	e := setup(t, "household_pets")
	min := func(m float64) time.Duration { return time.Duration(m * float64(time.Minute)) }
	g1 := e.take(shot{scene: "cat_ginger", session: "s1", at: 0})
	g2 := e.take(shot{scene: "cat_ginger", session: "s2", at: min(90)})
	e.take(shot{scene: "cat_tuxedo", session: "s3", at: min(200)})
	e.take(shot{scene: "cat_tuxedo", session: "s4", at: min(300)})

	if err := e.proc.ProfileSubjects(e.ctx); err != nil {
		t.Fatal(err)
	}
	subjects, err := e.library.ListSubjects(e.ctx, connect.NewRequest(&pidexv1.ListSubjectsRequest{TypeKey: "pet"}))
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*pidexv1.Subject{}
	for _, s := range subjects.Msg.Subjects {
		byName[s.DisplayName] = s
	}
	ginger, tuxedo := byName["Ginger cat"], byName["Tuxedo cat"]
	if ginger == nil || tuxedo == nil || ginger.Profile != "orange tabby coat\nfreckled pink nose\namber eyes" || ginger.ProfileByPerson {
		t.Fatalf("profiles written from photos: %v", byName)
	}

	chat := pidexv1connect.NewChatServiceClient(e.server.Client(), e.server.URL, withToken(e.token))
	find := func(text string) []string {
		t.Helper()
		plan, _ := json.Marshal(map[string]any{"mode": "photos", "subject_query": "", "subject_type": "", "tags": []string{},
			"date_from": "", "date_to": "", "text": text, "intent": "photos", "about_previous": false})
		e.searchAnswers.push(string(plan))
		res, err := chat.SendMessage(e.ctx, connect.NewRequest(&pidexv1.SendMessageRequest{Text: "the cat with the " + text, TimeZone: "UTC"}))
		if err != nil {
			t.Fatal(err)
		}
		got := ids(res.Msg.Reply.Photos)
		slices.Sort(got)
		return got
	}
	want := []string{g1.String(), g2.String()}
	slices.Sort(want)
	if got := find("freckled nose"); !slices.Equal(got, want) {
		t.Errorf("the freckled nose is the ginger cat's: %v", got)
	}

	// A person adds what the photos did not show; it is kept and searched.
	if _, err := e.library.SetSubjectProfile(e.ctx, connect.NewRequest(&pidexv1.SetSubjectProfileRequest{SubjectId: tuxedo.Id, Profile: "one notched ear"})); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(e.ctx, `UPDATE subjects SET profile_photos = 0 WHERE id = $1`, uuid.MustParse(tuxedo.Id)); err != nil {
		t.Fatal(err)
	}
	if err := e.proc.ProfileSubjects(e.ctx); err != nil {
		t.Fatal(err)
	}
	again, err := e.library.ListSubjects(e.ctx, connect.NewRequest(&pidexv1.ListSubjectsRequest{TypeKey: "pet"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range again.Msg.Subjects {
		if s.Id == tuxedo.Id && (s.Profile != "one notched ear" || !s.ProfileByPerson) {
			t.Errorf("a person's profile was rewritten: %q", s.Profile)
		}
	}
	if got := find("notched ear"); len(got) != 2 || slices.Contains(got, g1.String()) {
		t.Errorf("the notched ear is the tuxedo cat's: %v", got)
	}
}
