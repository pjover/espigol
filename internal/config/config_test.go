package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveHome_UsesEspigolHomeWhenSet(t *testing.T) {
	t.Setenv("ESPIGOL_HOME", "/custom/espigol")

	got, err := ResolveHome()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/custom/espigol" {
		t.Errorf("got %q, want %q", got, "/custom/espigol")
	}
}

func TestResolveHome_DefaultsToConfigDir(t *testing.T) {
	t.Setenv("ESPIGOL_HOME", "")
	t.Setenv("HOME", "/home/tester")

	got, err := ResolveHome()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join("/home/tester", ".config", "espigol")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEnsureHome_CreatesImportDir(t *testing.T) {
	home := filepath.Join(t.TempDir(), "espigol")
	if err := EnsureHome(home); err != nil {
		t.Fatalf("EnsureHome: %v", err)
	}
	fi, err := os.Stat(filepath.Join(home, "import"))
	if err != nil || !fi.IsDir() {
		t.Fatalf("expected import/ dir, err=%v", err)
	}
}

func TestLoad_SetsImportDir(t *testing.T) {
	home := t.TempDir()
	cfg, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ImportDir != filepath.Join(home, "import") {
		t.Errorf("ImportDir = %q, want %q", cfg.ImportDir, filepath.Join(home, "import"))
	}
}
