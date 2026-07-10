-- +goose Up
CREATE TABLE reconciliation_snapshot (
    year          INTEGER PRIMARY KEY,
    generated_at  TEXT NOT NULL,
    snapshot_json TEXT NOT NULL,
    pdf           BLOB NOT NULL,
    FOREIGN KEY (year) REFERENCES submission_window(year)
);

-- +goose Down
DROP TABLE reconciliation_snapshot;
