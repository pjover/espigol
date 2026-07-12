package ports

import "context"

// RepoSet is the set of transaction-scoped repositories handed to a WithinTx
// closure. All share one transaction.
type RepoSet struct {
	Partners                PartnerRepository
	Forecasts               ForecastRepository
	Windows                 WindowRepository
	Taxonomy                TaxonomyRepository
	Sections                SectionRepository
	Reports                 ReportRepository
	Audit                   AuditLog
	BoardAuth               BoardAuthorizationRepository
	Concessions             ConcessionRepository
	Invoices                InvoiceRepository
	ReconciliationSnapshots ReconciliationSnapshotRepository
}

// TxManager runs a unit of work inside a single database transaction, handing
// the closure a RepoSet bound to that transaction. The transaction commits if
// fn returns nil, and rolls back on error or panic.
type TxManager interface {
	WithinTx(ctx context.Context, fn func(RepoSet) error) error
}
