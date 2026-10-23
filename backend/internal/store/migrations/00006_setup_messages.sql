-- The conversation a collection was set up in, kept with it: what was
-- asked, each draft offered, and each version published from it. It is not
-- among the photo conversations; opening the collection's setup carries it
-- on.

-- +goose Up
CREATE TABLE setup_messages (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    seq            bigint GENERATED ALWAYS AS IDENTITY,
    team_id        uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    user_id        uuid REFERENCES users (id) ON DELETE SET NULL,
    role           text NOT NULL CHECK (role IN ('user', 'assistant', 'event')),
    text           text NOT NULL DEFAULT '',
    -- Assistant: the draft offered, as DraftConfigResponse JSON.
    draft          jsonb,
    -- Event: the config version published.
    config_version integer,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX setup_messages_team_idx ON setup_messages (team_id, seq);

-- +goose Down
DROP TABLE setup_messages;
