package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_DefaultsWhenNoFile(t *testing.T) {
	home := t.TempDir()

	cfg, err := Load(home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Home != home {
		t.Errorf("Home = %q, want %q", cfg.Home, home)
	}
	if cfg.DBPath != filepath.Join(home, "espigol.db") {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, filepath.Join(home, "espigol.db"))
	}
	if cfg.BusinessName != "Cooperativa d'Estellencs" {
		t.Errorf("BusinessName = %q, want %q", cfg.BusinessName, "Cooperativa d'Estellencs")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.OutputDir != filepath.Join(home, "reports") {
		t.Errorf("OutputDir = %q, want %q", cfg.OutputDir, filepath.Join(home, "reports"))
	}
	if cfg.BackupDir != filepath.Join(home, "backups") {
		t.Errorf("BackupDir = %q, want %q", cfg.BackupDir, filepath.Join(home, "backups"))
	}
	if cfg.LogoPath != filepath.Join(home, "logo.png") {
		t.Errorf("LogoPath = %q, want %q", cfg.LogoPath, filepath.Join(home, "logo.png"))
	}
}

func TestLoad_ReadsYamlFile(t *testing.T) {
	home := t.TempDir()
	yaml := "" +
		"business:\n" +
		"  name: Test Coop\n" +
		"server:\n" +
		"  port: 9090\n"
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BusinessName != "Test Coop" {
		t.Errorf("BusinessName = %q, want %q", cfg.BusinessName, "Test Coop")
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
	}
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ESPIGOL_SERVER_PORT", "7000")

	cfg, err := Load(home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 7000 {
		t.Errorf("Server.Port = %d, want 7000", cfg.Server.Port)
	}
}
