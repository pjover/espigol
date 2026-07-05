package application

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/model/report"
	"github.com/pjover/espigol/internal/domain/ports"
	"github.com/pjover/espigol/internal/domain/services"
)

// WindowService orchestrates the submission-window lifecycle.
type WindowService struct {
	tx       ports.TxManager
	renderer ports.ReportRenderer
	clock    ports.Clock
}

func NewWindowService(tx ports.TxManager, renderer ports.ReportRenderer, clock ports.Clock) *WindowService {
	return &WindowService{tx: tx, renderer: renderer, clock: clock}
}

// List returns every submission window, for admin/TUI listing purposes.
func (s *WindowService) List(ctx context.Context) ([]model.SubmissionWindow, error) {
	var out []model.SubmissionWindow
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		var err error
		out, err = r.Windows.List(ctx)
		return err
	})
	return out, err
}

// CreateYear creates a new DRAFT window, copying the most recent prior year's
// limits and taxonomy.
func (s *WindowService) CreateYear(ctx context.Context, year int) (model.SubmissionWindow, error) {
	var created model.SubmissionWindow
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		if _, ok, err := r.Windows.FindByYear(ctx, year); err != nil {
			return err
		} else if ok {
			return ErrYearExists
		}

		all, err := r.Windows.List(ctx)
		if err != nil {
			return err
		}
		prior, ok := mostRecentPrior(all, year)
		if !ok {
			return ErrNoPriorYear
		}

		deadline := time.Date(year, time.December, 31, 23, 59, 59, 0, time.UTC)
		w, err := model.NewSubmissionWindow(year, model.WindowDraft, nil, nil, deadline,
			prior.CurrentExpenseLimit(), prior.InvestmentExpenseLimit())
		if err != nil {
			return err
		}
		if err := r.Windows.Save(ctx, w); err != nil {
			return err
		}

		types, err := r.Taxonomy.ListTypes(ctx, prior.Year())
		if err != nil {
			return err
		}
		for _, t := range types {
			nt, err := model.NewExpenseType(year, t.Code(), t.Label(), t.Category())
			if err != nil {
				return err
			}
			if err := r.Taxonomy.SaveType(ctx, nt); err != nil {
				return err
			}
		}
		subs, err := r.Taxonomy.ListSubtypes(ctx, prior.Year())
		if err != nil {
			return err
		}
		for _, st := range subs {
			ns, err := model.NewExpenseSubtype(year, st.Code(), st.Label(), st.TypeCode())
			if err != nil {
				return err
			}
			if err := r.Taxonomy.SaveSubtype(ctx, ns); err != nil {
				return err
			}
		}
		created = w
		return nil
	})
	return created, err
}

// Open transitions a DRAFT window to OPEN after validation.
func (s *WindowService) Open(ctx context.Context, year int) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		w, ok, err := r.Windows.FindByYear(ctx, year)
		if err != nil {
			return err
		}
		if !ok {
			return ErrWindowNotFound
		}
		if w.State() != model.WindowDraft {
			return ErrWrongState
		}
		if !w.Deadline().After(now) {
			return ErrDeadlinePassed
		}

		types, err := r.Taxonomy.ListTypes(ctx, year)
		if err != nil {
			return err
		}
		hasCurrent, hasInvestment := false, false
		for _, t := range types {
			switch t.Category() {
			case model.CategoryCurrent:
				hasCurrent = true
			case model.CategoryInvestment:
				hasInvestment = true
			}
		}
		if !hasCurrent || !hasInvestment {
			return ErrIncompleteTaxonomy
		}

		all, err := r.Windows.List(ctx)
		if err != nil {
			return err
		}
		for _, ow := range all {
			if ow.Year() != year && ow.State() == model.WindowOpen {
				return ErrAnotherWindowOpen
			}
		}

		if err := r.Windows.Save(ctx, w.WithState(model.WindowOpen).WithOpenedAt(now)); err != nil {
			return err
		}
		return appendAudit(ctx, r, model.AuditWindowOpened, "SubmissionWindow", strconv.Itoa(year), now, "")
	})
}

// mostRecentPrior returns the window with the greatest year strictly less than `year`.
func mostRecentPrior(all []model.SubmissionWindow, year int) (model.SubmissionWindow, bool) {
	var best model.SubmissionWindow
	found := false
	for _, w := range all {
		if w.Year() < year && (!found || w.Year() > best.Year()) {
			best = w
			found = true
		}
	}
	return best, found
}

