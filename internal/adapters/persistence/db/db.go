// Package db opens the espigol SQLite database (pure-Go modernc driver),
// configures per-connection pragmas, and runs goose migrations to latest.
package db

import (
	"database/sql"
	"fmt"

	migrations "github.com/pjover/espigol/db/migrations"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

// Open opens the database at path with WAL, busy_timeout, and foreign_keys
// pragmas applied to every pooled connection, then runs migrations to latest.
func Open(path string) (*sql.DB, error) {
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
