// Package store implements the SQLite persistence layer for server profiles:
// database initialization, versioned schema migrations, and the
// ServerRepository. Credential values are never written to disk: the
// repository externalizes them to an OS-native SecretStore (PRD §9) and
// restores them on read. The repository interface is the seam where at-rest
// encryption of non-secret columns can be added later without changing
// callers.
package store

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB owns the SQLite connection and its schema. All repositories share it.
type DB struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path and applies any
// pending schema migrations. The caller must Close the returned DB.
func Open(path string) (*DB, error) {
	dsn := "file:" + filepath.ToSlash(path) +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: ping %s: %w", path, err)
	}
	d := &DB{db: db}
	if err := d.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return d, nil
}

// SQLDB exposes the underlying *sql.DB for repository construction and tests.
func (d *DB) SQLDB() *sql.DB { return d.db }

// Close releases the database connection.
func (d *DB) Close() error {
	if d == nil || d.db == nil {
		return nil
	}
	return d.db.Close()
}
