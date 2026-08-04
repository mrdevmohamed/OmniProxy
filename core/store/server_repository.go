package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"omniproxy/core/log"
	"omniproxy/core/models"
	"omniproxy/core/secret"
)

// ErrServerNotFound is returned when a server id does not exist.
var ErrServerNotFound = errors.New("store: server not found")

// Secret reference identifiers for credential fields persisted via secret.Store.
// The SQLite rows only ever hold empty strings for these columns; the values
// live in OS-native secure storage (PRD §9).
const (
	RefPassword      = "password"
	RefUUID          = "uuid"
	RefSSHPrivateKey = "ssh.privatekey"
)

// SecretStoreKey returns the secret.Store key for a server credential field.
func SecretStoreKey(serverID, ref string) string {
	return "omniproxy.server." + serverID + "." + ref
}

// ServerRepository is the persistence boundary for server profiles. Credential
// values are externalized through secret.Store; the concrete implementation
// restores them into the profiles it returns.
type ServerRepository interface {
	// Create validates and persists a new profile, assigning an id and
	// timestamps when absent, and returns the profile id.
	Create(p *models.ServerProfile) (string, error)
	// Update fully replaces an existing profile (full replace semantics),
	// preserving its CreatedAt and bumping UpdatedAt.
	Update(p *models.ServerProfile) error
	// Get returns one profile with secrets restored, or ErrServerNotFound.
	Get(id string) (*models.ServerProfile, error)
	// Delete removes a profile and its stored secrets.
	Delete(id string) error
	// List returns profiles matching q with secrets restored.
	List(q Query) ([]*models.ServerProfile, error)
}

// Sort orders supported by Query.Sort.
const (
	SortCreated  = "created"
	SortUpdated  = "updated"
	SortName     = "name"
	SortLatency  = "latency"
	SortProtocol = "protocol"
)

// Query carries list filtering and sorting options. Empty/zero values mean
// "any"/"default".
type Query struct {
	// Search matches name, address, username, group, and protocol (case-
	// insensitive substring).
	Search string
	// Group filters by exact group label.
	Group string
	// Protocol filters by exact protocol.
	Protocol string
	// Enabled filters by the enabled flag; nil means any.
	Enabled *bool
	// Favorite filters by the favorite flag; nil means any.
	Favorite *bool
	// Sort orders by one of the Sort* constants (default SortCreated).
	Sort string
	// Order is "asc" or "desc"; empty picks the default for the sort.
	Order string
}

// SQLiteServerRepository persists server profiles in SQLite with credentials
// externalized to secret.Store. All methods are safe for concurrent use
// (serialized by SQLite's internal locking).
type SQLiteServerRepository struct {
	db      *DB
	secrets secret.Store
	redact  func(secrets ...string)
}

// NewSQLiteServerRepository returns a repository bound to db.
func NewSQLiteServerRepository(db *DB, secrets secret.Store) *SQLiteServerRepository {
	return &SQLiteServerRepository{db: db, secrets: secrets}
}

// SetRedactor registers restored secret values with a logger redactor so they
// never reach a sink (PRD §9). Pass nil to disable.
func (r *SQLiteServerRepository) SetRedactor(red *log.Redactor) {
	if red == nil {
		r.redact = nil
		return
	}
	r.redact = red.Add
}

// Create implements ServerRepository.
func (r *SQLiteServerRepository) Create(p *models.ServerProfile) (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	cp := p.Clone()
	if cp.ID == "" {
		cp.ID = models.NewID()
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now().UTC()
	}
	if cp.UpdatedAt.IsZero() {
		cp.UpdatedAt = time.Now().UTC()
	}
	if err := r.persist(cp); err != nil {
		return "", err
	}
	return cp.ID, nil
}

// Update implements ServerRepository.
func (r *SQLiteServerRepository) Update(p *models.ServerProfile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	row, err := scanServer(r.db.SQLDB().QueryRow("SELECT "+serverColumns+" FROM servers WHERE id = ?", p.ID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrServerNotFound
		}
		return err
	}
	for _, ref := range row.secretRef {
		if !containsRef(SecretRefsFor(p), ref) {
			_ = r.secrets.Delete(SecretStoreKey(p.ID, ref))
		}
	}
	cp := p.Clone()
	cp.CreatedAt = row.profile.CreatedAt
	cp.UpdatedAt = time.Now().UTC()
	return r.persist(cp)
}

// Get implements ServerRepository.
func (r *SQLiteServerRepository) Get(id string) (*models.ServerProfile, error) {
	row, err := scanServer(r.db.SQLDB().QueryRow("SELECT "+serverColumns+" FROM servers WHERE id = ?", id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrServerNotFound
		}
		return nil, err
	}
	if err := r.restoreSecrets(row); err != nil {
		return nil, err
	}
	r.registerRedact(row.profile)
	return row.profile, nil
}

// Delete implements ServerRepository.
func (r *SQLiteServerRepository) Delete(id string) error {
	refs, err := r.querySecretRefs(id)
	if err != nil {
		return err
	}
	res, err := r.db.SQLDB().Exec("DELETE FROM servers WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrServerNotFound
	}
	for _, ref := range refs {
		_ = r.secrets.Delete(SecretStoreKey(id, ref))
	}
	return nil
}

