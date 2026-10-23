-- Platform schema. Customer-defined structure (subject types, identity fields,
-- tags) lives in team_configs and JSONB columns, so changing a team's
-- vocabulary never needs a migration here.

-- +goose Up
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE teams (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name                  text NOT NULL,
    data_region           text NOT NULL DEFAULT 'us',
    active_config_version int,
    created_at            timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email        text NOT NULL UNIQUE,
    display_name text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE team_members (
    team_id    uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role       text NOT NULL CHECK (role IN ('owner', 'admin', 'member', 'viewer')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, user_id)
);

-- Only the SHA-256 of a token is stored.
CREATE TABLE api_tokens (
    token_sha256 bytea PRIMARY KEY,
    team_id      uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    label        text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    revoked_at   timestamptz
);

-- Every published config is kept; photos record the version they used.
CREATE TABLE team_configs (
    team_id    uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    version    int NOT NULL,
    config     jsonb NOT NULL,
    created_by uuid REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, version)
);

CREATE TABLE subjects (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id        uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    type_key       text NOT NULL,
    parent_id      uuid REFERENCES subjects (id),
    display_name   text NOT NULL,
    -- Identity values as read or entered, keyed by field key.
    identity       jsonb NOT NULL DEFAULT '{}',
    -- Normalized identity values used for matching.
    identity_norm  jsonb NOT NULL DEFAULT '{}',
    attributes     jsonb NOT NULL DEFAULT '{}',
    merged_into_id uuid REFERENCES subjects (id),
    created_by     uuid REFERENCES users (id),
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX subjects_team_type_idx ON subjects (team_id, type_key) WHERE merged_into_id IS NULL;
CREATE INDEX subjects_identity_norm_idx ON subjects USING gin (identity_norm jsonb_path_ops);
CREATE INDEX subjects_display_name_trgm_idx ON subjects USING gin (display_name gin_trgm_ops);

CREATE TABLE photos (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id               uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    uploaded_by           uuid NOT NULL REFERENCES users (id),
    client_photo_id       text NOT NULL,
    session_id            uuid,
    object_key            text NOT NULL,
    content_type          text NOT NULL,
    size_bytes            bigint NOT NULL,
    sha256_hex            text NOT NULL DEFAULT '',
    status                text NOT NULL DEFAULT 'awaiting_upload'
        CHECK (status IN ('awaiting_upload', 'uploaded', 'processing', 'processed', 'failed')),
    captured_at           timestamptz,
    received_at           timestamptz,
    latitude              double precision,
    longitude             double precision,
    location_accuracy_m   double precision,
    on_device             jsonb NOT NULL DEFAULT '{}',
    locked_subject_id     uuid REFERENCES subjects (id),
    config_version        int,
    role                  text CHECK (role IN ('identity', 'document', 'content', 'junk')),
    -- Pipeline output: identity read, resolution, document fields, provenance.
    extracted             jsonb NOT NULL DEFAULT '{}',
    description           text NOT NULL DEFAULT '',
    subject_id            uuid REFERENCES subjects (id),
    suggested_subject_id  uuid REFERENCES subjects (id),
    assignment            text NOT NULL DEFAULT 'unassigned'
        CHECK (assignment IN ('auto', 'suggested', 'manual', 'unassigned')),
    assignment_confidence real NOT NULL DEFAULT 0,
    embedding             vector(768),
    processing_attempts   int NOT NULL DEFAULT 0,
    processing_error      text NOT NULL DEFAULT '',
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    UNIQUE (team_id, client_photo_id)
);

CREATE INDEX photos_subject_idx ON photos (team_id, subject_id, captured_at);
CREATE INDEX photos_session_idx ON photos (team_id, session_id, captured_at);
CREATE INDEX photos_review_idx ON photos (team_id, assignment) WHERE assignment IN ('suggested', 'unassigned');
CREATE INDEX photos_embedding_idx ON photos USING hnsw (embedding vector_cosine_ops);

CREATE TABLE photo_tags (
    photo_id   uuid NOT NULL REFERENCES photos (id) ON DELETE CASCADE,
    tag_key    text NOT NULL,
    confidence real NOT NULL,
    source     text NOT NULL CHECK (source IN ('ai', 'user')),
    -- A user removal is kept so reprocessing never re-adds the tag.
    rejected   boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (photo_id, tag_key)
);

CREATE INDEX photo_tags_key_idx ON photo_tags (tag_key) WHERE NOT rejected;

-- One row per model call, for cost monitoring and per-team budgets.
CREATE TABLE ai_calls (
    id            bigserial PRIMARY KEY,
    team_id       uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    photo_id      uuid REFERENCES photos (id) ON DELETE SET NULL,
    purpose       text NOT NULL,
    provider      text NOT NULL,
    model         text NOT NULL,
    input_tokens  int NOT NULL DEFAULT 0,
    output_tokens int NOT NULL DEFAULT 0,
    cost_usd      numeric(12, 6) NOT NULL DEFAULT 0,
    latency_ms    int NOT NULL DEFAULT 0,
    ok            boolean NOT NULL,
    error         text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ai_calls_team_time_idx ON ai_calls (team_id, created_at);

CREATE TABLE audit_log (
    id            bigserial PRIMARY KEY,
    team_id       uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    actor_user_id uuid REFERENCES users (id),
    actor_kind    text NOT NULL CHECK (actor_kind IN ('user', 'system')),
    action        text NOT NULL,
    target_type   text NOT NULL DEFAULT '',
    target_id     uuid,
    details       jsonb NOT NULL DEFAULT '{}',
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_log_team_time_idx ON audit_log (team_id, created_at);

-- +goose Down
DROP TABLE audit_log;
DROP TABLE ai_calls;
DROP TABLE photo_tags;
DROP TABLE photos;
DROP TABLE subjects;
DROP TABLE team_configs;
DROP TABLE api_tokens;
DROP TABLE team_members;
DROP TABLE users;
DROP TABLE teams;
