package config

import (
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
