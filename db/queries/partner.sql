-- name: UpsertPartner :exec
INSERT INTO partner (id, name, surname, vat_code, email, mobile, partner_type, ria_number, added_on, board_member, nick_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  name=excluded.name, surname=excluded.surname, vat_code=excluded.vat_code,
  email=excluded.email, mobile=excluded.mobile, partner_type=excluded.partner_type,
  ria_number=excluded.ria_number, added_on=excluded.added_on, board_member=excluded.board_member,
  nick_name=excluded.nick_name;

-- name: GetPartner :one
SELECT id, name, surname, vat_code, email, mobile, partner_type, ria_number, added_on, board_member, nick_name
FROM partner WHERE id = ?;

-- name: GetPartnerByEmail :one
SELECT id, name, surname, vat_code, email, mobile, partner_type, ria_number, added_on, board_member, nick_name
FROM partner WHERE email = ?;

-- name: ListPartners :many
SELECT id, name, surname, vat_code, email, mobile, partner_type, ria_number, added_on, board_member, nick_name
FROM partner ORDER BY id;
