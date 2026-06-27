// Package web is the socis-facing HTTP driving adapter. In phase 1 it only
// exposes a health endpoint; routes and auth arrive in a later phase.
package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/pjover/espigol/internal/config"
)

// NewHandler builds the HTTP handler tree.
func NewHandler(cfg *config.Config) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})
	return mux
}

// Run starts the HTTP server and shuts it down gracefully when ctx is cancelled.
func Run(ctx context.Context, cfg *config.Config) error {
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: NewHandler(cfg),
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
