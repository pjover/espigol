// Package services — reconciliation.go is the Phase 2 pure algorithm that
// turns the year's Concession + Invoice data into a per-forecast
// AssignedSubsidy snapshot. It has zero I/O; orchestration lives in
// internal/application/reconciliation_service.go.
package services

import (
	"time"

	"github.com/pjover/espigol/internal/domain/model"
)

// ReconciliationInput is everything ComputeReconciliation needs to compute
// per-forecast subsidies for a single year. The application service builds
// this from ports.RepoSet reads inside a TxManager.WithinTx.
type ReconciliationInput struct {
	Year        int
	Forecasts   []model.ExpenseForecast // ALL year's forecasts; algorithm filters Enabled==true
	Concessions []model.Concession
	Links       []model.ConcessionForecast // membership (year, groupCode, forecastID)
	Invoices    []model.Invoice            // aggregate: payments + links included
	Subtypes    []model.ExpenseSubtype     // year-scoped
	Types       []model.ExpenseType        // year-scoped (subtype→type→category lookup)
	Partners    []model.Partner
}

// ReconciliationData is the JSON-serialisable snapshot produced by
// ComputeReconciliation. Categories are ordered CURRENT then INVESTMENT.
// Empty categories/subtypes/concessions are omitted.
type ReconciliationData struct {
	Year       int                      `json:"year"`
	Categories []CategoryReconciliation `json:"categories"`
}

type CategoryReconciliation struct {
	Category     model.ExpenseCategory   `json:"category"`
	Requested    model.Money             `json:"requested"`
	Granted      model.Money             `json:"granted"`
	Executed     model.Money             `json:"executed"`
	Assigned     model.Money             `json:"assigned"`
	NetDeviation model.Money             `json:"netDeviation"` // Σ Subtype.Deviation
	Subtypes     []SubtypeReconciliation `json:"subtypes"`
}

type SubtypeReconciliation struct {
	Code        string                     `json:"code"`
	Label       string                     `json:"label"`
	Requested   model.Money                `json:"requested"`
	Granted     model.Money                `json:"granted"`
	Executed    model.Money                `json:"executed"`
	Assigned    model.Money                `json:"assigned"`
	Deviation   model.Money                `json:"deviation"` // Granted − Executed (raw)
	Concessions []ConcessionReconciliation `json:"concessions"`
}

type ConcessionReconciliation struct {
	GroupCode  string                    `json:"groupCode"`
	Concept    string                    `json:"concept"`
	Requested  model.Money               `json:"requested"`
	Granted    model.Money               `json:"granted"`
	Executed   model.Money               `json:"executed"`
	Assigned   model.Money               `json:"assigned"`
	Difference model.Money               `json:"difference"` // Granted − Executed
	Forecasts  []ForecastReconciliation  `json:"forecasts"`
}

type ForecastReconciliation struct {
	ForecastID     string                `json:"forecastId"`
	PartnerID      int                   `json:"partnerId"`
	Concept        string                `json:"concept"`
	GrossAmount    model.Money           `json:"grossAmount"`
	ApprovedAmount model.Money           `json:"approvedAmount"`
	Executed       model.Money           `json:"executed"`
	Pending        model.Money           `json:"pending"`
	Assigned       model.Money           `json:"assigned"`
	Status         ForecastReconStatus   `json:"status"`
	Invoices       []InvoiceContribution `json:"invoices"`
}

type InvoiceContribution struct {
	InvoiceID    int         `json:"invoiceId"`
	Issuer       string      `json:"issuer"`
	Number       string      `json:"number"`
	IssueDate    time.Time   `json:"issueDate"`
	LinkedAmount model.Money `json:"linkedAmount"`
	FullyPaid    bool        `json:"fullyPaid"`
	PaidOn       *time.Time  `json:"paidOn,omitempty"`
}

// ForecastReconStatus flags each forecast's reconciliation state. Precedence
// (first-match wins as applied by the algorithm): NoInvoice, PaymentPending,
// OverExecuted, PartiallyJustified, FullyJustified.
type ForecastReconStatus int

const (
	StatusFullyJustified ForecastReconStatus = iota
	StatusPartiallyJustified
	StatusOverExecuted
	StatusPaymentPending
	StatusNoInvoice
)

// ComputeReconciliation is the pure entry point. Given the year's forecasts,
// concessions, invoices, taxonomy, and partners, it returns the snapshot tree
// described by the Phase 2 spec. Skeleton in Task 1; filled in Tasks 2-5.
func ComputeReconciliation(in ReconciliationInput) (ReconciliationData, error) {
	return ReconciliationData{Year: in.Year}, nil
}
