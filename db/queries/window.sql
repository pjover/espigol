-- name: UpsertSubmissionWindow :exec
INSERT INTO submission_window (year, state, opened_at, closed_at, deadline, current_expense_limit, investment_expense_limit)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(year) DO UPDATE SET
  state=excluded.state, opened_at=excluded.opened_at, closed_at=excluded.closed_at,
  deadline=excluded.deadline, current_expense_limit=excluded.current_expense_limit,
  investment_expense_limit=excluded.investment_expense_limit;

-- name: GetSubmissionWindow :one
SELECT year, state, opened_at, closed_at, deadline, current_expense_limit, investment_expense_limit
FROM submission_window WHERE year = ?;

-- name: ListSubmissionWindows :many
SELECT year, state, opened_at, closed_at, deadline, current_expense_limit, investment_expense_limit
FROM submission_window ORDER BY year DESC;
