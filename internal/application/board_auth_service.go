package application

import (
	"context"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/ports"
)

// BoardAuthorizationService is the admin-facing service for granting and
// revoking board member web-edit authorizations.
type BoardAuthorizationService struct {
	tx         ports.TxManager
	clock      ports.Clock
	adminEmail string
}

func NewBoardAuthorizationService(tx ports.TxManager, clock ports.Clock, adminEmail string) *BoardAuthorizationService {
	return &BoardAuthorizationService{tx: tx, clock: clock, adminEmail: adminEmail}
}

// Grant authorizes a board member partner to edit the given scope. For
// ScopeSection, sectionCode must reference an existing section.
func (s *BoardAuthorizationService) Grant(ctx context.Context, partnerID int, scope model.ScopeKind, sectionCode string) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		partner, ok, err := r.Partners.FindByID(ctx, partnerID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrPartnerNotFound
		}
		if !partner.BoardMember() {
			return ErrNotBoardMember
		}
		if scope == model.ScopeSection {
			sections, err := r.Sections.List(ctx)
			if err != nil {
				return err
			}
			exists := false
			for _, sec := range sections {
				if sec.Code() == sectionCode {
					exists = true
					break
				}
			}
			if !exists {
				return ErrSectionNotFound
			}
		}
		existing, err := r.BoardAuth.ListByPartner(ctx, partnerID)
		if err != nil {
			return err
		}
		for _, a := range existing {
			if a.ScopeKind() == scope && a.SectionCode() == sectionCode {
				return ErrAuthExists
			}
		}
		auth, err := model.NewBoardAuthorization(partnerID, scope, sectionCode)
		if err != nil {
			return err
		}
		if err := r.BoardAuth.Save(ctx, auth); err != nil {
			return err
		}
		return adminAudit(ctx, r, s.adminEmail, model.AuditBoardAuthChanged, "BoardAuthorization", itoa(partnerID), now)
	})
}

// Revoke removes a board authorization. COMMON scope uses sectionCode="".
func (s *BoardAuthorizationService) Revoke(ctx context.Context, partnerID int, scope model.ScopeKind, sectionCode string) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		removed, err := r.BoardAuth.Remove(ctx, partnerID, scope, sectionCode)
		if err != nil {
			return err
		}
		if removed == 0 {
			return nil // nothing matched; don't audit a no-op revoke
		}
		return adminAudit(ctx, r, s.adminEmail, model.AuditBoardAuthChanged, "BoardAuthorization", itoa(partnerID), now)
	})
}

// ListByPartner returns all board authorizations granted to the given partner.
func (s *BoardAuthorizationService) ListByPartner(ctx context.Context, partnerID int) ([]model.BoardAuthorization, error) {
	var out []model.BoardAuthorization
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		var err error
		out, err = r.BoardAuth.ListByPartner(ctx, partnerID)
		return err
	})
	return out, err
}
