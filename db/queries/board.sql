-- name: UpsertBoardAuthorization :exec
INSERT INTO board_authorization (partner_id, scope_kind, section_code)
VALUES (?, ?, ?)
ON CONFLICT(partner_id, scope_kind, COALESCE(section_code, '')) DO NOTHING;

-- name: ListBoardAuthorizationsByPartner :many
SELECT partner_id, scope_kind, section_code
FROM board_authorization WHERE partner_id = ? ORDER BY scope_kind, section_code;
