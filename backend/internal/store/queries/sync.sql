-- name: SyncHorizon :one
-- Every transaction below this id has ended: what it wrote is visible.
SELECT pg_snapshot_xmin(pg_current_snapshot())::text::bigint AS horizon;

-- name: ListPhotosChangedSince :many
SELECT * FROM photos
WHERE team_id = @team_id AND sync_xid >= @since::bigint AND id > @after::uuid
ORDER BY id
LIMIT @page_limit;

-- name: ListSubjectsChangedSince :many
SELECT * FROM subjects
WHERE team_id = @team_id AND sync_xid >= @since::bigint
ORDER BY id;

-- name: ListTombstonesSince :many
SELECT kind, id FROM sync_tombstones
WHERE team_id = @team_id AND sync_xid >= @since::bigint;

-- name: PruneTombstones :execrows
DELETE FROM sync_tombstones WHERE deleted_at < @before;

-- name: ListPhotoKeys :many
SELECT id, object_key, thumb_key, status FROM photos
WHERE team_id = @team_id AND id = ANY(@ids::uuid[]);
