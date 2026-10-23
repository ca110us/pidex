-- What the camera's look match said of a shot before it was uploaded. When
-- the shot arrives filed by a person, it is kept with the photo: the
-- model's answer and the person's, side by side, are how matching by look
-- learns when it can file without asking.

-- +goose Up
CREATE TABLE shot_verdicts (
    team_id         uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    client_photo_id text NOT NULL,
    -- An AppearanceMatch, as kept in photos.extracted.
    appearance      jsonb NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, client_photo_id)
);

-- +goose Down
DROP TABLE shot_verdicts;
