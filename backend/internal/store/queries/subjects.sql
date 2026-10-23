-- name: CreateSubject :one
INSERT INTO subjects (team_id, type_key, parent_id, display_name, identity, identity_norm, created_by)
VALUES (@team_id, @type_key, sqlc.narg(parent_id), @display_name, @identity, @identity_norm, sqlc.narg(created_by))
RETURNING *;

-- name: GetSubject :one
SELECT * FROM subjects WHERE team_id = @team_id AND id = @id;

-- name: GetSubjectsByIDs :many
SELECT * FROM subjects WHERE team_id = @team_id AND id = ANY(@ids::uuid[]);

-- FindSubjectCandidates narrows matching to subjects sharing at least one
-- normalized identity value, or with a similar display name.
-- name: FindSubjectCandidates :many
SELECT * FROM subjects
WHERE team_id = @team_id
  AND type_key = @type_key
  AND merged_into_id IS NULL
  AND (
    identity_norm @> ANY (@probes::jsonb[])
    OR (@name_probe::text <> '' AND display_name % @name_probe::text)
  )
LIMIT 50;

-- name: ListSubjects :many
SELECT * FROM subjects
WHERE team_id = @team_id
  AND merged_into_id IS NULL
  AND (sqlc.narg(type_key)::text IS NULL OR type_key = sqlc.narg(type_key)::text)
  AND (
    sqlc.narg(query)::text IS NULL
    OR display_name ILIKE '%' || sqlc.narg(query)::text || '%'
    OR identity::text ILIKE '%' || sqlc.narg(query)::text || '%'
  )
  AND (
    sqlc.narg(before_created)::timestamptz IS NULL
    OR (created_at, id) < (sqlc.narg(before_created)::timestamptz, sqlc.narg(before_id)::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT @page_limit;

-- name: CountPhotosBySubject :many
SELECT subject_id::uuid AS subject_id, count(*)::int AS photo_count
FROM photos
WHERE team_id = @team_id AND subject_id = ANY(@subject_ids::uuid[])
GROUP BY subject_id;

-- name: CountSubjectsByType :many
SELECT type_key, count(*)::int AS subject_count
FROM subjects
WHERE team_id = @team_id AND merged_into_id IS NULL
GROUP BY type_key;

-- name: SubjectTagCounts :many
-- How often each tag appears on each subject's photos: what the subject is
-- and what was photographed of it.
SELECT p.subject_id::uuid AS subject_id, pt.tag_key, count(*)::int AS photo_count
FROM photos p
JOIN photo_tags pt ON pt.photo_id = p.id AND NOT pt.rejected
WHERE p.team_id = @team_id AND p.subject_id = ANY(@subject_ids::uuid[]) AND p.status = 'processed'
GROUP BY p.subject_id, pt.tag_key
ORDER BY p.subject_id, photo_count DESC, pt.tag_key;

-- name: RenameSubject :one
UPDATE subjects SET display_name = @display_name, updated_at = now()
WHERE team_id = @team_id AND id = @id AND merged_into_id IS NULL
RETURNING *;

-- MergeSubjects moves everything filed under ids to into_id, then marks
-- them merged so links to them can still be followed.
-- name: MergeSubjectPhotos :exec
UPDATE photos SET
    subject_id = CASE WHEN subject_id = ANY(@ids::uuid[]) THEN @into_id::uuid ELSE subject_id END,
    suggested_subject_id = CASE WHEN suggested_subject_id = ANY(@ids::uuid[]) THEN @into_id::uuid ELSE suggested_subject_id END,
    locked_subject_id = CASE WHEN locked_subject_id = ANY(@ids::uuid[]) THEN @into_id::uuid ELSE locked_subject_id END
WHERE team_id = @team_id
  AND (subject_id = ANY(@ids::uuid[]) OR suggested_subject_id = ANY(@ids::uuid[]) OR locked_subject_id = ANY(@ids::uuid[]));

-- name: MergeSubjectChildren :exec
UPDATE subjects SET parent_id = @into_id::uuid, updated_at = now()
WHERE team_id = @team_id AND parent_id = ANY(@ids::uuid[]);

-- name: MarkSubjectsMerged :execrows
UPDATE subjects SET merged_into_id = @into_id::uuid, updated_at = now()
WHERE team_id = @team_id AND id = ANY(@ids::uuid[]) AND id <> @into_id::uuid AND merged_into_id IS NULL;

-- name: SubjectCovers :many
-- A photo for each subject to show it by: one a person filed there first,
-- else the latest.
SELECT DISTINCT ON (subject_id) subject_id::uuid AS subject_id, id AS photo_id
FROM photos
WHERE team_id = @team_id AND subject_id = ANY(@subject_ids::uuid[])
  AND status = 'processed' AND coalesce(role, '') <> 'junk'
ORDER BY subject_id, (assignment = 'manual') DESC, coalesce(captured_at, created_at) DESC;

-- name: SubjectPhotoIDs :many
SELECT id FROM photos WHERE team_id = @team_id AND subject_id = @subject_id;

-- name: UnfileSubjectPhotos :many
-- Photos filed, suggested for or locked to a subject being deleted point
-- at nothing; those filed under it are unfiled, even ones a person filed.
UPDATE photos SET
    subject_id            = CASE WHEN subject_id = @subject_id THEN NULL ELSE subject_id END,
    assignment            = CASE WHEN subject_id = @subject_id OR suggested_subject_id = @subject_id THEN 'unassigned' ELSE assignment END,
    assignment_confidence = CASE WHEN subject_id = @subject_id OR suggested_subject_id = @subject_id THEN 0 ELSE assignment_confidence END,
    suggested_subject_id  = CASE WHEN suggested_subject_id = @subject_id THEN NULL ELSE suggested_subject_id END,
    locked_subject_id     = CASE WHEN locked_subject_id = @subject_id THEN NULL ELSE locked_subject_id END,
    updated_at            = now()
WHERE team_id = @team_id
  AND (subject_id = @subject_id OR suggested_subject_id = @subject_id OR locked_subject_id = @subject_id)
RETURNING id, session_id;

-- name: OrphanSubjectChildren :exec
-- A deleted subject's children stand alone.
UPDATE subjects SET parent_id = NULL, updated_at = now()
WHERE team_id = @team_id AND parent_id = @subject_id;

-- name: DeleteSubjectAliases :exec
-- Subjects merged into a deleted one were only other names for it.
DELETE FROM subjects WHERE team_id = @team_id AND merged_into_id = @subject_id;

-- name: DeleteSubject :execrows
DELETE FROM subjects WHERE team_id = @team_id AND id = @subject_id;

-- name: SubjectsToProfile :many
-- Subjects told apart by look whose profile is missing or written from
-- half the photos they have now, that a person has not written.
SELECT s.id, s.team_id, s.type_key, s.display_name, s.appearance_photo_id, n.photos
FROM subjects s
JOIN LATERAL (
    SELECT count(*)::int AS photos FROM photos p
    WHERE p.subject_id = s.id AND p.assignment IN ('auto', 'manual') AND p.status = 'processed'
) n ON true
WHERE s.appearance <> '' AND s.merged_into_id IS NULL AND NOT s.profile_by_person
  AND n.photos > 0 AND n.photos >= 2 * s.profile_photos
ORDER BY s.updated_at DESC
LIMIT @max_subjects;

-- name: SetSubjectProfile :exec
UPDATE subjects SET
    profile           = @profile,
    profile_embedding = sqlc.narg(profile_embedding),
    profile_photos    = @profile_photos,
    profile_by_person = @by_person,
    updated_at        = now()
WHERE team_id = @team_id AND id = @id AND (@by_person OR NOT profile_by_person);

-- name: ResetSubjectProfile :exec
-- Hands a profile back to be written from photos at the next sweep.
UPDATE subjects SET profile = '', profile_embedding = NULL, profile_photos = 0, profile_by_person = false, updated_at = now()
WHERE team_id = @team_id AND id = @id;
