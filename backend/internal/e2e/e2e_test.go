// Package e2e drives the real API, pipeline, Postgres and an S3-compatible
// store end to end. Only the vision model is scripted: each test image is a
// solid color that stands for a scene, and the script answers for it the
// way a model would.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/gen/pidex/v1/pidexv1connect"
	"pidex/internal/ai"
	"pidex/internal/api"
	"pidex/internal/configstore"
	"pidex/internal/pipeline"
	"pidex/internal/search"
	"pidex/internal/storage"
	"pidex/internal/store"
	"pidex/internal/store/dbq"
	"pidex/internal/teamconfig"
)

// scene is what a test image depicts and what the scripted model says.
type scene struct {
	c      color.RGBA
	answer string
}

var scenes = map[string]scene{
	"wristband_smith": {color.RGBA{220, 30, 30, 255}, `{"role":"identity","role_confidence":0.97,"subject_type":"patient",
		"identity":[{"field":"patient.name","value":"SMITH, JOHN","confidence":0.95},{"field":"patient.dob","value":"03/02/1985","confidence":0.93},{"field":"patient.mrn","value":"12345678","confidence":0.96}],
		"document_fields":[],"description":"A hospital wristband.","tags":[]}`},
	"wristband_doe": {color.RGBA{30, 30, 220, 255}, `{"role":"identity","role_confidence":0.96,"subject_type":"patient",
		"identity":[{"field":"patient.name","value":"DOE, JANE","confidence":0.94},{"field":"patient.dob","value":"07/14/1990","confidence":0.92},{"field":"patient.mrn","value":"87654321","confidence":0.95}],
		"document_fields":[],"description":"A hospital wristband.","tags":[]}`},
	"wristband_smudged": {color.RGBA{240, 240, 240, 255}, `{"role":"identity","role_confidence":0.8,"subject_type":"patient",
		"identity":[{"field":"patient.name","value":"Bob Lee","confidence":0.6}],
		"document_fields":[],"description":"A partly legible wristband.","tags":[]}`},
	"knee": {color.RGBA{30, 200, 30, 255}, `{"role":"content","role_confidence":0.95,"subject_type":"none","identity":[],"document_fields":[],
		"description":"A swollen right knee.","tags":[{"key":"knee","confidence":0.92},{"key":"swelling","confidence":0.81},{"key":"side_right","confidence":0.7},{"key":"bruising","confidence":0.3}]}`},
	"ankle": {color.RGBA{230, 220, 30, 255}, `{"role":"content","role_confidence":0.95,"subject_type":"none","identity":[],"document_fields":[],
		"description":"An ankle with an abrasion.","tags":[{"key":"ankle","confidence":0.9},{"key":"abrasion","confidence":0.75}]}`},
	"blurry": {color.RGBA{10, 10, 10, 255}, `{"role":"junk","role_confidence":0.9,"subject_type":"none","identity":[],"document_fields":[],"description":"A dark, blurred frame.","tags":[]}`},

	"site_sign": {color.RGBA{240, 140, 20, 255}, `{"role":"identity","role_confidence":0.9,"subject_type":"project",
		"identity":[{"field":"project.project_name","value":"Maple Court","confidence":0.9}],"document_fields":[],"description":"A site signboard.","tags":[]}`},
	"unit_door": {color.RGBA{20, 220, 220, 255}, `{"role":"identity","role_confidence":0.9,"subject_type":"unit",
		"identity":[{"field":"unit.building","value":"A","confidence":0.9},{"field":"unit.unit_number","value":"12B","confidence":0.92}],"document_fields":[],"description":"A door number plate.","tags":[]}`},
	"tiling": {color.RGBA{128, 128, 128, 255}, `{"role":"content","role_confidence":0.9,"subject_type":"none","identity":[],"document_fields":[],
		"description":"Bathroom wall tiling in progress.","tags":[{"key":"tiling","confidence":0.9},{"key":"in_progress","confidence":0.8}]}`},

	"cat_ginger": {color.RGBA{250, 160, 60, 255}, `{"role":"content","role_confidence":0.95,"subject_type":"pet","identity":[],"document_fields":[],
		"appearance":{"label":"Ginger cat","features":"orange tabby coat, white chest, amber eyes","named_description":"{subject} on a sofa."},
		"description":"A ginger cat on a sofa.","tags":[{"key":"indoors","confidence":0.9}]}`},
	"cat_tuxedo": {color.RGBA{60, 60, 70, 255}, `{"role":"content","role_confidence":0.95,"subject_type":"pet","identity":[],"document_fields":[],
		"appearance":{"label":"Tuxedo cat","features":"black coat, white bib and white paws"},
		"description":"A black and white cat asleep.","tags":[{"key":"sleeping","confidence":0.9}]}`},
	// The same ginger cat far away: recognizable, but not surely.
	"cat_ginger_far": {color.RGBA{200, 120, 40, 255}, `{"role":"content","role_confidence":0.9,"subject_type":"pet","identity":[],"document_fields":[],
		"appearance":{"label":"Ginger cat","features":"orange tabby coat, small in the frame","named_description":"{subject} across the room."},
		"description":"A ginger cat across the room.","tags":[{"key":"indoors","confidence":0.8}]}`},
	"cat_calico": {color.RGBA{240, 240, 200, 255}, `{"role":"content","role_confidence":0.95,"subject_type":"pet","identity":[],"document_fields":[],
		"appearance":{"label":"Calico cat","features":"white coat with orange and black patches"},
		"description":"A calico cat by the window.","tags":[]}`},
	"food_bowl": {color.RGBA{180, 40, 200, 255}, `{"role":"content","role_confidence":0.9,"subject_type":"none","identity":[],"document_fields":[],
		"appearance":{"label":"","features":""},"description":"A food bowl, half full.","tags":[{"key":"eating","confidence":0.7}]}`},
}

