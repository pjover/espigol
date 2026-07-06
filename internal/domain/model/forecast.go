package model

import (
	"fmt"
	"time"
)

type ExpenseForecast struct {
	id             string
	partner        Partner
	concept        string
	description    string
	grossAmount    Money
	approvedAmount Money
	approvedOn     *time.Time
	plannedDate    time.Time
	year           int
	subtypeCode    string
	scope          ExpenseScope
	addedOn        time.Time
	enabled        bool
}

func validateForecastFields(partner Partner, subtypeCode string, year int, plannedDate time.Time) error {
	if partner.ID() < 0 {
		return fmt.Errorf("partner.ID must be >= 0, got %d", partner.ID())
	}
	if subtypeCode == "" {
		return fmt.Errorf("subtypeCode must not be empty")
	}
	if year != plannedDate.Year() {
		return fmt.Errorf("year %d must equal plannedDate year %d", year, plannedDate.Year())
	}
	return nil
}

func NewExpenseForecast(id string, partner Partner, concept, description string,
	gross, approved Money, approvedOn *time.Time, plannedDate time.Time, year int,
	subtypeCode string, scope ExpenseScope, addedOn time.Time, enabled bool) (ExpenseForecast, error) {
	if id == "" {
		return ExpenseForecast{}, fmt.Errorf("forecast id must not be empty")
	}
	if err := validateForecastFields(partner, subtypeCode, year, plannedDate); err != nil {
		return ExpenseForecast{}, err
	}
	return ExpenseForecast{id, partner, concept, description, gross, approved,
		approvedOn, plannedDate, year, subtypeCode, scope, addedOn, enabled}, nil
}

// NewUnsavedExpenseForecast creates an ExpenseForecast without an id, for use before
// the repository allocates the real CPYYnnn id. All other validations still apply.
func NewUnsavedExpenseForecast(partner Partner, concept, description string,
	gross, approved Money, approvedOn *time.Time, plannedDate time.Time, year int,
	subtypeCode string, scope ExpenseScope, addedOn time.Time, enabled bool) (ExpenseForecast, error) {
	if err := validateForecastFields(partner, subtypeCode, year, plannedDate); err != nil {
		return ExpenseForecast{}, err
	}
	return ExpenseForecast{"", partner, concept, description, gross, approved,
		approvedOn, plannedDate, year, subtypeCode, scope, addedOn, enabled}, nil
}

func (f ExpenseForecast) ID() string             { return f.id }
func (f ExpenseForecast) Partner() Partner       { return f.partner }
func (f ExpenseForecast) Concept() string        { return f.concept }
func (f ExpenseForecast) Description() string    { return f.description }
func (f ExpenseForecast) GrossAmount() Money     { return f.grossAmount }
func (f ExpenseForecast) ApprovedAmount() Money  { return f.approvedAmount }
func (f ExpenseForecast) ApprovedOn() *time.Time { return f.approvedOn }
func (f ExpenseForecast) PlannedDate() time.Time { return f.plannedDate }
func (f ExpenseForecast) Year() int              { return f.year }
func (f ExpenseForecast) SubtypeCode() string    { return f.subtypeCode }
func (f ExpenseForecast) Scope() ExpenseScope    { return f.scope }
func (f ExpenseForecast) AddedOn() time.Time     { return f.addedOn }
func (f ExpenseForecast) Enabled() bool          { return f.enabled }

func (f ExpenseForecast) WithApprovedAmount(m Money) ExpenseForecast { f.approvedAmount = m; return f }
func (f ExpenseForecast) WithApprovedOn(t time.Time) ExpenseForecast { f.approvedOn = &t; return f }
func (f ExpenseForecast) WithEnabled(b bool) ExpenseForecast         { f.enabled = b; return f }
