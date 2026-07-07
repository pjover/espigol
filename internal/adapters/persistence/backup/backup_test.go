package backup_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence/backup"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
)

type fakeClock struct{ t time.Time }

func (c fakeClock) Now() time.Time { return c.t }

func newSvc(t *testing.T) (*backup.Service, string, string) {
	t.Helper()
	home := t.TempDir()
	dbPath := filepath.Join(home, "espigol.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	backupDir := filepath.Join(home, "backups")
	svc := backup.New(conn, dbPath, backupDir, fakeClock{t: time.Date(2025, 6, 1, 10, 20, 30, 0, time.UTC)})
	return svc, dbPath, backupDir
}

func TestBackup_ProducesOpenableCopy(t *testing.T) {
	svc, _, backupDir := newSvc(t)
	path, err := svc.Backup(context.Background())
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if filepath.Dir(path) != backupDir {
		t.Errorf("backup path %q not in backupDir %q", path, backupDir)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	// The copy is a valid, migrated database.
	conn, err := db.Open(path)
	if err != nil {
		t.Fatalf("opening backup copy: %v", err)
	}
	conn.Close()
}

func TestListBackups_NewestFirst(t *testing.T) {
	svc, _, backupDir := newSvc(t)
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"espigol-20250601-090000.db", "espigol-20250602-090000.db", "notabackup.txt"} {
		if err := os.WriteFile(filepath.Join(backupDir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("len = %d, want 2 (txt ignored)", len(files))
	}
	if files[0].Name != "espigol-20250602-090000.db" {
		t.Errorf("newest first failed: %q", files[0].Name)
	}
}

func TestStageRestore_WritesPendingAndSafetyBackup(t *testing.T) {
	svc, dbPath, backupDir := newSvc(t)
	src := filepath.Join(t.TempDir(), "chosen.db")
	if err := os.WriteFile(src, []byte("RESTORE-ME"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := svc.StageRestore(src); err != nil {
		t.Fatalf("StageRestore: %v", err)
	}
	pending := filepath.Join(filepath.Dir(dbPath), "restore-pending.db")
	got, err := os.ReadFile(pending)
	if err != nil || string(got) != "RESTORE-ME" {
		t.Fatalf("pending content = %q err=%v", got, err)
	}
	entries, _ := os.ReadDir(backupDir)
	if len(entries) == 0 {
		t.Error("expected a safety backup to be created before staging")
	}
}

func TestBackup_SameSecondCollisionSafety(t *testing.T) {
	svc, _, _ := newSvc(t)

	// Call Backup twice with the same fixed clock time (same-second collision scenario).
	path1, err := svc.Backup(context.Background())
	if err != nil {
		t.Fatalf("first Backup: %v", err)
	}

	path2, err := svc.Backup(context.Background())
	if err != nil {
		t.Fatalf("second Backup: %v", err)
	}

	// Both calls should have succeeded and returned different paths.
	if path1 == path2 {
		t.Errorf("backup paths are identical: %q", path1)
	}

	// Both files should exist on disk.
	if _, err := os.Stat(path1); err != nil {
		t.Errorf("first backup file missing: %v", err)
	}
	if _, err := os.Stat(path2); err != nil {
		t.Errorf("second backup file missing: %v", err)
	}
}
