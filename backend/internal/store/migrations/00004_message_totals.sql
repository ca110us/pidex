-- A reply keeps the first matches; photo_total says how many there were in
-- all, so the app can offer the rest without showing them in the chat.

-- +goose Up
ALTER TABLE conversation_messages ADD COLUMN photo_total integer NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE conversation_messages DROP COLUMN photo_total;
