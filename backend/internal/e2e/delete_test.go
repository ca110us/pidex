package e2e

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/gen/pidex/v1/pidexv1connect"
	"pidex/internal/api"
	"pidex/internal/store/dbq"
)

// TestDeleteCollection deletes a team with its photos, their files and
// everything else, only when its owner names it.
func TestDeleteCollection(t *testing.T) {
	e := setup(t, "construction_renovation")
	unit := e.addSubject("unit", "Maple Court · 203", map[string]string{"building": "Maple Court", "unit_number": "203"}, nil)
	photo := e.addPhoto(fixturePhoto{subject: &unit, description: "Tiling.", tags: []string{"tiling"}})
	stored := e.photo(photo)
	if stored.ThumbKey == nil {
		t.Fatal("the fixture photo has no thumbnail")
	}

	// A member cannot, and the owner must name it.
	member, err := e.q.UpsertUser(e.ctx, dbq.UpsertUserParams{Email: "member@example.test", DisplayName: "Member"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.q.AddTeamMember(e.ctx, dbq.AddTeamMemberParams{TeamID: e.teamID, UserID: member.ID, Role: "admin"}); err != nil {
		t.Fatal(err)
	}
	token, hash := api.NewToken()
	if err := e.q.CreateAPIToken(e.ctx, dbq.CreateAPITokenParams{TokenSha256: hash, TeamID: e.teamID, UserID: member.ID}); err != nil {
		t.Fatal(err)
	}
	admin := pidexv1connect.NewTeamServiceClient(e.server.Client(), e.server.URL, withToken(token))
	if _, err := admin.DeleteTeam(e.ctx, connect.NewRequest(&pidexv1.DeleteTeamRequest{ConfirmName: "E2E construction_renovation"})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("an admin deleting the collection: %v", err)
	}
	owner := pidexv1connect.NewTeamServiceClient(e.server.Client(), e.server.URL, withToken(e.token))
	if _, err := owner.DeleteTeam(e.ctx, connect.NewRequest(&pidexv1.DeleteTeamRequest{ConfirmName: "E2E"})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("deleting without the right name: %v", err)
	}

	res, err := owner.DeleteTeam(e.ctx, connect.NewRequest(&pidexv1.DeleteTeamRequest{ConfirmName: "E2E construction_renovation"}))
	if err != nil || res.Msg.DeletedPhotos != 1 {
		t.Fatalf("delete: %v %v", res, err)
	}
	for _, key := range []string{stored.ObjectKey, *stored.ThumbKey} {
		if _, exists, err := e.objects.Head(e.ctx, key); err != nil || exists {
			t.Errorf("file %s still stored: %v", key, err)
		}
	}
	var left int
	if err := e.pool.QueryRow(e.ctx, `SELECT (SELECT count(*) FROM photos WHERE team_id = $1) + (SELECT count(*) FROM subjects WHERE team_id = $1) + (SELECT count(*) FROM api_tokens WHERE team_id = $1)`, e.teamID).Scan(&left); err != nil || left != 0 {
		t.Errorf("%d rows left: %v", left, err)
	}
	if _, err := owner.GetTeam(e.ctx, connect.NewRequest(&pidexv1.GetTeamRequest{})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("its token after deletion: %v", err)
	}
	// Work queued for its photos finds nothing to do.
	if err := e.proc.ProcessPhoto(e.ctx, photo); err != nil {
		t.Errorf("processing a deleted photo: %v", err)
	}
}

// TestDeletePhotos deletes photos with what exists only because of them:
// files, the subject an identity photo made, its place in conversations,
// and the filing of the photos taken after it.
func TestDeletePhotos(t *testing.T) {
	e := setup(t, "construction_renovation")
	sign := e.take(shot{scene: "site_sign", session: "s1", at: 0})
	tile := e.take(shot{scene: "tiling", session: "s1", at: time.Minute})
	project := e.photo(sign).SubjectID
	if project == nil {
		t.Fatal("the sign made no subject")
	}
	e.expectFiled("tiling after the sign", tile, project, "auto")

	conv, err := e.q.CreateConversation(e.ctx, dbq.CreateConversationParams{TeamID: e.teamID, UserID: e.userID, Title: "tiling"})
	if err != nil {
		t.Fatal(err)
	}
	kind := "photos"
	if _, err := e.q.AddConversationMessage(e.ctx, dbq.AddConversationMessageParams{
		ConversationID: conv.ID, Role: "assistant", Intent: &kind, PhotoIds: []uuid.UUID{sign, tile}, MissingSubjectIds: []uuid.UUID{},
	}); err != nil {
		t.Fatal(err)
	}

	// A member deletes only their own photos.
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
	memberLib := pidexv1connect.NewLibraryServiceClient(e.server.Client(), e.server.URL, withToken(token))
	if _, err := memberLib.DeletePhotos(e.ctx, connect.NewRequest(&pidexv1.DeletePhotosRequest{PhotoIds: []string{sign.String()}})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("a member deleting someone else's photo: %v", err)
	}

	stored := e.photo(sign)
	res, err := e.library.DeletePhotos(e.ctx, connect.NewRequest(&pidexv1.DeletePhotosRequest{PhotoIds: []string{sign.String()}}))
	if err != nil || res.Msg.DeletedPhotos != 1 || res.Msg.DeletedSubjects != 1 {
		t.Fatalf("delete: %v %v", res, err)
	}
	if _, exists, _ := e.objects.Head(e.ctx, stored.ObjectKey); exists {
		t.Error("the photo's file is still stored")
	}
	if p := e.photo(tile); p.SubjectID != nil || p.Assignment != "unassigned" {
		t.Errorf("the photo after the deleted sign is still filed: %v %s", p.SubjectID, p.Assignment)
	}
	var subjects, inMessages int
	if err := e.pool.QueryRow(e.ctx, `SELECT (SELECT count(*) FROM subjects WHERE team_id = $1),
		(SELECT count(*) FROM conversation_messages WHERE $2 = ANY(photo_ids))`, e.teamID, sign).Scan(&subjects, &inMessages); err != nil {
		t.Fatal(err)
	}
	if subjects != 0 || inMessages != 0 {
		t.Errorf("left behind: %d subjects, %d messages showing the photo", subjects, inMessages)
	}
	if _, err := e.library.DeletePhotos(e.ctx, connect.NewRequest(&pidexv1.DeletePhotosRequest{PhotoIds: []string{sign.String()}})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("deleting it again: %v", err)
	}
}

// TestDeleteSubject deletes a subject, keeping its photos unfiled or
// deleting them too, and files its session again without it.
func TestDeleteSubject(t *testing.T) {
	e := setup(t, "construction_renovation")
	sign := e.take(shot{scene: "site_sign", session: "s1", at: 0})
	door := e.take(shot{scene: "unit_door", session: "s1", at: time.Minute})
	tile := e.take(shot{scene: "tiling", session: "s1", at: 2 * time.Minute})
	project, unit := e.photo(sign).SubjectID, e.photo(door).SubjectID
	if project == nil || unit == nil {
		t.Fatal("the sign and the door made no subjects")
	}
	e.expectFiled("tiling in the unit", tile, unit, "auto")

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
	memberLib := pidexv1connect.NewLibraryServiceClient(e.server.Client(), e.server.URL, withToken(token))
	if _, err := memberLib.DeleteSubject(e.ctx, connect.NewRequest(&pidexv1.DeleteSubjectRequest{SubjectId: unit.String()})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("a member deleting a subject: %v", err)
	}

	// The unit goes; its photos stay, filed under nothing, and the photo
	// after its door plate no longer follows it.
	res, err := e.library.DeleteSubject(e.ctx, connect.NewRequest(&pidexv1.DeleteSubjectRequest{SubjectId: unit.String()}))
	if err != nil || res.Msg.DeletedPhotos != 0 || res.Msg.UnfiledPhotos != 2 {
		t.Fatalf("delete the unit: %v %v", res, err)
	}
	e.expectFiled("door plate of a deleted unit", door, nil, "unassigned")
	e.expectFiled("tiling of a deleted unit", tile, nil, "unassigned")

	// The project goes with its sign photo.
	res, err = e.library.DeleteSubject(e.ctx, connect.NewRequest(&pidexv1.DeleteSubjectRequest{SubjectId: project.String(), WithPhotos: true}))
	if err != nil || res.Msg.DeletedPhotos != 1 {
		t.Fatalf("delete the project with its photos: %v %v", res, err)
	}
	var subjects, photos int
	if err := e.pool.QueryRow(e.ctx, `SELECT (SELECT count(*) FROM subjects WHERE team_id = $1), (SELECT count(*) FROM photos WHERE team_id = $1)`, e.teamID).Scan(&subjects, &photos); err != nil {
		t.Fatal(err)
	}
	if subjects != 0 || photos != 2 {
		t.Errorf("left: %d subjects, %d photos (want 0 and the 2 unfiled)", subjects, photos)
	}
	if _, err := e.library.DeleteSubject(e.ctx, connect.NewRequest(&pidexv1.DeleteSubjectRequest{SubjectId: project.String()})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("deleting it again: %v", err)
	}
}
