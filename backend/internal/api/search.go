package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/internal/ai"
	"pidex/internal/pipeline"
	"pidex/internal/search"
	"pidex/internal/store/dbq"
	"pidex/internal/teamconfig"
)

type searchService struct{ *Server }

const (
	maxQueryLen    = 500
	defaultResults = 60
	maxResults     = 200
	// askPhotos bounds how many photos an answer is drawn from: thumbnails
	// for all of them go to the vision model.
	askPhotos = 12
	// planTimeout bounds the model call that turns a request into filters.
	planTimeout = 6 * time.Second
)

func (s *searchService) SearchPhotos(ctx context.Context, req *connect.Request[pidexv1.SearchPhotosRequest]) (*connect.Response[pidexv1.SearchPhotosResponse], error) {
	pr := principalFrom(ctx)
	m := req.Msg
	cfg, _, err := s.Configs.Active(ctx, pr.TeamID)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	f, err := s.filters(ctx, pr.TeamID, cfg, m.Query, m.Filters, m.TimeZone)
	if err != nil {
		return nil, err
	}
	limit := int(m.PageSize)
	if limit <= 0 {
		limit = defaultResults
	}
	page := search.Page{Limit: min(limit, maxResults), Offset: max(int(m.Offset), 0), CountTags: m.TagCounts}
	var res search.Result
	if m.Filters == nil {
		// Planned from words: the model's tags may be relaxed.
		f, res, err = s.Search.RunPlanned(ctx, pr.TeamID, cfg, f, page)
	} else {
		res, err = s.Search.RunPage(ctx, pr.TeamID, cfg, f, page)
	}
	if err != nil {
		return nil, internal(err)
	}
	out := &pidexv1.SearchPhotosResponse{Filters: filtersProto(f, res.SubjectIDs), SubjectNotFound: res.SubjectNotFound,
		Total: int32(res.Total)}
	for _, c := range res.TagCounts {
		out.TagCounts = append(out.TagCounts, &pidexv1.TagCount{Key: c.Key, Count: int32(c.Count)})
	}
	for _, c := range res.PlaceCounts {
		out.PlaceCounts = append(out.PlaceCounts, &pidexv1.PlaceCount{Name: c.Key, Count: int32(c.Count)})
	}
	photos, err := s.photosInOrder(ctx, pr.TeamID, res.PhotoIDs)
	if err != nil {
		return nil, err
	}
	if m.Filters == nil {
		checked := s.checkFound(ctx, pr.TeamID, cfg, m.Query, f, res.SubjectNotFound, photos)
		out.Total = max(out.Total-int32(len(photos)-len(checked)), int32(len(checked)))
		photos = checked
	}
	if out.Photos, err = s.photosProto(ctx, s.Q, photos); err != nil {
		return nil, err
	}
	if out.Subjects, out.Missing, err = s.resultSubjects(ctx, pr.TeamID, res, photos); err != nil {
		return nil, err
	}
	return connect.NewResponse(out), nil
}

func (s *searchService) AskPhotos(ctx context.Context, req *connect.Request[pidexv1.AskPhotosRequest]) (*connect.Response[pidexv1.AskPhotosResponse], error) {
	pr := principalFrom(ctx)
	m := req.Msg
	cfg, _, err := s.Configs.Active(ctx, pr.TeamID)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	if cfg.Privacy == teamconfig.PrivacyHigh {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			errors.New("questions are off for high-privacy teams; search finds the photos without a model reading them"))
	}
	f, err := s.filters(ctx, pr.TeamID, cfg, m.Question, nil, m.TimeZone)
	if err != nil {
		return nil, err
	}
	f, res, err := s.findForAnswer(ctx, pr.TeamID, cfg, f)
	if err != nil {
		return nil, internal(err)
	}
	out := &pidexv1.AskPhotosResponse{Filters: filtersProto(f, res.SubjectIDs)}
	photos, err := s.photosInOrder(ctx, pr.TeamID, res.PhotoIDs)
	if err != nil {
		return nil, err
	}
	if out.Subjects, out.Missing, err = s.resultSubjects(ctx, pr.TeamID, res, photos); err != nil {
		return nil, err
	}
	// "Which units have no …" is answered by the database; the app words it.
	if f.Mode == search.ModeMissing || len(photos) == 0 {
		return connect.NewResponse(out), nil
	}
	answer, err := s.answer(ctx, pr.TeamID, cfg, m.Question, photos, out.Subjects, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}
	out.Answer = answer
	if out.Cited, err = s.photosProto(ctx, s.Q, photos); err != nil {
		return nil, err
	}
	return connect.NewResponse(out), nil
}

