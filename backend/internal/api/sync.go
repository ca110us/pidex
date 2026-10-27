package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/internal/store"
	"pidex/internal/store/dbq"
)

// Phones keep a copy of each collection and ask for what changed since a
// cursor. Every photo and subject carries the transaction that last wrote
// it, and deletions leave tombstones (migration 00012).
//
// A pass reads every row stamped at or after since, in id order, a page at
// a time. Before its first page it notes the horizon: every transaction
// below it has ended, so the next pass, from the horizon, cannot miss a
// row committed late, and a row written during a pass comes again in the
// next. Rows sent twice are kept once.
const (
	// syncReset starts a phone again from nothing a day before the
	// deletions since its cursor may have been pruned.
	syncReset          = store.TombstoneRetention - 24*time.Hour
	defaultSyncPage    = 500
	maxSyncPage        = 1000
	maxLinksPerRequest = 200
)

type syncCursor struct {
	// Since: rows stamped at or after it are this pass's.
	Since int64 `json:"s,omitempty"`
	// Horizon, noted before the pass's first page, starts the next pass.
	Horizon int64 `json:"h,omitempty"`
	// At is when the pass that ends at Since began.
	At int64 `json:"t,omitempty"`
	// After is the last photo sent in this pass; unset between passes.
	After *uuid.UUID `json:"a,omitempty"`
	// HorizonAt is when Horizon was noted.
	HorizonAt int64 `json:"ht,omitempty"`
}

func (c syncCursor) encode() string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeSyncCursor(s string) (syncCursor, error) {
	var c syncCursor
	if s == "" {
		return c, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err == nil {
		err = json.Unmarshal(b, &c)
	}
	if err != nil {
		return c, invalid("cursor is not one this server gave")
	}
	return c, nil
}

func (s *libraryService) SyncLibrary(ctx context.Context, req *connect.Request[pidexv1.SyncLibraryRequest]) (*connect.Response[pidexv1.SyncLibraryResponse], error) {
	pr := principalFrom(ctx)
	c, err := decodeSyncCursor(req.Msg.Cursor)
	if err != nil {
		return nil, err
	}
	out := &pidexv1.SyncLibraryResponse{}
	now := time.Now()
	if c.Since > 0 && now.Sub(time.Unix(c.At, 0)) > syncReset {
		c = syncCursor{}
		out.StartOver = true
	}
	limit := req.Msg.PageSize
	if limit <= 0 {
		limit = defaultSyncPage
	}
	limit = min(limit, maxSyncPage)

	first := c.After == nil
	if first {
		if c.Horizon, err = s.Q.SyncHorizon(ctx); err != nil {
			return nil, internal(err)
		}
		c.HorizonAt = now.Unix()
		// Subjects are few: all of a pass's go with its first page, with
		// what was deleted.
		subs, err := s.Q.ListSubjectsChangedSince(ctx, dbq.ListSubjectsChangedSinceParams{TeamID: pr.TeamID, Since: c.Since})
		if err != nil {
			return nil, internal(err)
		}
		for _, sub := range subs {
			if sub.MergedIntoID != nil {
				out.DeletedSubjectIds = append(out.DeletedSubjectIds, sub.ID.String())
			} else {
				out.Subjects = append(out.Subjects, subjectProto(sub, 0))
			}
		}
		if c.Since > 0 {
			gone, err := s.Q.ListTombstonesSince(ctx, dbq.ListTombstonesSinceParams{TeamID: pr.TeamID, Since: c.Since})
			if err != nil {
				return nil, internal(err)
			}
			for _, g := range gone {
				if g.Kind == "photo" {
					out.DeletedPhotoIds = append(out.DeletedPhotoIds, g.ID.String())
				} else {
					out.DeletedSubjectIds = append(out.DeletedSubjectIds, g.ID.String())
				}
			}
		}
	}
	after := uuid.Nil
	if c.After != nil {
		after = *c.After
	}
	rows, err := s.Q.ListPhotosChangedSince(ctx, dbq.ListPhotosChangedSinceParams{
		TeamID: pr.TeamID, Since: c.Since, After: after, PageLimit: limit + 1,
	})
	if err != nil {
		return nil, internal(err)
	}
	if len(rows) > int(limit) {
		rows = rows[:limit]
		last := rows[len(rows)-1].ID
		c.After = &last
		out.More = true
	} else {
		// The pass is done: the next starts at its horizon.
		c = syncCursor{Since: c.Horizon, At: c.HorizonAt}
	}
	if len(rows) > 0 {
		ids := make([]uuid.UUID, len(rows))
		for i, p := range rows {
			ids[i] = p.ID
		}
		tags, err := s.Q.ListPhotoTags(ctx, ids)
		if err != nil {
			return nil, internal(err)
		}
		names, err := subjectNames(ctx, s.Q, rows)
		if err != nil {
			return nil, err
		}
		for _, p := range rows {
			out.Photos = append(out.Photos, photoData(p, tags, names))
		}
	}
	out.Cursor = c.encode()
	return connect.NewResponse(out), nil
}

func (s *libraryService) GetPhotoLinks(ctx context.Context, req *connect.Request[pidexv1.GetPhotoLinksRequest]) (*connect.Response[pidexv1.GetPhotoLinksResponse], error) {
	pr := principalFrom(ctx)
	if len(req.Msg.Ids) > maxLinksPerRequest {
		return nil, invalid("at most %d photos at a time", maxLinksPerRequest)
	}
	ids := make([]uuid.UUID, 0, len(req.Msg.Ids))
	for _, raw := range req.Msg.Ids {
		id, err := parseID("ids", raw)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	rows, err := s.Q.ListPhotoKeys(ctx, dbq.ListPhotoKeysParams{TeamID: pr.TeamID, Ids: ids})
	if err != nil {
		return nil, internal(err)
	}
	out := &pidexv1.GetPhotoLinksResponse{Links: make(map[string]*pidexv1.PhotoLinks, len(rows))}
	for _, r := range rows {
		view, thumb := s.photoLinks(ctx, r.Status, r.ObjectKey, r.ThumbKey)
		out.Links[r.ID.String()] = &pidexv1.PhotoLinks{ViewUrl: view, ThumbnailUrl: thumb}
	}
	return connect.NewResponse(out), nil
}
