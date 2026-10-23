package e2e

import (
	"encoding/json"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/internal/pipeline"
)

// TestPetsByAppearance files pets nobody labels: each new one becomes a
// subject named from its look, later photos of it are recognized, and a
// photo of something else follows the pet shot before it.
func TestPetsByAppearance(t *testing.T) {
	e := setup(t, "household_pets")
	min := func(m float64) time.Duration { return time.Duration(m * float64(time.Minute)) }

	g1 := e.take(shot{scene: "cat_ginger", session: "s1", at: 0})
	if n := e.compares.Load(); n != 0 {
		t.Errorf("the first pet has nothing to be compared with, yet %d comparisons ran", n)
	}
	g2 := e.take(shot{scene: "cat_ginger", session: "s1", at: min(1)})
	bowl := e.take(shot{scene: "food_bowl", session: "s1", at: min(2)})
	t1 := e.take(shot{scene: "cat_tuxedo", session: "s1", at: min(3)})
	t2 := e.take(shot{scene: "cat_tuxedo", session: "s2", at: min(90)})

	subjects, err := e.library.ListSubjects(e.ctx, connect.NewRequest(&pidexv1.ListSubjectsRequest{TypeKey: "pet"}))
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]uuid.UUID{}
	for _, s := range subjects.Msg.Subjects {
		byName[s.DisplayName] = uuid.MustParse(s.Id)
		if s.Cover.GetViewUrl() == "" {
			t.Errorf("%s is listed without a photo to show it by", s.DisplayName)
		}
	}
	ginger, tuxedo := byName["Ginger cat"], byName["Tuxedo cat"]
	if len(byName) != 2 || ginger == uuid.Nil || tuxedo == uuid.Nil {
		t.Fatalf("one subject per pet, named from its look: %v", byName)
	}
	e.expectFiled("first ginger photo", g1, &ginger, "auto")
	e.expectFiled("ginger again", g2, &ginger, "auto")
	e.expectFiled("bowl after the ginger cat", bowl, &ginger, "auto")
	e.expectFiled("new tuxedo cat", t1, &tuxedo, "auto")
	e.expectFiled("tuxedo cat in a later session", t2, &tuxedo, "auto")

	// Picked by hand on the camera: filed as the person said, even a photo
	// the model would have filed elsewhere.
	before := e.compares.Load()
	picked := e.take(shot{scene: "cat_ginger", session: "s3", at: min(200), chosen: tuxedo})
	e.expectFiled("picked on the camera", picked, &tuxedo, "manual")
	if e.compares.Load() != before {
		t.Error("a photo picked by hand is not matched again")
	}
	if err := e.proc.ResegmentSession(e.ctx, e.teamID, e.sessions["s3"]); err != nil {
		t.Fatal(err)
	}
	e.expectFiled("still as picked after re-filing", picked, &tuxedo, "manual")

	// Shot together, the photos of one cat are one cat: a far-away shot
	// alone is only a suggestion, beside a clear one it is filed.
	clear := e.take(shot{scene: "cat_ginger", session: "s4", at: min(300)})
	far := e.take(shot{scene: "cat_ginger_far", session: "s4", at: min(301)})
	e.expectFiled("clear shot", clear, &ginger, "auto")
	e.expectFiled("far shot beside a clear one", far, &ginger, "auto")
	far1 := e.take(shot{scene: "cat_ginger_far", session: "s5", at: min(400)})
	far2 := e.take(shot{scene: "cat_ginger_far", session: "s5", at: min(401)})
	if p := e.photo(far2); p.Assignment != "suggested" || p.SuggestedSubjectID == nil || *p.SuggestedSubjectID != ginger {
		t.Fatalf("far shots alone are suggestions: %s %v", p.Assignment, p.SuggestedSubjectID)
	}
	describedAs := func(id uuid.UUID) string {
		t.Helper()
		res, err := e.library.GetPhoto(e.ctx, connect.NewRequest(&pidexv1.GetPhotoRequest{Id: id.String()}))
		if err != nil {
			t.Fatal(err)
		}
		return res.Msg.Photo.Description
	}
	// A suggestion may be wrong: it is described as seen, not by name.
	if d := describedAs(far2); d != "A ginger cat across the room." {
		t.Errorf("a suggested photo is described as %q", d)
	}
	// One confirmation answers for the whole batch.
	if _, err := e.library.AssignPhotos(e.ctx, connect.NewRequest(&pidexv1.AssignPhotosRequest{PhotoIds: []string{far1.String()}, SubjectId: ginger.String()})); err != nil {
		t.Fatal(err)
	}
	e.expectFiled("the rest of the batch once one is confirmed", far2, &ginger, "auto")

	// The camera asks who a shot shows before it is uploaded.
	known, err := e.capture.IdentifyShot(e.ctx, connect.NewRequest(&pidexv1.IdentifyShotRequest{Jpeg: jpegOf(scenes["cat_ginger"].c)}))
	if err != nil || known.Msg.Anchor.State != pidexv1.AnchorState_ANCHOR_STATE_MATCHED || known.Msg.Anchor.Subject.GetId() != ginger.String() {
		t.Errorf("a known cat on the camera: %v %v", known, err)
	}
	// Confirmed on the camera, the shot keeps what the model said of it: the
	// person's answer checks the model's. Here it was right.
	if _, err := e.capture.IdentifyShot(e.ctx, connect.NewRequest(&pidexv1.IdentifyShotRequest{Jpeg: jpegOf(scenes["cat_ginger_far"].c), ClientPhotoId: "cam-1"})); err != nil {
		t.Fatal(err)
	}
	checked := e.take(shot{scene: "cat_ginger_far", session: "s5b", at: min(450), chosen: ginger, clientID: "cam-1"})
	var ex pipeline.Extracted
	if err := json.Unmarshal(e.photo(checked).Extracted, &ex); err != nil {
		t.Fatal(err)
	}
	if a := ex.Appearance; a == nil || a.Verdict == nil || *a.Verdict != ginger || a.ModelConfidence != 0.7 || a.Unseen {
		t.Errorf("the camera's answer is not kept with the photo: %+v", a)
	}
	stranger, err := e.capture.IdentifyShot(e.ctx, connect.NewRequest(&pidexv1.IdentifyShotRequest{Jpeg: jpegOf(scenes["cat_calico"].c)}))
	if err != nil || stranger.Msg.Anchor.State != pidexv1.AnchorState_ANCHOR_STATE_NEW_SUBJECT || stranger.Msg.Anchor.ExtractedIdentity["name"] != "Calico cat" {
		t.Errorf("a new cat on the camera: %v %v", stranger, err)
	}

	// Named on the camera before any photo of it: the first photo teaches
	// it how it looks, and the next one is recognized.
	named, err := e.library.CreateSubject(e.ctx, connect.NewRequest(&pidexv1.CreateSubjectRequest{TypeKey: "pet", DisplayName: "Tutu"}))
	if err != nil {
		t.Fatal(err)
	}
	tutu := uuid.MustParse(named.Msg.Subject.Id)
	e.take(shot{scene: "cat_calico", session: "s6", at: min(500), chosen: tutu})
	again := e.take(shot{scene: "cat_calico", session: "s7", at: min(600)})
	e.expectFiled("the named cat, recognized next time", again, &tutu, "auto")
	// A photo the model finds unrelated, filed by hand, stays in view.
	blurred := e.take(shot{scene: "blurry", session: "s8", at: min(700), chosen: tutu})
	if p := e.photo(blurred); p.Role == nil || *p.Role != "content" {
		t.Errorf("a photo filed by hand is hidden as %v", p.Role)
	}

	// The owner names them, and adds one by name.
	renamed, err := e.library.RenameSubject(e.ctx, connect.NewRequest(&pidexv1.RenameSubjectRequest{SubjectId: ginger.String(), DisplayName: "Mimi"}))
	if err != nil || renamed.Msg.Subject.DisplayName != "Mimi" || renamed.Msg.Subject.PhotoCount != 8 {
		t.Fatalf("rename: %v %v", renamed, err)
	}
	// Filed photos are described by the name it has now; one of something
	// else filed with it is not.
	if d := describedAs(g1); d != "Mimi on a sofa." {
		t.Errorf("a photo of Mimi is described as %q", d)
	}
	if d := describedAs(bowl); d != "A food bowl, half full." {
		t.Errorf("the bowl is described as %q", d)
	}
	// A photo described before descriptions named their subject is
	// described again, by name.
	if _, err := e.pool.Exec(e.ctx, `UPDATE photos SET extracted = extracted - 'named_description' WHERE id = $1`, g2); err != nil {
		t.Fatal(err)
	}
	if d := describedAs(g2); d != "A ginger cat on a sofa." {
		t.Fatalf("without a named description: %q", d)
	}
	if err := e.proc.NameDescription(e.ctx, g2); err != nil {
		t.Fatal(err)
	}
	if d := describedAs(g2); d != "Mimi on a sofa." {
		t.Errorf("described again: %q", d)
	}
	if _, err := e.library.CreateSubject(e.ctx, connect.NewRequest(&pidexv1.CreateSubjectRequest{TypeKey: "pet"})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a pet needs a name: %v", err)
	}
	dup, err := e.library.CreateSubject(e.ctx, connect.NewRequest(&pidexv1.CreateSubjectRequest{TypeKey: "pet", DisplayName: "Mimi again"}))
	if err != nil {
		t.Fatal(err)
	}

	// Filed twice by mistake: merging moves the photos back together.
	if _, err := e.library.AssignPhotos(e.ctx, connect.NewRequest(&pidexv1.AssignPhotosRequest{PhotoIds: []string{g2.String()}, SubjectId: dup.Msg.Subject.Id})); err != nil {
		t.Fatal(err)
	}
	merged, err := e.library.MergeSubjects(e.ctx, connect.NewRequest(&pidexv1.MergeSubjectsRequest{SubjectIds: []string{dup.Msg.Subject.Id}, IntoId: ginger.String()}))
	if err != nil {
		t.Fatal(err)
	}
	if merged.Msg.Subject.PhotoCount != 8 {
		t.Errorf("merged subject has %d photos, want 8", merged.Msg.Subject.PhotoCount)
	}
	if p := e.photo(g2); p.SubjectID == nil || *p.SubjectID != ginger {
		t.Errorf("photo did not move back to Mimi: %v", p.SubjectID)
	}
	after, err := e.library.ListSubjects(e.ctx, connect.NewRequest(&pidexv1.ListSubjectsRequest{TypeKey: "pet"}))
	if err != nil || len(after.Msg.Subjects) != 3 {
		t.Errorf("merged subjects are no longer listed: %v %v", after, err)
	}
	if _, err := e.library.MergeSubjects(e.ctx, connect.NewRequest(&pidexv1.MergeSubjectsRequest{SubjectIds: []string{tuxedo.String()}, IntoId: dup.Msg.Subject.Id})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("merging into a merged subject: %v", err)
	}
}
