package tui

import (
	"github.com/pjover/espigol/internal/adapters/persistence/backup"
	"github.com/pjover/espigol/internal/adapters/report"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/config"
	"github.com/pjover/espigol/internal/domain/ports"
)

// Deps bundles every application service and adapter the TUI panels need.
// It is built once by the wiring layer (internal/wire, Task 13) and passed
// down to NewApp; individual panels (Task 11/12) receive it unchanged so
// they can call the application services directly.
type Deps struct {
	Partners               *application.PartnerService
	Sections               *application.SectionService
	Taxonomy               *application.TaxonomyService
	BoardAuth              *application.BoardAuthorizationService
	Forecasts              *application.ForecastService
	Windows                *application.WindowService
	Reports                *application.ReportService
	Reconciliation         *application.ReconciliationService
	Exporter               report.ReportExporter
	ReconciliationExporter ports.ReconciliationExporter
	Backup                 backup.Backuper
	ActiveYear             ports.ActiveYearStore
	Clock                  ports.Clock
	Cfg                    *config.Config
}
