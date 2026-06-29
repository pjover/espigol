// Package web is the socis-facing HTTP driving adapter.
package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/pjover/espigol/internal/adapters/auth"
	reportadapter "github.com/pjover/espigol/internal/adapters/report"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/config"
	"github.com/pjover/espigol/internal/domain/model"
)

// reportReader is the minimal interface web needs for report access.
type reportReader interface {
	FindLatestByYear(ctx context.Context, year int) (model.Report, bool, error)
}

// taxonomyReader is the minimal interface web needs for taxonomy access.
type taxonomyReader interface {
	ListSubtypes(ctx context.Context, year int) ([]model.ExpenseSubtype, error)
}

// Deps holds all the dependencies needed to build the web server.
type Deps struct {
	Forecasts *application.ForecastService
	Auth      auth.Authenticator
	Sessions  *auth.SessionStore
	Partners  auth.PartnerLookup
	Reports   reportReader
	HTML      reportadapter.HTMLRenderer
	Taxonomy  taxonomyReader
	Cfg       *config.Config
	Secure    bool
}

// Server is the socis HTTP server.
type Server struct {
	deps    Deps
	handler http.Handler
}

// NewServer constructs the server with a fully built HTTP mux.
func NewServer(deps Deps) *Server {
	s := &Server{deps: deps}
	s.handler = s.buildMux()
	return s
}

// Handler returns the assembled http.Handler for use in tests or external wiring.
func (s *Server) Handler() http.Handler {
	return s.handler
}

// Run starts the HTTP server and shuts it down gracefully when ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	port := s.deps.Cfg.Server.Port
	if port == 0 {
		port = 8080
	}
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: s.handler,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("espigol server listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// buildMux constructs the complete routing tree.
func (s *Server) buildMux() http.Handler {
	mux := http.NewServeMux()

	// Public routes
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})
	mux.HandleFunc("GET /login", s.deps.Auth.Login)
	mux.HandleFunc("GET /oauth2/callback", s.deps.Auth.Complete)

	if s.deps.Auth.IsDev() {
		mux.HandleFunc("POST /dev-login", s.deps.Auth.Complete)
	}

	mux.HandleFunc("POST /logout", s.handleLogout)
	mux.HandleFunc("GET /access-denied", s.handleAccessDenied)

	// Static assets
	mux.Handle("GET /css/", http.StripPrefix("/css/", staticFileServer()))

	// Authenticated routes — wrap with RequireAuth
	authedMux := http.NewServeMux()
	authedMux.HandleFunc("GET /", s.handleDashboard)
	authedMux.HandleFunc("GET /forecasts/new", s.handleForecastNew)
	authedMux.HandleFunc("POST /forecasts", s.handleForecastCreate)
	authedMux.HandleFunc("GET /forecasts/{id}/edit", s.handleForecastEdit)
	authedMux.HandleFunc("POST /forecasts/{id}", s.handleForecastUpdate)
	authedMux.HandleFunc("POST /forecasts/{id}/delete", s.handleForecastDelete)
	authedMux.HandleFunc("GET /reports/{year}", s.handleReport)

	protected := auth.RequireAuth(s.deps.Sessions, s.deps.Partners, authedMux)

	// Mount protected routes on the main mux — use blank pattern to catch all
	// that aren't already matched by public routes above.
	mux.Handle("/", protected)

	return mux
}