// appendAudit writes a system-actor audit event for any entity.
func appendAudit(ctx context.Context, r ports.RepoSet, kind model.AuditKind, entityType, entityID string, at time.Time, payload string) error {
	var payloadPtr *string
	if payload != "" {
		payloadPtr = &payload
	}
	e, err := model.NewAuditEvent(0, nil, "system@espigol", kind, entityType, entityID, at, payloadPtr)
	if err != nil {
		return err
	}
	return r.Audit.Append(ctx, e)
}

// Close runs the allocation for an OPEN year, persists approved amounts, stores
// a Report snapshot, flips the window to CLOSED, and audits — atomically.
func (s *WindowService) Close(ctx context.Context, year int) (model.Report, error) {
	now := s.clock.Now()
	var saved model.Report
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		w, ok, err := r.Windows.FindByYear(ctx, year)
		if err != nil {
			return err
		}
		if !ok {
			return ErrWindowNotFound
		}
		if w.State() != model.WindowOpen {
			return ErrWrongState
		}

		rd, err := computeReportData(ctx, r, w)
		if err != nil {
			return err
		}
		writes, err := persistApproved(ctx, r, year, rd, now)
		if err != nil {
			return err
		}
		rep, err := s.buildReport(ctx, r, year, rd, now)
		if err != nil {
			return err
		}

		if err := r.Windows.Save(ctx, w.WithState(model.WindowClosed).WithClosedAt(now)); err != nil {
			return err
		}
		payload := fmt.Sprintf(`{"reportId":%d,"forecastsApproved":%d}`, rep.ID(), writes)
		if err := appendAudit(ctx, r, model.AuditWindowClosed, "SubmissionWindow", strconv.Itoa(year), now, payload); err != nil {
			return err
		}
		saved = rep
		return nil
	})
	return saved, err
}

// computeReportData gathers inputs from the tx repos and runs the allocation.
func computeReportData(ctx context.Context, r ports.RepoSet, w model.SubmissionWindow) (report.ReportData, error) {
	year := w.Year()
	all, err := r.Forecasts.ListByYear(ctx, year)
	if err != nil {
		return report.ReportData{}, err
	}
	enabled := make([]model.ExpenseForecast, 0, len(all))
	for _, f := range all {
		if f.Enabled() {
			enabled = append(enabled, f)
		}
	}
	partners, err := r.Partners.List(ctx)
	if err != nil {
		return report.ReportData{}, err
	}
	sections, err := r.Sections.List(ctx)
	if err != nil {
		return report.ReportData{}, err
	}
	memberships, err := r.Sections.ListMemberships(ctx)
	if err != nil {
		return report.ReportData{}, err
	}
	subCat, err := buildSubtypeCategory(ctx, r, year)
	if err != nil {
		return report.ReportData{}, err
	}
	return services.Compute(services.AllocationInput{
		Year:            year,
		Forecasts:       enabled,
		Partners:        partners,
		Sections:        sections,
		Memberships:     memberships,
		SubtypeCategory: subCat,
		CurrentLimit:    w.CurrentExpenseLimit(),
		InvestmentLimit: w.InvestmentExpenseLimit(),
	})
}

// buildReport serializes the snapshot, renders the pdf, and inserts the Report row.
func (s *WindowService) buildReport(ctx context.Context, r ports.RepoSet, year int, rd report.ReportData, now time.Time) (model.Report, error) {
	snapshot, err := SnapshotToJSON(rd)
	if err != nil {
		return model.Report{}, err
	}
	pdf, err := s.renderer.Render(rd, now)
	if err != nil {
		return model.Report{}, err
	}
	rep, err := model.NewReport(0, year, now, snapshot, pdf, nil)
	if err != nil {
		return model.Report{}, err
	}
	id, err := r.Reports.Insert(ctx, rep)
	if err != nil {
		return model.Report{}, err
	}
	return model.NewReport(id, year, now, snapshot, pdf, nil)
}

// buildSubtypeCategory maps each subtype code to its type's category for the year.
func buildSubtypeCategory(ctx context.Context, r ports.RepoSet, year int) (map[string]model.ExpenseCategory, error) {
	types, err := r.Taxonomy.ListTypes(ctx, year)
	if err != nil {
		return nil, err
	}
	catByType := make(map[string]model.ExpenseCategory, len(types))
	for _, t := range types {
		catByType[t.Code()] = t.Category()
	}
	subs, err := r.Taxonomy.ListSubtypes(ctx, year)
	if err != nil {
		return nil, err
	}
	out := make(map[string]model.ExpenseCategory, len(subs))
	for _, st := range subs {
		if cat, ok := catByType[st.TypeCode()]; ok {
			out[st.Code()] = cat
		}
	}
	return out, nil
}

