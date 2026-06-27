package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pjover/espigol/internal/config"
)

func TestHealthEndpoint(t *testing.T) {
	h := NewHandler(&config.Config{})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "OK\n" {
		t.Errorf("body = %q, want %q", got, "OK\n")
	}
}
