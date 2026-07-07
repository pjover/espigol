package application

import (
	"context"
	"fmt"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/ports"
)

// ForecastImportEntry is one parsed row from an import file, already converted
// to domain-typed values by the importer adapter. AdminImport consumes these.
type ForecastImportEntry struct {
	PartnerID   int
	Scope       model.ScopeKind
	SectionCode string
	SubtypeCode string
	Concept     string
	Description string
	GrossAmount model.Money
	PlannedDate time.Time
}

// ImportResult reports how many forecasts a replace-all import removed and added.
type ImportResult struct {
	Deleted int
	Created int
}

// AdminImport replaces every forecast for year with entries, in one
// transaction. The year's window must be OPEN. Every entry's partner, subtype
// (year-scoped) and section (when SECTION-scoped) must already exist, otherwise
// the whole import rolls back and the year's existing forecasts are untouched.
// Imported forecasts are fresh: approved = 0, approvedOn = nil, enabled = true.
func (s *ForecastService) AdminImport(ctx context.Context, actorEmail string, year int,
	entries []ForecastImportEntry) (ImportResult, error) {
	now := s.clock.Now()
	var result ImportResult
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		w, ok, err := r.Windows.FindByYear(ctx, year)
		if err != nil {
			return err
		}
		if !ok {
			return ErrWindowNotFound
		}
		if w.State() != model.WindowOpen {
			return ErrWindowNotOpen
		}

		subtypes, err := r.Taxonomy.ListSubtypes(ctx, year)
		if err != nil {
			return err
		}
		subtypeCodes := make(map[string]bool, len(subtypes))
		for _, st := range subtypes {
			subtypeCodes[st.Code()] = true
		}
		sections, err := r.Sections.List(ctx)
		if err != nil {
			return err
		}
		sectionCodes := make(map[string]bool, len(sections))
		for _, sec := range sections {
			sectionCodes[sec.Code()] = true
		}

		// Validate and build every forecast before mutating anything.
		built := make([]model.ExpenseForecast, 0, len(entries))
		for i, e := range entries {
			partner, ok, err := r.Partners.FindByID(ctx, e.PartnerID)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("forecast[%d]: partner %d not found", i, e.PartnerID)
			}
			if !subtypeCodes[e.SubtypeCode] {
				return fmt.Errorf("forecast[%d]: subtype %q not found for year %d", i, e.SubtypeCode, year)
			}
			if e.Scope == model.ScopeSection && !sectionCodes[e.SectionCode] {
				return fmt.Errorf("forecast[%d]: section %q not found", i, e.SectionCode)
			}
			scope, err := model.NewScope(e.Scope, e.SectionCode)
			if err != nil {
				return fmt.Errorf("forecast[%d]: %w", i, err)
			}
			f, err := model.NewUnsavedExpenseForecast(partner, e.Concept, e.Description,
				e.GrossAmount, model.ZeroMoney(), nil, e.PlannedDate, year, e.SubtypeCode, scope, now, true)
			if err != nil {
				return fmt.Errorf("forecast[%d]: %w", i, err)
			}
			built = append(built, f)
		}

		// Replace-all: delete the year's existing forecasts, then insert.
		existing, err := r.Forecasts.ListByYear(ctx, year)
		if err != nil {
			return err
		}
		for _, old := range existing {
			if err := r.Forecasts.Delete(ctx, old.ID()); err != nil {
				return err
			}
			if err := forecastAuditEmail(ctx, r, actorEmail, model.AuditForecastDeleted, old.ID(), now); err != nil {
				return err
			}
			result.Deleted++
		}
		for _, f := range built {
			saved, err := r.Forecasts.Create(ctx, f)
			if err != nil {
				return err
			}
			if err := forecastAuditEmail(ctx, r, actorEmail, model.AuditForecastCreated, saved.ID(), now); err != nil {
				return err
			}
			result.Created++
		}
		return nil
	})
	if err != nil {
		return ImportResult{}, err
	}
	return result, nil
}
