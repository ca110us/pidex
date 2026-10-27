package store

import (
	"context"
	"time"

	"pidex/internal/store/dbq"
)

// TombstoneRetention is how long deletions are remembered for phones that
// keep a copy of a collection (migration 00012); a phone away longer starts
// again from nothing.
const TombstoneRetention = 30 * 24 * time.Hour

// PruneTombstones forgets deletions older than TombstoneRetention.
func PruneTombstones(ctx context.Context, q *dbq.Queries) (int64, error) {
	return q.PruneTombstones(ctx, time.Now().Add(-TombstoneRetention))
}
