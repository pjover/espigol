package model

import "fmt"

// Concession is a granted subsidy for a (year, groupCode) bundle — the funder's
// "Concedit" per "Grup". It bundles one or more ExpenseForecasts that share a
// (subtypeCode, Concept); membership lives in ConcessionForecast.
type Concession struct {
	year           int
	groupCode      string
	subtypeCode    string
	concept        string
	requestedTotal Money
	grantedAmount  Money
}

func NewConcession(year int, groupCode, subtypeCode, concept string, requested, granted Money) (Concession, error) {
	if groupCode == "" {
		return Concession{}, fmt.Errorf("concession groupCode must not be empty")
	}
	if subtypeCode == "" {
		return Concession{}, fmt.Errorf("concession subtypeCode must not be empty")
	}
	return Concession{year, groupCode, subtypeCode, concept, requested, granted}, nil
}

func (c Concession) Year() int              { return c.year }
func (c Concession) GroupCode() string      { return c.groupCode }
func (c Concession) SubtypeCode() string    { return c.subtypeCode }
func (c Concession) Concept() string        { return c.concept }
func (c Concession) RequestedTotal() Money  { return c.requestedTotal }
func (c Concession) GrantedAmount() Money   { return c.grantedAmount }

// ConcessionForecast links one ExpenseForecast to its Concession group. The
// (year, forecastID) pair is unique — a forecast belongs to at most one group.
type ConcessionForecast struct {
	year       int
	groupCode  string
	forecastID string
}

func NewConcessionForecast(year int, groupCode, forecastID string) (ConcessionForecast, error) {
	if groupCode == "" {
		return ConcessionForecast{}, fmt.Errorf("concessionForecast groupCode must not be empty")
	}
	if forecastID == "" {
		return ConcessionForecast{}, fmt.Errorf("concessionForecast forecastID must not be empty")
	}
	return ConcessionForecast{year, groupCode, forecastID}, nil
}

func (c ConcessionForecast) Year() int          { return c.year }
func (c ConcessionForecast) GroupCode() string  { return c.groupCode }
func (c ConcessionForecast) ForecastID() string { return c.forecastID }
