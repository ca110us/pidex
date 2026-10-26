-- CreatePhotoUpload is idempotent on (team_id, client_photo_id): a retry
-- returns the row created by the first attempt.
-- name: CreatePhotoUpload :one
INSERT INTO photos (id, team_id, uploaded_by, client_photo_id, object_key, content_type, size_bytes, sha256_hex)
VALUES (@id, @team_id, @uploaded_by, @client_photo_id, @object_key, @content_type, @size_bytes, @sha256_hex)
ON CONFLICT (team_id, client_photo_id) DO UPDATE SET updated_at = now()
RETURNING *;

-- name: GetPhoto :one
SELECT * FROM photos WHERE team_id = @team_id AND id = @id;

-- name: GetPhotoByID :one
SELECT * FROM photos WHERE id = @id;

-- name: CompletePhotoUpload :one
UPDATE photos SET
    status              = 'uploaded',
    received_at         = coalesce(received_at, now()),
    session_id          = sqlc.narg(session_id),
    captured_at         = @captured_at,
    latitude            = sqlc.narg(latitude),
    longitude           = sqlc.narg(longitude),
    location_accuracy_m = sqlc.narg(location_accuracy_m),
    on_device           = @on_device,
    locked_subject_id   = sqlc.narg(locked_subject_id),
    config_version      = @config_version,
    size_bytes          = @size_bytes,
    updated_at          = now()
WHERE team_id = @team_id AND id = @id AND status IN ('awaiting_upload', 'uploaded')
RETURNING *;

-- name: MarkPhotoProcessing :one
UPDATE photos SET
    status              = 'processing',
    processing_attempts = processing_attempts + 1,
    updated_at          = now()
WHERE id = @id
RETURNING *;

-- name: SavePhotoAnalysis :exec
UPDATE photos SET
    status           = 'processed',
    role             = @role,
    extracted        = @extracted,
    description      = @description,
    embedding        = sqlc.narg(embedding),
    processing_error = '',
    updated_at       = now()
WHERE id = @id;

-- name: MarkPhotoFailed :exec
UPDATE photos SET status = 'failed', processing_error = @processing_error, updated_at = now()
WHERE id = @id;

-- name: ListSessionPhotos :many
SELECT * FROM photos
WHERE team_id = @team_id AND session_id = @session_id
ORDER BY captured_at NULLS LAST, created_at;

-- UpdateAutoAssignment never overrides a person's decision.
-- name: UpdateAutoAssignment :execrows
UPDATE photos SET
    subject_id            = sqlc.narg(subject_id),
    suggested_subject_id  = sqlc.narg(suggested_subject_id),
    assignment            = @assignment,
    assignment_confidence = @assignment_confidence,
    updated_at            = now()
WHERE id = @id AND assignment <> 'manual';

-- name: ManualAssignPhotos :many
UPDATE photos SET
    subject_id            = @subject_id,
    suggested_subject_id  = NULL,
    assignment            = 'manual',
    assignment_confidence = 1,
    updated_at            = now()
WHERE team_id = @team_id AND id = ANY(@ids::uuid[])
RETURNING *;

-- name: ListPhotos :many
SELECT p.* FROM photos p
WHERE p.team_id = @team_id
  AND (sqlc.narg(subject_id)::uuid IS NULL OR p.subject_id = sqlc.narg(subject_id)::uuid)
  AND (sqlc.narg(role)::text IS NULL OR p.role = sqlc.narg(role)::text)
  AND (cardinality(@assignments::text[]) = 0 OR p.assignment = ANY(@assignments::text[]))
  AND (
    cardinality(@tag_keys::text[]) = 0
    OR (
      SELECT count(*) FROM photo_tags t
      WHERE t.photo_id = p.id AND NOT t.rejected AND t.tag_key = ANY(@tag_keys::text[])
    ) = cardinality(@tag_keys::text[])
  )
  AND (
    sqlc.narg(before_created)::timestamptz IS NULL
    OR (p.created_at, p.id) < (sqlc.narg(before_created)::timestamptz, sqlc.narg(before_id)::uuid)
  )
ORDER BY p.created_at DESC, p.id DESC
LIMIT @page_limit;

-- name: ListSubjectPhotos :many
SELECT * FROM photos
WHERE team_id = @team_id AND subject_id = @subject_id
ORDER BY captured_at NULLS LAST, created_at;

-- name: UpsertAITag :exec
INSERT INTO photo_tags (photo_id, tag_key, confidence, source)
VALUES (@photo_id, @tag_key, @confidence, 'ai')
ON CONFLICT (photo_id, tag_key) DO UPDATE SET confidence = EXCLUDED.confidence
WHERE photo_tags.source = 'ai' AND NOT photo_tags.rejected;

