package model

import (
	"fmt"
	"time"
)

type ExpenseForecast struct {
	id             string
	partnerID      int
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

func NewExpenseForecast(id string, partnerID int, concept, description string,
	gross, approved Money, approvedOn *time.Time, plannedDate time.Time, year int,
	subtypeCode string, scope ExpenseScope, addedOn time.Time, enabled bool) (ExpenseForecast, error) {
	if partnerID < 0 {
		return ExpenseForecast{}, fmt.Errorf("partnerID must be >= 0, got %d", partnerID)
	}
	if subtypeCode == "" {
		return ExpenseForecast{}, fmt.Errorf("subtypeCode must not be empty")
	}
	if year != plannedDate.Year() {
		return ExpenseForecast{}, fmt.Errorf("year %d must equal plannedDate year %d", year, plannedDate.Year())
	}
	return ExpenseForecast{id, partnerID, concept, description, gross, approved,
		approvedOn, plannedDate, year, subtypeCode, scope, addedOn, enabled}, nil
}

func (f ExpenseForecast) ID() string           { return f.id }
func (f ExpenseForecast) PartnerID() int        { return f.partnerID }
func (f ExpenseForecast) Concept() string       { return f.concept }
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
