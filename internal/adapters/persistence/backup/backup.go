// Package backup creates and stages restores of the espigol SQLite database
// for the admin TUI. Backups use VACUUM INTO — a consistent, compact
// single-file copy that is safe on the live WAL database and needs no external
// tools. Restores are staged as <db dir>/restore-pending.db and applied on the
// next process start by db.ApplyPendingRestore.
package backup

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Clock returns the current time. It mirrors ports.Clock but is redeclared here
// so this adapter does not depend on the domain.
type Clock interface{ Now() time.Time }

// BackupFile describes one backup on disk.
type BackupFile struct {
	Path    string
	Name    string
	ModTime time.Time
	Size    int64
}

// Backuper is the behaviour the TUI Admin panel consumes.
type Backuper interface {
	Backup(ctx context.Context) (string, error)
	ListBackups() ([]BackupFile, error)
	StageRestore(srcPath string) error
}

// Service implements Backuper against a live *sql.DB and the on-disk layout.
type Service struct {
	db        *sql.DB
	dbPath    string
	backupDir string
	clock     Clock
}

// New builds a backup Service. db is the live connection to dbPath; backupDir is
// where snapshots are written.
func New(db *sql.DB, dbPath, backupDir string, clock Clock) *Service {
	return &Service{db: db, dbPath: dbPath, backupDir: backupDir, clock: clock}
}

// Backup writes a consistent snapshot to backups/espigol-YYYYMMDD-HHMMSS.db and
// returns its path. If a file already exists at the computed destination (same-second
// collision), it appends a numeric suffix (e.g., -2.db, -3.db) until a free path is found.
func (s *Service) Backup(ctx context.Context) (string, error) {
	if err := os.MkdirAll(s.backupDir, 0o700); err != nil {
		return "", fmt.Errorf("creating backup dir: %w", err)
	}
	name := fmt.Sprintf("espigol-%s.db", s.clock.Now().Format("20060102-150405"))
	dest := filepath.Join(s.backupDir, name)

	// Disambiguate if a file already exists at dest (same-second collision safety).
	if _, err := os.Stat(dest); err == nil {
		// File exists; find a free path by appending numeric suffix.
		base := strings.TrimSuffix(name, ".db")
		for i := 2; i < 1000; i++ {
			candidateName := fmt.Sprintf("%s-%d.db", base, i)
			candidate := filepath.Join(s.backupDir, candidateName)
			if _, err := os.Stat(candidate); err != nil && os.IsNotExist(err) {
				dest = candidate
				break
			}
		}
	}

	// VACUUM INTO takes a SQL string literal for the destination path.
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO "+sqlQuote(dest)); err != nil {
		return "", fmt.Errorf("vacuum into %s: %w", dest, err)
	}
	return dest, nil
}

// ListBackups returns the espigol-*.db files in backupDir, newest first. Because
// the filenames are timestamped, lexical-descending order is newest-first.
func (s *Service) ListBackups() ([]BackupFile, error) {
	entries, err := os.ReadDir(s.backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading backup dir: %w", err)
	}
	var files []BackupFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "espigol-") || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		files = append(files, BackupFile{
			Path:    filepath.Join(s.backupDir, e.Name()),
			Name:    e.Name(),
			ModTime: info.ModTime(),
			Size:    info.Size(),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name > files[j].Name })
	return files, nil
}

// StageRestore first takes a safety backup of the current database, then copies
// srcPath to <db dir>/restore-pending.db, which db.ApplyPendingRestore swaps in
// on the next start.
func (s *Service) StageRestore(srcPath string) error {
	if _, err := s.Backup(context.Background()); err != nil {
		return fmt.Errorf("safety backup before restore: %w", err)
	}
	pending := filepath.Join(filepath.Dir(s.dbPath), "restore-pending.db")
	if err := copyFile(srcPath, pending); err != nil {
		return fmt.Errorf("staging restore: %w", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		os.Remove(dst)
		return copyErr
	}
	return closeErr
}

// sqlQuote wraps s in single quotes for a SQL string literal, doubling any
// embedded single quotes.
func sqlQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