// Reopen transitions a CLOSED window back to OPEN, clearing the closed-at
// timestamp so that forecasts can be submitted again.
func (s *WindowService) Reopen(ctx context.Context, year int) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		w, ok, err := r.Windows.FindByYear(ctx, year)
		if err != nil {
			return err
		}
		if !ok {
			return ErrWindowNotFound
		}
		if w.State() != model.WindowClosed {
			return ErrWrongState
		}

		all, err := r.Windows.List(ctx)
		if err != nil {
			return err
		}
		for _, ow := range all {
			if ow.Year() != year && ow.State() == model.WindowOpen {
				return ErrAnotherWindowOpen
			}
		}

		if err := r.Windows.Save(ctx, w.WithState(model.WindowOpen)); err != nil {
			return err
		}
		return appendAudit(ctx, r, model.AuditWindowOpened, "SubmissionWindow", strconv.Itoa(year), now, "reopen")
	})
}

// EditYear updates the deadline and expense limits for a DRAFT or OPEN year.
// Returns ErrWrongState for CLOSED years (use Amend to regenerate the report instead).
func (s *WindowService) EditYear(ctx context.Context, year int, deadline time.Time, current, investment model.Money) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		w, ok, err := r.Windows.FindByYear(ctx, year)
		if err != nil {
			return err
		}
		if !ok {
			return ErrWindowNotFound
		}
		if w.State() == model.WindowClosed {
			return ErrWrongState
		}
		if err := r.Windows.Save(ctx, w.WithDeadline(deadline).WithLimits(current, investment)); err != nil {
			return err
		}
		return appendAudit(ctx, r, model.AuditWindowEdited, "SubmissionWindow", strconv.Itoa(year), now, "")
	})
}

// Amend re-runs the allocation for a CLOSED year, supersedes the current report,
// inserts a new one, and updates approved amounts — without changing window state.
func (s *WindowService) Amend(ctx context.Context, year int) (model.Report, error) {
	now := s.clock.Now()
	var saved model.Report
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		w, ok, err := r.Windows.FindByYear(ctx, year)
		if err != nil {
			return err
		}
		if !ok {
			return ErrWindowNotFound
		}
		if w.State() != model.WindowClosed {
			return ErrWrongState
		}

		rd, err := computeReportData(ctx, r, w)
		if err != nil {
			return err
		}
		if _, err := persistApproved(ctx, r, year, rd, now); err != nil {
			return err
		}

		if latest, ok, err := r.Reports.FindLatestByYear(ctx, year); err != nil {
			return err
		} else if ok {
			if err := r.Reports.MarkSuperseded(ctx, latest.ID(), now); err != nil {
				return err
			}
		}

		rep, err := s.buildReport(ctx, r, year, rd, now)
		if err != nil {
			return err
		}
		if err := appendAudit(ctx, r, model.AuditReportGenerated, "Report", strconv.Itoa(rep.ID()), now, ""); err != nil {
			return err
		}
		saved = rep
		return nil
	})
	return saved, err
}

// collectApproved gathers approved amounts from every detail item, keyed by forecast id.
func collectApproved(rd report.ReportData) map[string]model.Money {
	out := map[string]model.Money{}
	for _, cat := range rd.Categories {
		for _, item := range cat.Common.Items {
			out[item.CpCode] = item.ApprovedAmount
		}
		for _, sd := range cat.Sections.SectionDetails {
			for _, item := range sd.Items {
				out[item.CpCode] = item.ApprovedAmount
			}
		}
		for _, pd := range cat.Sections.Partners.PartnerDetails {
			for _, item := range pd.Items {
				out[item.CpCode] = item.ApprovedAmount
			}
		}
	}
	return out
}

// persistApproved writes approved amounts onto enabled forecasts (skipping
// unchanged ones). Returns the number of rows written.
func persistApproved(ctx context.Context, r ports.RepoSet, year int, rd report.ReportData, now time.Time) (int, error) {
	approved := collectApproved(rd)
	all, err := r.Forecasts.ListByYear(ctx, year)
	if err != nil {
		return 0, err
	}
	byID := make(map[string]model.ExpenseForecast, len(all))
	for _, f := range all {
		if f.Enabled() {
			byID[f.ID()] = f
		}
	}
	ids := make([]string, 0, len(approved))
	for id := range approved {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	writes := 0
	for _, id := range ids {
		f, ok := byID[id]
		if !ok {
			continue
		}
		amt := approved[id]
		if f.ApprovedAmount().Cmp(amt) == 0 && f.ApprovedOn() != nil {
			continue
		}
		if err := r.Forecasts.Save(ctx, f.WithApprovedAmount(amt).WithApprovedOn(now)); err != nil {
			return 0, err
		}
		writes++
	}
	return writes, nil
}
