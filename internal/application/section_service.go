package application

import (
	"context"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/ports"
)

// SectionInput is the form data for creating/updating a section.
type SectionInput struct {
	Code         string
	Label        string
	Active       bool
	DisplayOrder int
}

// SectionService is the admin-facing CRUD service for Section.
type SectionService struct {
	tx         ports.TxManager
	clock      ports.Clock
	adminEmail string
}

func NewSectionService(tx ports.TxManager, clock ports.Clock, adminEmail string) *SectionService {
	return &SectionService{tx: tx, clock: clock, adminEmail: adminEmail}
}

func (s *SectionService) Create(ctx context.Context, in SectionInput) (model.Section, error) {
	now := s.clock.Now()
	var created model.Section
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		sections, err := r.Sections.List(ctx)
		if err != nil {
			return err
		}
		for _, sec := range sections {
			if sec.Code() == in.Code {
				return ErrSectionExists
			}
		}
		sec, err := model.NewSection(in.Code, in.Label, in.Active, in.DisplayOrder)
		if err != nil {
			return err
		}
		if err := r.Sections.Save(ctx, sec); err != nil {
			return err
		}
		created = sec
		return adminAudit(ctx, r, s.adminEmail, model.AuditSectionSaved, "Section", in.Code, now)
	})
	return created, err
}

func (s *SectionService) Update(ctx context.Context, code string, in SectionInput) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		sections, err := r.Sections.List(ctx)
		if err != nil {
			return err
		}
		exists := false
		for _, sec := range sections {
			if sec.Code() == code {
				exists = true
				break
			}
		}
		if !exists {
			return ErrSectionNotFound
		}

		if !in.Active {
			windows, err := r.Windows.List(ctx)
			if err != nil {
				return err
			}
			for _, w := range windows {
				if w.State() == model.WindowClosed {
					continue
				}
				forecasts, err := r.Forecasts.ListByYear(ctx, w.Year())
				if err != nil {
					return err
				}
				for _, f := range forecasts {
					if f.Scope().Kind() == model.ScopeSection && f.Scope().SectionCode() == code {
						return ErrSectionInUse
					}
				}
			}
		}

		sec, err := model.NewSection(code, in.Label, in.Active, in.DisplayOrder)
		if err != nil {
			return err
		}
		if err := r.Sections.Save(ctx, sec); err != nil {
			return err
		}
		return adminAudit(ctx, r, s.adminEmail, model.AuditSectionSaved, "Section", code, now)
	})
}

func (s *SectionService) List(ctx context.Context) ([]model.Section, error) {
	var out []model.Section
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		var err error
		out, err = r.Sections.List(ctx)
		return err
	})
	return out, err
}
