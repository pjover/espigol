-- name: UpsertExpenseType :exec
INSERT INTO expense_type (year, code, label, category)
VALUES (?, ?, ?, ?)
ON CONFLICT(year, code) DO UPDATE SET label=excluded.label, category=excluded.category;

-- name: UpsertExpenseSubtype :exec
INSERT INTO expense_subtype (year, code, label, type_code)
VALUES (?, ?, ?, ?)
ON CONFLICT(year, code) DO UPDATE SET label=excluded.label, type_code=excluded.type_code;

-- name: ListExpenseTypes :many
SELECT year, code, label, category FROM expense_type WHERE year = ? ORDER BY code;

-- name: ListExpenseSubtypes :many
SELECT year, code, label, type_code FROM expense_subtype WHERE year = ? ORDER BY code;
