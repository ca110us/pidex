package e2e

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"math"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/gen/pidex/v1/pidexv1connect"
	"pidex/internal/ai"
	"pidex/internal/search"
	"pidex/internal/store/dbq"
)

// wordEmbedder stands in for an embedding model: texts sharing words (or,
// for Chinese, characters) get similar vectors. Deterministic and free.
type wordEmbedder struct{}

func (wordEmbedder) Provider() string { return "test" }
func (wordEmbedder) Model() string    { return "words" }
func (wordEmbedder) Dimensions() int  { return search.EmbeddingDimensions }

func (wordEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, ai.Usage, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v := make([]float32, search.EmbeddingDimensions)
		for _, w := range strings.FieldsFunc(strings.ToLower(t), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
			if len(w) < 3 && !unicode.Is(unicode.Han, []rune(w)[0]) {
				continue
			}
			h := fnv.New32a()
			h.Write([]byte(strings.TrimSuffix(w, "s")))
			v[h.Sum32()%search.EmbeddingDimensions]++
		}
		var n float64
		for _, x := range v {
			n += float64(x * x)
		}
		for j := range v {
			v[j] /= float32(math.Sqrt(n) + 1e-9)
		}
		out[i] = v
	}
	return out, ai.Usage{}, nil
}

type fixturePhoto struct {
	subject     *uuid.UUID
	description string
	tags        []string
	role        string
	daysAgo     int
}

