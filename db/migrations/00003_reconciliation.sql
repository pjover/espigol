-- +goose Up
CREATE TABLE concession (
    year            INTEGER NOT NULL,
    group_code      TEXT NOT NULL,
    subtype_code    TEXT NOT NULL,
    concept         TEXT NOT NULL,
    requested_total TEXT NOT NULL,
    granted_amount  TEXT NOT NULL,
    PRIMARY KEY (year, group_code),
    FOREIGN KEY (year, subtype_code) REFERENCES expense_subtype(year, code)
);

CREATE TABLE concession_forecast (
    year        INTEGER NOT NULL,
    forecast_id TEXT NOT NULL,
    group_code  TEXT NOT NULL,
    PRIMARY KEY (year, forecast_id),
    FOREIGN KEY (year, group_code) REFERENCES concession(year, group_code),
    FOREIGN KEY (forecast_id) REFERENCES expense_forecast(id)
);
CREATE INDEX idx_concession_forecast_group ON concession_forecast(year, group_code);

CREATE TABLE invoice (
    id         INTEGER PRIMARY KEY,
    year       INTEGER NOT NULL,
    issuer     TEXT NOT NULL,
    nif        TEXT NOT NULL,
    number     TEXT NOT NULL,
    issue_date TEXT NOT NULL,
    net_amount TEXT NOT NULL,
    file_path  TEXT,
    notes      TEXT,
    UNIQUE (year, nif, number),
    FOREIGN KEY (year) REFERENCES submission_window(year)
);

CREATE TABLE invoice_payment (
    id         INTEGER PRIMARY KEY,
    invoice_id INTEGER NOT NULL,
    paid_on    TEXT NOT NULL,
    amount     TEXT NOT NULL,
    FOREIGN KEY (invoice_id) REFERENCES invoice(id) ON DELETE CASCADE
);
CREATE INDEX idx_invoice_payment_invoice ON invoice_payment(invoice_id);

CREATE TABLE forecast_invoice (
    forecast_id TEXT NOT NULL,
    invoice_id  INTEGER NOT NULL,
    amount      TEXT NOT NULL,
    PRIMARY KEY (forecast_id, invoice_id),
    FOREIGN KEY (forecast_id) REFERENCES expense_forecast(id),
    FOREIGN KEY (invoice_id) REFERENCES invoice(id) ON DELETE CASCADE
);
CREATE INDEX idx_forecast_invoice_invoice ON forecast_invoice(invoice_id);

-- +goose Down
DROP TABLE forecast_invoice;
DROP TABLE invoice_payment;
DROP TABLE invoice;
DROP TABLE concession_forecast;
DROP TABLE concession;
