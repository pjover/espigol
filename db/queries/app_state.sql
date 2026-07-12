-- name: GetActiveYear :one
SELECT active_year FROM app_state WHERE id = 1;

-- name: SetActiveYear :exec
INSERT INTO app_state (id, active_year) VALUES (1, ?)
ON CONFLICT(id) DO UPDATE SET active_year = excluded.active_year;