// findForAnswer finds the photos a question is answered from. Unlike a
// search it always finds some when the scope has any: a question naming no
// subject on file falls back to photos, and one whose words match nothing
// to the latest photos in scope ("what do we have?" matches no description,
// yet the photos answer it).
func (s *Server) findForAnswer(ctx context.Context, teamID uuid.UUID, cfg *teamconfig.Config, f search.Filters) (search.Filters, search.Result, error) {
	f, res, err := s.Search.RunPlanned(ctx, teamID, cfg, f, search.Page{Limit: askPhotos})
	if err != nil {
		return f, res, err
	}
	if f.Mode == search.ModeMissing && res.SubjectNotFound {
		f.Mode = search.ModePhotos
		if res, err = s.Search.Run(ctx, teamID, cfg, f, askPhotos); err != nil {
			return f, res, err
		}
	}
	if len(res.PhotoIDs) == 0 && f.Mode == search.ModePhotos && (f.Text != "" || len(f.Tags) > 0) {
		broad := f
		broad.Text, broad.Request, broad.Tags, broad.TagsRequired = "", "", nil, false
		if res.SubjectNotFound {
			broad.SubjectQuery = ""
		}
		if res, err = s.Search.Run(ctx, teamID, cfg, broad, askPhotos); err != nil {
			return f, res, err
		}
	}
	return f, res, nil
}

// filters are the client's own (edited chips: no model call), or a plan
// written by the search model from the request.
func (s *searchService) filters(ctx context.Context, teamID uuid.UUID, cfg *teamconfig.Config, query string, given *pidexv1.SearchFilters, zone string) (search.Filters, error) {
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "" {
		loc = time.UTC
	}
	if given != nil {
		return filtersFrom(given, cfg)
	}
	query = strings.TrimSpace(query)
	switch {
	case query == "":
		return search.Filters{Mode: search.ModePhotos}, nil
	case len(query) > maxQueryLen:
		return search.Filters{}, invalid("the request is longer than %d characters", maxQueryLen)
	case s.SearchModel.Empty():
		// Without a model the whole request is matched as text.
		return search.Filters{Mode: search.ModePhotos, Text: query}, nil
	}
	// Search is interactive: a stalled model call degrades to word and
	// meaning matching instead of a spinner.
	planCtx, cancel := context.WithTimeout(ctx, planTimeout)
	defer cancel()
	f, err := search.Plan(planCtx, s.SearchModel, teamID, cfg, query, time.Now().In(loc))
	if err != nil {
		s.Log.Warn("search plan failed; matching the request as text", "err", err)
		return search.Filters{Mode: search.ModePhotos, Text: query}, nil
	}
	return f, nil
}

// checkFound drops photos a planned search found whose words do not show
// what was asked (see search.Check). If the check cannot be made, the
// photos stay as found.
//
// Words that named no subject were matched as content, so they are checked
// as content too.
func (s *Server) checkFound(ctx context.Context, teamID uuid.UUID, cfg *teamconfig.Config, request string, f search.Filters, subjectNotFound bool, photos []dbq.Photo) []dbq.Photo {
	if subjectNotFound {
		f = search.AsContent(f)
	}
	if !search.Checks(f) || len(photos) == 0 || s.SearchModel.Empty() || strings.TrimSpace(request) == "" {
		return photos
	}
	tags, err := s.Q.ListPhotoTags(ctx, photoIDs(photos))
	if err != nil {
		s.Log.Warn("search check skipped", "err", err)
		return photos
	}
	subjects, err := subjectsOf(ctx, s.Q, photos)
	if err != nil {
		s.Log.Warn("search check skipped", "err", err)
		return photos
	}
	tagNames := map[string]string{}
	for _, t := range cfg.Tags {
		tagNames[t.Key] = t.Name
	}
	found := make([]search.Found, len(photos))
	for i, p := range photos {
		// As described, with its look: a name in place of "a golden
		// shaded cat" would hide the colour asked about.
		found[i].Description = p.Description
		if p.SubjectID != nil && (p.Assignment == "auto" || p.Assignment == "manual") {
			sub := subjects[*p.SubjectID]
			found[i].Subject, found[i].Looks = sub.DisplayName, sub.Profile
		}
		for _, t := range tags {
			if t.PhotoID == p.ID && tagNames[t.TagKey] != "" {
				found[i].Tags = append(found[i].Tags, tagNames[t.TagKey])
			}
		}
	}
	checkCtx, cancel := context.WithTimeout(ctx, planTimeout)
	defer cancel()
	keep, err := search.Check(checkCtx, s.SearchModel, teamID, cfg, request, f, found)
	if err != nil {
		s.Log.Warn("search check failed; showing the photos as found", "err", err)
		return photos
	}
	out := make([]dbq.Photo, 0, len(photos))
	for i, p := range photos {
		if keep[i] {
			out = append(out, p)
		}
	}
	return out
}

