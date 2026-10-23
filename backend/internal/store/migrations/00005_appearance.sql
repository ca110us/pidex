-- Subjects told apart by appearance (pets, mugs, plants) have no code to
-- match. Each keeps what tells it from others of its kind, as words and as
-- a vector for finding look-alikes, and the photo that introduced it.

-- +goose Up
ALTER TABLE subjects
    ADD COLUMN appearance           text NOT NULL DEFAULT '',
    ADD COLUMN appearance_embedding vector(1024),
    ADD COLUMN appearance_photo_id  uuid REFERENCES photos (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE subjects
    DROP COLUMN appearance_photo_id,
    DROP COLUMN appearance_embedding,
    DROP COLUMN appearance;
