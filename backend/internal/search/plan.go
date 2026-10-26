package search

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"pidex/internal/ai"
	"pidex/internal/teamconfig"
)

type Mode string

const (
	ModePhotos  Mode = "photos"
	ModeMissing Mode = "missing"
)

// Filters is a search request in the team's vocabulary. A model writes it
// from plain language; the user can then edit it as chips.
type Filters struct {
	Mode         Mode
	SubjectQuery string
	SubjectIDs   []uuid.UUID
	SubjectType  string
	// Tags rank photos; see SearchFilters.tags.
	Tags []string
	// TagsRequired makes every tag a condition, for tags the user picked.
	TagsRequired bool
	// Within limits a search to these photos: "only the back ones" said of
	// photos just shown narrows those, not the whole collection.
	Within []uuid.UUID
	// From is inclusive and To exclusive.
	From, To *time.Time
	Roles    []string
	// Place is a place the photos were taken in ("Dalian", "Xinghai
	// Square"), matched against the names of where each was taken.
	Place string
	// Text is matched by words, and by meaning when Request is empty.
	Text string
	// Request is the user's own words without the subject's: what a
	// planned search ranks by meaning. Splitting a request into tags and
	// leftover text loses its sense ("the HVAC unit outside" is not "hvac"
	// plus "outside").
	Request string
}

// Model is the slice of ai.Router the planner uses.
type Model interface {
	Structured(ctx context.Context, meta ai.CallMeta, req ai.StructuredRequest) (json.RawMessage, ai.Usage, error)
}

// Plan turns a plain-language request into filters. Tags and subject types
// can only be ones the team has; dates are resolved in the user's zone.
func Plan(ctx context.Context, m Model, teamID uuid.UUID, cfg *teamconfig.Config, query string, now time.Time) (Filters, error) {
	t, err := PlanTurn(ctx, m, teamID, cfg, query, now, Context{})
	return t.Filters, err
}

// Intent is what a request wants back.
type Intent string

const (
	// IntentPhotos wants to see photos.
	IntentPhotos Intent = "photos"
	// IntentAnswer wants words: yes or no, how many, what, a description.
	IntentAnswer Intent = "answer"
	// IntentUnrelated is not about the photos at all: small talk, questions
	// about the assistant, general knowledge. Nothing is looked up.
	IntentUnrelated Intent = "unrelated"
)

// Turn is an earlier exchange in a conversation.
type Turn struct {
	Request string
	// Reply is an answer, or what was found ("12 photos").
	Reply string
}

// Context is what a conversation has established so far.
type Context struct {
	// Previous is the last search, which a follow-up may adjust.
	Previous *Filters
	// Turns are the last few exchanges, oldest first.
	Turns []Turn
}

// PlannedTurn is a request in a conversation, understood.
type PlannedTurn struct {
	Filters Filters
	Intent  Intent
	// AboutPrevious: the request is about the photos just shown ("the
	// second one", "which of these"), not a new search.
	AboutPrevious bool
}

// PlanTurn plans a request in the light of the conversation before it.
func PlanTurn(ctx context.Context, m Model, teamID uuid.UUID, cfg *teamconfig.Config, query string, now time.Time, history Context) (PlannedTurn, error) {
	raw, _, err := m.Structured(ctx, ai.CallMeta{TeamID: teamID, Purpose: "search_plan"}, ai.StructuredRequest{
		Instructions:    planInstructions(cfg, now),
		Prompt:          historyPrompt(history, now.Location()) + "Request: " + query,
		Schema:          planSchema(cfg),
		MaxOutputTokens: 800,
	})
	if err != nil {
		return PlannedTurn{}, err
	}
	f, err := parsePlan(raw, cfg, now.Location())
	if err != nil {
		return PlannedTurn{}, err
	}
	var extra struct {
		Intent        string `json:"intent"`
		AboutPrevious bool   `json:"about_previous"`
		RequireTags   bool   `json:"require_tags"`
	}
	_ = json.Unmarshal(raw, &extra)
	t := PlannedTurn{Filters: f, Intent: IntentPhotos, AboutPrevious: extra.AboutPrevious && history.Previous != nil}
	switch Intent(extra.Intent) {
	case IntentAnswer, IntentUnrelated:
		t.Intent = Intent(extra.Intent)
	}
	t.Filters.TagsRequired = extra.RequireTags && len(t.Filters.Tags) > 0
	t.Filters.Request = strings.TrimSpace(strings.Replace(query, f.SubjectQuery, " ", 1))
	return t, nil
}

