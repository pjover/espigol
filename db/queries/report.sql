-- name: InsertReport :one
INSERT INTO report (year, generated_at, snapshot_json, pdf, superseded_at)
VALUES (?, ?, ?, ?, ?)
RETURNING id;

-- name: GetLatestReportByYear :one
SELECT id, year, generated_at, snapshot_json, pdf, superseded_at
FROM report
WHERE year = ? AND superseded_at IS NULL
ORDER BY generated_at DESC
LIMIT 1;

-- name: MarkReportSuperseded :exec
UPDATE report SET superseded_at = ? WHERE id = ?;
