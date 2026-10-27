package e2e

import (
	"encoding/base64"
	"fmt"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/internal/store"
)

// syncedCopy is what a phone keeps of a collection.
type syncedCopy struct {
	cursor   string
	photos   map[string]*pidexv1.Photo
	subjects map[string]*pidexv1.Subject
}

// sync asks for changes until there are no more, as the phone does, and
// returns what this round changed.
func (c *syncedCopy) sync(e *env, pageSize int32) (changed, deleted []string, startedOver bool) {
	e.t.Helper()
	for {
		res, err := e.library.SyncLibrary(e.ctx, connect.NewRequest(&pidexv1.SyncLibraryRequest{Cursor: c.cursor, PageSize: pageSize}))
		if err != nil {
			e.t.Fatal(err)
		}
		m := res.Msg
		if m.StartOver {
			startedOver = true
			c.photos, c.subjects = map[string]*pidexv1.Photo{}, map[string]*pidexv1.Subject{}
		}
		for _, p := range m.Photos {
			if p.ViewUrl != "" || p.ThumbnailUrl != "" {
				e.t.Error("a synced photo carries links")
			}
			c.photos[p.Id] = p
			changed = append(changed, p.Id)
		}
		for _, s := range m.Subjects {
			c.subjects[s.Id] = s
		}
		for _, id := range m.DeletedPhotoIds {
			delete(c.photos, id)
			deleted = append(deleted, id)
		}
		for _, id := range m.DeletedSubjectIds {
			delete(c.subjects, id)
		}
		c.cursor = m.Cursor
		if !m.More {
			return changed, deleted, startedOver
		}
	}
}

func idSet(ids ...uuid.UUID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	slices.Sort(out)
	return out
}

func sortedUnique(xs []string) []string {
	out := slices.Clone(xs)
	slices.Sort(out)
	return slices.Compact(out)
}

