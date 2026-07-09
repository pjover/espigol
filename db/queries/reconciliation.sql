-- name: ListConcessionsByYear :many
SELECT year, group_code, subtype_code, concept, requested_total, granted_amount
FROM concession WHERE year = ? ORDER BY group_code;

-- name: UpsertConcession :exec
INSERT INTO concession (year, group_code, subtype_code, concept, requested_total, granted_amount)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(year, group_code) DO UPDATE SET
    subtype_code=excluded.subtype_code, concept=excluded.concept,
    requested_total=excluded.requested_total, granted_amount=excluded.granted_amount;

-- name: DeleteConcession :exec
DELETE FROM concession WHERE year = ? AND group_code = ?;

-- name: DeleteConcessionsByYear :exec
DELETE FROM concession WHERE year = ?;

-- name: ListConcessionForecastsByYear :many
SELECT year, forecast_id, group_code FROM concession_forecast
WHERE year = ? ORDER BY group_code, forecast_id;

-- name: InsertConcessionForecast :exec
INSERT INTO concession_forecast (year, forecast_id, group_code) VALUES (?, ?, ?);

-- name: DeleteConcessionForecastsByGroup :exec
DELETE FROM concession_forecast WHERE year = ? AND group_code = ?;

-- name: DeleteConcessionForecastsByYear :exec
DELETE FROM concession_forecast WHERE year = ?;

-- name: ListInvoicesByYear :many
SELECT id, year, issuer, nif, number, issue_date, net_amount, file_path, notes
FROM invoice WHERE year = ? ORDER BY id;

-- name: InsertInvoice :one
INSERT INTO invoice (year, issuer, nif, number, issue_date, net_amount, file_path, notes)
VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id;

-- name: UpdateInvoice :exec
UPDATE invoice SET year=?, issuer=?, nif=?, number=?, issue_date=?, net_amount=?, file_path=?, notes=?
WHERE id=?;

-- name: DeleteInvoice :exec
DELETE FROM invoice WHERE id = ?;

-- name: DeleteInvoicesByYear :exec
DELETE FROM invoice WHERE year = ?;

-- name: ListInvoicePaymentsByYear :many
SELECT p.id, p.invoice_id, p.paid_on, p.amount FROM invoice_payment p
JOIN invoice i ON i.id = p.invoice_id WHERE i.year = ? ORDER BY p.invoice_id, p.id;

-- name: InsertInvoicePayment :exec
INSERT INTO invoice_payment (invoice_id, paid_on, amount) VALUES (?, ?, ?);

-- name: DeletePaymentsByInvoice :exec
DELETE FROM invoice_payment WHERE invoice_id = ?;

-- name: ListForecastInvoicesByYear :many
SELECT fi.forecast_id, fi.invoice_id, fi.amount FROM forecast_invoice fi
JOIN invoice i ON i.id = fi.invoice_id WHERE i.year = ? ORDER BY fi.invoice_id, fi.forecast_id;

-- name: InsertForecastInvoice :exec
INSERT INTO forecast_invoice (forecast_id, invoice_id, amount) VALUES (?, ?, ?);

-- name: DeleteForecastInvoicesByInvoice :exec
DELETE FROM forecast_invoice WHERE invoice_id = ?;
