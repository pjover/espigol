package db

import (
	"os"
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

func TestApplyPendingRestore_SwapsFileAndClearsSidecars(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "espigol.db")
	if err := os.WriteFile(dbPath, []byte("OLD"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath+"-wal", []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath+"-shm", []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	pending := filepath.Join(dir, "restore-pending.db")
	if err := os.WriteFile(pending, []byte("NEW"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ApplyPendingRestore(dbPath); err != nil {
		t.Fatalf("ApplyPendingRestore: %v", err)
	}

	got, _ := os.ReadFile(dbPath)
	if string(got) != "NEW" {
		t.Errorf("db content = %q, want NEW", got)
	}
	if _, err := os.Stat(pending); !os.IsNotExist(err) {
		t.Errorf("pending marker should be gone, err=%v", err)
	}
	if _, err := os.Stat(dbPath + "-wal"); !os.IsNotExist(err) {
		t.Errorf("-wal sidecar should be removed")
	}
	if _, err := os.Stat(dbPath + "-shm"); !os.IsNotExist(err) {
		t.Errorf("-shm sidecar should be removed")
	}
}

func TestApplyPendingRestore_NoPendingIsNoOp(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "espigol.db")
	if err := os.WriteFile(dbPath, []byte("KEEP"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPendingRestore(dbPath); err != nil {
		t.Fatalf("ApplyPendingRestore: %v", err)
	}
	got, _ := os.ReadFile(dbPath)
	if string(got) != "KEEP" {
		t.Errorf("db content = %q, want KEEP", got)
	}
}
