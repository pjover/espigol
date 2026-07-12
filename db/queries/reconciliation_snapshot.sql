-- name: UpsertReconciliationSnapshot :exec
INSERT INTO reconciliation_snapshot (year, generated_at, snapshot_json, pdf)
VALUES (?, ?, ?, ?)
ON CONFLICT(year) DO UPDATE SET
    generated_at  = excluded.generated_at,
    snapshot_json = excluded.snapshot_json,
    pdf           = excluded.pdf;

-- name: GetReconciliationSnapshotByYear :one
SELECT year, generated_at, snapshot_json, pdf
FROM reconciliation_snapshot
WHERE year = ?;