// List implements ServerRepository.
func (r *SQLiteServerRepository) List(q Query) ([]*models.ServerProfile, error) {
	var where []string
	var args []any

	if s := strings.TrimSpace(q.Search); s != "" {
		like := "%" + strings.ToLower(s) + "%"
		where = append(where, `(LOWER(name) LIKE ? OR LOWER(address) LIKE ? OR LOWER(username) LIKE ? OR LOWER(group_name) LIKE ? OR LOWER(protocol) LIKE ?)`)
		args = append(args, like, like, like, like, like)
	}
	if q.Group != "" {
		where = append(where, "group_name = ?")
		args = append(args, q.Group)
	}
	if q.Protocol != "" {
		where = append(where, "protocol = ?")
		args = append(args, q.Protocol)
	}
	if q.Enabled != nil {
		where = append(where, "enabled = ?")
		args = append(args, boolInt(*q.Enabled))
	}
	if q.Favorite != nil {
		where = append(where, "favorite = ?")
		args = append(args, boolInt(*q.Favorite))
	}

	query := "SELECT " + serverColumns + " FROM servers"
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY " + orderBy(q.Sort, q.Order)

	rows, err := r.db.SQLDB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.ServerProfile
	for rows.Next() {
		row, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		if err := r.restoreSecrets(row); err != nil {
			return nil, err
		}
		r.registerRedact(row.profile)
		out = append(out, row.profile)
	}
	return out, rows.Err()
}

// persist sanitizes secrets into secret.Store and upserts the row.
func (r *SQLiteServerRepository) persist(p *models.ServerProfile) error {
	refs := SecretRefsFor(p)
	for _, ref := range refs {
		if err := r.secrets.Set(SecretStoreKey(p.ID, ref), SecretValueFor(p, ref)); err != nil {
			return fmt.Errorf("store: store secret %s: %w", ref, err)
		}
	}
	san := p.Clone()
	san.Password, san.UUID, san.SSH.PrivateKey = "", "", ""
	vals, err := serverValues(san, refs)
	if err != nil {
		return err
	}

	placeholders := strings.Repeat("?, ", len(serverColumnList)-1) + "?"
	query := "INSERT INTO servers (" + serverColumns + ") VALUES (" + placeholders + ")" +
		" ON CONFLICT(id) DO UPDATE SET " + updateClause()
	args := append(vals, vals[1:]...) // update clause rebinds every non-id column
	if _, err := r.db.SQLDB().Exec(query, args...); err != nil {
		return fmt.Errorf("store: upsert server: %w", err)
	}
	r.registerRedact(p)
	return nil
}

// restoreSecrets fills credential values from secret.Store. Missing secrets
// are skipped: the profile keeps empty credential fields and the caller
// decides how to treat them.
func (r *SQLiteServerRepository) restoreSecrets(row *serverRow) error {
	for _, ref := range row.secretRef {
		v, err := r.secrets.Get(SecretStoreKey(row.profile.ID, ref))
		if err != nil {
			if errors.Is(err, secret.ErrNotFound) {
				continue
			}
			return err
		}
		SetSecretValue(row.profile, ref, v)
	}
	return nil
}

func (r *SQLiteServerRepository) registerRedact(p *models.ServerProfile) {
	if r.redact != nil {
		r.redact(p.Password, p.UUID, p.SSH.PrivateKey)
	}
}

// SecretRefsFor returns the credential refs a profile needs stored. A ref is
// only included when its value is present, so empty credentials are not
// round-tripped.
func SecretRefsFor(p *models.ServerProfile) []string {
	var refs []string
	if p.Password != "" {
		refs = append(refs, RefPassword)
	}
	if p.UUID != "" {
		refs = append(refs, RefUUID)
	}
	if p.SSH.PrivateKey != "" {
		refs = append(refs, RefSSHPrivateKey)
	}
	return refs
}

// SecretValueFor returns the secret value for a ref.
func SecretValueFor(p *models.ServerProfile, ref string) string {
	switch ref {
	case RefPassword:
		return p.Password
	case RefUUID:
		return p.UUID
	case RefSSHPrivateKey:
		return p.SSH.PrivateKey
	default:
		return ""
	}
}

// SetSecretValue writes a restored secret value back into a profile.
func SetSecretValue(p *models.ServerProfile, ref, v string) {
	switch ref {
	case RefPassword:
		p.Password = v
	case RefUUID:
		p.UUID = v
	case RefSSHPrivateKey:
		p.SSH.PrivateKey = v
	}
}

func containsRef(refs []string, want string) bool {
	for _, r := range refs {
		if r == want {
			return true
		}
	}
	return false
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func orderBy(sort, order string) string {
	sort, order = strings.ToLower(sort), strings.ToLower(order)
	col := "created_at"
	switch sort {
	case SortUpdated:
		col = "updated_at"
	case SortName:
		col = "name COLLATE NOCASE"
	case SortLatency:
		col = "CASE WHEN last_latency_ms > 0 THEN last_latency_ms ELSE NULL END"
	case SortProtocol:
		col = "protocol"
	}
	if order != "asc" && order != "desc" {
		switch sort {
		case SortUpdated, SortLatency:
			order = "desc"
		default:
			order = "asc"
		}
	}
	return col + " " + strings.ToUpper(order) + ", created_at ASC"
}
