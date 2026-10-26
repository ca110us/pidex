-- What the subject of a photo told apart by appearance looks like, as a
-- vector from the look service, and the model that made it. Vectors of
-- different models (or sizes) are not compared, so the column has no fixed
-- size and every query keeps to one model. A team's photos are few enough
-- to compare without an index.

-- +goose Up
ALTER TABLE photos
    ADD COLUMN look_embedding vector,
    ADD COLUMN look_model     text;

-- +goose Down
ALTER TABLE photos
    DROP COLUMN look_model,
    DROP COLUMN look_embedding;
