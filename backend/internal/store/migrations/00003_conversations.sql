-- Conversations: a person's back-and-forth with a collection's photos.
-- Messages keep references (photo and subject ids, filters), never copies,
-- so a conversation shows photos as they are now.

-- +goose Up
CREATE TABLE conversations (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id    uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title      text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX conversations_owner_idx ON conversations (team_id, user_id, updated_at DESC);

CREATE TABLE conversation_messages (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Order of writing: a request and its reply share one transaction, and
    -- so one now().
    seq                 bigint GENERATED ALWAYS AS IDENTITY,
    conversation_id     uuid NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    role                text NOT NULL CHECK (role IN ('user', 'assistant')),
    text                text NOT NULL DEFAULT '',
    -- Assistant replies: how the message was understood and what it found.
    intent              text CHECK (intent IN ('photos', 'answer', 'missing')),
    filters             jsonb,
    photo_ids           uuid[] NOT NULL DEFAULT '{}',
    missing_subject_ids uuid[] NOT NULL DEFAULT '{}',
    subject_not_found   boolean NOT NULL DEFAULT false,
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX conversation_messages_idx ON conversation_messages (conversation_id, seq);

-- +goose Down
DROP TABLE conversation_messages;
DROP TABLE conversations;
