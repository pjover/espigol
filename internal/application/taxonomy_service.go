package application

import (
	"context"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/ports"
)

// TypeInput is the form data for creating/updating an ExpenseType.
type TypeInput struct {
	Year     int
	Code     string
	Label    string
	Category model.ExpenseCategory
}

// SubtypeInput is the form data for creating/updating an ExpenseSubtype.
type SubtypeInput struct {
	Year     int
	Code     string
	Label    string
	TypeCode string
}

// TaxonomyService is the admin-facing CRUD service for ExpenseType and
// ExpenseSubtype. All mutations are only allowed while the year's submission
// window is in DRAFT state.
type TaxonomyService struct {
	tx         ports.TxManager
	clock      ports.Clock
	adminEmail string
}

func NewTaxonomyService(tx ports.TxManager, clock ports.Clock, adminEmail string) *TaxonomyService {
	return &TaxonomyService{tx: tx, clock: clock, adminEmail: adminEmail}
}

// requireDraft loads the year's submission window and ensures it is in
// DRAFT state, returning ErrWindowNotFound or ErrTaxonomyLocked otherwise.
func requireDraft(ctx context.Context, r ports.RepoSet, year int) error {
	w, ok, err := r.Windows.FindByYear(ctx, year)
	if err != nil {
		return err
	}
	if !ok {
		return ErrWindowNotFound
	}
	if w.State() != model.WindowDraft {
		return ErrTaxonomyLocked
	}
	return nil
}

func (s *TaxonomyService) CreateType(ctx context.Context, in TypeInput) error {
	return s.saveType(ctx, in)
}

func (s *TaxonomyService) UpdateType(ctx context.Context, in TypeInput) error {
	return s.saveType(ctx, in)
}

func (s *TaxonomyService) saveType(ctx context.Context, in TypeInput) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		if err := requireDraft(ctx, r, in.Year); err != nil {
			return err
		}
		t, err := model.NewExpenseType(in.Year, in.Code, in.Label, in.Category)
		if err != nil {
			return err
		}
		if err := r.Taxonomy.SaveType(ctx, t); err != nil {
			return err
		}
		return adminAudit(ctx, r, s.adminEmail, model.AuditTaxonomySaved, "ExpenseType", in.Code, now)
	})
}

func (s *TaxonomyService) DeleteType(ctx context.Context, year int, code string) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		if err := requireDraft(ctx, r, year); err != nil {
			return err
		}
		subtypes, err := r.Taxonomy.ListSubtypes(ctx, year)
		if err != nil {
			return err
		}
		for _, st := range subtypes {
			if st.TypeCode() == code {
				return ErrTypeInUse
			}
		}
		if err := r.Taxonomy.DeleteType(ctx, year, code); err != nil {
			return err
		}
		return adminAudit(ctx, r, s.adminEmail, model.AuditTaxonomyDeleted, "ExpenseType", code, now)
	})
}

func (s *TaxonomyService) CreateSubtype(ctx context.Context, in SubtypeInput) error {
	return s.saveSubtype(ctx, in)
}

func (s *TaxonomyService) UpdateSubtype(ctx context.Context, in SubtypeInput) error {
	return s.saveSubtype(ctx, in)
}

func (s *TaxonomyService) saveSubtype(ctx context.Context, in SubtypeInput) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		if err := requireDraft(ctx, r, in.Year); err != nil {
			return err
		}
		types, err := r.Taxonomy.ListTypes(ctx, in.Year)
		if err != nil {
			return err
		}
		found := false
		for _, t := range types {
			if t.Code() == in.TypeCode {
				found = true
				break
			}
		}
		if !found {
			return ErrTypeNotFound
		}
		st, err := model.NewExpenseSubtype(in.Year, in.Code, in.Label, in.TypeCode)
		if err != nil {
			return err
		}
		if err := r.Taxonomy.SaveSubtype(ctx, st); err != nil {
			return err
		}
		return adminAudit(ctx, r, s.adminEmail, model.AuditTaxonomySaved, "ExpenseSubtype", in.Code, now)
	})
}

func (s *TaxonomyService) DeleteSubtype(ctx context.Context, year int, code string) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		if err := requireDraft(ctx, r, year); err != nil {
			return err
		}
		forecasts, err := r.Forecasts.ListByYear(ctx, year)
		if err != nil {
			return err
		}
		for _, f := range forecasts {
			if f.SubtypeCode() == code {
				return ErrSubtypeInUse
			}
		}
		if err := r.Taxonomy.DeleteSubtype(ctx, year, code); err != nil {
			return err
		}
		return adminAudit(ctx, r, s.adminEmail, model.AuditTaxonomyDeleted, "ExpenseSubtype", code, now)
	})
}

func (s *TaxonomyService) ListTypes(ctx context.Context, year int) ([]model.ExpenseType, error) {
	var out []model.ExpenseType
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		var err error
		out, err = r.Taxonomy.ListTypes(ctx, year)
		return err
	})
	return out, err
}

func (s *TaxonomyService) ListSubtypes(ctx context.Context, year int) ([]model.ExpenseSubtype, error) {
	var out []model.ExpenseSubtype
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		var err error
		out, err = r.Taxonomy.ListSubtypes(ctx, year)
		return err
	})
	return out, err
}
