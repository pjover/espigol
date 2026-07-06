package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_AdminEmail_Default(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Admin.Email != "admin@espigol" {
		t.Errorf("Admin.Email default = %q", cfg.Admin.Email)
	}
}

func TestLoad_AdminEmail_EnvOverride(t *testing.T) {
	t.Setenv("ESPIGOL_ADMIN_EMAIL", "boss@coop.cat")
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Admin.Email != "boss@coop.cat" {
		t.Errorf("Admin.Email = %q", cfg.Admin.Email)
	}
}

func TestEnsureHome_CreatesTree(t *testing.T) {
	home := filepath.Join(t.TempDir(), "espigol")

	if err := EnsureHome(home); err != nil {
		t.Fatal(err)
	}

	for _, sub := range []string{".", "reports", "backups"} {
		p := filepath.Join(home, sub)
		fi, err := os.Stat(p)
		if err != nil {
			t.Errorf("missing %s: %v", p, err)
			continue
		}
		if !fi.IsDir() {
			t.Errorf("%s is not a directory", p)
		}
	}

	if _, err := os.Stat(filepath.Join(home, "config.yaml")); err != nil {
		t.Errorf("missing config.yaml: %v", err)
	}
}

func TestEnsureHome_Idempotent(t *testing.T) {
	home := t.TempDir()

	if err := EnsureHome(home); err != nil {
		t.Fatal("first call:", err)
	}
	if err := EnsureHome(home); err != nil {
		t.Fatal("second call:", err)
	}
}

func TestEnsureHome_PreservesExistingConfig(t *testing.T) {
	home := t.TempDir()
	cfgPath := filepath.Join(home, "config.yaml")
	customContent := []byte("business:\n  name: Custom\n")
	if err := os.WriteFile(cfgPath, customContent, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := EnsureHome(home); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(customContent) {
		t.Errorf("config.yaml was overwritten: got %q", got)
	}
}