// refImages maps each candidate letter in a comparison prompt to its image
// numbers ("A: ... Images 2 to 3.").
var refImages = regexp.MustCompile(`(?m)^([A-Z]): .*?Images? (\d+)(?: to (\d+))?\.`)

// compareScenes answers an appearance comparison the way a good model
// would: the known one whose photos show the same individual, else new.
// A far-away shot ("_far") is matched less surely.
func compareScenes(req ai.StructuredRequest) (json.RawMessage, error) {
	shot, err := sceneOf(req.Images[0].Data)
	if err != nil {
		return nil, err
	}
	want, conf := strings.TrimSuffix(shot, "_far"), "0.98"
	if want != shot {
		conf = "0.7"
	}
	for _, m := range refImages.FindAllStringSubmatch(req.Prompt, -1) {
		first, _ := strconv.Atoi(m[2])
		last := first
		if m[3] != "" {
			last, _ = strconv.Atoi(m[3])
		}
		for i := first; i <= last && i <= len(req.Images); i++ {
			if got, _ := sceneOf(req.Images[i-1].Data); strings.TrimSuffix(got, "_far") == want {
				return json.RawMessage(`{"reason":"same coat","match":"` + m[1] + `","confidence":` + conf + `}`), nil
			}
		}
	}
	return json.RawMessage(`{"reason":"none look alike","match":"new","confidence":0.93}`), nil
}

func jpegOf(c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 48, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95})
	return buf.Bytes()
}

func sceneOf(data []byte) (string, error) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	r, g, b, _ := img.At(img.Bounds().Dx()/2, img.Bounds().Dy()/2).RGBA()
	best, bestDist := "", 1<<62
	for name, s := range scenes {
		dr, dg, db := int(r>>8)-int(s.c.R), int(g>>8)-int(s.c.G), int(b>>8)-int(s.c.B)
		if d := dr*dr + dg*dg + db*db; d < bestDist {
			best, bestDist = name, d
		}
	}
	return best, nil
}

type recordingQueue struct {
	mu      sync.Mutex
	ids     []uuid.UUID
	indexed []uuid.UUID
}

func (r *recordingQueue) EnqueueProcessPhoto(_ context.Context, _ pgx.Tx, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids = append(r.ids, id)
	return nil
}

func (r *recordingQueue) EnqueueIndexPhoto(_ context.Context, _ pgx.Tx, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.indexed = append(r.indexed, id)
	return nil
}