// historyPrompt states the conversation so far and the last search, in the
// form the plan is written in, so a follow-up can adjust it.
func historyPrompt(h Context, loc *time.Location) string {
	if h.Previous == nil && len(h.Turns) == 0 {
		return ""
	}
	var b strings.Builder
	if len(h.Turns) > 0 {
		b.WriteString("Conversation so far, oldest first:\n")
		for _, t := range h.Turns {
			fmt.Fprintf(&b, "- Request: %s\n  Reply: %s\n", t.Request, t.Reply)
		}
	}
	if p := h.Previous; p != nil {
		prev := planForm{Mode: string(p.Mode), SubjectQuery: p.SubjectQuery, SubjectType: p.SubjectType, Tags: p.Tags, Text: p.Text}
		if p.From != nil {
			prev.DateFrom = p.From.In(loc).Format("2006-01-02")
		}
		if p.To != nil {
			prev.DateTo = p.To.In(loc).Format("2006-01-02")
		}
		data, _ := json.Marshal(prev)
		fmt.Fprintf(&b, "The previous search: %s\n", data)
	}
	b.WriteString(`A follow-up that narrows, widens or adjusts the previous search ("only last week's", "and the back?", "without the phone") starts from the previous search and changes only what it asks to change. A follow-up that only swaps what a previous question was about ("and the tablet?" after "does the phone have a case?") asks the same question again: keep its intent. A new topic ignores the previous search.

`)
	return b.String()
}

type planForm struct {
	Mode         string   `json:"mode"`
	SubjectQuery string   `json:"subject_query"`
	SubjectType  string   `json:"subject_type"`
	Tags         []string `json:"tags"`
	DateFrom     string   `json:"date_from"`
	DateTo       string   `json:"date_to"`
	Place        string   `json:"place"`
	Text         string   `json:"text"`
}

func planInstructions(cfg *teamconfig.Config, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, `You turn a request to find work photos into search filters. Today is %s (%s).

The team: %s (%s). Each photo keeps when and where it was taken (the names of its place: country, region, city, district, a landmark or street).
`, now.Format("Monday 2006-01-02"), now.Location(), cfg.Name, cfg.Industry)
	if len(cfg.SubjectTypes) == 0 {
		b.WriteString("Photos are not filed under subjects: they are kept by place and time.\n")
	} else {
		b.WriteString("Photos are filed under subjects:\n")
	}
	for _, st := range cfg.SubjectTypes {
		fmt.Fprintf(&b, "- %s: %s, identified by", st.Key, st.Name)
		for i, f := range st.IdentityFields {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, " %s", f.Name)
		}
		b.WriteString("\n")
	}
	b.WriteString("\nTags:\n")
	for _, t := range cfg.Tags {
		fmt.Fprintf(&b, "- %s: %s", t.Key, t.Name)
		if t.Group != "" {
			fmt.Fprintf(&b, " (%s)", t.Group)
		}
		b.WriteString("\n")
	}
	b.WriteString(`
Rules:
- subject_query: only the identity of one particular subject as the team identifies it (a person's name, a record, unit or plate number, a pet's name), copied from the request; "" if none. Rooms, body parts, objects and kinds of work are never subjects, and neither is how something looks: a colour, a breed, a size or a pattern ("the yellow cat", "black ones", "the big mug") goes in text.
- subject_type: only when the request asks which subjects lack something (mode "missing") or names a subject whose type matters; "" otherwise.
- tags: the tags that describe what the photos should show. They rank results, so include every clearly related tag. When the request asks for all photos of a subject or a period without describing their content ("last week's photos"), or asks about the whole collection (what is there, how many, list everything, an overview), leave tags and text empty.
- place: a place the photos were taken in that the request names (a country, city, district, landmark, street, park, venue), copied as written; "" if none. A subject the team files photos under (a project, a unit, a site with a name) goes in subject_query instead.
- date_from and date_to: YYYY-MM-DD, date_to exclusive, only when the request mentions time ("yesterday", "last week", "in March"); "" otherwise.
- text: what the request still asks for that filters do not cover, in the request's language; "" if nothing is left.
- require_tags: true only when the request explicitly limits results to photos showing those tags ("only the back", "just the damaged ones"); false when tags only describe what to look for, including a question whether such a photo exists ("is there a photo of it in the bag?").
- intent: "answer" when the request wants words back (a yes or no, how many, what or which, a description, a comparison); "photos" when it asks to see or find photos; "unrelated" when it is not about the team's photos or what they show at all (greetings, small talk, questions about you or the app, general knowledge, requests to write or calculate something). A request naming a subject, a place, a time or anything photographed is never unrelated.
- about_previous: true only when the request is about the photos just shown ("the second one", "which of these", "are they all intact?") rather than a new search.
- mode: "missing" only when the request asks which or how many subjects have no such photo yet ("which <subjects> have no <kind of> photo"); "photos" otherwise, including yes/no questions about one thing ("does <it> have <something>?"), which photos answer.
`)
	return b.String()
}

