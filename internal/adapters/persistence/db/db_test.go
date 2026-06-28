package db

import (
	"path/filepath"
	"testing"
)

func TestOpen_MigratesAndEnablesForeignKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "espigol.db")

	conn, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	// foreign_keys pragma must be ON for every connection.
	var fk int
	if err := conn.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}

	// journal mode must be WAL.
	var mode string
	if err := conn.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}

	// migrations created the core tables.
	var n int
	if err := conn.QueryRow(
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('partner','section','expense_forecast','board_authorization')",
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("expected 4 core tables, found %d", n)
	}
}
