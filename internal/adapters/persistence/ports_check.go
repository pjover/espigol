package persistence

import "github.com/pjover/espigol/internal/domain/ports"

var (
	_ ports.PartnerRepository            = (*PartnerRepository)(nil)
	_ ports.SectionRepository            = (*SectionRepository)(nil)
	_ ports.TaxonomyRepository           = (*TaxonomyRepository)(nil)
	_ ports.WindowRepository             = (*WindowRepository)(nil)
	_ ports.ForecastRepository           = (*ForecastRepository)(nil)
	_ ports.ReportRepository             = (*ReportRepository)(nil)
	_ ports.AuditLog                     = (*AuditLog)(nil)
	_ ports.BoardAuthorizationRepository = (*BoardAuthorizationRepository)(nil)
	_ ports.ConcessionRepository         = (*ConcessionRepository)(nil)
)
