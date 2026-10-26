package e2e

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/internal/looks"
	"pidex/internal/pipeline"
)

// fakeLooks stands in for the look service: each scene has its own
// direction, so photos of one subject look identical and others nothing
// alike, and a far-away shot ("_far") looks only somewhat like its subject
// (0.8, below what files by look alone).
func fakeLooks(t *testing.T) *httptest.Server {
	t.Helper()
	var names []string
	for name := range scenes {
		if !strings.HasSuffix(name, "_far") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	dims := len(names) + 1
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		scene, err := sceneOf(data)
		if err != nil {
			http.Error(w, "not an image", http.StatusBadRequest)
			return
		}
		v := make([]float32, dims)
		base := strings.TrimSuffix(scene, "_far")
		for i, n := range names {
			if n == base {
				v[i] = 1
			}
		}
		if base != scene {
			for i := range v {
				v[i] *= 0.8
			}
			v[dims-1] = float32(math.Sqrt(1 - 0.64))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"model": "fake@1", "dims": dims, "vector": v})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestFilingByLook: with a look service, a photo that surely looks like a
// subject's photos is filed without asking the model; one that only
// somewhat does goes to the model, with that subject among the candidates;
// photos filed by hand are compared with too. Without the service, filing
// goes on as before.
func TestFilingByLook(t *testing.T) {
	e := setup(t, "household_pets")
	e.proc.Looks = looks.New(fakeLooks(t).URL)
	min := func(m float64) time.Duration { return time.Duration(m * float64(time.Minute)) }
	appearance := func(id uuid.UUID) *pipeline.AppearanceMatch {
		t.Helper()
		var ex pipeline.Extracted
		if err := json.Unmarshal(e.photo(id).Extracted, &ex); err != nil {
			t.Fatal(err)
		}
		return ex.Appearance
	}

	g1 := e.take(shot{scene: "cat_ginger", session: "s1", at: 0})
	ginger := *e.photo(g1).SubjectID
	before := e.compares.Load()
	g2 := e.take(shot{scene: "cat_ginger", session: "s2", at: min(90)})
	e.expectFiled("the same cat, by look", g2, &ginger, "auto")
	if e.compares.Load() != before {
		t.Error("a photo that surely looks like a subject's was sent to the model")
	}
	if a := appearance(g2); a == nil || !a.ByLook || a.Look < 0.99 {
		t.Errorf("not recorded as filed by look: %+v", a)
	}

	// Only somewhat alike: the model decides, as before.
	far := e.take(shot{scene: "cat_ginger_far", session: "s3", at: min(200)})
	if e.compares.Load() != before+1 {
		t.Error("a photo only somewhat alike was not compared by the model")
	}
	if p := e.photo(far); p.SuggestedSubjectID == nil || *p.SuggestedSubjectID != ginger {
		t.Errorf("far shot: %s %v", p.Assignment, p.SuggestedSubjectID)
	}
	if a := appearance(far); a.ByLook || math.Abs(a.Look-0.8) > 0.01 {
		t.Errorf("far shot recorded as %+v", a)
	}

	// Nothing alike: the model says it is new.
	t1 := e.take(shot{scene: "cat_tuxedo", session: "s4", at: min(300)})
	tuxedo := *e.photo(t1).SubjectID
	if tuxedo == ginger {
		t.Fatal("the tuxedo cat was filed as the ginger one")
	}

	// A photo filed by hand on the camera is one more to compare with.
	picked := e.take(shot{scene: "cat_calico", session: "s5", at: min(400), chosen: tuxedo})
	var model *string
	if err := e.pool.QueryRow(e.ctx, `SELECT look_model FROM photos WHERE id = $1`, picked).Scan(&model); err != nil || model == nil || *model != "fake@1" {
		t.Errorf("a photo filed by hand has no look: %v %v", model, err)
	}

	// The camera, too, knows a cat by its look without the model.
	before = e.compares.Load()
	known, err := e.capture.IdentifyShot(e.ctx, connect.NewRequest(&pidexv1.IdentifyShotRequest{Jpeg: jpegOf(scenes["cat_ginger"].c)}))
	if err != nil || known.Msg.Anchor.State != pidexv1.AnchorState_ANCHOR_STATE_MATCHED || known.Msg.Anchor.Subject.GetId() != ginger.String() {
		t.Errorf("the camera by look: %v %v", known, err)
	}
	if e.compares.Load() != before {
		t.Error("the camera sent a sure look to the model")
	}

	// Photos filed before the service ran get their look later.
	if _, err := e.pool.Exec(e.ctx, `UPDATE photos SET look_embedding = NULL, look_model = NULL WHERE id = $1`, g1); err != nil {
		t.Fatal(err)
	}
	if err := e.proc.EmbedLook(e.ctx, g1); err != nil {
		t.Fatal(err)
	}
	if err := e.pool.QueryRow(e.ctx, `SELECT look_model FROM photos WHERE id = $1`, g1).Scan(&model); err != nil || model == nil {
		t.Errorf("not embedded later: %v", err)
	}

	// The service down: filing goes on without it.
	e.proc.Looks = looks.New("http://127.0.0.1:1")
	before = e.compares.Load()
	g3 := e.take(shot{scene: "cat_ginger", session: "s6", at: min(500)})
	e.expectFiled("the ginger cat without a look service", g3, &ginger, "auto")
	if e.compares.Load() != before+1 {
		t.Error("without a look service the model did not compare")
	}
}