func planSchema(cfg *teamconfig.Config) map[string]any {
	types := []string{""}
	for _, st := range cfg.SubjectTypes {
		types = append(types, st.Key)
	}
	tagKeys := make([]string, 0, len(cfg.Tags))
	for _, t := range cfg.Tags {
		tagKeys = append(tagKeys, t.Key)
	}
	tagList := map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	if len(tagKeys) > 0 {
		tagList["items"] = map[string]any{"type": "string", "enum": tagKeys}
	}
	str := map[string]any{"type": "string"}
	props := map[string]any{
		"mode":           map[string]any{"type": "string", "enum": []string{string(ModePhotos), string(ModeMissing)}},
		"intent":         map[string]any{"type": "string", "enum": []string{string(IntentPhotos), string(IntentAnswer), string(IntentUnrelated)}},
		"about_previous": map[string]any{"type": "boolean"},
		"require_tags":   map[string]any{"type": "boolean"},
		"subject_query":  str,
		"subject_type":   map[string]any{"type": "string", "enum": types},
		"tags":           tagList,
		"date_from":      str,
		"date_to":        str,
		"place":          str,
		"text":           str,
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return map[string]any{"type": "object", "properties": props, "required": keys, "additionalProperties": false}
}

// parsePlan checks the answer against the config: unknown tags, types and
// roles are dropped rather than trusted.
func parsePlan(raw json.RawMessage, cfg *teamconfig.Config, loc *time.Location) (Filters, error) {
	var p planForm
	if err := json.Unmarshal(raw, &p); err != nil {
		return Filters{}, fmt.Errorf("search plan is not the requested JSON: %w", err)
	}
	f := Filters{Mode: ModePhotos, SubjectQuery: strings.TrimSpace(p.SubjectQuery), Text: strings.TrimSpace(p.Text), Place: strings.TrimSpace(p.Place)}
	if p.Mode == string(ModeMissing) {
		f.Mode = ModeMissing
	}
	if _, ok := cfg.SubjectType(p.SubjectType); ok {
		f.SubjectType = p.SubjectType
	}
	known := func(keys []string) []string {
		var out []string
		for _, k := range keys {
			if cfg.HasTag(k) && !slices.Contains(out, k) {
				out = append(out, k)
			}
		}
		return out
	}
	f.Tags = known(p.Tags)
	day := func(s string) *time.Time {
		t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(s), loc)
		if err != nil {
			return nil
		}
		return &t
	}
	f.From, f.To = day(p.DateFrom), day(p.DateTo)
	if f.From != nil && f.To != nil && !f.To.After(*f.From) {
		// A single day given twice, or reversed: cover the day(s) between.
		from, to := *f.From, *f.To
		if to.Before(from) {
			from, to = to, from
		}
		to = to.AddDate(0, 0, 1)
		f.From, f.To = &from, &to
	}
	return f, nil
}
