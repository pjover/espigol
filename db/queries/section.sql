-- name: UpsertSection :exec
INSERT INTO section (code, label, active, display_order)
VALUES (?, ?, ?, ?)
ON CONFLICT(code) DO UPDATE SET
  label=excluded.label, active=excluded.active, display_order=excluded.display_order;

-- name: ListSections :many
SELECT code, label, active, display_order FROM section ORDER BY display_order, code;

-- name: AddPartnerSection :exec
INSERT INTO partner_section (partner_id, section_code) VALUES (?, ?)
ON CONFLICT(partner_id, section_code) DO NOTHING;

-- name: ListPartnerSectionsByPartner :many
SELECT partner_id, section_code FROM partner_section WHERE partner_id = ? ORDER BY section_code;

-- name: ListAllPartnerSections :many
SELECT partner_id, section_code FROM partner_section ORDER BY partner_id, section_code;
