// Package wire assembles the socis web server for `espigol --server` and the
// admin TUI for `espigol` (default mode).
package wire

import (
	"fmt"

	"github.com/pjover/espigol/internal/adapters/auth"
	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/backup"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	reportadapter "github.com/pjover/espigol/internal/adapters/report"
	"github.com/pjover/espigol/internal/adapters/system"
	"github.com/pjover/espigol/internal/adapters/tui"
	"github.com/pjover/espigol/internal/adapters/web"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/config"
)

// Server opens the database and assembles the socis web server.
func Server(cfg *config.Config) (*web.Server, error) {
	conn, err := db.Open(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	q := sqlc.New(conn)
	clock := system.SystemClock{}

	partners := persistence.NewPartnerRepository(q)
	reports := persistence.NewReportRepository(q)
	taxonomy := persistence.NewTaxonomyRepository(q)
	txm := persistence.NewTxManager(conn)

	sessions := auth.NewSessionStore(q, clock)
	authn := auth.NewAuthenticator(cfg, sessions, partners)
	forecasts := application.NewForecastService(txm, clock)

	deps := web.Deps{
		Forecasts: forecasts,
		Auth:      authn,
		Sessions:  sessions,
		Partners:  partners,
		Reports:   reports,
		HTML:      reportadapter.HTMLRenderer{},
		Taxonomy:  taxonomy,
		Cfg:       cfg,
		Secure:    !authn.IsDev(),
	}
	return web.NewServer(deps), nil
}

// TUI opens the database and assembles the admin TUI, with the real
// PDFRenderer wired into WindowService.Close (the deferred Phase-5 wiring,
// no longer the no-op).
func TUI(cfg *config.Config) (*tui.App, error) {
	conn, err := db.Open(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	clock := system.SystemClock{}
	txm := persistence.NewTxManager(conn)

	pdf := reportadapter.PDFRenderer{BusinessName: cfg.BusinessName, LogoPath: cfg.LogoPath}
	// Stopgap: real PDF renderer, but not yet wired to a TUI key. Task 9 adds
	// the "g" keybinding that drives ReconciliationService.GenerateReport.
	reconciliationRenderer := reportadapter.ReconciliationPDFRenderer{
		BusinessName: cfg.BusinessName,
		LogoPath:     cfg.LogoPath,
	}

	deps := tui.Deps{
		Partners:       application.NewPartnerService(txm, clock, cfg.Admin.Email),
		Sections:       application.NewSectionService(txm, clock, cfg.Admin.Email),
		Taxonomy:       application.NewTaxonomyService(txm, clock, cfg.Admin.Email),
		BoardAuth:      application.NewBoardAuthorizationService(txm, clock, cfg.Admin.Email),
		Forecasts:      application.NewForecastService(txm, clock),
		Windows:        application.NewWindowService(txm, pdf, clock),
		Reports:        application.NewReportService(txm),
		Reconciliation: application.NewReconciliationService(txm, clock, reconciliationRenderer),
		Exporter:       reportadapter.NewReportExporter(pdf),
		Backup:         backup.New(conn, cfg.DBPath, cfg.BackupDir, clock),
		Cfg:            cfg,
	}

	panels := []tui.Panel{
		tui.NewYearsPanel(deps),
		tui.NewPartnersPanel(deps),
		tui.NewSectionsPanel(deps),
		tui.NewTaxonomyPanel(deps),
		tui.NewForecastsPanel(deps),
		tui.NewAjutsPanel(deps),
		tui.NewAdminPanel(deps),
	}

	return tui.NewApp(deps, panels), nil
}
