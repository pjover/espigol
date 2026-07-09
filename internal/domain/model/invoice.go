package model

import (
	"fmt"
	"time"
)

// InvoicePayment is one transfer against an invoice. An invoice may be paid in
// several payments; zero payments means unpaid.
type InvoicePayment struct {
	id        int
	invoiceID int
	paidOn    time.Time
	amount    Money
}

func NewInvoicePayment(id, invoiceID int, paidOn time.Time, amount Money) InvoicePayment {
	return InvoicePayment{id, invoiceID, paidOn, amount}
}

func (p InvoicePayment) ID() int           { return p.id }
func (p InvoicePayment) InvoiceID() int    { return p.invoiceID }
func (p InvoicePayment) PaidOn() time.Time { return p.paidOn }
func (p InvoicePayment) Amount() Money     { return p.amount }

// ForecastInvoice links an Invoice to an ExpenseForecast with the euro share of
// that invoice charged to the forecast — the M–N reconciliation truth.
type ForecastInvoice struct {
	forecastID string
	invoiceID  int
	amount     Money
}

func NewForecastInvoice(forecastID string, invoiceID int, amount Money) (ForecastInvoice, error) {
	if forecastID == "" {
		return ForecastInvoice{}, fmt.Errorf("forecastInvoice forecastID must not be empty")
	}
	return ForecastInvoice{forecastID, invoiceID, amount}, nil
}

func (f ForecastInvoice) ForecastID() string { return f.forecastID }
func (f ForecastInvoice) InvoiceID() int     { return f.invoiceID }
func (f ForecastInvoice) Amount() Money      { return f.amount }

// Invoice is a supplier invoice (Factura) and the aggregate root over its
// payments and forecast-links. NetAmount is the ex-VAT executed spend.
type Invoice struct {
	id        int
	year      int
	issuer    string
	nif       string
	number    string
	issueDate time.Time
	netAmount Money
	filePath  *string
	notes     *string
	payments  []InvoicePayment
	links     []ForecastInvoice
}

func NewInvoice(id, year int, issuer, nif, number string, issueDate time.Time,
	net Money, filePath, notes *string, payments []InvoicePayment, links []ForecastInvoice) (Invoice, error) {
	if issuer == "" {
		return Invoice{}, fmt.Errorf("invoice issuer must not be empty")
	}
	if number == "" {
		return Invoice{}, fmt.Errorf("invoice number must not be empty")
	}
	return Invoice{id, year, issuer, nif, number, issueDate, net, filePath, notes, payments, links}, nil
}

func (i Invoice) ID() int                    { return i.id }
func (i Invoice) Year() int                  { return i.year }
func (i Invoice) Issuer() string             { return i.issuer }
func (i Invoice) Nif() string                { return i.nif }
func (i Invoice) Number() string             { return i.number }
func (i Invoice) IssueDate() time.Time       { return i.issueDate }
func (i Invoice) NetAmount() Money           { return i.netAmount }
func (i Invoice) FilePath() *string          { return i.filePath }
func (i Invoice) Notes() *string             { return i.notes }
func (i Invoice) Payments() []InvoicePayment { return i.payments }
func (i Invoice) Links() []ForecastInvoice   { return i.links }

// PaidTotal sums the invoice's payment amounts.
func (i Invoice) PaidTotal() Money {
	total := ZeroMoney()
	for _, p := range i.payments {
		total = total.Plus(p.amount)
	}
	return total
}

// WithID returns a copy with the id set (used after the repository allocates it).
func (i Invoice) WithID(id int) Invoice { i.id = id; return i }