type env struct {
	t     *testing.T
	ctx   context.Context
	pool  *pgxpool.Pool
	q     *dbq.Queries
	proc  *pipeline.Processor
	queue *recordingQueue
	calls *atomic.Int32
	// compares counts appearance comparisons.
	compares *atomic.Int32
	server   *httptest.Server
	capture  pidexv1connect.CaptureServiceClient
	library  pidexv1connect.LibraryServiceClient
	teamID   uuid.UUID
	sessions map[string]uuid.UUID
	userID   uuid.UUID
	token    string
	// configAnswers are the scripted config model's answers, in order.
	configAnswers *answerQueue
	searchAnswers *answerQueue
	askRequests   *requestLog
	indexer       *search.Indexer
	objects       *storage.S3
}

type requestLog struct {
	mu   sync.Mutex
	reqs []ai.StructuredRequest
}

func (l *requestLog) add(r ai.StructuredRequest) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.reqs = append(l.reqs, r)
}

type answerQueue struct {
	mu      sync.Mutex
	answers []string
	prompts []string
}

func (a *answerQueue) push(answers ...string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.answers = append(a.answers, answers...)
}

func (a *answerQueue) next(prompt string) (json.RawMessage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.prompts = append(a.prompts, prompt)
	if len(a.answers) == 0 {
		return nil, fmt.Errorf("config model: no scripted answer")
	}
	out := a.answers[0]
	a.answers = a.answers[1:]
	return json.RawMessage(out), nil
}

// checkFound answers a check of search results as a careful model would:
// a photo matches when its words mention what the request was understood
// as (any word of four letters or more).
func checkFound(prompt string) json.RawMessage {
	var asked []string
	var matching []int
	for _, line := range strings.Split(prompt, "\n") {
		if rest, ok := strings.CutPrefix(line, "Understood as: "); ok {
			for _, w := range strings.Fields(strings.ToLower(strings.ReplaceAll(rest, ";", " "))) {
				if len(w) >= 4 {
					asked = append(asked, w)
				}
			}
			continue
		}
		n, desc, ok := strings.Cut(line, ". ")
		num, err := strconv.Atoi(n)
		if !ok || err != nil {
			continue
		}
		for _, a := range asked {
			if strings.Contains(strings.ToLower(desc), a) {
				matching = append(matching, num)
				break
			}
		}
	}
	out, _ := json.Marshal(map[string]any{"matching": matching})
	return out
}

// withToken adds the bearer token to every call of a client.
func withToken(token string) connect.ClientOption {
	return connect.WithInterceptors(connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			req.Header().Set("Authorization", "Bearer "+token)
			return next(ctx, req)
		}
	}))
}

// newDatabase creates a throwaway database on the development Postgres.
func newDatabase(t *testing.T, ctx context.Context) string {
	t.Helper()
	admin := os.Getenv("PIDEX_TEST_DATABASE_URL")
	if admin == "" {
		admin = "postgres://pidex:pidex@localhost:55432/pidex?sslmode=disable"
	}
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Skipf("no test database (start it with `docker compose up -d`): %v", err)
	}
	name := fmt.Sprintf("pidex_e2e_%d", rand.Uint32())
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)")
		conn.Close(context.Background())
	})
	cfg, _ := pgx.ParseConfig(admin)
	return strings.Replace(admin, "/"+cfg.Database+"?", "/"+name+"?", 1)
}

