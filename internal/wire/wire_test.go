package wire_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/pjover/espigol/internal/config"
	"github.com/pjover/espigol/internal/wire"
)

func TestServer_AssemblesAndServesHealth(t *testing.T) {
	cfg := &config.Config{DBPath: filepath.Join(t.TempDir(), "wire.db")}
	cfg.Server.Port = 0
	srv, err := wire.Server(cfg)
	if err != nil {
		t.Fatalf("wire.Server: %v", err)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("health = %d, want 200", rec.Code)
	}
}
