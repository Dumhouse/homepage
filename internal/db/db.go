// Package db opens the SQLite database and applies embedded migrations.
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"math"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Open opens (creating if needed) the database at path and migrates it to the latest version.
// Use ":memory:" in tests.
func Open(path string) (*sql.DB, error) {
	d, err := openRaw(path)
	if err != nil {
		return nil, err
	}
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return nil, err
	}
	if err := goose.Up(d, "migrations"); err != nil {
		d.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return d, nil
}

func openRaw(path string) (*sql.DB, error) {
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	if path == ":memory:" {
		dsn = "file::memory:?cache=shared&_pragma=foreign_keys(1)"
	}
	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(1) // SQLite: one writer; keeps things simple and safe
	return d, nil
}

// OpenNoMigrate opens an existing database without applying migrations. Used by
// the backup command so a backup never changes the schema.
func OpenNoMigrate(path string) (*sql.DB, error) { return openRaw(path) }

// Pending reports the database schema version and the latest embedded migration
// without applying anything. Used by `homeservices migrate --check` in update.sh.
func Pending(path string) (current, latest int64, err error) {
	d, err := openRaw(path)
	if err != nil {
		return 0, 0, err
	}
	defer d.Close()
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return 0, 0, err
	}
	current, err = goose.GetDBVersion(d)
	if err != nil {
		return 0, 0, err
	}
	ms, err := goose.CollectMigrations("migrations", 0, math.MaxInt64)
	if err != nil {
		return 0, 0, err
	}
	for _, m := range ms {
		if m.Version > latest {
			latest = m.Version
		}
	}
	return current, latest, nil
}