// TestSyncLibrary: a phone keeps a copy of a collection, page by page, and
// then hears only of what changed: new photos, tags changed, a subject
// renamed (its photos' descriptions name it), photos and subjects deleted,
// and a write that commits after the phone asked. Away too long, it starts
// again from nothing.
func TestSyncLibrary(t *testing.T) {
	e := setup(t, "household_pets")
	min := func(m float64) time.Duration { return time.Duration(m * float64(time.Minute)) }
	g1 := e.take(shot{scene: "cat_ginger", session: "s1", at: 0})
	g2 := e.take(shot{scene: "cat_ginger", session: "s2", at: min(90)})
	t1 := e.take(shot{scene: "cat_tuxedo", session: "s3", at: min(200)})
	ginger := *e.photo(g1).SubjectID

	// From nothing, two photos a page.
	c := &syncedCopy{photos: map[string]*pidexv1.Photo{}, subjects: map[string]*pidexv1.Subject{}}
	changed, _, _ := c.sync(e, 2)
	if got := sortedUnique(changed); !slices.Equal(got, idSet(g1, g2, t1)) {
		t.Fatalf("first sync: %v", got)
	}
	if s := c.subjects[ginger.String()]; s == nil || s.DisplayName == "" {
		t.Errorf("subjects not kept: %v", c.subjects)
	}
	if p := c.photos[g1.String()]; p.CreatedAt == nil || p.Status != pidexv1.PhotoStatus_PHOTO_STATUS_PROCESSED || len(p.Tags) == 0 {
		t.Errorf("a synced photo: %v", p)
	}

	// Nothing changed: nothing comes.
	if changed, deleted, _ := c.sync(e, 2); len(changed)+len(deleted) != 0 {
		t.Errorf("unchanged, yet %v %v", changed, deleted)
	}

	// A new photo, a tag added by hand, a subject renamed.
	t2 := e.take(shot{scene: "cat_tuxedo", session: "s4", at: min(300)})
	if _, err := e.library.UpdatePhotoTags(e.ctx, connect.NewRequest(&pidexv1.UpdatePhotoTagsRequest{PhotoId: t1.String(), Add: []string{"vet_visit"}})); err != nil {
		t.Fatal(err)
	}
	if _, err := e.library.RenameSubject(e.ctx, connect.NewRequest(&pidexv1.RenameSubjectRequest{SubjectId: ginger.String(), DisplayName: "Mango"})); err != nil {
		t.Fatal(err)
	}
	changed, _, _ = c.sync(e, 2)
	if got := sortedUnique(changed); !slices.Equal(got, idSet(g1, g2, t1, t2)) {
		t.Errorf("after changes: %v", got)
	}
	if c.subjects[ginger.String()].DisplayName != "Mango" {
		t.Errorf("rename not synced: %v", c.subjects[ginger.String()])
	}
	if !slices.ContainsFunc(c.photos[t1.String()].Tags, func(tag *pidexv1.Tag) bool { return tag.Key == "vet_visit" }) {
		t.Error("tag not synced")
	}

	// A write that commits only after the phone asked still arrives.
	tx, err := e.pool.Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(e.ctx, `UPDATE photos SET assignment_confidence = 0.42 WHERE id = $1`, g2); err != nil {
		t.Fatal(err)
	}
	if changed, _, _ := c.sync(e, 2); slices.Contains(changed, g2.String()) {
		t.Error("an uncommitted write was sent")
	}
	if err := tx.Commit(e.ctx); err != nil {
		t.Fatal(err)
	}
	if changed, _, _ := c.sync(e, 2); !slices.Contains(changed, g2.String()) || c.photos[g2.String()].AssignmentConfidence != 0.42 {
		t.Errorf("a late commit was missed: %v", changed)
	}

	// Deleted photos are dropped from the copy, with a subject left empty.
	if _, err := e.library.DeletePhotos(e.ctx, connect.NewRequest(&pidexv1.DeletePhotosRequest{PhotoIds: []string{t1.String(), t2.String()}})); err != nil {
		t.Fatal(err)
	}
	_, deleted, _ := c.sync(e, 2)
	if got := sortedUnique(deleted); !slices.Equal(got, idSet(t1, t2)) {
		t.Errorf("deleted: %v", got)
	}
	if got := sortedUnique(mapsKeys(c.photos)); !slices.Equal(got, idSet(g1, g2)) {
		t.Errorf("the copy after deleting: %v", got)
	}
	if len(c.subjects) != 1 {
		t.Errorf("the emptied subject is still kept: %v", c.subjects)
	}

	// Links for photos the phone knows: only this collection's. (Tests make
	// no thumbnails; indexing does.)
	links, err := e.library.GetPhotoLinks(e.ctx, connect.NewRequest(&pidexv1.GetPhotoLinksRequest{Ids: []string{g1.String(), uuid.NewString()}}))
	if err != nil || len(links.Msg.Links) != 1 || links.Msg.Links[g1.String()].GetViewUrl() == "" {
		t.Errorf("links: %v %v", links, err)
	}
	if _, err := e.library.GetPhotoLinks(e.ctx, connect.NewRequest(&pidexv1.GetPhotoLinksRequest{Ids: make([]string, 201)})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("too many links at once: %v", err)
	}

	// Away longer than deletions are remembered: start again.
	old := base64.RawURLEncoding.EncodeToString(fmt.Appendf(nil, `{"s":1,"t":%d}`, time.Now().Add(-store.TombstoneRetention).Unix()))
	c.cursor = old
	changed, _, over := c.sync(e, 2)
	if !over || !slices.Equal(sortedUnique(changed), idSet(g1, g2)) || len(c.photos) != 2 {
		t.Errorf("starting over: %v %v", over, changed)
	}

	// Old tombstones are pruned.
	if _, err := e.pool.Exec(e.ctx, `UPDATE sync_tombstones SET deleted_at = now() - interval '31 days'`); err != nil {
		t.Fatal(err)
	}
	if n, err := store.PruneTombstones(e.ctx, e.q); err != nil || n == 0 {
		t.Errorf("pruned %d: %v", n, err)
	}
}

func mapsKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
