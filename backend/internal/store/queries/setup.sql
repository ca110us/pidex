-- name: ListSetupMessages :many
SELECT * FROM setup_messages WHERE team_id = @team_id ORDER BY seq;

-- name: AddSetupMessage :one
INSERT INTO setup_messages (team_id, user_id, role, text, draft, config_version)
VALUES (@team_id, sqlc.narg(user_id), @role, @text, sqlc.narg(draft), sqlc.narg(config_version))
RETURNING *;
