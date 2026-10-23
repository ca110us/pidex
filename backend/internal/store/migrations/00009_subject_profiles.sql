-- What a subject told apart by appearance looks like, in full: coat and
-- markings, nose, eyes, paws, size, collar. A black nose belongs to the
-- cat, not to any one photo (many do not show it), so it is written once
-- per subject from several of its photos, and searched there. A person may
-- write or correct it; then it is theirs and is not rewritten.

-- +goose Up
ALTER TABLE subjects
    ADD COLUMN profile            text NOT NULL DEFAULT '',
    ADD COLUMN profile_embedding  vector(1024),
    -- Photos it was written from; it is written again as they double.
    ADD COLUMN profile_photos     integer NOT NULL DEFAULT 0,
    ADD COLUMN profile_by_person  boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE subjects
    DROP COLUMN profile_by_person,
    DROP COLUMN profile_photos,
    DROP COLUMN profile_embedding,
    DROP COLUMN profile;
