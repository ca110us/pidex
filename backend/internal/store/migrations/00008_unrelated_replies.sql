-- A reply to a request that was not about the photos: nothing was looked up.

-- +goose Up
ALTER TABLE conversation_messages DROP CONSTRAINT conversation_messages_intent_check;
ALTER TABLE conversation_messages ADD CONSTRAINT conversation_messages_intent_check
    CHECK (intent IN ('photos', 'answer', 'missing', 'unrelated'));

-- +goose Down
DELETE FROM conversation_messages WHERE intent = 'unrelated';
ALTER TABLE conversation_messages DROP CONSTRAINT conversation_messages_intent_check;
ALTER TABLE conversation_messages ADD CONSTRAINT conversation_messages_intent_check
    CHECK (intent IN ('photos', 'answer', 'missing'));
