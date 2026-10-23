-- name: SetPhotoThumb :exec
UPDATE photos SET thumb_key = @thumb_key WHERE id = @id;

-- name: SetPhotoIndex :exec
UPDATE photos
SET search_text = @search_text,
    text_embedding = sqlc.narg(text_embedding),
    text_embedding_model = sqlc.narg(text_embedding_model),
    indexed_at = now()
WHERE id = @id;

-- name: ListPhotosToIndex :many
-- Processed photos never indexed, or indexed by another embedding model.
SELECT id FROM photos
WHERE status = 'processed'
  AND (indexed_at IS NULL
       OR thumb_key IS NULL
       OR (@model::text <> '' AND search_text <> '' AND text_embedding_model IS DISTINCT FROM @model::text))
ORDER BY created_at;

-- name: GetPhotosByIDs :many
SELECT * FROM photos WHERE team_id = @team_id AND id = ANY(@ids::uuid[]);

-- name: TeamOverviewCounts :one
SELECT
    (SELECT count(*) FROM photos p
     WHERE p.team_id = @team_id AND p.status = 'processed' AND coalesce(p.role, 'content') <> 'junk')::int AS photo_count,
    (SELECT count(*) FROM subjects s WHERE s.team_id = @team_id AND s.merged_into_id IS NULL)::int AS subject_count;

-- name: TeamTagCounts :many
SELECT pt.tag_key, count(*)::int AS photo_count
FROM photo_tags pt
JOIN photos p ON p.id = pt.photo_id
WHERE p.team_id = @team_id AND NOT pt.rejected AND p.status = 'processed' AND coalesce(p.role, 'content') <> 'junk'
GROUP BY pt.tag_key
ORDER BY photo_count DESC, pt.tag_key
LIMIT @page_limit;
