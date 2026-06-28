// Package legacy reads the old espigol-java SQLite schema into plain structs.
package legacy

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

const legacyTimeLayout = "2006-01-02 15:04:05.000"

// Partner holds a row from the legacy partner table.
type Partner struct {
	ID                                    int
	Name, Surname, VatCode, Email, Mobile string
	PartnerType                           string
	RiaNumber                             int
	OliveSection, LivestockSection        bool
	AddedOn                               time.Time
	BoardMember                           bool
}

// ExpenseType holds a row from the legacy expense_type table.
type ExpenseType struct {
	Year           int
	Code, Label, Category string
}

// ExpenseSubtype holds a row from the legacy expense_subtype table.
type ExpenseSubtype struct {
	Year               int
	Code, Label, TypeCode string
}

// SubmissionWindow holds a row from the legacy submission_window table.
type SubmissionWindow struct {
	Year                          int
	State                         string
	OpenedAt, ClosedAt            *time.Time
	Deadline                      time.Time
	CurrentLimit, InvestmentLimit string // exact decimal strings
}

// ExpenseForecast holds a row from the legacy expense_forecast table.
type ExpenseForecast struct {
	ID                          string
	PartnerID                   int
	Concept, Description        string
	GrossAmount, ApprovedAmount string // exact decimal strings
	ApprovedOn                  *time.Time
	PlannedDate                 time.Time
	Year                        int
	SubtypeCode                 string
	Scope                       string // Catalan
	AddedOn                     time.Time
	Enabled                     bool
}

// Report holds a row from the legacy report table.
type Report struct {
	ID           int
	Year         int
	GeneratedAt  time.Time
	SnapshotJSON string
	Pdf          []byte
	SupersededAt *time.Time
}

// AuditEvent holds a row from the legacy audit_event table.
type AuditEvent struct {
	ID                                     int
	ActorID                                *int
	ActorEmail, Kind, EntityType, EntityID string
	Timestamp                              time.Time
	Payload                                *string
}

// Dump contains all rows read from the legacy database.
type Dump struct {
	Partners  []Partner
	Types     []ExpenseType
	Subtypes  []ExpenseSubtype
	Windows   []SubmissionWindow
	Forecasts []ExpenseForecast
	Reports   []Report
	Audits    []AuditEvent
}

func parseTime(s string) (time.Time, error) {
	return time.ParseInLocation(legacyTimeLayout, s, time.UTC)
}

