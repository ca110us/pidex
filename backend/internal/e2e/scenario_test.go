package e2e

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"connectrpc.com/connect"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/gen/pidex/v1/pidexv1connect"
	"pidex/internal/api"
	"pidex/internal/store/dbq"
	"pidex/internal/teamconfig"
)

// plantsAnswer is what a config model might answer for "I look after the
// plants at home; each pot has a tag with a QR code". tags is spliced in so
// the update step can change it.
func plantsAnswer(tags string) string {
	return `{"summary":"Photos are filed by plant, identified by the QR tag in each pot.",
	"test_tips":["Photograph a pot's QR tag first.","Then photograph the leaves."],
	"config":{"language":"en","name":"Home plants","industry":"home","description":"Plant care photos.",
	"strictness":"lenient","privacy":"standard","date_order":"dmy",
	"subject_types":[{"key":"plant","name":"Plant","parent":"","carriers":"A tag in the pot with a QR code and the plant's name.",
	"identity_fields":[{"key":"plant_id","name":"Plant ID","kind":"id","description":"","pattern":""},
	{"key":"plant_name","name":"Plant name","kind":"text","description":"","pattern":""}],
	"display_fields":["plant_name","plant_id"],"min_fields":1,
	"barcode":{"enabled":true,"field":"plant_id","pattern":"^PLANT-([0-9]{4})$","strong":true}}],
	"tags":[` + tags + `],
	"session":{"gap_minutes":20,"anchor_valid_minutes":0,"max_jump_meters":0}}}`
}

const (
	tagYellowLeaves = `{"key":"yellow_leaves","name":"Yellow leaves","group":"condition","description":""}`
	tagPests        = `{"key":"pests","name":"Pests","group":"condition","description":""}`
	tagRepotting    = `{"key":"repotting","name":"Repotting","group":"care","description":""}`
)

