package wire_test

import (
	"path/filepath"
	"testing"

	"github.com/pjover/espigol/internal/config"
	"github.com/pjover/espigol/internal/wire"
)

func TestTUI_Assembles(t *testing.T) {
	cfg := &config.Config{
		DBPath:       filepath.Join(t.TempDir(), "tui.db"),
		BusinessName: "Cooperativa Test",
		OutputDir:    t.TempDir(),
	}
	cfg.Admin.Email = "admin@example.com"

	app, err := wire.TUI(cfg)
	if err != nil {
		t.Fatalf("wire.TUI: %v", err)
	}
	if app == nil {
		t.Fatal("wire.TUI returned nil app")
	}
}