// addSubject and addPhoto write processed rows directly: these tests are
// about finding photos, not filing them.
func (e *env) addSubject(typeKey, name string, identity map[string]string, parent *uuid.UUID) uuid.UUID {
	e.t.Helper()
	raw, _ := json.Marshal(identity)
	norm := map[string]string{}
	for k, v := range identity {
		norm[k] = strings.ToUpper(strings.ReplaceAll(v, " ", ""))
	}
	normRaw, _ := json.Marshal(norm)
	s, err := e.q.CreateSubject(e.ctx, dbq.CreateSubjectParams{
		TeamID: e.teamID, TypeKey: typeKey, ParentID: parent, DisplayName: name, Identity: raw, IdentityNorm: normRaw,
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return s.ID
}

func (e *env) addPhoto(p fixturePhoto) uuid.UUID {
	e.t.Helper()
	id := uuid.New()
	key := "test/" + id.String() + ".jpg"
	if err := e.objects.Put(e.ctx, key, "image/jpeg", jpegOf(scenes["wristband_doe"].c)); err != nil {
		e.t.Fatal(err)
	}
	role := p.role
	if role == "" {
		role = "content"
	}
	at := time.Now().AddDate(0, 0, -p.daysAgo)
	_, err := e.pool.Exec(e.ctx, `
		INSERT INTO photos (id, team_id, uploaded_by, client_photo_id, object_key, content_type, size_bytes, status,
			captured_at, config_version, role, description, subject_id, assignment)
		VALUES ($1, $2, $3, $4, $5, 'image/jpeg', 1, 'processed', $6, 1, $7, $8, $9, CASE WHEN $9::uuid IS NULL THEN 'unassigned' ELSE 'auto' END)`,
		id, e.teamID, e.userID, id.String(), key, at, role, p.description, p.subject)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, t := range p.tags {
		if err := e.q.UpsertAITag(e.ctx, dbq.UpsertAITagParams{PhotoID: id, TagKey: t, Confidence: 0.9}); err != nil {
			e.t.Fatal(err)
		}
	}
	if err := e.indexer.IndexPhoto(e.ctx, id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

func plan(mode, subject, subjectType string, tags []string, text string) string {
	b, _ := json.Marshal(map[string]any{
		"mode": mode, "subject_query": subject, "subject_type": subjectType, "tags": tags,
		"date_from": "", "date_to": "", "text": text,
	})
	return string(b)
}

func ids(photos []*pidexv1.Photo) []string {
	var out []string
	for _, p := range photos {
		out = append(out, p.Id)
	}
	return out
}

// TestSearchByPlainLanguage drives search on a renovation team: a project
// with two units, photos with tags and descriptions, and requests the
// scripted search model turns into filters.
func TestSearchByPlainLanguage(t *testing.T) {
	e := setup(t, "construction_renovation")
	maple := e.addSubject("project", "Maple Court", map[string]string{"project_name": "Maple Court"}, nil)
	u203 := e.addSubject("unit", "Maple Court · 203", map[string]string{"building": "Maple Court", "unit_number": "203"}, &maple)
	u305 := e.addSubject("unit", "Maple Court · 305", map[string]string{"building": "Maple Court", "unit_number": "305"}, &maple)
	other := e.addSubject("project", "Oak Street", map[string]string{"project_name": "Oak Street"}, nil)

	wp203 := e.addPhoto(fixturePhoto{subject: &u203, description: "Waterproofing membrane on the bathroom floor.", tags: []string{"waterproofing", "concealed_work"}})
	leak203 := e.addPhoto(fixturePhoto{subject: &u203, description: "Water leak under the kitchen sink.", tags: []string{"plumbing", "water_damage"}, daysAgo: 10})
	tile305 := e.addPhoto(fixturePhoto{subject: &u305, description: "Bathroom wall tiling in progress.", tags: []string{"tiling", "in_progress"}})
	oak := e.addPhoto(fixturePhoto{subject: &other, description: "Waterproofing on the balcony.", tags: []string{"waterproofing"}})
	junk := e.addPhoto(fixturePhoto{description: "Blurred floor.", role: "junk"})

	var thumbs int
	if err := e.pool.QueryRow(e.ctx, `SELECT count(*) FROM photos WHERE thumb_key IS NOT NULL AND text_embedding IS NOT NULL AND text_embedding_model = 'test/words'`).Scan(&thumbs); err != nil || thumbs != 4 {
		t.Fatalf("indexed photos with thumbnail and vector: %d (junk gets no vector) %v", thumbs, err)
	}

	// The collection card: counts without junk, newest photos for a cover,
	// tags by use.
	teams := pidexv1connect.NewTeamServiceClient(e.server.Client(), e.server.URL, withToken(e.token))
	ov, err := teams.GetOverview(e.ctx, connect.NewRequest(&pidexv1.GetOverviewRequest{RecentCount: 2}))
	if err != nil {
		t.Fatal(err)
	}
	if ov.Msg.PhotoCount != 4 || ov.Msg.SubjectCount != 4 || len(ov.Msg.Recent) != 2 || ov.Msg.Recent[0].ThumbnailUrl == "" {
		t.Errorf("overview: %d photos, %d subjects, %d recent", ov.Msg.PhotoCount, ov.Msg.SubjectCount, len(ov.Msg.Recent))
	}
	if top := ov.Msg.Tags[0]; top.Key != "waterproofing" || top.Count != 2 {
		t.Errorf("most used tag %v", top)
	}
	if slices.ContainsFunc(ov.Msg.Recent, func(p *pidexv1.Photo) bool { return p.Id == junk.String() || p.Id == leak203.String() }) {
		t.Error("the cover shows junk or an older photo")
	}

	client := pidexv1connect.NewSearchServiceClient(e.server.Client(), e.server.URL, withToken(e.token))
	searchFor := func(answer, query string) *pidexv1.SearchPhotosResponse {
		t.Helper()
		e.searchAnswers.push(answer)
		res, err := client.SearchPhotos(e.ctx, connect.NewRequest(&pidexv1.SearchPhotosRequest{Query: query, TimeZone: "America/New_York"}))
		if err != nil {
			t.Fatal(err)
		}
		return res.Msg
	}

	// A unit by its number, and a tag.
	res := searchFor(plan("photos", "unit 203", "", []string{"waterproofing"}, ""), "waterproofing in unit 203")
	if got := ids(res.Photos); !slices.Equal(got, []string{wp203.String()}) {
		t.Errorf("unit 203 waterproofing: %v", got)
	}
	if res.Filters.SubjectIds[0] != u203.String() || len(res.Subjects) != 1 || res.Subjects[0].DisplayName != "Maple Court · 203" {
		t.Errorf("subjects %v filters %v", res.Subjects, res.Filters)
	}
	if res.Photos[0].ThumbnailUrl == "" {
		t.Error("results need thumbnails")
	}
	if !strings.Contains(e.searchAnswers.prompts[0], "waterproofing in unit 203") {
		t.Error("the request goes to the search model")
	}

	// A project covers its units; junk never shows.
	res = searchFor(plan("photos", "Maple Court", "", nil, ""), "everything at Maple Court")
	if got := ids(res.Photos); len(got) != 3 || slices.Contains(got, oak.String()) || slices.Contains(got, junk.String()) {
		t.Errorf("Maple Court photos: %v", got)
	}

	// Browsing (no request) lists the latest photos, never junk, even when
	// its own words match.
	recent, err := client.SearchPhotos(e.ctx, connect.NewRequest(&pidexv1.SearchPhotosRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(recent.Msg.Photos); len(got) != 4 || got[len(got)-1] != leak203.String() || slices.Contains(got, junk.String()) {
		t.Errorf("recent photos, newest first, without junk: %v", got)
	}
	// A page at a time, with the count of all.
	page, err := client.SearchPhotos(e.ctx, connect.NewRequest(&pidexv1.SearchPhotosRequest{PageSize: 3}))
	if err != nil {
		t.Fatal(err)
	}
	rest, err := client.SearchPhotos(e.ctx, connect.NewRequest(&pidexv1.SearchPhotosRequest{PageSize: 3, Offset: 3}))
	if err != nil {
		t.Fatal(err)
	}
	if page.Msg.Total != 4 || len(page.Msg.Photos) != 3 || rest.Msg.Total != 4 || !slices.Equal(ids(rest.Msg.Photos), []string{leak203.String()}) {
		t.Errorf("pages: %d of %d, then %v of %d", len(page.Msg.Photos), page.Msg.Total, ids(rest.Msg.Photos), rest.Msg.Total)
	}
	// Picked tags narrow together, counting what another tag would leave.
	picked, err := client.SearchPhotos(e.ctx, connect.NewRequest(&pidexv1.SearchPhotosRequest{
		Filters: &pidexv1.SearchFilters{Tags: []string{"waterproofing"}, TagsRequired: true}, TagCounts: true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int32{}
	for _, c := range picked.Msg.TagCounts {
		counts[c.Key] = c.Count
	}
	if picked.Msg.Total != 2 || counts["waterproofing"] != 2 || counts["concealed_work"] != 1 || counts["tiling"] != 0 {
		t.Errorf("waterproofing: %d photos, tag counts %v", picked.Msg.Total, counts)
	}
	both, err := client.SearchPhotos(e.ctx, connect.NewRequest(&pidexv1.SearchPhotosRequest{
		Filters: &pidexv1.SearchFilters{Tags: []string{"waterproofing", "concealed_work"}, TagsRequired: true},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(both.Msg.Photos), []string{wp203.String()}) {
		t.Errorf("both tags: %v", ids(both.Msg.Photos))
	}
	res = searchFor(plan("photos", "", "", nil, "blurred floor"), "blurred floor")
	if slices.Contains(ids(res.Photos), junk.String()) {
		t.Error("junk found by its words")
	}

	// Text ranks by meaning across the team.
	res = searchFor(plan("photos", "", "", nil, "leak under the sink"), "where was the sink leaking")
	if len(res.Photos) == 0 || res.Photos[0].Id != leak203.String() {
		t.Errorf("leak first: %v", ids(res.Photos))
	}

	// Which units have no waterproofing photo?
	res = searchFor(plan("missing", "", "unit", []string{"waterproofing"}, ""), "which units haven't had waterproofing photographed")
	if len(res.Missing) != 1 || res.Missing[0].Id != u305.String() || res.Filters.Mode != pidexv1.SearchMode_SEARCH_MODE_MISSING {
		t.Errorf("missing: %v", res.Missing)
	}

	// Edited chips come back as filters: no model call.
	prompts := len(e.searchAnswers.prompts)
	week := timestamppb.New(time.Now().AddDate(0, 0, -7))
	direct, err := client.SearchPhotos(e.ctx, connect.NewRequest(&pidexv1.SearchPhotosRequest{Filters: &pidexv1.SearchFilters{
		SubjectIds: []string{maple.String()}, From: week,
	}}))
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(direct.Msg.Photos); len(got) != 2 || slices.Contains(got, leak203.String()) || len(e.searchAnswers.prompts) != prompts {
		t.Errorf("last week at Maple Court without a model call: %v", got)
	}
	if !slices.Contains(ids(direct.Msg.Photos), tile305.String()) {
		t.Error("tiling photo missing")
	}

	// Words that name no subject on file are said so, then matched as
	// content: "sink" is no unit, but a photo shows one.
	res = searchFor(plan("photos", "sink", "", nil, "leak"), "the sink leak")
	if !res.SubjectNotFound || len(res.Photos) == 0 || res.Photos[0].Id != leak203.String() {
		t.Errorf("unknown subject words matched as content: not found %v, photos %v", res.SubjectNotFound, ids(res.Photos))
	}
	res = searchFor(plan("missing", "unit 999", "unit", []string{"waterproofing"}, ""), "is unit 999 missing waterproofing")
	if !res.SubjectNotFound || len(res.Missing) != 0 {
		t.Errorf("missing mode needs a real subject: %+v", res)
	}

	// Tags from the app are checked like the model's.
	_, err = client.SearchPhotos(e.ctx, connect.NewRequest(&pidexv1.SearchPhotosRequest{Filters: &pidexv1.SearchFilters{Tags: []string{"knee"}}}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("unknown tag: %v", err)
	}

	// Questions are answered from the photos found: from their words,
	// without sending the photos again.
	e.searchAnswers.push(plan("photos", "203", "", []string{"waterproofing"}, ""))
	ask, err := client.AskPhotos(e.ctx, connect.NewRequest(&pidexv1.AskPhotosRequest{Question: "Has 203 been waterproofed?"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ask.Msg.Answer, "[1]") || len(ask.Msg.Cited) != 1 || ask.Msg.Cited[0].Id != wp203.String() {
		t.Errorf("answer %q cited %v", ask.Msg.Answer, ids(ask.Msg.Cited))
	}
	req := e.askRequests.reqs[len(e.askRequests.reqs)-1]
	if len(req.Images) != 0 || !strings.Contains(req.Prompt, "[1]") || !strings.Contains(req.Prompt, "Waterproofing membrane") {
		t.Errorf("ask request: %d images, prompt %q", len(req.Images), req.Prompt)
	}

	// What no description says is looked for in the photos themselves.
	asked := len(e.askRequests.reqs)
	e.searchAnswers.push(plan("photos", "203", "", []string{"waterproofing"}, ""))
	colour, err := client.AskPhotos(e.ctx, connect.NewRequest(&pidexv1.AskPhotosRequest{Question: "What colour is the membrane in 203?"}))
	if err != nil {
		t.Fatal(err)
	}
	if reqs := e.askRequests.reqs[asked:]; colour.Msg.Answer == "" || len(reqs) != 2 || len(reqs[0].Images) != 0 || len(reqs[1].Images) != 1 {
		t.Errorf("a question the words cannot answer: answer %q after %d calls", colour.Msg.Answer, len(reqs))
	}

	// A question whose words match nothing is not answered from whatever
	// photos there are: there is nothing to answer from.
	asked = len(e.askRequests.reqs)
	e.searchAnswers.push(plan("photos", "", "", nil, "a black cat"))
	cat, err := client.AskPhotos(e.ctx, connect.NewRequest(&pidexv1.AskPhotosRequest{Question: "Do we have a black cat?"}))
	if err != nil {
		t.Fatal(err)
	}
	if cat.Msg.Answer != "" || len(cat.Msg.Cited) != 0 || len(e.askRequests.reqs) != asked {
		t.Errorf("a question about nothing on file: answer %q from %d photos", cat.Msg.Answer, len(cat.Msg.Cited))
	}

	// A question about the whole collection is planned with no words, and
	// answered from the latest photos and the register of everything on
	// file.
	e.searchAnswers.push(plan("photos", "", "", nil, ""))
	broad, err := client.AskPhotos(e.ctx, connect.NewRequest(&pidexv1.AskPhotosRequest{Question: "What do we have on file?"}))
	if err != nil {
		t.Fatal(err)
	}
	req = e.askRequests.reqs[len(e.askRequests.reqs)-1]
	if broad.Msg.Answer == "" || len(broad.Msg.Cited) == 0 {
		t.Errorf("broad question: answer %q from %d photos", broad.Msg.Answer, len(broad.Msg.Cited))
	}
	for _, want := range []string{"Subject register:", "Unit (2 in total):", "Maple Court · 305: 1 photos; tags on its photos: In progress ×1, Tiling ×1",
		"Project (2 in total):", "Oak Street: 1 photos; tags on its photos: Waterproofing ×1"} {
		if !strings.Contains(req.Prompt, want) {
			t.Errorf("the register misses %q:\n%s", want, req.Prompt)
		}
	}

	// A question planned as "missing" for a subject that is not on file is
	// answered from photos instead.
	e.searchAnswers.push(plan("missing", "kitchen sink", "unit", []string{"plumbing"}, "leak"))
	sink, err := client.AskPhotos(e.ctx, connect.NewRequest(&pidexv1.AskPhotosRequest{Question: "Does the kitchen sink leak?"}))
	if err != nil {
		t.Fatal(err)
	}
	if sink.Msg.Answer == "" || len(sink.Msg.Cited) == 0 || sink.Msg.Filters.Mode != pidexv1.SearchMode_SEARCH_MODE_PHOTOS {
		t.Errorf("yes/no question planned as missing: answer %q, %d photos, mode %v", sink.Msg.Answer, len(sink.Msg.Cited), sink.Msg.Filters.Mode)
	}

	// A tag edit queues the photo for indexing again.
	lib := pidexv1connect.NewLibraryServiceClient(e.server.Client(), e.server.URL, withToken(e.token))
	if _, err := lib.UpdatePhotoTags(e.ctx, connect.NewRequest(&pidexv1.UpdatePhotoTagsRequest{PhotoId: tile305.String(), Add: []string{"defect"}})); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(e.queue.indexed, tile305) {
		t.Error("tag edit did not queue indexing")
	}
}

// TestAskIsOffForHighPrivacy: a hospital team can search but not have a
// model read its photos to answer questions.
func TestAskIsOffForHighPrivacy(t *testing.T) {
	e := setup(t, "hospital_orthopedics")
	client := pidexv1connect.NewSearchServiceClient(e.server.Client(), e.server.URL, withToken(e.token))
	_, err := client.AskPhotos(e.ctx, connect.NewRequest(&pidexv1.AskPhotosRequest{Question: "How is the wound healing?"}))
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("ask on a high-privacy team: %v", err)
	}
	e.searchAnswers.push(plan("photos", "", "", []string{"knee"}, ""))
	if _, err := client.SearchPhotos(e.ctx, connect.NewRequest(&pidexv1.SearchPhotosRequest{Query: "knee photos"})); err != nil {
		t.Errorf("search on a high-privacy team: %v", err)
	}
}

// TestPickedTagsAreRequired: a tag the user picks from the tag row is a
// condition; the same tag read by a model from a request only ranks, so a
// photo whose words match can come along.
func TestPickedTagsAreRequired(t *testing.T) {
	e := setup(t, "construction_renovation")
	tagged := e.addPhoto(fixturePhoto{description: "Bathroom wall tiling in progress.", tags: []string{"tiling"}})
	mentioned := e.addPhoto(fixturePhoto{description: "Tiling still to do in the hallway.", tags: []string{"in_progress"}})
	client := pidexv1connect.NewSearchServiceClient(e.server.Client(), e.server.URL, withToken(e.token))

	picked, err := client.SearchPhotos(e.ctx, connect.NewRequest(&pidexv1.SearchPhotosRequest{
		Filters: &pidexv1.SearchFilters{Tags: []string{"tiling"}, TagsRequired: true},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(picked.Msg.Photos); !slices.Equal(got, []string{tagged.String()}) || !picked.Msg.Filters.TagsRequired {
		t.Errorf("picked tag: %v", got)
	}

	soft, err := client.SearchPhotos(e.ctx, connect.NewRequest(&pidexv1.SearchPhotosRequest{
		Filters: &pidexv1.SearchFilters{Tags: []string{"tiling"}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(soft.Msg.Photos); len(got) != 2 || got[0] != tagged.String() || got[1] != mentioned.String() {
		t.Errorf("a read tag ranks the tagged photo first and keeps the one whose words match: %v", got)
	}
}
