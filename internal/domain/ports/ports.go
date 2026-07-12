package ports

import (
	"context"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/services"
)

// Clock returns the current time.
type Clock interface {
	Now() time.Time
}

// PartnerRepository manages Partner aggregates.
type PartnerRepository interface {
	Save(ctx context.Context, p model.Partner) error
	FindByID(ctx context.Context, id int) (model.Partner, bool, error)
	FindByEmail(ctx context.Context, email string) (model.Partner, bool, error)
	List(ctx context.Context) ([]model.Partner, error)
}

// SectionRepository manages Section aggregates and their memberships.
type SectionRepository interface {
	Save(ctx context.Context, s model.Section) error
	List(ctx context.Context) ([]model.Section, error)
	AddMembership(ctx context.Context, m model.PartnerSection) error
	ListMembershipsByPartner(ctx context.Context, partnerID int) ([]model.PartnerSection, error)
	ListMemberships(ctx context.Context) ([]model.PartnerSection, error)
	RemoveMembershipsByPartner(ctx context.Context, partnerID int) error
}

// TaxonomyRepository manages ExpenseType and ExpenseSubtype.
type TaxonomyRepository interface {
	SaveType(ctx context.Context, t model.ExpenseType) error
	SaveSubtype(ctx context.Context, s model.ExpenseSubtype) error
	ListTypes(ctx context.Context, year int) ([]model.ExpenseType, error)
	ListSubtypes(ctx context.Context, year int) ([]model.ExpenseSubtype, error)
	DeleteType(ctx context.Context, year int, code string) error
	DeleteSubtype(ctx context.Context, year int, code string) error
}

// ConcessionRepository manages Concession grants and their forecast membership.
type ConcessionRepository interface {
	ListByYear(ctx context.Context, year int) ([]model.Concession, error)
	ListForecastLinksByYear(ctx context.Context, year int) ([]model.ConcessionForecast, error)
	Save(ctx context.Context, c model.Concession) error
	Delete(ctx context.Context, year int, groupCode string) error
	ReplaceMembership(ctx context.Context, year int, groupCode string, forecastIDs []string) error
	ReplaceForYear(ctx context.Context, year int, concessions []model.Concession, links []model.ConcessionForecast) error
}

// WindowRepository manages SubmissionWindow aggregates.
type WindowRepository interface {
	Save(ctx context.Context, w model.SubmissionWindow) error
	FindByYear(ctx context.Context, year int) (model.SubmissionWindow, bool, error)
	List(ctx context.Context) ([]model.SubmissionWindow, error)
}

// ForecastRepository manages ExpenseForecast aggregates.
type ForecastRepository interface {
	// Create inserts a new forecast, allocating the next CPYYnnn id for its year,
	// and returns the stored forecast (with its id set).
	Create(ctx context.Context, f model.ExpenseForecast) (model.ExpenseForecast, error)
	Save(ctx context.Context, f model.ExpenseForecast) error // update existing by id
	FindByID(ctx context.Context, id string) (model.ExpenseForecast, bool, error)
	ListByYear(ctx context.Context, year int) ([]model.ExpenseForecast, error)
	Delete(ctx context.Context, id string) error
}

// ReportRepository manages Report aggregates.
type ReportRepository interface {
	Insert(ctx context.Context, r model.Report) (int, error) // returns new id
	FindLatestByYear(ctx context.Context, year int) (model.Report, bool, error)
	MarkSuperseded(ctx context.Context, id int, at time.Time) error
}

// AuditLog appends and retrieves audit events.
type AuditLog interface {
	Append(ctx context.Context, e model.AuditEvent) error
	List(ctx context.Context) ([]model.AuditEvent, error)
}

// BoardAuthorizationRepository manages BoardAuthorization aggregates.
type BoardAuthorizationRepository interface {
	Save(ctx context.Context, a model.BoardAuthorization) error
	ListByPartner(ctx context.Context, partnerID int) ([]model.BoardAuthorization, error)
	// Remove deletes the matching authorization, returning the rows removed (0 if none).
	Remove(ctx context.Context, partnerID int, scopeKind model.ScopeKind, sectionCode string) (int64, error)
}

// InvoiceRepository manages Invoice aggregates (header + payments + forecast links).
type InvoiceRepository interface {
	ListByYear(ctx context.Context, year int) ([]model.Invoice, error)
	Save(ctx context.Context, inv model.Invoice) (model.Invoice, error)
	Delete(ctx context.Context, invoiceID int) error
	ReplaceForYear(ctx context.Context, year int, invoices []model.Invoice) error
}

// ReconciliationSnapshotRepository stores and retrieves the per-year
// reconciliation snapshot (one row per year, upsert semantics).
type ReconciliationSnapshotRepository interface {
	Save(ctx context.Context, s model.ReconciliationSnapshot) error
	FindByYear(ctx context.Context, year int) (model.ReconciliationSnapshot, bool, error)
}

// ActiveYearStore persists the TUI's last-selected year so it survives across
// sessions (single-row, upsert semantics). ActiveYear reports found=false when
// nothing has been stored yet.
type ActiveYearStore interface {
	ActiveYear(ctx context.Context) (year int, found bool, err error)
	SetActiveYear(ctx context.Context, year int) error
}

// ReconciliationRenderer renders a ReconciliationData snapshot to PDF bytes.
type ReconciliationRenderer interface {
	Render(rd services.ReconciliationData, generatedAt time.Time) ([]byte, error)
}

// ReconciliationExporter writes the PDF + Markdown files to outputDir and
// returns their paths.
type ReconciliationExporter interface {
	Export(rec model.ReconciliationSnapshot, outputDir string) ([]string, error)
}
