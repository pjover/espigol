// Package importer also reads reconciliation import files
// (Home/import/reconciliation-<year>.json) into an application.ReconciliationImport.
// It performs format/parse validation only; referential integrity is enforced by
// ReconciliationService.AdminImport.
package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

type reconDoc struct {
	Year        int               `json:"year"`
	Concessions []reconConcession `json:"concessions"`
	Invoices    []reconInvoice    `json:"invoices"`
}

type reconConcession struct {
	GroupCode      string   `json:"groupCode"`
	SubtypeCode    string   `json:"subtypeCode"`
	Concept        string   `json:"concept"`
	RequestedTotal string   `json:"requestedTotal"`
	GrantedAmount  string   `json:"grantedAmount"`
	ForecastIDs    []string `json:"forecastIds"`
}

type reconInvoice struct {
	Issuer    string         `json:"issuer"`
	Nif       string         `json:"nif"`
	Number    string         `json:"number"`
	IssueDate string         `json:"issueDate"`
	NetAmount string         `json:"netAmount"`
	FilePath  string         `json:"filePath"`
	Notes     string         `json:"notes"`
	Payments  []reconPayment `json:"payments"`
	Links     []reconLink    `json:"links"`
}

type reconPayment struct {
	PaidOn string `json:"paidOn"`
	Amount string `json:"amount"`
}

type reconLink struct {
	ForecastID string `json:"forecastId"`
	Amount     string `json:"amount"`
}

const reconDateLayout = "2006-01-02"

func LoadReconciliation(path string, year int) (application.ReconciliationImport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return application.ReconciliationImport{}, fmt.Errorf("reading import file: %w", err)
	}
	var doc reconDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return application.ReconciliationImport{}, fmt.Errorf("parsing import file: %w", err)
	}
	if doc.Year != year {
		return application.ReconciliationImport{}, fmt.Errorf("file year %d does not match selected year %d", doc.Year, year)
	}

	out := application.ReconciliationImport{Year: year}
	for i, c := range doc.Concessions {
		req, err := model.MoneyFromString(c.RequestedTotal)
		if err != nil {
			return application.ReconciliationImport{}, fmt.Errorf("concession[%d]: invalid requestedTotal %q: %w", i, c.RequestedTotal, err)
		}
		granted, err := model.MoneyFromString(c.GrantedAmount)
		if err != nil {
			return application.ReconciliationImport{}, fmt.Errorf("concession[%d]: invalid grantedAmount %q: %w", i, c.GrantedAmount, err)
		}
		out.Concessions = append(out.Concessions, application.ConcessionInput{
			Year: year, GroupCode: c.GroupCode, SubtypeCode: c.SubtypeCode, Concept: c.Concept,
			RequestedTotal: req, GrantedAmount: granted, ForecastIDs: c.ForecastIDs,
		})
	}
	for i, inv := range doc.Invoices {
		net, err := model.MoneyFromString(inv.NetAmount)
		if err != nil {
			return application.ReconciliationImport{}, fmt.Errorf("invoice[%d]: invalid netAmount %q: %w", i, inv.NetAmount, err)
		}
		issued, err := time.Parse(reconDateLayout, inv.IssueDate)
		if err != nil {
			return application.ReconciliationImport{}, fmt.Errorf("invoice[%d]: invalid issueDate %q: %w", i, inv.IssueDate, err)
		}
		var pays []application.PaymentInput
		for j, p := range inv.Payments {
			amt, err := model.MoneyFromString(p.Amount)
			if err != nil {
				return application.ReconciliationImport{}, fmt.Errorf("invoice[%d].payment[%d]: invalid amount %q: %w", i, j, p.Amount, err)
			}
			paid, err := time.Parse(reconDateLayout, p.PaidOn)
			if err != nil {
				return application.ReconciliationImport{}, fmt.Errorf("invoice[%d].payment[%d]: invalid paidOn %q: %w", i, j, p.PaidOn, err)
			}
			pays = append(pays, application.PaymentInput{PaidOn: paid, Amount: amt})
		}
		var links []application.LinkInput
		for j, l := range inv.Links {
			amt, err := model.MoneyFromString(l.Amount)
			if err != nil {
				return application.ReconciliationImport{}, fmt.Errorf("invoice[%d].link[%d]: invalid amount %q: %w", i, j, l.Amount, err)
			}
			links = append(links, application.LinkInput{ForecastID: l.ForecastID, Amount: amt})
		}
		out.Invoices = append(out.Invoices, application.InvoiceInput{
			Year: year, Issuer: inv.Issuer, Nif: inv.Nif, Number: inv.Number, IssueDate: issued,
			NetAmount: net, FilePath: inv.FilePath, Notes: inv.Notes, Payments: pays, Links: links,
		})
	}
	return out, nil
}
