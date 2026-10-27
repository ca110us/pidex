package e2e

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"connectrpc.com/connect"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/internal/configstore"
)

// put uploads data where a signed upload says.
func (e *env) put(url string, headers map[string]string, data []byte) {
	e.t.Helper()
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(data))
	for k, v := range headers {
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
}

func get(t *testing.T, url string) []byte {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	return data
}

// TestPhotoFiles: after a photo, the file as the camera made it, and a Live
// Photo's motion where the collection keeps them. Phones hear of both
// through sync, fetch them by link, and they go when the photo does.
func TestPhotoFiles(t *testing.T) {
	e := setup(t, "household_pets")
	id := e.take(shot{scene: "cat_ginger", session: "s1", at: 0})
	original, motion := []byte("heic bytes as the camera made them"), []byte("quicktime motion")

	send := func(kind pidexv1.PhotoFileKind, contentType string, data []byte) error {
		up, err := e.capture.CreateFileUpload(e.ctx, connect.NewRequest(&pidexv1.CreateFileUploadRequest{
			PhotoId: id.String(), Kind: kind, ContentType: contentType, SizeBytes: int64(len(data)),
		}))
		if err != nil {
			return err
		}
		if !up.Msg.AlreadyUploaded {
			e.put(up.Msg.UploadUrl, up.Msg.UploadHeaders, data)
		}
		_, err = e.capture.CompleteFileUpload(e.ctx, connect.NewRequest(&pidexv1.CompleteFileUploadRequest{PhotoId: id.String(), Kind: kind}))
		return err
	}

	// Not sent yet: completing says so; a video is not an original.
	if _, err := e.capture.CompleteFileUpload(e.ctx, connect.NewRequest(&pidexv1.CompleteFileUploadRequest{
		PhotoId: id.String(), Kind: pidexv1.PhotoFileKind_PHOTO_FILE_KIND_ORIGINAL,
	})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("completing before uploading: %v", err)
	}
	if err := send(pidexv1.PhotoFileKind_PHOTO_FILE_KIND_ORIGINAL, "video/quicktime", original); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a video as the original: %v", err)
	}

	c := &syncedCopy{photos: map[string]*pidexv1.Photo{}, subjects: map[string]*pidexv1.Subject{}}
	c.sync(e, 10)
	if p := c.photos[id.String()]; p.OriginalKept || p.Live {
		t.Errorf("files before any were sent: %v", p)
	}

	if err := send(pidexv1.PhotoFileKind_PHOTO_FILE_KIND_ORIGINAL, "image/heic", original); err != nil {
		t.Fatal(err)
	}
	// Sent again (a retry after a lost answer): nothing changes.
	if err := send(pidexv1.PhotoFileKind_PHOTO_FILE_KIND_ORIGINAL, "image/heic", original); err != nil {
		t.Fatal(err)
	}
	// The collection keeps no Live Photos.
	if err := send(pidexv1.PhotoFileKind_PHOTO_FILE_KIND_LIVE, "video/quicktime", motion); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("motion for a collection without Live Photos: %v", err)
	}
	cfg, _, err := e.proc.Configs.Active(e.ctx, e.teamID)
	if err != nil {
		t.Fatal(err)
	}
	live := *cfg
	live.LivePhotos = true
	if _, err := configstore.Publish(e.ctx, e.pool, e.teamID, &e.userID, &live, "test"); err != nil {
		t.Fatal(err)
	}
	if err := send(pidexv1.PhotoFileKind_PHOTO_FILE_KIND_LIVE, "video/quicktime", motion); err != nil {
		t.Fatal(err)
	}

	changed, _, _ := c.sync(e, 10)
	if p := c.photos[id.String()]; len(changed) == 0 || !p.OriginalKept || !p.Live {
		t.Errorf("files not synced: %v", p)
	}
	links, err := e.library.GetPhotoLinks(e.ctx, connect.NewRequest(&pidexv1.GetPhotoLinksRequest{Ids: []string{id.String()}}))
	if err != nil {
		t.Fatal(err)
	}
	l := links.Msg.Links[id.String()]
	if !bytes.Equal(get(t, l.GetOriginalUrl()), original) || !bytes.Equal(get(t, l.GetLiveUrl()), motion) {
		t.Errorf("files by link: %v", l)
	}
	if bytes.Equal(get(t, l.GetViewUrl()), original) {
		t.Error("the working copy was replaced")
	}

	// Deleting the photo deletes its files.
	if _, err := e.library.DeletePhotos(e.ctx, connect.NewRequest(&pidexv1.DeletePhotosRequest{PhotoIds: []string{id.String()}})); err != nil {
		t.Fatal(err)
	}
	if get(t, l.GetOriginalUrl()) != nil || get(t, l.GetLiveUrl()) != nil {
		t.Error("files of a deleted photo remain")
	}
}
