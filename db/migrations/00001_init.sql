-- +goose Up
PRAGMA foreign_keys = ON;

CREATE TABLE partner (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL,
    surname      TEXT NOT NULL,
    vat_code     TEXT NOT NULL,
    email        TEXT NOT NULL UNIQUE,
    mobile       TEXT NOT NULL,
    partner_type TEXT NOT NULL,
    ria_number   INTEGER NOT NULL,
    added_on     TEXT NOT NULL,
    board_member INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE section (
    code          TEXT PRIMARY KEY,
    label         TEXT NOT NULL,
    active        INTEGER NOT NULL DEFAULT 1,
    display_order INTEGER NOT NULL
);

CREATE TABLE partner_section (
    partner_id   INTEGER NOT NULL,
    section_code TEXT NOT NULL,
    PRIMARY KEY (partner_id, section_code),
    FOREIGN KEY (partner_id) REFERENCES partner(id),
    FOREIGN KEY (section_code) REFERENCES section(code)
);

CREATE TABLE submission_window (
    year                     INTEGER PRIMARY KEY,
    state                    TEXT NOT NULL CHECK (state IN ('DRAFT','OPEN','CLOSED')),
    opened_at                TEXT,
    closed_at                TEXT,
    deadline                 TEXT NOT NULL,
    current_expense_limit    TEXT NOT NULL,
    investment_expense_limit TEXT NOT NULL
);

CREATE UNIQUE INDEX one_open_window
    ON submission_window(state) WHERE state = 'OPEN';

CREATE TABLE expense_type (
    year     INTEGER NOT NULL,
    code     TEXT NOT NULL,
    label    TEXT NOT NULL,
    category TEXT NOT NULL CHECK (category IN ('CURRENT','INVESTMENT')),
    PRIMARY KEY (year, code),
    FOREIGN KEY (year) REFERENCES submission_window(year)
);

CREATE TABLE expense_subtype (
    year      INTEGER NOT NULL,
    code      TEXT NOT NULL,
    label     TEXT NOT NULL,
    type_code TEXT NOT NULL,
    PRIMARY KEY (year, code),
    FOREIGN KEY (year, type_code) REFERENCES expense_type(year, code)
);

CREATE TABLE expense_forecast (
    id              TEXT PRIMARY KEY,
    partner_id      INTEGER NOT NULL,
    concept         TEXT NOT NULL,
    description     TEXT NOT NULL,
    gross_amount    TEXT NOT NULL,
    approved_amount TEXT NOT NULL,
    approved_on     TEXT,
    planned_date    TEXT NOT NULL,
    year            INTEGER NOT NULL,
    subtype_code    TEXT NOT NULL,
    scope_kind      TEXT NOT NULL CHECK (scope_kind IN ('COMMON','SECTION','PARTNER')),
    section_code    TEXT,
    added_on        TEXT NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1,
    CHECK ((scope_kind = 'SECTION') = (section_code IS NOT NULL)),
    FOREIGN KEY (partner_id) REFERENCES partner(id),
    FOREIGN KEY (year) REFERENCES submission_window(year),
    FOREIGN KEY (year, subtype_code) REFERENCES expense_subtype(year, code),
    FOREIGN KEY (section_code) REFERENCES section(code)
);

CREATE INDEX idx_forecast_year_enabled ON expense_forecast(year, enabled);
CREATE INDEX idx_forecast_partner ON expense_forecast(partner_id);

CREATE TABLE report (
    id            INTEGER PRIMARY KEY,
    year          INTEGER NOT NULL,
    generated_at  TEXT NOT NULL,
    snapshot_json TEXT NOT NULL,
    pdf           BLOB NOT NULL,
    superseded_at TEXT,
    FOREIGN KEY (year) REFERENCES submission_window(year)
);

-- Only one active (non-superseded) report per year is allowed.
CREATE UNIQUE INDEX uq_report_active_per_year
    ON report(year, generated_at) WHERE superseded_at IS NULL;

CREATE INDEX idx_report_latest_per_year
    ON report(year, generated_at) WHERE superseded_at IS NULL;

CREATE TABLE audit_event (
    id          INTEGER PRIMARY KEY,
    actor_id    INTEGER,
    actor_email TEXT NOT NULL,
    kind        TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id   TEXT NOT NULL,
    timestamp   TEXT NOT NULL,
    payload     TEXT,
    FOREIGN KEY (actor_id) REFERENCES partner(id)
);

CREATE INDEX idx_audit_timestamp ON audit_event(timestamp DESC);
CREATE INDEX idx_audit_entity ON audit_event(entity_type, entity_id);

CREATE TABLE board_authorization (
    partner_id   INTEGER NOT NULL,
    scope_kind   TEXT NOT NULL CHECK (scope_kind IN ('COMMON','SECTION')),
    section_code TEXT,
    CHECK ((scope_kind = 'SECTION') = (section_code IS NOT NULL)),
    FOREIGN KEY (partner_id) REFERENCES partner(id),
    FOREIGN KEY (section_code) REFERENCES section(code)
);

CREATE UNIQUE INDEX uq_board_authorization
    ON board_authorization(partner_id, scope_kind, COALESCE(section_code, ''));

-- +goose Down
DROP TABLE board_authorization;
DROP TABLE audit_event;
DROP TABLE report;
DROP TABLE expense_forecast;
DROP TABLE expense_subtype;
DROP TABLE expense_type;
DROP TABLE submission_window;
DROP TABLE partner_section;
DROP TABLE section;
DROP TABLE partner;