-- name: DeleteStaleAITags :exec
DELETE FROM photo_tags
WHERE photo_id = @photo_id AND source = 'ai' AND NOT rejected AND NOT (tag_key = ANY(@keep::text[]));

-- name: AddUserTag :exec
INSERT INTO photo_tags (photo_id, tag_key, confidence, source)
VALUES (@photo_id, @tag_key, 1, 'user')
ON CONFLICT (photo_id, tag_key) DO UPDATE SET confidence = 1, source = 'user', rejected = false;

-- name: RejectTag :exec
INSERT INTO photo_tags (photo_id, tag_key, confidence, source, rejected)
VALUES (@photo_id, @tag_key, 0, 'user', true)
ON CONFLICT (photo_id, tag_key) DO UPDATE SET source = 'user', rejected = true;

-- name: ListPhotoTags :many
SELECT * FROM photo_tags
WHERE photo_id = ANY(@photo_ids::uuid[]) AND NOT rejected
ORDER BY confidence DESC;

-- name: DeletePhotos :many
-- Tags go with the photo (cascade); cost records keep their sums, unlinked.
DELETE FROM photos WHERE team_id = @team_id AND id = ANY(@ids::uuid[])
RETURNING id, object_key, thumb_key, session_id, subject_id, suggested_subject_id, locked_subject_id;

-- name: ForgetPhotosInMessages :exec
-- Replies keep their words; the photos they showed are gone.
UPDATE conversation_messages m
SET photo_ids = ARRAY(SELECT x FROM unnest(m.photo_ids) x WHERE x <> ALL(@ids::uuid[]))
FROM conversations c
WHERE c.id = m.conversation_id AND c.team_id = @team_id AND m.photo_ids && @ids::uuid[];

-- name: DeleteEmptyAutoSubjects :many
-- Subjects the system made that no photo is filed under, suggested for or
-- locked to any more, and that nothing else points at.
DELETE FROM subjects s
WHERE s.team_id = @team_id AND s.id = ANY(@ids::uuid[]) AND s.created_by IS NULL AND s.merged_into_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM photos p WHERE p.team_id = @team_id
      AND (p.subject_id = s.id OR p.suggested_subject_id = s.id OR p.locked_subject_id = s.id))
  AND NOT EXISTS (SELECT 1 FROM subjects c WHERE c.parent_id = s.id OR c.merged_into_id = s.id)
RETURNING s.id;

-- name: ForgetSubjectsInMessages :exec
UPDATE conversation_messages m
SET missing_subject_ids = ARRAY(SELECT x FROM unnest(m.missing_subject_ids) x WHERE x <> ALL(@ids::uuid[]))
FROM conversations c
WHERE c.id = m.conversation_id AND c.team_id = @team_id AND m.missing_subject_ids && @ids::uuid[];

-- name: PhotoUploaders :many
SELECT id, uploaded_by FROM photos WHERE team_id = @team_id AND id = ANY(@ids::uuid[]);

-- name: GetPhotoByClientID :one
SELECT * FROM photos WHERE team_id = @team_id AND client_photo_id = @client_photo_id;

-- name: SaveShotVerdict :exec
INSERT INTO shot_verdicts (team_id, client_photo_id, appearance)
VALUES (@team_id, @client_photo_id, @appearance)
ON CONFLICT (team_id, client_photo_id) DO UPDATE SET appearance = EXCLUDED.appearance, created_at = now();

-- name: GetShotVerdict :one
SELECT appearance FROM shot_verdicts WHERE team_id = @team_id AND client_photo_id = @client_photo_id;

-- name: ForgetOldShotVerdicts :exec
-- Shots deleted on the phone, or long since uploaded and filed.
DELETE FROM shot_verdicts WHERE team_id = @team_id AND created_at < now() - interval '30 days';

-- name: ListPhotosToName :many
-- Photos of subjects told apart by look, described before descriptions
-- named them.
SELECT id FROM photos
WHERE status = 'processed' AND extracted->'appearance' IS NOT NULL AND NOT extracted ? 'named_description'
ORDER BY created_at;

-- name: ListPhotosToEmbedLook :many
-- Photos of subjects told apart by look with no look yet; with all, every
-- such photo (after the look model changed).
SELECT id FROM photos
WHERE status = 'processed' AND extracted->'appearance' IS NOT NULL
  AND (@every_photo::bool OR look_embedding IS NULL)
ORDER BY created_at;
