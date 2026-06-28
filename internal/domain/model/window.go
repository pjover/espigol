package model

import (
	"fmt"
	"time"
)

type SubmissionWindow struct {
	year                   int
	state                  WindowState
	openedAt               *time.Time
	closedAt               *time.Time
	deadline               time.Time
	currentExpenseLimit    Money
	investmentExpenseLimit Money
}

func NewSubmissionWindow(year int, state WindowState, openedAt, closedAt *time.Time,
	deadline time.Time, current, investment Money) (SubmissionWindow, error) {
	if year < 1900 {
		return SubmissionWindow{}, fmt.Errorf("year out of range: %d", year)
	}
	if _, err := ParseWindowState(string(state)); err != nil {
		return SubmissionWindow{}, err
	}
	return SubmissionWindow{year, state, openedAt, closedAt, deadline, current, investment}, nil
}

func (w SubmissionWindow) Year() int                       { return w.year }
func (w SubmissionWindow) State() WindowState              { return w.state }
func (w SubmissionWindow) OpenedAt() *time.Time            { return w.openedAt }
func (w SubmissionWindow) ClosedAt() *time.Time            { return w.closedAt }
func (w SubmissionWindow) Deadline() time.Time             { return w.deadline }
func (w SubmissionWindow) CurrentExpenseLimit() Money      { return w.currentExpenseLimit }
func (w SubmissionWindow) InvestmentExpenseLimit() Money   { return w.investmentExpenseLimit }

func (w SubmissionWindow) WithState(s WindowState) SubmissionWindow { w.state = s; return w }
func (w SubmissionWindow) WithOpenedAt(t time.Time) SubmissionWindow { w.openedAt = &t; return w }
func (w SubmissionWindow) WithClosedAt(t time.Time) SubmissionWindow { w.closedAt = &t; return w }
