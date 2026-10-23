package e2e

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/gen/pidex/v1/pidexv1connect"
	"pidex/internal/api"
	"pidex/internal/store/dbq"
)

func chatPlan(subject string, tags []string, intent string, aboutPrevious bool) string {
	b, _ := json.Marshal(map[string]any{
		"mode": "photos", "subject_query": subject, "subject_type": "", "tags": tags,
		"date_from": "", "date_to": "", "text": "", "intent": intent, "about_previous": aboutPrevious,
	})
	return string(b)
}

// TestConversation: a search, a follow-up that narrows it, and a question
// about the photo just shown; then the history, read back and kept private.
func TestConversation(t *testing.T) {
	e := setup(t, "construction_renovation")
	maple := e.addSubject("project", "Maple Court", map[string]string{"project_name": "Maple Court"}, nil)
	oakSt := e.addSubject("project", "Oak Street", map[string]string{"project_name": "Oak Street"}, nil)
	wp := e.addPhoto(fixturePhoto{subject: &maple, description: "Waterproofing membrane on the bathroom floor.", tags: []string{"waterproofing"}})
	oak := e.addPhoto(fixturePhoto{subject: &oakSt, description: "Waterproofing on the balcony.", tags: []string{"waterproofing"}, daysAgo: 1})
	chat := pidexv1connect.NewChatServiceClient(e.server.Client(), e.server.URL, withToken(e.token))
	send := func(conv, text, plan string) *pidexv1.SendMessageResponse {
		t.Helper()
		e.searchAnswers.push(plan)
		res, err := chat.SendMessage(e.ctx, connect.NewRequest(&pidexv1.SendMessageRequest{ConversationId: conv, Text: text, TimeZone: "UTC"}))
		if err != nil {
			t.Fatal(err)
		}
		return res.Msg
	}

	first := send("", "show me the waterproofing", chatPlan("", []string{"waterproofing"}, "photos", false))
	conv := first.Conversation.Id
	found := ids(first.Reply.Photos)
	slices.Sort(found)
	want := []string{wp.String(), oak.String()}
	slices.Sort(want)
	if first.Reply.Kind != pidexv1.ReplyKind_REPLY_KIND_PHOTOS || !slices.Equal(found, want) || first.Reply.PhotoTotal != 2 {
		t.Fatalf("first reply %v %v of %d", first.Reply.Kind, ids(first.Reply.Photos), first.Reply.PhotoTotal)
	}
	if first.Message.Text != "show me the waterproofing" || first.Conversation.Title != "show me the waterproofing" {
		t.Errorf("message %q title %q", first.Message.Text, first.Conversation.Title)
	}

	// "Only Oak Street's" said of the photos just shown narrows those.
	narrow := send(conv, "only Oak Street's", chatPlan("Oak Street", []string{"waterproofing"}, "photos", true))
	if !slices.Equal(ids(narrow.Reply.Photos), []string{oak.String()}) {
		t.Errorf("narrowing the photos shown: %v", ids(narrow.Reply.Photos))
	}

	// A follow-up is planned with the conversation and the last search.
	second := send(conv, "only at Maple Court", chatPlan("Maple Court", []string{"waterproofing"}, "photos", false))
	prompt := e.searchAnswers.prompts[len(e.searchAnswers.prompts)-1]
	for _, want := range []string{"Request: show me the waterproofing", "Reply: 2 photos shown", "Request: only Oak Street's", `"tags":["waterproofing"]`} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the follow-up was planned without %q:\n%s", want, prompt)
		}
	}
	if !slices.Equal(ids(second.Reply.Photos), []string{wp.String()}) {
		t.Errorf("narrowed: %v", ids(second.Reply.Photos))
	}

	// A question about the photo just shown is answered from it, with the
	// conversation, and without a new search.
	asks := len(e.askRequests.reqs)
	third := send(conv, "is it on the bathroom floor?", chatPlan("", nil, "answer", true))
	if third.Reply.Kind != pidexv1.ReplyKind_REPLY_KIND_ANSWER || third.Reply.Text == "" || !slices.Equal(ids(third.Reply.Photos), []string{wp.String()}) {
		t.Errorf("answer about the previous photo: %v %q %v", third.Reply.Kind, third.Reply.Text, ids(third.Reply.Photos))
	}
	if len(e.askRequests.reqs) != asks+1 || !strings.Contains(e.askRequests.reqs[asks].Prompt, "Q: only at Maple Court") {
		t.Error("the answer must see the conversation so far")
	}

	// The history comes back with photos resolved now.
	list, err := chat.ListConversations(e.ctx, connect.NewRequest(&pidexv1.ListConversationsRequest{}))
	if err != nil || len(list.Msg.Conversations) != 1 || list.Msg.Conversations[0].MessageCount != 8 {
		t.Fatalf("list: %v %+v", err, list)
	}
	got, err := chat.GetConversation(e.ctx, connect.NewRequest(&pidexv1.GetConversationRequest{Id: conv}))
	if err != nil {
		t.Fatal(err)
	}
	msgs := got.Msg.Messages
	if len(msgs) != 8 || msgs[0].Role != pidexv1.ChatRole_CHAT_ROLE_USER || msgs[7].Kind != pidexv1.ReplyKind_REPLY_KIND_ANSWER {
		t.Fatalf("messages: %d", len(msgs))
	}
	if msgs[1].Photos[0].ThumbnailUrl == "" || msgs[5].Filters.SubjectQuery != "Maple Court" || len(msgs[5].Subjects) != 1 {
		t.Errorf("a stored reply must come back with fresh links, its filters and subjects: %+v", msgs[5])
	}

	// Conversations are private to their author, even in the same team.
	member, err := e.q.UpsertUser(e.ctx, dbq.UpsertUserParams{Email: "colleague@example.test", DisplayName: "Colleague"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.q.AddTeamMember(e.ctx, dbq.AddTeamMemberParams{TeamID: e.teamID, UserID: member.ID, Role: "member"}); err != nil {
		t.Fatal(err)
	}
	token, hash := api.NewToken()
	if err := e.q.CreateAPIToken(e.ctx, dbq.CreateAPITokenParams{TokenSha256: hash, TeamID: e.teamID, UserID: member.ID}); err != nil {
		t.Fatal(err)
	}
	other := pidexv1connect.NewChatServiceClient(e.server.Client(), e.server.URL, withToken(token))
	if _, err := other.GetConversation(e.ctx, connect.NewRequest(&pidexv1.GetConversationRequest{Id: conv})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("a colleague read the conversation: %v", err)
	}
	if l, _ := other.ListConversations(e.ctx, connect.NewRequest(&pidexv1.ListConversationsRequest{})); len(l.Msg.Conversations) != 0 {
		t.Error("a colleague sees the conversation listed")
	}

	if _, err := chat.DeleteConversation(e.ctx, connect.NewRequest(&pidexv1.DeleteConversationRequest{Id: conv})); err != nil {
		t.Fatal(err)
	}
	if _, err := chat.GetConversation(e.ctx, connect.NewRequest(&pidexv1.GetConversationRequest{Id: conv})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("deleted conversation: %v", err)
	}
}

// TestConversationNothingFound: a search that finds nothing is a reply,
// not an error, as when asking every collection about a subject only one
// of them has.
func TestConversationNothingFound(t *testing.T) {
	e := setup(t, "construction_renovation")
	chat := pidexv1connect.NewChatServiceClient(e.server.Client(), e.server.URL, withToken(e.token))
	e.searchAnswers.push(chatPlan("", []string{"waterproofing"}, "photos", false))
	res, err := chat.SendMessage(e.ctx, connect.NewRequest(&pidexv1.SendMessageRequest{Text: "show me the waterproofing", TimeZone: "UTC"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Msg.Reply.Photos) != 0 || res.Msg.Reply.PhotoTotal != 0 {
		t.Errorf("found %d of %d", len(res.Msg.Reply.Photos), res.Msg.Reply.PhotoTotal)
	}
}

// TestConversationUnrelated: a request not about the photos ("what model
// are you?") is neither searched nor answered.
func TestConversationUnrelated(t *testing.T) {
	e := setup(t, "construction_renovation")
	maple := e.addSubject("project", "Maple Court", map[string]string{"project_name": "Maple Court"}, nil)
	e.addPhoto(fixturePhoto{subject: &maple, description: "Waterproofing membrane on the bathroom floor.", tags: []string{"waterproofing"}})
	chat := pidexv1connect.NewChatServiceClient(e.server.Client(), e.server.URL, withToken(e.token))
	asks := len(e.askRequests.reqs)
	e.searchAnswers.push(chatPlan("", nil, "unrelated", false))
	res, err := chat.SendMessage(e.ctx, connect.NewRequest(&pidexv1.SendMessageRequest{Text: "what model are you?", TimeZone: "UTC"}))
	if err != nil {
		t.Fatal(err)
	}
	r := res.Msg.Reply
	if r.Kind != pidexv1.ReplyKind_REPLY_KIND_UNRELATED || len(r.Photos) != 0 || r.Text != "" || len(e.askRequests.reqs) != asks {
		t.Errorf("an unrelated request was looked up or answered: %v %d photos %q", r.Kind, len(r.Photos), r.Text)
	}
}

// TestConversationRelaxesModelTags: tags the model required that no photo
// has all of (AI tags miss things) only rank, so the photo the words
// describe is still found, and the reply's filters say so.
func TestConversationRelaxesModelTags(t *testing.T) {
	e := setup(t, "construction_renovation")
	maple := e.addSubject("project", "Maple Court", map[string]string{"project_name": "Maple Court"}, nil)
	wp := e.addPhoto(fixturePhoto{subject: &maple, description: "Waterproofing membrane on the bathroom floor.", tags: []string{"waterproofing"}})
	chat := pidexv1connect.NewChatServiceClient(e.server.Client(), e.server.URL, withToken(e.token))
	plan, _ := json.Marshal(map[string]any{
		"mode": "photos", "subject_query": "Maple Court", "subject_type": "", "tags": []string{"waterproofing", "concealed_work"},
		"date_from": "", "date_to": "", "text": "membrane", "intent": "photos", "about_previous": false, "require_tags": true,
	})
	e.searchAnswers.push(string(plan))
	res, err := chat.SendMessage(e.ctx, connect.NewRequest(&pidexv1.SendMessageRequest{Text: "is there a photo of the membrane at Maple Court?", TimeZone: "UTC"}))
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(res.Msg.Reply.Photos); !slices.Equal(got, []string{wp.String()}) || res.Msg.Reply.Filters.TagsRequired {
		t.Errorf("found %v, tags required %v", got, res.Msg.Reply.Filters.TagsRequired)
	}
}

// TestConversationShowsOnlyWhatWasAsked: the subject's photos closest in
// meaning are not shown when their words say they show something else, as
// a cat on the floor is no answer to "on the sofa".
func TestConversationShowsOnlyWhatWasAsked(t *testing.T) {
	e := setup(t, "construction_renovation")
	maple := e.addSubject("project", "Maple Court", map[string]string{"project_name": "Maple Court"}, nil)
	e.addPhoto(fixturePhoto{subject: &maple, description: "Kitchen wall tiling in progress.", tags: []string{"tiling"}})
	wp := e.addPhoto(fixturePhoto{subject: &maple, description: "Membrane on the bathroom floor.", tags: []string{"waterproofing"}, daysAgo: 1})
	chat := pidexv1connect.NewChatServiceClient(e.server.Client(), e.server.URL, withToken(e.token))
	send := func(text, plan string) *pidexv1.ChatMessage {
		t.Helper()
		e.searchAnswers.push(plan)
		res, err := chat.SendMessage(e.ctx, connect.NewRequest(&pidexv1.SendMessageRequest{Text: text, TimeZone: "UTC"}))
		if err != nil {
			t.Fatal(err)
		}
		return res.Msg.Reply
	}
	if r := send("any Maple Court photos of the ceiling?", `{"mode":"photos","subject_query":"Maple Court","subject_type":"","tags":[],"date_from":"","date_to":"","text":"ceiling","intent":"photos","about_previous":false}`); len(r.Photos) != 0 || r.PhotoTotal != 0 {
		t.Errorf("photos of other things shown for the ceiling: %v of %d", ids(r.Photos), r.PhotoTotal)
	}
	if r := send("Maple Court waterproofing", chatPlan("Maple Court", []string{"waterproofing"}, "photos", false)); !slices.Equal(ids(r.Photos), []string{wp.String()}) {
		t.Errorf("waterproofing: %v", ids(r.Photos))
	}
	// Only a subject asked for: nothing to check, all its photos.
	if r := send("Maple Court", chatPlan("Maple Court", nil, "photos", false)); len(r.Photos) != 2 {
		t.Errorf("all of Maple Court: %v", ids(r.Photos))
	}
}

// TestConversationChecksWordsTakenForASubject: words read as a subject's
// name that name none ("the yellow cat" read as a cat called Yellow) are
// matched as content, and checked as content: photos that only resemble
// the request in meaning are not shown.
func TestConversationChecksWordsTakenForASubject(t *testing.T) {
	e := setup(t, "construction_renovation")
	e.addPhoto(fixturePhoto{description: "Kitchen wall tiling in progress.", tags: []string{"tiling"}})
	wp := e.addPhoto(fixturePhoto{description: "Membrane on the bathroom floor.", tags: []string{"waterproofing"}, daysAgo: 1})
	chat := pidexv1connect.NewChatServiceClient(e.server.Client(), e.server.URL, withToken(e.token))
	e.searchAnswers.push(chatPlan("membrane", nil, "photos", false))
	res, err := chat.SendMessage(e.ctx, connect.NewRequest(&pidexv1.SendMessageRequest{Text: "find the membrane", TimeZone: "UTC"}))
	if err != nil {
		t.Fatal(err)
	}
	r := res.Msg.Reply
	if !r.SubjectNotFound || !slices.Equal(ids(r.Photos), []string{wp.String()}) {
		t.Errorf("not found %v, photos %v", r.SubjectNotFound, ids(r.Photos))
	}
}
