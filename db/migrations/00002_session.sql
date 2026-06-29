-- +goose Up
CREATE TABLE session (
    token      TEXT PRIMARY KEY,
    partner_id INTEGER NOT NULL,
    email      TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    FOREIGN KEY (partner_id) REFERENCES partner(id)
);

CREATE INDEX idx_session_expires ON session(expires_at);

-- +goose Down
DROP TABLE session;
