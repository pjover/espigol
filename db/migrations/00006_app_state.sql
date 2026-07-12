-- +goose Up
CREATE TABLE app_state (
    id          INTEGER PRIMARY KEY CHECK (id = 1),
    active_year INTEGER NOT NULL
);

-- +goose Down
DROP TABLE app_state;
