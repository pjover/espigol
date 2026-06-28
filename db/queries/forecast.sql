-- name: InsertForecast :exec
INSERT INTO expense_forecast
  (id, partner_id, concept, description, gross_amount, approved_amount, approved_on,
   planned_date, year, subtype_code, scope_kind, section_code, added_on, enabled)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateForecast :exec
UPDATE expense_forecast SET
  partner_id=?, concept=?, description=?, gross_amount=?, approved_amount=?, approved_on=?,
  planned_date=?, year=?, subtype_code=?, scope_kind=?, section_code=?, added_on=?, enabled=?
WHERE id=?;

-- name: GetForecast :one
SELECT id, partner_id, concept, description, gross_amount, approved_amount, approved_on,
       planned_date, year, subtype_code, scope_kind, section_code, added_on, enabled
FROM expense_forecast WHERE id = ?;

-- name: ListForecastsByYear :many
SELECT id, partner_id, concept, description, gross_amount, approved_amount, approved_on,
       planned_date, year, subtype_code, scope_kind, section_code, added_on, enabled
FROM expense_forecast WHERE year = ? ORDER BY id;

-- name: ListForecastIDsByYear :many
SELECT id FROM expense_forecast WHERE year = ?;
