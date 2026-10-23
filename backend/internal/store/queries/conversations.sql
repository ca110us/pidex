-- name: CreateConversation :one
INSERT INTO conversations (team_id, user_id, title)
VALUES (@team_id, @user_id, @title)
RETURNING *;

-- name: GetConversation :one
SELECT * FROM conversations
WHERE id = @id AND team_id = @team_id AND user_id = @user_id;

-- name: ListConversations :many
SELECT c.*,
    (SELECT count(*) FROM conversation_messages m WHERE m.conversation_id = c.id)::int AS message_count
FROM conversations c
WHERE c.team_id = @team_id AND c.user_id = @user_id
ORDER BY c.updated_at DESC
LIMIT @page_limit;

-- name: TouchConversation :exec
UPDATE conversations SET updated_at = now() WHERE id = @id;

-- name: DeleteConversation :execrows
DELETE FROM conversations WHERE id = @id AND team_id = @team_id AND user_id = @user_id;

-- name: AddConversationMessage :one
INSERT INTO conversation_messages (conversation_id, role, text, intent, filters, photo_ids, missing_subject_ids, subject_not_found, photo_total)
VALUES (@conversation_id, @role, @text, sqlc.narg(intent), sqlc.narg(filters), @photo_ids, @missing_subject_ids, @subject_not_found, @photo_total)
RETURNING *;

-- name: ListConversationMessages :many
SELECT * FROM conversation_messages
WHERE conversation_id = @conversation_id
ORDER BY seq;
