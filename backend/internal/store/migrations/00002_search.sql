-- Search: thumbnails for browsing, the text each photo is found by, and a
-- vector of that text for searching by meaning.

-- +goose Up
ALTER TABLE photos
    ADD COLUMN thumb_key            text,
    ADD COLUMN search_text          text NOT NULL DEFAULT '',
    -- Fixed at 1024 dimensions; models are asked for that size, so the
    -- model can change without a migration (photos are then re-indexed).
    ADD COLUMN text_embedding       vector(1024),
    ADD COLUMN text_embedding_model text,
    ADD COLUMN indexed_at           timestamptz;

CREATE INDEX photos_search_text_trgm ON photos USING gin (search_text gin_trgm_ops);
CREATE INDEX photos_text_embedding_idx ON photos USING hnsw (text_embedding vector_cosine_ops);
CREATE INDEX photos_captured_idx ON photos (team_id, captured_at DESC);
CREATE INDEX photo_tags_tag_idx ON photo_tags (tag_key, photo_id) WHERE NOT rejected;

-- +goose Down
DROP INDEX photo_tags_tag_idx;
DROP INDEX photos_captured_idx;
DROP INDEX photos_text_embedding_idx;
DROP INDEX photos_search_text_trgm;
ALTER TABLE photos
    DROP COLUMN indexed_at,
    DROP COLUMN text_embedding_model,
    DROP COLUMN text_embedding,
    DROP COLUMN search_text,
    DROP COLUMN thumb_key;
