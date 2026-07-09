package application

import (
	"context"
	"fmt"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/ports"
	"github.com/pjover/espigol/internal/domain/services"
)

// PaymentInput / LinkInput / ConcessionInput / InvoiceInput are the driving-side
// DTOs the TUI and importer build. Amounts are already model.Money.
type PaymentInput struct {
	PaidOn time.Time
	Amount model.Money
}

type LinkInput struct {
	ForecastID string
	Amount     model.Money
}

type ConcessionInput struct {
	Year           int
	GroupCode      string
	SubtypeCode    string
	Concept        string
	RequestedTotal model.Money
	GrantedAmount  model.Money
	ForecastIDs    []string
}

type InvoiceInput struct {
	ID        int
	Year      int
	Issuer    string
	Nif       string
	Number    string
	IssueDate time.Time
	NetAmount model.Money
	FilePath  string
	Notes     string
	Payments  []PaymentInput
	Links     []LinkInput
}

type ReconciliationImport struct {
	Year        int
	Concessions []ConcessionInput
	Invoices    []InvoiceInput
}

type ReconciliationImportResult struct {
	Concessions int
	Invoices    int
	Warnings    []string
}

type ReconciliationService struct {
	tx ports.TxManager
}

func NewReconciliationService(tx ports.TxManager) *ReconciliationService {
	return &ReconciliationService{tx: tx}
}

func strOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// buildInvoice converts an InvoiceInput into a model.Invoice aggregate.
func buildInvoice(in InvoiceInput) (model.Invoice, error) {
	pays := make([]model.InvoicePayment, 0, len(in.Payments))
	for _, p := range in.Payments {
		pays = append(pays, model.NewInvoicePayment(0, in.ID, p.PaidOn, p.Amount))
	}
	links := make([]model.ForecastInvoice, 0, len(in.Links))
	for _, l := range in.Links {
		fi, err := model.NewForecastInvoice(l.ForecastID, in.ID, l.Amount)
		if err != nil {
			return model.Invoice{}, err
		}
		links = append(links, fi)
	}
	return model.NewInvoice(in.ID, in.Year, in.Issuer, in.Nif, in.Number, in.IssueDate,
		in.NetAmount, strOrNil(in.FilePath), strOrNil(in.Notes), pays, links)
}

// cent is the 0,01 € tolerance for soft integrity checks (matches the workbook's
// ABS(diff) < 0.01 OK? test).
var cent = mustCent()

func mustCent() model.Money {
	c, _ := model.MoneyFromString("0.01")
	return c
}

// validateReferences checks subtype + forecast existence for the year and
// returns soft-check warnings (Catalan). Hard failures return an error.
func validateReferences(ctx context.Context, r ports.RepoSet, in ReconciliationImport) ([]string, error) {
	subs, err := r.Taxonomy.ListSubtypes(ctx, in.Year)
	if err != nil {
		return nil, err
	}
	subCodes := map[string]bool{}
	for _, s := range subs {
		subCodes[s.Code()] = true
	}
	forecasts, err := r.Forecasts.ListByYear(ctx, in.Year)
	if err != nil {
		return nil, err
	}
	grossByID := map[string]model.Money{}
	for _, f := range forecasts {
		grossByID[f.ID()] = f.GrossAmount()
	}

	var warnings []string
	for _, c := range in.Concessions {
		if !subCodes[c.SubtypeCode] {
			return nil, fmt.Errorf("%w: group %s subtype %q", ErrConcessionSubtypeMissing, c.GroupCode, c.SubtypeCode)
		}
		sumPrevist := model.ZeroMoney()
		for _, fid := range c.ForecastIDs {
			g, ok := grossByID[fid]
			if !ok {
				return nil, fmt.Errorf("%w: group %s forecast %q", ErrReconForecastMissing, c.GroupCode, fid)
			}
			sumPrevist = sumPrevist.Plus(g)
		}
		if c.GrantedAmount.Cmp(c.RequestedTotal) > 0 {
			warnings = append(warnings, fmt.Sprintf("Concessió %s: Concedit (%s) > Demanat (%s)",
				c.GroupCode, c.GrantedAmount, c.RequestedTotal))
		}
		if len(c.ForecastIDs) > 0 && c.RequestedTotal.Minus(sumPrevist).Decimal().Abs().Cmp(cent.Decimal()) > 0 {
			warnings = append(warnings, fmt.Sprintf("Concessió %s: Demanat (%s) ≠ Σ Previst (%s)",
				c.GroupCode, c.RequestedTotal, sumPrevist))
		}
	}
	for _, inv := range in.Invoices {
		sumLinks := model.ZeroMoney()
		for _, l := range inv.Links {
			if _, ok := grossByID[l.ForecastID]; !ok {
				return nil, fmt.Errorf("%w: invoice %s forecast %q", ErrReconForecastMissing, inv.Number, l.ForecastID)
			}
			sumLinks = sumLinks.Plus(l.Amount)
		}
		if sumLinks.Cmp(inv.NetAmount) > 0 {
			warnings = append(warnings, fmt.Sprintf("Factura %s: Σ enllaços (%s) > Import (%s)",
				inv.Number, sumLinks, inv.NetAmount))
		}
		sumPays := model.ZeroMoney()
		for _, p := range inv.Payments {
			sumPays = sumPays.Plus(p.Amount)
		}
		if sumPays.Cmp(inv.NetAmount) > 0 {
			warnings = append(warnings, fmt.Sprintf("Factura %s: Σ pagaments (%s) > Import (%s)",
				inv.Number, sumPays, inv.NetAmount))
		}
	}
	return warnings, nil
}