func (s *Server) photosInOrder(ctx context.Context, teamID uuid.UUID, ids []uuid.UUID) ([]dbq.Photo, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.Q.GetPhotosByIDs(ctx, dbq.GetPhotosByIDsParams{TeamID: teamID, Ids: ids})
	if err != nil {
		return nil, internal(err)
	}
	byID := make(map[uuid.UUID]dbq.Photo, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	out := make([]dbq.Photo, 0, len(ids))
	for _, id := range ids {
		if p, ok := byID[id]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

// resultSubjects returns the subjects the request named or the photos are
// filed under, and the missing ones, with photo counts.
func (s *Server) resultSubjects(ctx context.Context, teamID uuid.UUID, res search.Result, photos []dbq.Photo) (named, missing []*pidexv1.Subject, err error) {
	ids := slices.Clone(res.SubjectIDs)
	for _, p := range photos {
		if p.SubjectID != nil && !slices.Contains(ids, *p.SubjectID) {
			ids = append(ids, *p.SubjectID)
		}
	}
	load := func(ids []uuid.UUID) ([]*pidexv1.Subject, error) {
		if len(ids) == 0 {
			return nil, nil
		}
		rows, err := s.Q.GetSubjectsByIDs(ctx, dbq.GetSubjectsByIDsParams{TeamID: teamID, Ids: ids})
		if err != nil {
			return nil, internal(err)
		}
		counts, err := s.photoCounts(ctx, teamID, rows)
		if err != nil {
			return nil, err
		}
		byID := map[uuid.UUID]dbq.Subject{}
		for _, r := range rows {
			byID[r.ID] = r
		}
		var out []*pidexv1.Subject
		for _, id := range ids {
			if r, ok := byID[id]; ok {
				out = append(out, subjectProto(r, counts[id]))
			}
		}
		return out, nil
	}
	if named, err = load(ids); err != nil {
		return nil, nil, err
	}
	missing, err = load(res.MissingIDs)
	return named, missing, err
}

// answer asks the vision model a question about the given photos only,
// numbered [1]..[n], with their thumbnails.
func (s *Server) answer(ctx context.Context, teamID uuid.UUID, cfg *teamconfig.Config, question string, photos []dbq.Photo, subjects []*pidexv1.Subject, turns []search.Turn) (string, error) {
	names := map[string]string{}
	byID := map[uuid.UUID]string{}
	for _, sub := range subjects {
		names[sub.Id] = sub.DisplayName
		if id, err := uuid.Parse(sub.Id); err == nil {
			byID[id] = sub.DisplayName
		}
	}
	tags, err := s.Q.ListPhotoTags(ctx, photoIDs(photos))
	if err != nil {
		return "", err
	}
	tagNames := map[string]string{}
	for _, t := range cfg.Tags {
		tagNames[t.Key] = t.Name
	}
	var b strings.Builder
	if len(turns) > 0 {
		b.WriteString("Earlier in this conversation, oldest first:\n")
		for _, t := range turns {
			fmt.Fprintf(&b, "- Q: %s\n  A: %s\n", t.Request, t.Reply)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Question: %s\n\n", question)
	if err := s.writeSubjectRegister(ctx, &b, teamID, cfg); err != nil {
		return "", err
	}
	b.WriteString("Photos:\n")
	var images []ai.Image
	for i, p := range photos {
		at := p.CreatedAt
		if p.CapturedAt != nil {
			at = *p.CapturedAt
		}
		fmt.Fprintf(&b, "[%d] %s", i+1, at.Format("2006-01-02 15:04"))
		if p.SubjectID != nil {
			fmt.Fprintf(&b, ", %s", names[p.SubjectID.String()])
		}
		if where := search.PlaceWords(p.Place); where != "" && cfg.Privacy != teamconfig.PrivacyHigh {
			fmt.Fprintf(&b, ", at %s", where)
		}
		var ts []string
		for _, t := range tags {
			if t.PhotoID == p.ID {
				ts = append(ts, tagNames[t.TagKey])
			}
		}
		if len(ts) > 0 {
			fmt.Fprintf(&b, ", tags: %s", strings.Join(ts, ", "))
		}
		fmt.Fprintf(&b, ". %s\n", described(p, pipeline.DecodeExtracted(p.Extracted), byID))
		if p.ThumbKey != nil {
			if data, err := s.Storage.Get(ctx, *p.ThumbKey, 2<<20); err == nil {
				images = append(images, ai.Image{Data: data, MIMEType: "image/jpeg"})
				continue
			}
		}
		// A photo without a thumbnail is described by text only; keep the
		// numbering of images aligned by saying so.
		fmt.Fprintf(&b, "    (no image for [%d])\n", i+1)
	}
	instructions := fmt.Sprintf(`You answer a question from a team that files work photos (%s). Use only the subject register, the photos and the notes given; images come in the order of the numbered photos that have one. The register lists every subject on file and is complete: use it for what, which and how many. A subject's identity rarely says what kind of thing it is; tell that from the tags on its photos and from the photos themselves, and count only the subjects that are of the kind asked about. Cite every statement drawn from photos with their numbers, like [2] or [1][3]; statements from the register need no number. If neither answers the question, say so plainly. Answer in the language of the question, in at most five short sentences. Do not give medical, legal or safety advice; describe what the photos show.`, cfg.Name)
	raw, _, err := s.Processor.Vision.Structured(ctx, ai.CallMeta{TeamID: teamID, Purpose: "ask"}, ai.StructuredRequest{
		Instructions: instructions, Prompt: b.String(), Images: images, MaxOutputTokens: 1200,
		Schema: map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"answer"},
			"properties": map[string]any{"answer": map[string]any{"type": "string"}},
		},
	})
	if err != nil {
		return "", err
	}
	var out struct {
		Answer string `json:"answer"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.Answer), nil
}

// registerLimit bounds the subjects listed to the model; totals per type
// stay exact beyond it. registerTags is how many of a subject's most used
// tags describe it.
const (
	registerLimit = 100
	registerTags  = 6
)

// writeSubjectRegister lists the team's subjects (the authoritative record
// of what is on file) so questions about what, which and how many are
// answered from it rather than from the few photos a search returns.
func (s *Server) writeSubjectRegister(ctx context.Context, b *strings.Builder, teamID uuid.UUID, cfg *teamconfig.Config) error {
	rows, err := s.Q.ListSubjects(ctx, dbq.ListSubjectsParams{TeamID: teamID, PageLimit: registerLimit + 1})
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		b.WriteString("Subject register: nothing filed yet.\n\n")
		return nil
	}
	rows = rows[:min(len(rows), registerLimit)]
	counts, err := s.photoCounts(ctx, teamID, rows)
	if err != nil {
		return err
	}
	// Totals come from the database, so "how many" stays exact however
	// many subjects there are; only the listing is cut.
	totals, err := s.Q.CountSubjectsByType(ctx, teamID)
	if err != nil {
		return err
	}
	total := map[string]int32{}
	for _, t := range totals {
		total[t.TypeKey] = t.SubjectCount
	}
	byType := map[string][]dbq.Subject{}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		byType[r.TypeKey] = append(byType[r.TypeKey], r)
		ids[i] = r.ID
	}
	tagRows, err := s.Q.SubjectTagCounts(ctx, dbq.SubjectTagCountsParams{TeamID: teamID, SubjectIds: ids})
	if err != nil {
		return err
	}
	tagNames := map[string]string{}
	for _, t := range cfg.Tags {
		tagNames[t.Key] = t.Name
	}
	// A subject's identity rarely says what it is (a serial number); the
	// tags on its photos do ("tablet", "phone").
	tagsOf := map[uuid.UUID][]string{}
	for _, t := range tagRows {
		if name, ok := tagNames[t.TagKey]; ok && len(tagsOf[t.SubjectID]) < registerTags {
			tagsOf[t.SubjectID] = append(tagsOf[t.SubjectID], fmt.Sprintf("%s ×%d", name, t.PhotoCount))
		}
	}
	b.WriteString("Subject register:\n")
	for _, st := range cfg.SubjectTypes {
		if total[st.Key] == 0 {
			continue
		}
		fmt.Fprintf(b, "- %s (%d in total):\n", st.Name, total[st.Key])
		for _, sub := range byType[st.Key] {
			fmt.Fprintf(b, "  - %s: %d photos", sub.DisplayName, counts[sub.ID])
			if tags := tagsOf[sub.ID]; len(tags) > 0 {
				fmt.Fprintf(b, "; tags on its photos: %s", strings.Join(tags, ", "))
			}
			b.WriteString("\n")
		}
		if n := len(byType[st.Key]); int32(n) < total[st.Key] {
			fmt.Fprintf(b, "  (the %d most recent of %d are listed)\n", n, total[st.Key])
		}
	}
	b.WriteString("\n")
	return nil
}

func photoIDs(photos []dbq.Photo) []uuid.UUID {
	ids := make([]uuid.UUID, len(photos))
	for i, p := range photos {
		ids[i] = p.ID
	}
	return ids
}

var roleNames = map[pidexv1.PhotoRole]string{
	pidexv1.PhotoRole_PHOTO_ROLE_IDENTITY: "identity",
	pidexv1.PhotoRole_PHOTO_ROLE_DOCUMENT: "document",
	pidexv1.PhotoRole_PHOTO_ROLE_CONTENT:  "content",
}

func filtersProto(f search.Filters, subjects []uuid.UUID) *pidexv1.SearchFilters {
	out := &pidexv1.SearchFilters{
		Mode: pidexv1.SearchMode_SEARCH_MODE_PHOTOS, SubjectQuery: f.SubjectQuery, SubjectType: f.SubjectType,
		Tags: f.Tags, TagsRequired: f.TagsRequired, From: ts(f.From), To: ts(f.To), Text: f.Text, Place: f.Place,
	}
	if f.Mode == search.ModeMissing {
		out.Mode = pidexv1.SearchMode_SEARCH_MODE_MISSING
	}
	for _, id := range subjects {
		out.SubjectIds = append(out.SubjectIds, id.String())
	}
	for _, r := range f.Roles {
		for k, v := range roleNames {
			if v == r {
				out.Roles = append(out.Roles, k)
			}
		}
	}
	return out
}

// filtersFrom checks filters sent back by the app against the config, as
// the planner's are.
func filtersFrom(in *pidexv1.SearchFilters, cfg *teamconfig.Config) (search.Filters, error) {
	f := search.Filters{Mode: search.ModePhotos, SubjectQuery: strings.TrimSpace(in.SubjectQuery), Text: strings.TrimSpace(in.Text), Place: strings.TrimSpace(in.Place)}
	if in.Mode == pidexv1.SearchMode_SEARCH_MODE_MISSING {
		f.Mode = search.ModeMissing
	}
	if len(f.Text) > maxQueryLen || len(f.SubjectQuery) > maxQueryLen || len(f.Place) > maxQueryLen {
		return f, invalid("filter text is longer than %d characters", maxQueryLen)
	}
	for _, id := range in.SubjectIds {
		u, err := parseID("subject_ids", id)
		if err != nil {
			return f, err
		}
		f.SubjectIDs = append(f.SubjectIDs, u)
	}
	if in.SubjectType != "" {
		if _, ok := cfg.SubjectType(in.SubjectType); !ok {
			return f, invalid("unknown subject type %q", in.SubjectType)
		}
		f.SubjectType = in.SubjectType
	}
	for _, k := range in.Tags {
		if !cfg.HasTag(k) {
			return f, invalid("unknown tag %q", k)
		}
	}
	f.Tags, f.TagsRequired = in.Tags, in.TagsRequired
	if in.From != nil {
		t := in.From.AsTime()
		f.From = &t
	}
	if in.To != nil {
		t := in.To.AsTime()
		f.To = &t
	}
	for _, r := range in.Roles {
		if name, ok := roleNames[r]; ok {
			f.Roles = append(f.Roles, name)
		}
	}
	return f, nil
}
