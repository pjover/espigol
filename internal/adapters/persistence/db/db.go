// Package db opens the espigol SQLite database (pure-Go modernc driver),
// configures per-connection pragmas, and runs goose migrations to latest.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	migrations "github.com/pjover/espigol/db/migrations"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

// Open opens the database at path with WAL, busy_timeout, and foreign_keys
// pragmas applied to every pooled connection, then runs migrations to latest.
func Open(path string) (*sql.DB, error) {
	if err := ApplyPendingRestore(path); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)",
		path,
	)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("pinging sqlite: %w", err)
	}
	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func migrate(conn *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.Up(conn, "."); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	return nil
}

// ApplyPendingRestore swaps a staged restore into place before the database is
// opened. If <dir>/restore-pending.db exists it replaces the database file,
// removes the stale -wal/-shm sidecars (which belong to the old database), and
// deletes the marker. It is a no-op when no restore is pending. Running here,
// the single open choke point, means both the TUI and --server apply a
// pending restore on their next start.
func ApplyPendingRestore(dbPath string) error {
	pending := filepath.Join(filepath.Dir(dbPath), "restore-pending.db")
	if _, err := os.Stat(pending); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("checking pending restore: %w", err)
	}
	if err := os.Rename(pending, dbPath); err != nil {
		return fmt.Errorf("applying pending restore: %w", err)
	}
	for _, sidecar := range []string{dbPath + "-wal", dbPath + "-shm"} {
		if err := os.Remove(sidecar); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing %s: %w", sidecar, err)
		}
	}
	return nil
}