// TestScenarioFromDescription walks the "new scenario" flow: describe the
// work, review the draft, create a team with it, shoot with it, change it,
// and roll the change back.
func TestScenarioFromDescription(t *testing.T) {
	e := setup(t, "hospital_orthopedics")
	clients := func(token string) (pidexv1connect.ConfigServiceClient, pidexv1connect.TeamServiceClient, pidexv1connect.CaptureServiceClient) {
		o := withToken(token)
		return pidexv1connect.NewConfigServiceClient(e.server.Client(), e.server.URL, o),
			pidexv1connect.NewTeamServiceClient(e.server.Client(), e.server.URL, o),
			pidexv1connect.NewCaptureServiceClient(e.server.Client(), e.server.URL, o)
	}
	oldConfig, oldTeam, _ := clients(e.token)

	// 1. Draft from a description. Nothing is stored yet.
	e.configAnswers.push(plantsAnswer(tagYellowLeaves + "," + tagPests))
	draft, err := oldConfig.DraftConfig(e.ctx, connect.NewRequest(&pidexv1.DraftConfigRequest{
		Description: "I look after the plants at home. Each pot has a tag with a QR code.", ForNewTeam: true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.configAnswers.prompts[0], "Each pot has a tag") {
		t.Errorf("description not sent to the model: %q", e.configAnswers.prompts[0])
	}
	if len(draft.Msg.Warnings) != 0 || len(draft.Msg.TestTips) != 2 || draft.Msg.Summary == "" {
		t.Errorf("draft for a new team: %+v", draft.Msg)
	}

	// 2. Create a team with it. The caller owns it and gets its token.
	created, err := oldTeam.CreateTeam(e.ctx, connect.NewRequest(&pidexv1.CreateTeamRequest{ConfigJson: draft.Msg.ConfigJson}))
	if err != nil {
		t.Fatal(err)
	}
	team := created.Msg.Team
	if team.Name != "Home plants" || team.Role != "owner" || team.ConfigVersion != 1 || !strings.HasPrefix(created.Msg.Token, "cbk_") {
		t.Fatalf("created team %+v", team)
	}
	plantConfig, plantTeam, plantCapture := clients(created.Msg.Token)
	got, err := plantTeam.GetTeam(e.ctx, connect.NewRequest(&pidexv1.GetTeamRequest{}))
	if err != nil || got.Msg.Team.Id != team.Id {
		t.Fatalf("new token: %v %+v", err, got)
	}

	// 3. Shooting with the new team uses the new vocabulary at once.
	anchor, err := plantCapture.ResolveAnchor(e.ctx, connect.NewRequest(&pidexv1.ResolveAnchorRequest{OnDevice: &pidexv1.OnDeviceAnalysis{
		Barcodes: []*pidexv1.BarcodeObservation{{Payload: "PLANT-0007", Symbology: "qr"}},
	}}))
	if err != nil {
		t.Fatal(err)
	}
	if anchor.Msg.State != pidexv1.AnchorState_ANCHOR_STATE_NEW_SUBJECT || anchor.Msg.SubjectTypeKey != "plant" || anchor.Msg.ExtractedIdentity["plant_id"] != "0007" {
		t.Errorf("plant QR resolved to %+v", anchor.Msg)
	}

	// The old team is untouched.
	oldActive, err := oldConfig.GetActiveConfig(e.ctx, connect.NewRequest(&pidexv1.GetActiveConfigRequest{}))
	if err != nil || !strings.Contains(oldActive.Msg.ConfigJson, "Orthopedics") {
		t.Fatalf("old team's config changed: %v", err)
	}

	// 4. Change it: the model gets the current config; removing a tag warns.
	e.configAnswers.push(plantsAnswer(tagYellowLeaves + "," + tagRepotting))
	change, err := plantConfig.DraftConfig(e.ctx, connect.NewRequest(&pidexv1.DraftConfigRequest{Description: "We don't get pests; add repotting."}))
	if err != nil {
		t.Fatal(err)
	}
	if p := e.configAnswers.prompts[1]; !strings.Contains(p, `"key": "pests"`) || !strings.Contains(p, "add repotting") {
		t.Errorf("the change request must include the current config: %q", p)
	}
	if w := change.Msg.Warnings; len(w) != 1 || w[0].Kind != pidexv1.ConfigChangeKind_CONFIG_CHANGE_KIND_TAG_REMOVED || w[0].Tag != "Pests" ||
		w[0].Message != `Tag "Pests" is removed. Photos that have it keep it, but it is no longer suggested.` {
		t.Errorf("warnings %v", w)
	}
	pub, err := plantConfig.PublishConfig(e.ctx, connect.NewRequest(&pidexv1.PublishConfigRequest{
		Source: &pidexv1.PublishConfigRequest_ConfigJson{ConfigJson: change.Msg.ConfigJson},
	}))
	if err != nil || pub.Msg.Version != 2 {
		t.Fatalf("publish change: %v %+v", err, pub)
	}

	// 5. Roll back: version 1 comes back as version 3.
	restored, err := plantConfig.PublishConfig(e.ctx, connect.NewRequest(&pidexv1.PublishConfigRequest{
		Source: &pidexv1.PublishConfigRequest_RestoreVersion{RestoreVersion: 1},
	}))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := teamconfig.Parse([]byte(restored.Msg.ConfigJson))
	if err != nil || restored.Msg.Version != 3 || !cfg.HasTag("pests") || cfg.HasTag("repotting") {
		t.Fatalf("restore: %v version %d", err, restored.Msg.Version)
	}
	versions, err := plantConfig.ListConfigVersions(e.ctx, connect.NewRequest(&pidexv1.ListConfigVersionsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	var list []string
	for _, v := range versions.Msg.Versions {
		mark := ""
		if v.Active {
			mark = "*"
		}
		list = append(list, fmt.Sprint(v.Version)+mark)
	}
	if strings.Join(list, ",") != "3*,2,1" {
		t.Errorf("versions %v", list)
	}
	if _, err := plantConfig.PublishConfig(e.ctx, connect.NewRequest(&pidexv1.PublishConfigRequest{
		Source: &pidexv1.PublishConfigRequest_RestoreVersion{RestoreVersion: 9},
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("restoring a missing version: %v", err)
	}
}

func TestScenarioPermissionsAndFailures(t *testing.T) {
	e := setup(t, "construction_renovation")

	// A plain member may draft a scenario of their own but not change the
	// team's config.
	member, err := e.q.UpsertUser(e.ctx, dbq.UpsertUserParams{Email: "member@example.test", DisplayName: "Member"})
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
	cfg := pidexv1connect.NewConfigServiceClient(e.server.Client(), e.server.URL, withToken(token))
	_, err = cfg.DraftConfig(e.ctx, connect.NewRequest(&pidexv1.DraftConfigRequest{Description: "Add a tag for mold."}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("member changing the team config: %v", err)
	}
	e.configAnswers.push(plantsAnswer(tagPests))
	if _, err := cfg.DraftConfig(e.ctx, connect.NewRequest(&pidexv1.DraftConfigRequest{Description: "My plants.", ForNewTeam: true})); err != nil {
		t.Errorf("member drafting a new scenario: %v", err)
	}
	if _, err := cfg.AddSetupMessages(e.ctx, connect.NewRequest(&pidexv1.AddSetupMessagesRequest{Messages: []*pidexv1.SetupMessage{
		{Role: pidexv1.SetupRole_SETUP_ROLE_USER, Text: "Add a tag."},
	}})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("member adding to the setup conversation: %v", err)
	}

	// A model that never produces a valid config gives up with Aborted.
	owner := pidexv1connect.NewConfigServiceClient(e.server.Client(), e.server.URL, withToken(e.token))
	bad := plantsAnswer(tagPests + "," + tagPests)
	e.configAnswers.push(bad, bad, bad)
	_, err = owner.DraftConfig(e.ctx, connect.NewRequest(&pidexv1.DraftConfigRequest{Description: "Plants."}))
	var ce *connect.Error
	if !errors.As(err, &ce) || ce.Code() != connect.CodeAborted || !strings.Contains(ce.Message(), `duplicate tag "pests"`) {
		t.Errorf("invalid drafts: %v", err)
	}

	for _, d := range []string{"", strings.Repeat("x", 5000)} {
		if _, err := owner.DraftConfig(e.ctx, connect.NewRequest(&pidexv1.DraftConfigRequest{Description: d})); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("description of %d bytes: %v", len(d), err)
		}
	}
	team := pidexv1connect.NewTeamServiceClient(e.server.Client(), e.server.URL, withToken(e.token))
	if _, err := team.CreateTeam(e.ctx, connect.NewRequest(&pidexv1.CreateTeamRequest{ConfigJson: `{"name":"x","language":"not a language"}`})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("creating a team with an invalid config: %v", err)
	}
}

// TestSetupConversation keeps the conversation a collection was set up in,
// drafts and all, apart from the photo conversations.
func TestSetupConversation(t *testing.T) {
	e := setup(t, "construction_renovation")
	cfg := pidexv1connect.NewConfigServiceClient(e.server.Client(), e.server.URL, withToken(e.token))
	draft := &pidexv1.DraftConfigResponse{ConfigJson: `{"name":"Plants"}`, Summary: "Files photos by plant.", TestTips: []string{"Photograph a plant."}}
	added, err := cfg.AddSetupMessages(e.ctx, connect.NewRequest(&pidexv1.AddSetupMessagesRequest{Messages: []*pidexv1.SetupMessage{
		{Role: pidexv1.SetupRole_SETUP_ROLE_USER, Text: "I look after houseplants."},
		{Role: pidexv1.SetupRole_SETUP_ROLE_ASSISTANT, Text: draft.Summary, Draft: draft},
		{Role: pidexv1.SetupRole_SETUP_ROLE_EVENT, Text: "Created", ConfigVersion: 1},
	}}))
	if err != nil || len(added.Msg.Messages) != 3 {
		t.Fatalf("add: %v %v", added, err)
	}
	if _, err := cfg.AddSetupMessages(e.ctx, connect.NewRequest(&pidexv1.AddSetupMessagesRequest{Messages: []*pidexv1.SetupMessage{
		{Role: pidexv1.SetupRole_SETUP_ROLE_USER, Text: "Add a tag for pests."},
	}})); err != nil {
		t.Fatal(err)
	}
	got, err := cfg.GetSetupConversation(e.ctx, connect.NewRequest(&pidexv1.GetSetupConversationRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	m := got.Msg.Messages
	if len(m) != 4 || m[0].Text != "I look after houseplants." || m[1].Draft.GetSummary() != draft.Summary ||
		m[1].Draft.GetTestTips()[0] != "Photograph a plant." || m[2].ConfigVersion != 1 || m[3].Role != pidexv1.SetupRole_SETUP_ROLE_USER {
		t.Errorf("conversation as kept: %v", m)
	}
	chats := pidexv1connect.NewChatServiceClient(e.server.Client(), e.server.URL, withToken(e.token))
	if list, err := chats.ListConversations(e.ctx, connect.NewRequest(&pidexv1.ListConversationsRequest{})); err != nil || len(list.Msg.Conversations) != 0 {
		t.Errorf("setup is not among the photo conversations: %v %v", list, err)
	}
	if _, err := cfg.AddSetupMessages(e.ctx, connect.NewRequest(&pidexv1.AddSetupMessagesRequest{Messages: []*pidexv1.SetupMessage{{Text: "no role"}}})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a message without a role: %v", err)
	}
}