func setup(t *testing.T, template string) *env {
	ctx := context.Background()
	dbURL := newDatabase(t, ctx)
	if err := store.MigrateUp(ctx, dbURL, io.Discard); err != nil {
		t.Fatal(err)
	}
	pool, err := store.OpenPool(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	fake, err := storage.StartDevS3("127.0.0.1:0", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fake.Close() })
	objects, err := storage.NewS3(ctx, storage.Config{Endpoint: fake.URL, Region: "us-east-1", Bucket: "e2e", AccessKey: "k", SecretKey: "s", PathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := objects.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}

	q := dbq.New(pool)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	calls := &atomic.Int32{}
	compares := &atomic.Int32{}
	askRequests := &requestLog{}
	model := &ai.Scripted{Name: "script", Fn: func(_ context.Context, req ai.StructuredRequest) (json.RawMessage, error) {
		calls.Add(1)
		if strings.HasPrefix(req.Instructions, "You answer a question") {
			askRequests.add(req)
			return json.RawMessage(`{"answer":"Unit 203 has waterproofing photographed [1]."}`), nil
		}
		if strings.HasPrefix(req.Instructions, "You tell individual things apart") {
			compares.Add(1)
			return compareScenes(req)
		}
		if strings.HasPrefix(req.Instructions, "You describe one individual") {
			// A profile: what no single photo's description says.
			name, _ := sceneOf(req.Images[0].Data)
			facts := map[string]string{
				"cat_ginger": `["orange tabby coat","freckled pink nose","amber eyes"]`,
				"cat_tuxedo": `["coat dark with a white bib","white paws","green eyes"]`,
			}[strings.TrimSuffix(name, "_far")]
			if facts == "" {
				facts = "[]"
			}
			return json.RawMessage(`{"facts":` + facts + `}`), nil
		}
		name, err := sceneOf(req.Images[0].Data)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(scenes[name].answer), nil
	}}
	rec := &ai.DBRecorder{Q: q, Log: log}
	configs := configstore.New(q)
	proc := &pipeline.Processor{
		Pool: pool, Configs: configs, Storage: objects, Vision: ai.NewRouter(rec, log, model),
		TextEmbedder: wordEmbedder{}, Recorder: rec, Log: log, MaxImageEdge: 1280, MaxObjectBytes: 30 << 20,
	}
	queue := &recordingQueue{}
	configAnswers := &answerQueue{}
	configModel := &ai.Scripted{Name: "config-script", Fn: func(_ context.Context, req ai.StructuredRequest) (json.RawMessage, error) {
		return configAnswers.next(req.Prompt)
	}}
	searchAnswers := &answerQueue{}
	searchModel := &ai.Scripted{Name: "search-script", Fn: func(_ context.Context, req ai.StructuredRequest) (json.RawMessage, error) {
		if _, ok := req.Schema["properties"].(map[string]any)["matching"]; ok {
			return checkFound(req.Prompt), nil
		}
		return searchAnswers.next(req.Prompt)
	}}
	embedder := wordEmbedder{}
	indexer := &search.Indexer{Pool: pool, Configs: configs, Storage: objects, Embedder: embedder, Recorder: rec, Log: log, MaxObjectBytes: 30 << 20}
	srv := &api.Server{
		Pool: pool, Q: q, Configs: configs, Storage: objects, Jobs: queue, Processor: proc, Log: log,
		ConfigModel: ai.NewRouter(rec, log, configModel), SearchModel: ai.NewRouter(rec, log, searchModel),
		// Cutoffs belong to the embedding model; the word-count stand-in
		// scores lower than a real model, so it gets its own.
		Search: &search.Searcher{Pool: pool, Embedder: embedder, Recorder: rec,
			Cutoffs: search.Cutoffs{Max: 0.8, Relative: 0.3, Missing: 0.5}},
		MaxUploadBytes: 30 << 20, UploadURLTTL: 15 * time.Minute,
	}
	server := httptest.NewServer(srv.Handler())
	t.Cleanup(server.Close)

	team, err := q.CreateTeam(ctx, dbq.CreateTeamParams{Name: "E2E " + template, DataRegion: "us"})
	if err != nil {
		t.Fatal(err)
	}
	user, err := q.UpsertUser(ctx, dbq.UpsertUserParams{Email: template + "@example.test", DisplayName: "Tester"})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.AddTeamMember(ctx, dbq.AddTeamMemberParams{TeamID: team.ID, UserID: user.ID, Role: "owner"}); err != nil {
		t.Fatal(err)
	}
	token, hash := api.NewToken()
	if err := q.CreateAPIToken(ctx, dbq.CreateAPITokenParams{TokenSha256: hash, TeamID: team.ID, UserID: user.ID}); err != nil {
		t.Fatal(err)
	}
	tcfg, err := teamconfig.LoadTemplate(template)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := configstore.Publish(ctx, pool, team.ID, &user.ID, tcfg, "test"); err != nil {
		t.Fatal(err)
	}

	auth := withToken(token)
	return &env{
		t: t, ctx: ctx, pool: pool, q: q, proc: proc, queue: queue, calls: calls, compares: compares, server: server, teamID: team.ID,
		capture:  pidexv1connect.NewCaptureServiceClient(server.Client(), server.URL, auth),
		library:  pidexv1connect.NewLibraryServiceClient(server.Client(), server.URL, auth),
		sessions: map[string]uuid.UUID{},
		userID:   user.ID, token: token, configAnswers: configAnswers,
		searchAnswers: searchAnswers, askRequests: askRequests, indexer: indexer, objects: objects,
	}
}

type shot struct {
	scene   string
	session string
	at      time.Duration
	device  *pidexv1.OnDeviceAnalysis
	// chosen is a subject picked by hand on the camera.
	chosen uuid.UUID
	// clientID is the id the phone gave the shot; a new one if empty.
	clientID string
}

// take goes through the same steps as the phone: request an upload URL, PUT
// the bytes, complete with on-device results, then lets the worker run.
func (e *env) take(s shot) uuid.UUID {
	e.t.Helper()
	data := jpegOf(scenes[s.scene].c)
	clientID := s.clientID
	if clientID == "" {
		clientID = uuid.NewString()
	}
	up, err := e.capture.CreateUpload(e.ctx, connect.NewRequest(&pidexv1.CreateUploadRequest{
		ClientPhotoId: clientID, ContentType: "image/jpeg", SizeBytes: int64(len(data)),
	}))
	if err != nil {
		e.t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPut, up.Msg.UploadUrl, bytes.NewReader(data))
	for k, v := range up.Msg.UploadHeaders {
		if k != "Content-Length" {
			req.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("upload: %s", resp.Status)
	}
	sid, ok := e.sessions[s.session]
	if !ok {
		sid = uuid.New()
		e.sessions[s.session] = sid
	}
	base := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	// Each test shot says it identifies itself; the camera's grouping of
	// shots is the app's, and the filing is the pipeline's.
	complete := &pidexv1.CompleteUploadRequest{
		PhotoId: up.Msg.PhotoId, SessionId: sid.String(), CapturedAt: timestamppb.New(base.Add(s.at)), OnDevice: s.device,
		IdentityClientPhotoId: clientID,
	}
	if s.chosen != uuid.Nil {
		complete.LockedSubjectId, complete.SubjectChosen = s.chosen.String(), true
	}
	done, err := e.capture.CompleteUpload(e.ctx, connect.NewRequest(complete))
	if err != nil {
		e.t.Fatal(err)
	}
	id := uuid.MustParse(done.Msg.Photo.Id)
	if err := e.proc.ProcessPhoto(e.ctx, id); err != nil {
		e.t.Fatalf("process %s: %v", s.scene, err)
	}
	return id
}

func (e *env) photo(id uuid.UUID) dbq.Photo {
	e.t.Helper()
	p, err := e.q.GetPhoto(e.ctx, dbq.GetPhotoParams{TeamID: e.teamID, ID: id})
	if err != nil {
		e.t.Fatal(err)
	}
	return p
}

func (e *env) expectFiled(label string, id uuid.UUID, subject *uuid.UUID, assignment string) {
	e.t.Helper()
	p := e.photo(id)
	if p.Assignment != assignment || (subject == nil) != (p.SubjectID == nil) || (subject != nil && *subject != *p.SubjectID) {
		e.t.Errorf("%s: assignment=%s subject=%v conf=%.3f; want %s %v", label, p.Assignment, p.SubjectID, p.AssignmentConfidence, assignment, subject)
	}
}

func text(lines ...string) []*pidexv1.TextObservation {
	out := make([]*pidexv1.TextObservation, len(lines))
	for i, l := range lines {
		out[i] = &pidexv1.TextObservation{Text: l, Confidence: 0.9}
	}
	return out
}

func TestWardRound(t *testing.T) {
	e := setup(t, "hospital_orthopedics")
	min := func(m float64) time.Duration { return time.Duration(m * float64(time.Minute)) }

	// Session 1: two known-to-be-new patients, a duplicate burst, a blurry
	// frame and a wristband nobody can read.
	p1 := e.take(shot{scene: "wristband_smith", session: "s1", at: 0, device: &pidexv1.OnDeviceAnalysis{
		Barcodes: []*pidexv1.BarcodeObservation{{Symbology: "code128", Payload: "12345678"}},
		Text:     text("SMITH, JOHN", "DOB 03/02/1985", "MRN 12345678"),
	}})
	p2 := e.take(shot{scene: "knee", session: "s1", at: min(1), device: &pidexv1.OnDeviceAnalysis{PerceptualHash: "f0f0f0f0f0f0f0f0"}})
	p3 := e.take(shot{scene: "knee", session: "s1", at: min(1) + 5*time.Second, device: &pidexv1.OnDeviceAnalysis{PerceptualHash: "f0f0f0f0f0f0f0f1"}})
	p4 := e.take(shot{scene: "blurry", session: "s1", at: min(2)})
	p5 := e.take(shot{scene: "wristband_doe", session: "s1", at: min(4), device: &pidexv1.OnDeviceAnalysis{
		Text: text("DOE, JANE", "07/14/1990", "MRN 87654321"),
	}})
	p6 := e.take(shot{scene: "ankle", session: "s1", at: min(5)})
	p7 := e.take(shot{scene: "wristband_smudged", session: "s1", at: min(8)})
	p8 := e.take(shot{scene: "knee", session: "s1", at: min(9)})

	subjects, err := e.library.ListSubjects(e.ctx, connect.NewRequest(&pidexv1.ListSubjectsRequest{TypeKey: "patient"}))
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]uuid.UUID{}
	for _, s := range subjects.Msg.Subjects {
		byName[s.DisplayName] = uuid.MustParse(s.Id)
	}
	smith, okA := byName["SMITH, JOHN · 03/02/1985"]
	doe, okB := byName["DOE, JANE · 07/14/1990"]
	if !okA || !okB || len(byName) != 2 {
		t.Fatalf("want exactly Smith and Doe created automatically, got %v", byName)
	}

	e.expectFiled("wristband (barcode)", p1, &smith, "auto")
	e.expectFiled("knee after wristband", p2, &smith, "auto")
	e.expectFiled("near-duplicate knee", p3, &smith, "auto")
	e.expectFiled("blurry frame", p4, nil, "unassigned")
	e.expectFiled("wristband (OCR-confirmed)", p5, &doe, "auto")
	e.expectFiled("ankle after second wristband", p6, &doe, "auto")
	e.expectFiled("unreadable wristband", p7, nil, "unassigned")
	e.expectFiled("knee after unreadable wristband", p8, nil, "unassigned")

	if ex := pipeline.DecodeExtracted(e.photo(p3).Extracted); ex.Source != pipeline.SourceReused || *ex.ReusedFrom != p2 {
		t.Errorf("duplicate should reuse p2's analysis, got source=%s", ex.Source)
	}
	if ex := pipeline.DecodeExtracted(e.photo(p5).Extracted); !ex.IdentityRead.ConfirmedByOCR {
		t.Error("Doe's wristband should be confirmed by on-device OCR")
	}
	// p3 reused p2's answer: 7 calls for 8 photos.
	if got := e.calls.Load(); got != 7 {
		t.Errorf("model calls = %d, want 7", got)
	}
	var recorded int
	if err := e.pool.QueryRow(e.ctx, "SELECT count(*) FROM ai_calls WHERE team_id = $1 AND ok", e.teamID).Scan(&recorded); err != nil || recorded != 7 {
		t.Errorf("ai_calls recorded = %d (%v), want 7", recorded, err)
	}
	if len(e.queue.ids) != 8 {
		t.Errorf("enqueued %d jobs, want 8", len(e.queue.ids))
	}

	summary, err := e.capture.GetSessionSummary(e.ctx, connect.NewRequest(&pidexv1.GetSessionSummaryRequest{SessionId: e.sessions["s1"].String()}))
	if err != nil {
		t.Fatal(err)
	}
	sm := summary.Msg
	if sm.TotalCount != 8 || sm.JunkCount != 1 || sm.ProcessingCount != 0 || len(sm.Groups) != 2 || len(sm.NeedsReview) != 2 {
		t.Fatalf("summary: total=%d junk=%d processing=%d groups=%d review=%d", sm.TotalCount, sm.JunkCount, sm.ProcessingCount, len(sm.Groups), len(sm.NeedsReview))
	}
	if sm.Groups[0].PhotoCount != 3 || sm.Groups[1].PhotoCount != 2 {
		t.Errorf("group sizes %d, %d", sm.Groups[0].PhotoCount, sm.Groups[1].PhotoCount)
	}
	if sm.NeedsReview[0].ViewUrl == "" {
		t.Error("review photos need a view URL")
	}

	// The nurse fixes the unreadable wristband once; the photo after it follows.
	lee, err := e.library.CreateSubject(e.ctx, connect.NewRequest(&pidexv1.CreateSubjectRequest{
		TypeKey: "patient", Identity: map[string]string{"name": "Bob Lee", "dob": "01/01/1970", "mrn": "55556666"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	leeID := uuid.MustParse(lee.Msg.Subject.Id)
	if _, err := e.library.AssignPhotos(e.ctx, connect.NewRequest(&pidexv1.AssignPhotosRequest{
		PhotoIds: []string{p7.String()}, SubjectId: leeID.String(),
	})); err != nil {
		t.Fatal(err)
	}
	e.expectFiled("corrected wristband", p7, &leeID, "manual")
	e.expectFiled("knee after corrected wristband", p8, &leeID, "auto")
	var feedback int
	_ = e.pool.QueryRow(e.ctx, "SELECT count(*) FROM audit_log WHERE action = 'photo.manually_filed' AND details->>'previous_assignment' = 'unassigned'").Scan(&feedback)
	if feedback != 1 {
		t.Errorf("manual correction should be logged with what the system had decided, got %d", feedback)
	}

	// Session 2, two hours later: Smith returns, no barcode this time.
	p9 := e.take(shot{scene: "wristband_smith", session: "s2", at: 2 * time.Hour, device: &pidexv1.OnDeviceAnalysis{
		Text: text("SMITH, JOHN", "DOB 03/02/1985", "MRN 12345678"),
	}})
	p10 := e.take(shot{scene: "knee", session: "s2", at: 2*time.Hour + min(1)})
	e.expectFiled("returning patient", p9, &smith, "auto")
	e.expectFiled("returning patient's knee", p10, &smith, "auto")

	timeline, err := e.library.GetSubject(e.ctx, connect.NewRequest(&pidexv1.GetSubjectRequest{Id: smith.String()}))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(timeline.Msg.Photos); n != 5 {
		t.Errorf("Smith's timeline has %d photos, want 5", n)
	}

	// Search by tag, then a correction that must survive reprocessing.
	knees, err := e.library.ListPhotos(e.ctx, connect.NewRequest(&pidexv1.ListPhotosRequest{TagKeys: []string{"knee", "swelling"}}))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(knees.Msg.Photos); n != 4 {
		t.Errorf("knee+swelling photos = %d, want 4", n)
	}
	for _, ph := range knees.Msg.Photos {
		for _, tg := range ph.Tags {
			if tg.Key == "bruising" {
				t.Error("tags below the keep threshold must not be stored")
			}
		}
	}
	if _, err := e.library.UpdatePhotoTags(e.ctx, connect.NewRequest(&pidexv1.UpdatePhotoTagsRequest{PhotoId: p2.String(), Remove: []string{"knee"}})); err != nil {
		t.Fatal(err)
	}
	if err := e.proc.ProcessPhoto(e.ctx, p2); err != nil {
		t.Fatal(err)
	}
	knees, _ = e.library.ListPhotos(e.ctx, connect.NewRequest(&pidexv1.ListPhotosRequest{TagKeys: []string{"knee"}}))
	for _, ph := range knees.Msg.Photos {
		if ph.Id == p2.String() {
			t.Error("a tag a person removed came back after reprocessing")
		}
	}
}

func TestResolveAnchorOnDevice(t *testing.T) {
	e := setup(t, "hospital_orthopedics")
	e.take(shot{scene: "wristband_smith", session: "s", at: 0, device: &pidexv1.OnDeviceAnalysis{
		Barcodes: []*pidexv1.BarcodeObservation{{Symbology: "code128", Payload: "12345678"}},
		Text:     text("SMITH, JOHN", "DOB 03/02/1985", "MRN 12345678"),
	}})
	e.take(shot{scene: "wristband_doe", session: "s", at: time.Minute, device: &pidexv1.OnDeviceAnalysis{
		Text: text("DOE, JANE", "07/14/1990", "MRN 87654321"),
	}})
	calls := e.calls.Load()

	resolve := func(od *pidexv1.OnDeviceAnalysis) *pidexv1.ResolveAnchorResponse {
		t.Helper()
		r, err := e.capture.ResolveAnchor(e.ctx, connect.NewRequest(&pidexv1.ResolveAnchorRequest{OnDevice: od}))
		if err != nil {
			t.Fatal(err)
		}
		return r.Msg
	}
	if r := resolve(&pidexv1.OnDeviceAnalysis{Barcodes: []*pidexv1.BarcodeObservation{{Symbology: "VNBarcodeSymbologyCode128", Payload: "MRN 12345678"}}}); r.State != pidexv1.AnchorState_ANCHOR_STATE_MATCHED || !strings.HasPrefix(r.Subject.GetDisplayName(), "SMITH") {
		t.Errorf("wristband barcode: %v %v", r.State, r.Subject.GetDisplayName())
	}
	if r := resolve(&pidexv1.OnDeviceAnalysis{Barcodes: []*pidexv1.BarcodeObservation{{Symbology: "upca", Payload: "012345678905"}}}); r.State != pidexv1.AnchorState_ANCHOR_STATE_NOT_ANCHOR {
		t.Errorf("product barcode must not identify anyone: %v", r.State)
	}
	if r := resolve(&pidexv1.OnDeviceAnalysis{Text: text("PATIENT", "DOE, JANE", "DOB 07/14/1990", "MRN 87654321")}); r.State != pidexv1.AnchorState_ANCHOR_STATE_MATCHED || !strings.HasPrefix(r.Subject.GetDisplayName(), "DOE") {
		t.Errorf("OCR-only wristband: %v %v", r.State, r.Subject.GetDisplayName())
	}
	if r := resolve(&pidexv1.OnDeviceAnalysis{Barcodes: []*pidexv1.BarcodeObservation{{Symbology: "code128", Payload: "99990000"}}}); r.State != pidexv1.AnchorState_ANCHOR_STATE_NEW_SUBJECT || r.ExtractedIdentity["mrn"] != "99990000" {
		t.Errorf("unknown wristband: %v %v", r.State, r.ExtractedIdentity)
	}
	if e.calls.Load() != calls {
		t.Error("the banner path must not call a model")
	}
}

// TestSameEngineDifferentIndustry runs the construction template through
// the identical code: projects contain units, and photos land on the unit.
func TestSameEngineDifferentIndustry(t *testing.T) {
	e := setup(t, "construction_renovation")
	sign := e.take(shot{scene: "site_sign", session: "day1", at: 0})
	door := e.take(shot{scene: "unit_door", session: "day1", at: time.Minute})
	tile := e.take(shot{scene: "tiling", session: "day1", at: 2 * time.Minute})

	project := e.photo(sign).SubjectID
	unit := e.photo(door).SubjectID
	if project == nil || unit == nil {
		t.Fatalf("lenient team should auto-create project and unit: %v %v", project, unit)
	}
	u, err := e.q.GetSubject(e.ctx, dbq.GetSubjectParams{TeamID: e.teamID, ID: *unit})
	if err != nil {
		t.Fatal(err)
	}
	if u.TypeKey != "unit" || u.ParentID == nil || *u.ParentID != *project || u.DisplayName != "A · 12B" {
		t.Errorf("unit %q type=%s parent=%v, want child of project %v", u.DisplayName, u.TypeKey, u.ParentID, *project)
	}
	e.expectFiled("tiling photo", tile, unit, "auto")
}
