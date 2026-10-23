package e2e

import (
	"testing"

	"connectrpc.com/connect"

	pidexv1 "pidex/gen/pidex/v1"
)

// TestIdentifyBeforeUpload is the camera asking "who is this?" about a shot
// still on the phone: first from the phone's own text, then, when that
// cannot tell, by having the vision model read the shot.
func TestIdentifyBeforeUpload(t *testing.T) {
	e := setup(t, "construction_renovation")
	created, err := e.library.CreateSubject(e.ctx, connect.NewRequest(&pidexv1.CreateSubjectRequest{
		TypeKey: "project", Identity: map[string]string{"project_name": "Maple Court", "project_code": "MC-2041"},
	}))
	if err != nil {
		t.Fatal(err)
	}

	// A screen full of numbers: no single one reads as the project code,
	// but a known code is among them.
	screen := &pidexv1.OnDeviceAnalysis{Text: text("Version 18.1", "Storage 256 GB", "Code MC-2041", "Build 22B83")}
	r, err := e.capture.ResolveAnchor(e.ctx, connect.NewRequest(&pidexv1.ResolveAnchorRequest{OnDevice: screen}))
	if err != nil {
		t.Fatal(err)
	}
	if r.Msg.State != pidexv1.AnchorState_ANCHOR_STATE_MATCHED || r.Msg.Subject.GetId() != created.Msg.Subject.Id {
		t.Errorf("known code among other text: %v %v", r.Msg.State, r.Msg.Subject)
	}
	other := &pidexv1.OnDeviceAnalysis{Text: text("Version 18.1", "Storage 256 GB")}
	if r, err := e.capture.ResolveAnchor(e.ctx, connect.NewRequest(&pidexv1.ResolveAnchorRequest{OnDevice: other})); err != nil || r.Msg.State != pidexv1.AnchorState_ANCHOR_STATE_NOT_ANCHOR {
		t.Errorf("no known code: %v %v", r, err)
	}

	// The phone cannot read a door plate it has never seen; the model can.
	calls := e.calls.Load()
	shot, err := e.capture.IdentifyShot(e.ctx, connect.NewRequest(&pidexv1.IdentifyShotRequest{
		Jpeg: jpegOf(scenes["unit_door"].c), OnDevice: &pidexv1.OnDeviceAnalysis{Text: text("12B")},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if a := shot.Msg.Anchor; a.State != pidexv1.AnchorState_ANCHOR_STATE_NEW_SUBJECT || a.ExtractedIdentity["unit_number"] != "12B" {
		t.Errorf("model read of a new unit: %v", a)
	}
	if e.calls.Load() != calls+1 {
		t.Errorf("one model call, got %d", e.calls.Load()-calls)
	}
	nothing, err := e.capture.IdentifyShot(e.ctx, connect.NewRequest(&pidexv1.IdentifyShotRequest{Jpeg: jpegOf(scenes["tiling"].c)}))
	if err != nil || nothing.Msg.Anchor.State != pidexv1.AnchorState_ANCHOR_STATE_NOT_ANCHOR {
		t.Errorf("a work photo identifies nobody: %v %v", nothing, err)
	}
	if subjects, _ := e.library.ListSubjects(e.ctx, connect.NewRequest(&pidexv1.ListSubjectsRequest{})); len(subjects.Msg.Subjects) != 1 {
		t.Errorf("identifying stores nothing: %d subjects", len(subjects.Msg.Subjects))
	}
}