func (s *ReconciliationService) AdminImport(ctx context.Context, in ReconciliationImport) (ReconciliationImportResult, error) {
	var res ReconciliationImportResult
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		if _, ok, err := r.Windows.FindByYear(ctx, in.Year); err != nil {
			return err
		} else if !ok {
			return ErrWindowNotFound
		}

		warnings, err := validateReferences(ctx, r, in)
		if err != nil {
			return err
		}

		// Build concessions + membership links.
		concessions := make([]model.Concession, 0, len(in.Concessions))
		var links []model.ConcessionForecast
		for _, c := range in.Concessions {
			mc, err := model.NewConcession(c.Year, c.GroupCode, c.SubtypeCode, c.Concept, c.RequestedTotal, c.GrantedAmount)
			if err != nil {
				return err
			}
			concessions = append(concessions, mc)
			for _, fid := range c.ForecastIDs {
				cf, err := model.NewConcessionForecast(c.Year, c.GroupCode, fid)
				if err != nil {
					return err
				}
				links = append(links, cf)
			}
		}
		if err := r.Concessions.ReplaceForYear(ctx, in.Year, concessions, links); err != nil {
			return err
		}

		invoices := make([]model.Invoice, 0, len(in.Invoices))
		for _, invIn := range in.Invoices {
			inv, err := buildInvoice(invIn)
			if err != nil {
				return err
			}
			invoices = append(invoices, inv)
		}
		if err := r.Invoices.ReplaceForYear(ctx, in.Year, invoices); err != nil {
			return err
		}

		res = ReconciliationImportResult{Concessions: len(concessions), Invoices: len(invoices), Warnings: warnings}
		return nil
	})
	if err != nil {
		return ReconciliationImportResult{}, err
	}
	return res, nil
}

func (s *ReconciliationService) ListConcessions(ctx context.Context, year int) ([]model.Concession, error) {
	var out []model.Concession
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		var err error
		out, err = r.Concessions.ListByYear(ctx, year)
		return err
	})
	return out, err
}

func (s *ReconciliationService) ListConcessionLinks(ctx context.Context, year int) ([]model.ConcessionForecast, error) {
	var out []model.ConcessionForecast
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		var err error
		out, err = r.Concessions.ListForecastLinksByYear(ctx, year)
		return err
	})
	return out, err
}

func (s *ReconciliationService) SaveConcession(ctx context.Context, in ConcessionInput) error {
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		subs, err := r.Taxonomy.ListSubtypes(ctx, in.Year)
		if err != nil {
			return err
		}
		found := false
		for _, sub := range subs {
			if sub.Code() == in.SubtypeCode {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%w: %q", ErrConcessionSubtypeMissing, in.SubtypeCode)
		}
		c, err := model.NewConcession(in.Year, in.GroupCode, in.SubtypeCode, in.Concept, in.RequestedTotal, in.GrantedAmount)
		if err != nil {
			return err
		}
		if err := r.Concessions.Save(ctx, c); err != nil {
			return err
		}
		return r.Concessions.ReplaceMembership(ctx, in.Year, in.GroupCode, in.ForecastIDs)
	})
}

func (s *ReconciliationService) DeleteConcession(ctx context.Context, year int, groupCode string) error {
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		return r.Concessions.Delete(ctx, year, groupCode)
	})
}

func (s *ReconciliationService) ListInvoices(ctx context.Context, year int) ([]model.Invoice, error) {
	var out []model.Invoice
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		var err error
		out, err = r.Invoices.ListByYear(ctx, year)
		return err
	})
	return out, err
}

func (s *ReconciliationService) SaveInvoice(ctx context.Context, in InvoiceInput) (model.Invoice, error) {
	var saved model.Invoice
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		inv, err := buildInvoice(in)
		if err != nil {
			return err
		}
		saved, err = r.Invoices.Save(ctx, inv)
		return err
	})
	return saved, err
}

func (s *ReconciliationService) DeleteInvoice(ctx context.Context, invoiceID int) error {
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		return r.Invoices.Delete(ctx, invoiceID)
	})
}

// Compute produces the year's reconciliation snapshot: per-forecast
// AssignedSubsidy plus subtype/category roll-ups and category-net deviations.
// Read-only — runs inside a single WithinTx for a consistent snapshot but
// never writes. No window-state gate: reconciliation is a year-keyed overlay
// editable in any window state (matches the rest of ReconciliationService).
func (s *ReconciliationService) Compute(ctx context.Context, year int) (services.ReconciliationData, error) {
	var out services.ReconciliationData
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		forecasts, err := r.Forecasts.ListByYear(ctx, year)
		if err != nil {
			return err
		}
		concessions, err := r.Concessions.ListByYear(ctx, year)
		if err != nil {
			return err
		}
		links, err := r.Concessions.ListForecastLinksByYear(ctx, year)
		if err != nil {
			return err
		}
		invoices, err := r.Invoices.ListByYear(ctx, year)
		if err != nil {
			return err
		}
		subtypes, err := r.Taxonomy.ListSubtypes(ctx, year)
		if err != nil {
			return err
		}
		types, err := r.Taxonomy.ListTypes(ctx, year)
		if err != nil {
			return err
		}

		out, err = services.ComputeReconciliation(services.ReconciliationInput{
			Year:        year,
			Forecasts:   forecasts,
			Concessions: concessions,
			Links:       links,
			Invoices:    invoices,
			Subtypes:    subtypes,
			Types:       types,
		})
		return err
	})
	return out, err
}
