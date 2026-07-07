package backup_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/backup"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
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

// seedSection inserts a single section row into the database at dbPath using
// a fresh connection, so callers don't need to plumb a live *sql.DB through
// the backup.Service (which keeps its connection private).
func seedSection(t *testing.T, dbPath, code, label string) {
	t.Helper()
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("opening %s to seed: %v", dbPath, err)
	}
	defer conn.Close()
	sec, err := model.NewSection(code, label, true, 1)
	if err != nil {
		t.Fatalf("NewSection: %v", err)
	}
	if err := persistence.NewSectionRepository(sqlc.New(conn)).Save(context.Background(), sec); err != nil {
		t.Fatalf("saving section: %v", err)
	}
}

// readSectionLabels opens dbPath and returns the label of every section
// present, keyed by code.
func readSectionLabels(t *testing.T, dbPath string) map[string]string {
	t.Helper()
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("opening %s to read: %v", dbPath, err)
	}
	defer conn.Close()
	secs, err := persistence.NewSectionRepository(sqlc.New(conn)).List(context.Background())
	if err != nil {
		t.Fatalf("listing sections in %s: %v", dbPath, err)
	}
	out := make(map[string]string, len(secs))
	for _, s := range secs {
		out[s.Code()] = s.Label()
	}
	return out
}

func TestBackup_ProducesOpenableCopy(t *testing.T) {
	svc, dbPath, backupDir := newSvc(t)
	seedSection(t, dbPath, "oliva", "Secció d'oliva")

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

	// The copy must contain the seeded row's content, not just be openable.
	labels := readSectionLabels(t, path)
	if got, want := labels["oliva"], "Secció d'oliva"; got != want {
		t.Errorf("backup copy section label = %q, want %q", got, want)
	}
}

// TestRestoreRoundTrip_DataSurvives proves that the full backup -> restore
// cycle preserves DB content: a row present when the backup was taken (state
// X) survives being restored over a database that was later mutated away
// from X.
func TestRestoreRoundTrip_DataSurvives(t *testing.T) {
	svc, dbPath, _ := newSvc(t)

	// State X: the row that must survive the round trip.
	seedSection(t, dbPath, "oliva", "Secció d'oliva (state X)")

	path, err := svc.Backup(context.Background())
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	// The backup copy must actually contain state X's content.
	backedUp := readSectionLabels(t, path)
	if got, want := backedUp["oliva"], "Secció d'oliva (state X)"; got != want {
		t.Fatalf("backup copy section label = %q, want %q", got, want)
	}

	// Mutate the live DB away from state X.
	seedSection(t, dbPath, "ramaderia", "Secció de ramaderia (post-backup mutation)")

	if err := svc.StageRestore(path); err != nil {
		t.Fatalf("StageRestore: %v", err)
	}
	if err := db.ApplyPendingRestore(dbPath); err != nil {
		t.Fatalf("ApplyPendingRestore: %v", err)
	}

	restored := readSectionLabels(t, dbPath)
	if got, want := restored["oliva"], "Secció d'oliva (state X)"; got != want {
		t.Errorf("restored section label = %q, want %q (state X must survive)", got, want)
	}
	if _, present := restored["ramaderia"]; present {
		t.Error("restored DB still has the post-backup mutation; restore should have reverted it")
	}
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
