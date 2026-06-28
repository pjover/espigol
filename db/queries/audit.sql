-- name: InsertAuditEvent :exec
INSERT INTO audit_event (actor_id, actor_email, kind, entity_type, entity_id, timestamp, payload)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListAuditEvents :many
SELECT id, actor_id, actor_email, kind, entity_type, entity_id, timestamp, payload
FROM audit_event ORDER BY id;
