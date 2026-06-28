package application

import (
	"context"
	"strconv"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/ports"
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
		return appendAudit(ctx, r, model.AuditWindowOpened, year, now, "")
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

// appendAudit writes a system-actor audit event for a window/year.
func appendAudit(ctx context.Context, r ports.RepoSet, kind model.AuditKind, year int, at time.Time, payload string) error {
	var payloadPtr *string
	if payload != "" {
		payloadPtr = &payload
	}
	e, err := model.NewAuditEvent(0, nil, "system@espigol", kind, "SubmissionWindow", strconv.Itoa(year), at, payloadPtr)
	if err != nil {
		return err
	}
	return r.Audit.Append(ctx, e)
}
