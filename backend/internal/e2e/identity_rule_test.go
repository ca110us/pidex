package e2e

import (
	"bytes"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	pidexv1 "pidex/gen/pidex/v1"
)

// TestUploadsSayWhoTheyShow: a photo that names neither its subject nor the
// shot identifying it is refused, as is one naming a shot from elsewhere.
func TestUploadsSayWhoTheyShow(t *testing.T) {
	e := setup(t, "construction_renovation")
	session := uuid.NewString()
	upload := func(clientID string) string {
		t.Helper()
		data := jpegOf(scenes["tiling"].c)
		up, err := e.capture.CreateUpload(e.ctx, connect.NewRequest(&pidexv1.CreateUploadRequest{
			ClientPhotoId: clientID, ContentType: "image/jpeg", SizeBytes: int64(len(data)),
		}))
		if err != nil {
			t.Fatal(err)
		}
		req, _ := http.NewRequest(http.MethodPut, up.Msg.UploadUrl, bytes.NewReader(data))
		for k, v := range up.Msg.UploadHeaders {
			if k != "Content-Length" {
				req.Header.Set(k, v)
			}
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return up.Msg.PhotoId
	}
	complete := func(photoID, identity string) error {
		_, err := e.capture.CompleteUpload(e.ctx, connect.NewRequest(&pidexv1.CompleteUploadRequest{
			PhotoId: photoID, SessionId: session, IdentityClientPhotoId: identity,
		}))
		return err
	}

	label, after, stray := uuid.NewString(), uuid.NewString(), uuid.NewString()
	labelPhoto, afterPhoto, strayPhoto := upload(label), upload(after), upload(stray)
	if err := complete(strayPhoto, ""); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a photo saying nothing of who it shows: %v", err)
	}
	if err := complete(afterPhoto, label); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("naming a shot not uploaded yet: %v", err)
	}
	if err := complete(labelPhoto, label); err != nil {
		t.Errorf("a shot identifying itself: %v", err)
	}
	if err := complete(afterPhoto, label); err != nil {
		t.Errorf("a photo after its identity shot: %v", err)
	}
	if err := complete(strayPhoto, uuid.NewString()); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("naming a shot that does not exist: %v", err)
	}
}
