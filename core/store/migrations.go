package store

import (
	"database/sql"
	"fmt"
	"time"
)

// migration is one forward-only schema migration. Once a version is recorded
// in schema_migrations it is never re-run or edited; schema changes are always
// new versions.
type migration struct {
	version int
	name    string
	up      func(tx *sql.Tx) error
}

// migrations runs in order from the current schema_migrations high-water mark.
// Append new versions at the end; never edit existing entries.
var migrations = []migration{
	{version: 1, name: "servers", up: migrateV1},
}

// migrate applies all pending migrations in a fresh schema_migrations table.
func (d *DB) migrate() error {
	if _, err := d.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("store: init migrations table: %w", err)
	}
	for _, m := range migrations {
		var applied int
		if err := d.db.QueryRow(
			`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`,
			m.version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("store: check migration %d: %w", m.version, err)
		}
		if applied > 0 {
			continue
		}
		if err := d.apply(m); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) apply(m migration) error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin migration %d: %w", m.version, err)
	}
	defer tx.Rollback()

	if err := m.up(tx); err != nil {
		return fmt.Errorf("store: migration %d (%s): %w", m.version, m.name, err)
	}
	if _, err := tx.Exec(
		`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
		m.version,
		time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("store: record migration %d: %w", m.version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit migration %d: %w", m.version, err)
	}
	return nil
}

// migrateV1 creates the server profile table. Credential columns (password,
// uuid, ssh_private_key) always store an empty value; the actual secrets live
// in the SecretStore under the refs recorded in secret_refs. Non-secret fields
// are stored as columns so querying/searching/sorting happen in SQL. Reality
// columns are reserved: the engine does not support reality yet, so they are
// always disabled until Phase 2 wiring lands.
func migrateV1(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS servers (
    id                               TEXT PRIMARY KEY,
    name                             TEXT NOT NULL,
    protocol                         TEXT NOT NULL,
    address                          TEXT NOT NULL,
    port                             INTEGER NOT NULL,
    username                         TEXT NOT NULL DEFAULT '',
    password                         TEXT NOT NULL DEFAULT '',
    cipher                           TEXT NOT NULL DEFAULT '',
    uuid                             TEXT NOT NULL DEFAULT '',
    flow                             TEXT NOT NULL DEFAULT '',
    security                         TEXT NOT NULL DEFAULT '',
    global_padding                   INTEGER NOT NULL DEFAULT 0,
    authenticated_length             INTEGER NOT NULL DEFAULT 0,
    packet_encoding                  TEXT NOT NULL DEFAULT '',
    tls_enabled                      INTEGER NOT NULL DEFAULT 0,
    tls_allow_insecure               INTEGER NOT NULL DEFAULT 0,
    tls_server_name                  TEXT NOT NULL DEFAULT '',
    tls_alpn                         TEXT NOT NULL DEFAULT '[]',
    tls_fingerprint                  TEXT NOT NULL DEFAULT '',
    transport_type                   TEXT NOT NULL DEFAULT '',
    transport_path                   TEXT NOT NULL DEFAULT '',
    transport_host                   TEXT NOT NULL DEFAULT '',
    transport_max_early_data         INTEGER NOT NULL DEFAULT 0,
    transport_early_data_header_name TEXT NOT NULL DEFAULT '',
    ssh_user                         TEXT NOT NULL DEFAULT '',
    ssh_host_key                     TEXT NOT NULL DEFAULT '',
    ssh_private_key                  TEXT NOT NULL DEFAULT '',
    reality_enabled                  INTEGER NOT NULL DEFAULT 0,
    reality_public_key               TEXT NOT NULL DEFAULT '',
    reality_short_id                 TEXT NOT NULL DEFAULT '',
    reality_spider_x                 TEXT NOT NULL DEFAULT '',
    enabled                          INTEGER NOT NULL DEFAULT 1,
    tags                             TEXT NOT NULL DEFAULT '[]',
    group_name                       TEXT NOT NULL DEFAULT '',
    favorite                         INTEGER NOT NULL DEFAULT 0,
    last_latency_ms                  INTEGER NOT NULL DEFAULT 0,
    last_tested_at                   TEXT NOT NULL DEFAULT '',
    created_at                       TEXT NOT NULL,
    updated_at                       TEXT NOT NULL,
    secret_refs                      TEXT NOT NULL DEFAULT '[]'
);
CREATE INDEX IF NOT EXISTS idx_servers_protocol ON servers(protocol);
CREATE INDEX IF NOT EXISTS idx_servers_group    ON servers(group_name);
CREATE INDEX IF NOT EXISTS idx_servers_favorite ON servers(favorite);
CREATE INDEX IF NOT EXISTS idx_servers_enabled  ON servers(enabled);
`)
	return err
}
