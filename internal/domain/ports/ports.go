package ports

import (
	"context"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
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
}

// TaxonomyRepository manages ExpenseType and ExpenseSubtype.
type TaxonomyRepository interface {
	SaveType(ctx context.Context, t model.ExpenseType) error
	SaveSubtype(ctx context.Context, s model.ExpenseSubtype) error
	ListTypes(ctx context.Context, year int) ([]model.ExpenseType, error)
	ListSubtypes(ctx context.Context, year int) ([]model.ExpenseSubtype, error)
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
}
