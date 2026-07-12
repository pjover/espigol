package application

import (
	"context"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/ports"
)

type PartnerInput struct {
	ID                                              int
	Name, NickName, Surname, VatCode, Email, Mobile string
	PartnerType                                     model.PartnerType
	RiaNumber                                       int
	BoardMember                                     bool
}

type PartnerService struct {
	tx         ports.TxManager
	clock      ports.Clock
	adminEmail string
}

func NewPartnerService(tx ports.TxManager, clock ports.Clock, adminEmail string) *PartnerService {
	return &PartnerService{tx: tx, clock: clock, adminEmail: adminEmail}
}

func (s *PartnerService) Create(ctx context.Context, in PartnerInput) (model.Partner, error) {
	now := s.clock.Now()
	var created model.Partner
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		if _, ok, err := r.Partners.FindByID(ctx, in.ID); err != nil {
			return err
		} else if ok {
			return ErrPartnerExists
		}
		if in.Email != "" {
			if _, ok, err := r.Partners.FindByEmail(ctx, in.Email); err != nil {
				return err
			} else if ok {
				return ErrEmailTaken
			}
		}
		p, err := model.NewPartner(in.ID, in.Name, in.NickName, in.Surname, in.VatCode, in.Email, in.Mobile,
			in.PartnerType, in.RiaNumber, now, in.BoardMember)
		if err != nil {
			return err
		}
		if err := r.Partners.Save(ctx, p); err != nil {
			return err
		}
		created = p
		return adminAudit(ctx, r, s.adminEmail, model.AuditPartnerCreated, "Partner", itoa(in.ID), now)
	})
	return created, err
}

func (s *PartnerService) Update(ctx context.Context, id int, in PartnerInput) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		existing, ok, err := r.Partners.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if !ok {
			return ErrPartnerNotFound
		}
		// email uniqueness if changed
		if in.Email != "" && in.Email != existing.Email() {
			if _, taken, err := r.Partners.FindByEmail(ctx, in.Email); err != nil {
				return err
			} else if taken {
				return ErrEmailTaken
			}
		}
		p, err := model.NewPartner(id, in.Name, in.NickName, in.Surname, in.VatCode, in.Email, in.Mobile,
			in.PartnerType, in.RiaNumber, existing.AddedOn(), in.BoardMember)
		if err != nil {
			return err
		}
		if err := r.Partners.Save(ctx, p); err != nil {
			return err
		}
		return adminAudit(ctx, r, s.adminEmail, model.AuditPartnerEdited, "Partner", itoa(id), now)
	})
}

func (s *PartnerService) SetBoardMember(ctx context.Context, id int, board bool) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		existing, ok, err := r.Partners.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if !ok {
			return ErrPartnerNotFound
		}
		if err := r.Partners.Save(ctx, existing.WithBoardMember(board)); err != nil {
			return err
		}
		return adminAudit(ctx, r, s.adminEmail, model.AuditPartnerEdited, "Partner", itoa(id), now)
	})
}

func (s *PartnerService) SetSectionMemberships(ctx context.Context, partnerID int, sectionCodes []string) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		if _, ok, err := r.Partners.FindByID(ctx, partnerID); err != nil {
			return err
		} else if !ok {
			return ErrPartnerNotFound
		}
		sections, err := r.Sections.List(ctx)
		if err != nil {
			return err
		}
		valid := map[string]bool{}
		for _, sec := range sections {
			valid[sec.Code()] = true
		}
		if err := r.Sections.RemoveMembershipsByPartner(ctx, partnerID); err != nil {
			return err
		}
		for _, code := range sectionCodes {
			if !valid[code] {
				return ErrSectionNotFound
			}
			m, err := model.NewPartnerSection(partnerID, code)
			if err != nil {
				return err
			}
			if err := r.Sections.AddMembership(ctx, m); err != nil {
				return err
			}
		}
		return adminAudit(ctx, r, s.adminEmail, model.AuditPartnerEdited, "Partner", itoa(partnerID), now)
	})
}

func (s *PartnerService) List(ctx context.Context) ([]model.Partner, error) {
	var out []model.Partner
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		var err error
		out, err = r.Partners.List(ctx)
		return err
	})
	return out, err
}
