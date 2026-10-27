-- name: CreateTeam :one
INSERT INTO teams (name, data_region)
VALUES (@name, @data_region)
RETURNING *;

-- name: GetTeam :one
SELECT * FROM teams WHERE id = @id;

-- name: UpsertUser :one
INSERT INTO users (email, display_name)
VALUES (@email, @display_name)
ON CONFLICT (email) DO UPDATE SET display_name = EXCLUDED.display_name
RETURNING *;

-- name: AddTeamMember :exec
INSERT INTO team_members (team_id, user_id, role)
VALUES (@team_id, @user_id, @role)
ON CONFLICT (team_id, user_id) DO UPDATE SET role = EXCLUDED.role;

-- name: CreateAPIToken :exec
INSERT INTO api_tokens (token_sha256, team_id, user_id, label)
VALUES (@token_sha256, @team_id, @user_id, @label);

-- name: GetPrincipalByToken :one
SELECT t.team_id, t.user_id, m.role
FROM api_tokens t
JOIN team_members m ON m.team_id = t.team_id AND m.user_id = t.user_id
WHERE t.token_sha256 = @token_sha256 AND t.revoked_at IS NULL;

-- name: InsertTeamConfig :one
INSERT INTO team_configs (team_id, version, config, created_by)
VALUES (
    @team_id,
    (SELECT coalesce(max(version), 0) + 1 FROM team_configs WHERE team_id = @team_id),
    @config,
    sqlc.narg(created_by)
)
RETURNING *;

-- name: SetActiveConfigVersion :exec
UPDATE teams SET active_config_version = @version WHERE id = @id;

-- name: GetTeamConfig :one
SELECT * FROM team_configs WHERE team_id = @team_id AND version = @version;

-- name: GetActiveTeamConfig :one
SELECT c.*
FROM team_configs c
JOIN teams t ON t.id = c.team_id AND t.active_config_version = c.version
WHERE c.team_id = @team_id;

-- name: InsertAICall :exec
INSERT INTO ai_calls (team_id, photo_id, purpose, provider, model, input_tokens, output_tokens, cost_usd, latency_ms, ok, error)
VALUES (@team_id, sqlc.narg(photo_id), @purpose, @provider, @model, @input_tokens, @output_tokens, @cost_usd, @latency_ms, @ok, @error);

-- name: InsertAudit :exec
INSERT INTO audit_log (team_id, actor_user_id, actor_kind, action, target_type, target_id, details)
VALUES (@team_id, sqlc.narg(actor_user_id), @actor_kind, @action, @target_type, sqlc.narg(target_id), @details);

-- name: ListAudit :many
SELECT * FROM audit_log
WHERE team_id = @team_id
ORDER BY id DESC
LIMIT @page_limit;

-- name: ListTeamConfigs :many
SELECT c.version, coalesce(c.config ->> 'name', '')::text AS name, c.created_at, (t.active_config_version = c.version)::bool AS active
FROM team_configs c
JOIN teams t ON t.id = c.team_id
WHERE c.team_id = @team_id
ORDER BY c.version DESC
LIMIT 100;

-- name: TeamObjectKeys :many
-- Every stored file of a team's photos: working copies, thumbnails,
-- originals and Live Photos' motion.
SELECT object_key, thumb_key, original_key, live_key FROM photos WHERE team_id = @team_id;

-- name: DeleteTeam :execrows
-- Everything else belongs to the team and goes with it.
DELETE FROM teams WHERE id = @id;
