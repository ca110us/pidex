-- +goose Up
-- A photo's other files, beside the working copy every model and thumbnail
-- reads: the file as the camera made it (HEIC, full size), and a Live
-- Photo's motion. A key is set once its file has arrived; the original's
-- type is noted when its upload is asked for.
ALTER TABLE photos
    ADD COLUMN original_key   text,
    ADD COLUMN original_type  text,
    ADD COLUMN original_bytes bigint,
    ADD COLUMN live_key       text,
    ADD COLUMN live_bytes     bigint;

-- +goose Down
ALTER TABLE photos
    DROP COLUMN original_key,
    DROP COLUMN original_type,
    DROP COLUMN original_bytes,
    DROP COLUMN live_key,
    DROP COLUMN live_bytes;
