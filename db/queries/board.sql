-- name: UpsertBoardAuthorization :exec
INSERT INTO board_authorization (partner_id, scope_kind, section_code)
VALUES (?, ?, ?)
ON CONFLICT(partner_id, scope_kind, COALESCE(section_code, '')) DO NOTHING;

-- name: ListBoardAuthorizationsByPartner :many
SELECT partner_id, scope_kind, section_code
FROM board_authorization WHERE partner_id = ? ORDER BY scope_kind, section_code;

-- name: DeleteBoardAuthorization :execrows
-- section_code uses `IS` (not `=`) so a NULL bind matches COMMON-scope rows,
-- whose section_code is NULL (SQL `NULL = NULL` is never true).
DELETE FROM board_authorization
WHERE partner_id = ? AND scope_kind = ? AND section_code IS ?;
