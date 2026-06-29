package application

import (
	"context"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/ports"
)

// ForecastInput is the form data for creating/updating a forecast.
type ForecastInput struct {
	Concept     string
	Description string
	GrossAmount model.Money
	PlannedDate time.Time
	SubtypeCode string
	ScopeKind   model.ScopeKind
	SectionCode string
}

// DashboardView is the soci dashboard data.
type DashboardView struct {
	Year        int
	Deadline    time.Time
	Mine        []model.ExpenseForecast
	BoardScoped []model.ExpenseForecast
	ClosedYears []int
}

// ForecastService is the socis-facing forecast CRUD service.
type ForecastService struct {
	tx    ports.TxManager
	clock ports.Clock
}

func NewForecastService(tx ports.TxManager, clock ports.Clock) *ForecastService {
	return &ForecastService{tx: tx, clock: clock}
}

func openWindow(ctx context.Context, r ports.RepoSet) (model.SubmissionWindow, error) {
	all, err := r.Windows.List(ctx)
	if err != nil {
		return model.SubmissionWindow{}, err
	}
	for _, w := range all {
		if w.State() == model.WindowOpen {
			return w, nil
		}
	}
	return model.SubmissionWindow{}, ErrNoOpenWindow
}

// authorizeScope checks that the actor may act on a forecast of (scope, ownerID).
func authorizeScope(ctx context.Context, r ports.RepoSet, actor model.Partner, scope model.ExpenseScope, ownerID int) error {
	switch scope.Kind() {
	case model.ScopePartner:
		if ownerID != actor.ID() {
			return ErrForbidden
		}
		return nil
	case model.ScopeCommon, model.ScopeSection:
		if !actor.BoardMember() {
			return ErrForbidden
		}
		auths, err := r.BoardAuth.ListByPartner(ctx, actor.ID())
		if err != nil {
			return err
		}
		for _, a := range auths {
			if a.ScopeKind() == scope.Kind() && a.SectionCode() == scope.SectionCode() {
				return nil
			}
		}
		return ErrForbidden
	default:
		return ErrForbidden
	}
}

func buildScope(in ForecastInput) (model.ExpenseScope, error) {
	switch in.ScopeKind {
	case model.ScopeCommon:
		return model.NewCommonScope(), nil
	case model.ScopePartner:
		return model.NewPartnerScope(), nil
	case model.ScopeSection:
		return model.NewSectionScope(in.SectionCode)
	default:
		return model.ExpenseScope{}, ErrForbidden
	}
}

func (s *ForecastService) Create(ctx context.Context, actor model.Partner, in ForecastInput) (model.ExpenseForecast, error) {
	now := s.clock.Now()
	var created model.ExpenseForecast
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		w, err := openWindow(ctx, r)
		if err != nil {
			return err
		}
		scope, err := buildScope(in)
		if err != nil {
			return err
		}
		// the owner for PARTNER scope is the actor; for COMMON/SECTION it is also the actor (the entering board member)
		if err := authorizeScope(ctx, r, actor, scope, actor.ID()); err != nil {
			return err
		}
		f, err := model.NewUnsavedExpenseForecast(actor.ID(), in.Concept, in.Description,
			in.GrossAmount, model.ZeroMoney(), nil, in.PlannedDate, w.Year(), in.SubtypeCode, scope, now, true)
		if err != nil {
			return err
		}
		saved, err := r.Forecasts.Create(ctx, f)
		if err != nil {
			return err
		}
		created = saved
		return forecastAudit(ctx, r, actor, model.AuditForecastCreated, saved.ID(), now)
	})
	return created, err
}

