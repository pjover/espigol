// Package wire assembles the socis web server for `espigol --server`.
package wire

import (
	"fmt"

	"github.com/pjover/espigol/internal/adapters/auth"
	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	reportadapter "github.com/pjover/espigol/internal/adapters/report"
	"github.com/pjover/espigol/internal/adapters/system"
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
