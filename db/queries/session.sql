-- name: InsertSession :exec
INSERT INTO session (token, partner_id, email, created_at, expires_at)
VALUES (?, ?, ?, ?, ?);

-- name: GetSession :one
SELECT token, partner_id, email, created_at, expires_at
FROM session WHERE token = ?;

-- name: DeleteSession :exec
DELETE FROM session WHERE token = ?;

-- name: DeleteExpiredSessions :exec
DELETE FROM session WHERE expires_at < ?;