func (s *ForecastService) Update(ctx context.Context, actor model.Partner, id string, in ForecastInput) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		w, err := openWindow(ctx, r)
		if err != nil {
			return err
		}
		existing, ok, err := r.Forecasts.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if !ok || existing.Year() != w.Year() {
			return ErrForecastNotFound
		}
		if err := authorizeScope(ctx, r, actor, existing.Scope(), existing.PartnerID()); err != nil {
			return err
		}
		scope, err := buildScope(in)
		if err != nil {
			return err
		}
		// the new scope must also be one the actor may use
		if err := authorizeScope(ctx, r, actor, scope, existing.PartnerID()); err != nil {
			return err
		}
		updated, err := model.NewExpenseForecast(id, existing.PartnerID(), in.Concept, in.Description,
			in.GrossAmount, existing.ApprovedAmount(), existing.ApprovedOn(), in.PlannedDate, w.Year(),
			in.SubtypeCode, scope, existing.AddedOn(), existing.Enabled())
		if err != nil {
			return err
		}
		if err := r.Forecasts.Save(ctx, updated); err != nil {
			return err
		}
		return forecastAudit(ctx, r, actor, model.AuditForecastEdited, id, now)
	})
}

func (s *ForecastService) Delete(ctx context.Context, actor model.Partner, id string) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		w, err := openWindow(ctx, r)
		if err != nil {
			return err
		}
		existing, ok, err := r.Forecasts.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if !ok || existing.Year() != w.Year() {
			return ErrForecastNotFound
		}
		if err := authorizeScope(ctx, r, actor, existing.Scope(), existing.PartnerID()); err != nil {
			return err
		}
		if err := r.Forecasts.Delete(ctx, id); err != nil {
			return err
		}
		return forecastAudit(ctx, r, actor, model.AuditForecastDeleted, id, now)
	})
}

func (s *ForecastService) Get(ctx context.Context, actor model.Partner, id string) (model.ExpenseForecast, error) {
	var out model.ExpenseForecast
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		f, ok, err := r.Forecasts.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if !ok {
			return ErrForecastNotFound
		}
		if err := authorizeScope(ctx, r, actor, f.Scope(), f.PartnerID()); err != nil {
			return err
		}
		out = f
		return nil
	})
	return out, err
}

func (s *ForecastService) Dashboard(ctx context.Context, actor model.Partner) (DashboardView, error) {
	var view DashboardView
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		all, err := r.Windows.List(ctx)
		if err != nil {
			return err
		}
		var open *model.SubmissionWindow
		for i := range all {
			if all[i].State() == model.WindowOpen {
				open = &all[i]
			}
			if all[i].State() == model.WindowClosed {
				view.ClosedYears = append(view.ClosedYears, all[i].Year())
			}
		}
		if open == nil {
			return nil // no open year; ClosedYears still populated
		}
		view.Year = open.Year()
		view.Deadline = open.Deadline()

		forecasts, err := r.Forecasts.ListByYear(ctx, open.Year())
		if err != nil {
			return err
		}
		var authCommon bool
		authSection := map[string]bool{}
		if actor.BoardMember() {
			auths, err := r.BoardAuth.ListByPartner(ctx, actor.ID())
			if err != nil {
				return err
			}
			for _, a := range auths {
				if a.ScopeKind() == model.ScopeCommon {
					authCommon = true
				}
				if a.ScopeKind() == model.ScopeSection {
					authSection[a.SectionCode()] = true
				}
			}
		}
		for _, f := range forecasts {
			switch f.Scope().Kind() {
			case model.ScopePartner:
				if f.PartnerID() == actor.ID() {
					view.Mine = append(view.Mine, f)
				}
			case model.ScopeCommon:
				if authCommon {
					view.BoardScoped = append(view.BoardScoped, f)
				}
			case model.ScopeSection:
				if authSection[f.Scope().SectionCode()] {
					view.BoardScoped = append(view.BoardScoped, f)
				}
			}
		}
		return nil
	})
	return view, err
}

// forecastAudit records a forecast mutation with the soci as actor.
func forecastAudit(ctx context.Context, r ports.RepoSet, actor model.Partner, kind model.AuditKind, forecastID string, at time.Time) error {
	actorID := actor.ID()
	e, err := model.NewAuditEvent(0, &actorID, actor.Email(), kind, "ExpenseForecast", forecastID, at, nil)
	if err != nil {
		return err
	}
	return r.Audit.Append(ctx, e)
}