func parseNullTime(ns sql.NullString) (*time.Time, error) {
	if !ns.Valid {
		return nil, nil
	}
	t, err := parseTime(ns.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// Read opens the legacy DB read-only and loads every table into a Dump.
func Read(path string) (*Dump, error) {
	conn, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	d := &Dump{}
	if d.Partners, err = readPartners(conn); err != nil {
		return nil, fmt.Errorf("partners: %w", err)
	}
	if d.Types, err = readTypes(conn); err != nil {
		return nil, fmt.Errorf("types: %w", err)
	}
	if d.Subtypes, err = readSubtypes(conn); err != nil {
		return nil, fmt.Errorf("subtypes: %w", err)
	}
	if d.Windows, err = readWindows(conn); err != nil {
		return nil, fmt.Errorf("windows: %w", err)
	}
	if d.Forecasts, err = readForecasts(conn); err != nil {
		return nil, fmt.Errorf("forecasts: %w", err)
	}
	if d.Reports, err = readReports(conn); err != nil {
		return nil, fmt.Errorf("reports: %w", err)
	}
	if d.Audits, err = readAudits(conn); err != nil {
		return nil, fmt.Errorf("audits: %w", err)
	}
	return d, nil
}

func readPartners(conn *sql.DB) ([]Partner, error) {
	rows, err := conn.Query(`
		SELECT id, name, surname, vat_code, email, mobile,
		       partner_type, ria_number, olive_section, livestock_section,
		       added_on, board_member
		FROM partner ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Partner
	for rows.Next() {
		var p Partner
		var addedOn string
		var oliveSection, livestockSection, boardMember int
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Surname, &p.VatCode, &p.Email, &p.Mobile,
			&p.PartnerType, &p.RiaNumber, &oliveSection, &livestockSection,
			&addedOn, &boardMember,
		); err != nil {
			return nil, err
		}
		p.OliveSection = oliveSection == 1
		p.LivestockSection = livestockSection == 1
		p.BoardMember = boardMember == 1
		if p.AddedOn, err = parseTime(addedOn); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func readTypes(conn *sql.DB) ([]ExpenseType, error) {
	rows, err := conn.Query(`
		SELECT year, code, label, category
		FROM expense_type ORDER BY year, code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExpenseType
	for rows.Next() {
		var t ExpenseType
		if err := rows.Scan(&t.Year, &t.Code, &t.Label, &t.Category); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func readSubtypes(conn *sql.DB) ([]ExpenseSubtype, error) {
	rows, err := conn.Query(`
		SELECT year, code, label, type_code
		FROM expense_subtype ORDER BY year, code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExpenseSubtype
	for rows.Next() {
		var s ExpenseSubtype
		if err := rows.Scan(&s.Year, &s.Code, &s.Label, &s.TypeCode); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// exactMoney verifies that the 2-decimal rendering of a money column equals its
// full-precision rendering (i.e. no precision was lost by SQLite's storage).
// twoDecimal is printf('%.2f', col), tenDecimal is printf('%.10f', col).
// It returns twoDecimal unchanged when the values agree, or an error.
func exactMoney(table, col, id, twoDecimal, tenDecimal string) (string, error) {
	// Parse both as float64 and compare at 2-decimal precision.
	// Equal strings trivially pass; mismatches are caught by comparing the
	// 2-decimal rendering of the full-precision value.
	var full float64
	if _, err := fmt.Sscanf(tenDecimal, "%f", &full); err != nil {
		return "", fmt.Errorf("money precision check: cannot parse %q for %s.%s id %s: %w",
			tenDecimal, table, col, id, err)
	}
	// Re-render full at 2 decimals and compare.
	if fmt.Sprintf("%.2f", full) != twoDecimal {
		return "", fmt.Errorf("money precision loss reading %s.%s for id %s: stored value %s rounds to %s, not %s",
			table, col, id, tenDecimal, fmt.Sprintf("%.2f", full), twoDecimal)
	}
	return twoDecimal, nil
}

func readWindows(conn *sql.DB) ([]SubmissionWindow, error) {
	rows, err := conn.Query(`
		SELECT year, state, opened_at, closed_at, deadline,
		       printf('%.2f', current_expense_limit),
		       printf('%.10f', current_expense_limit),
		       printf('%.2f', investment_expense_limit),
		       printf('%.10f', investment_expense_limit)
		FROM submission_window ORDER BY year`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SubmissionWindow
	for rows.Next() {
		var w SubmissionWindow
		var openedAt, closedAt sql.NullString
		var deadline string
		var currentFull, investmentFull string
		if err := rows.Scan(
			&w.Year, &w.State, &openedAt, &closedAt, &deadline,
			&w.CurrentLimit, &currentFull,
			&w.InvestmentLimit, &investmentFull,
		); err != nil {
			return nil, err
		}
		yearID := fmt.Sprintf("%d", w.Year)
		if w.CurrentLimit, err = exactMoney("submission_window", "current_expense_limit", yearID, w.CurrentLimit, currentFull); err != nil {
			return nil, err
		}
		if w.InvestmentLimit, err = exactMoney("submission_window", "investment_expense_limit", yearID, w.InvestmentLimit, investmentFull); err != nil {
			return nil, err
		}
		if w.OpenedAt, err = parseNullTime(openedAt); err != nil {
			return nil, err
		}
		if w.ClosedAt, err = parseNullTime(closedAt); err != nil {
			return nil, err
		}
		if w.Deadline, err = parseTime(deadline); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func readForecasts(conn *sql.DB) ([]ExpenseForecast, error) {
	rows, err := conn.Query(`
		SELECT id, partner_id, concept, description,
		       printf('%.2f', gross_amount), printf('%.10f', gross_amount),
		       printf('%.2f', approved_amount), printf('%.10f', approved_amount),
		       approved_on, planned_date, year, subtype_code, scope, added_on, enabled
		FROM expense_forecast ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExpenseForecast
	for rows.Next() {
		var f ExpenseForecast
		var approvedOn sql.NullString
		var planned, added string
		var enabled int
		var grossFull, approvedFull string
		if err := rows.Scan(
			&f.ID, &f.PartnerID, &f.Concept, &f.Description,
			&f.GrossAmount, &grossFull,
			&f.ApprovedAmount, &approvedFull,
			&approvedOn, &planned, &f.Year,
			&f.SubtypeCode, &f.Scope, &added, &enabled,
		); err != nil {
			return nil, err
		}
		if f.GrossAmount, err = exactMoney("expense_forecast", "gross_amount", f.ID, f.GrossAmount, grossFull); err != nil {
			return nil, err
		}
		if f.ApprovedAmount, err = exactMoney("expense_forecast", "approved_amount", f.ID, f.ApprovedAmount, approvedFull); err != nil {
			return nil, err
		}
		if f.ApprovedOn, err = parseNullTime(approvedOn); err != nil {
			return nil, err
		}
		if f.PlannedDate, err = parseTime(planned); err != nil {
			return nil, err
		}
		if f.AddedOn, err = parseTime(added); err != nil {
			return nil, err
		}
		f.Enabled = enabled == 1
		out = append(out, f)
	}
	return out, rows.Err()
}

func readReports(conn *sql.DB) ([]Report, error) {
	rows, err := conn.Query(`
		SELECT id, year, generated_at, snapshot_json, pdf, superseded_at
		FROM report ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Report
	for rows.Next() {
		var r Report
		var generatedAt string
		var supersededAt sql.NullString
		if err := rows.Scan(
			&r.ID, &r.Year, &generatedAt, &r.SnapshotJSON, &r.Pdf, &supersededAt,
		); err != nil {
			return nil, err
		}
		if r.GeneratedAt, err = parseTime(generatedAt); err != nil {
			return nil, err
		}
		if r.SupersededAt, err = parseNullTime(supersededAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func readAudits(conn *sql.DB) ([]AuditEvent, error) {
	rows, err := conn.Query(`
		SELECT id, actor_id, actor_email, kind, entity_type, entity_id, timestamp, payload
		FROM audit_event ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEvent
	for rows.Next() {
		var a AuditEvent
		var actorID sql.NullInt64
		var ts string
		var payload sql.NullString
		if err := rows.Scan(
			&a.ID, &actorID, &a.ActorEmail, &a.Kind, &a.EntityType, &a.EntityID,
			&ts, &payload,
		); err != nil {
			return nil, err
		}
		if actorID.Valid {
			v := int(actorID.Int64)
			a.ActorID = &v
		}
		if a.Timestamp, err = parseTime(ts); err != nil {
			return nil, err
		}
		if payload.Valid {
			s := payload.String
			a.Payload = &s
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
