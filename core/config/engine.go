// Package config implements the Configuration Engine (PRD §7.1): validation and
// persistence of server profiles and application settings. The on-disk
// document is encrypted at rest (AES-256-GCM) under a data key held in OS
// secure storage, and credential values live only in that secure storage —
// never in the plaintext configuration (PRD §9).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"omniproxy/core/log"
	"omniproxy/core/models"
	"omniproxy/core/secret"
)

const currentVersion = 1

// Store is the raw persistence boundary for the encrypted configuration blob.
type Store interface {
	// Load returns the stored bytes, or ErrNotExist.
	Load() ([]byte, error)
	// Save atomically persists bytes.
	Save(data []byte) error
}

// Errors surfaced by the engine.
var (
	// ErrNotExist indicates no configuration has been stored yet.
	ErrNotExist = errors.New("config: not found")
	// ErrServerNotFound indicates a server id does not exist.
	ErrServerNotFound = errors.New("config: server not found")
)

// FileStore persists the configuration blob to a file (atomic write, 0600).
type FileStore struct {
	path string
}

// NewFileStore returns a store rooted at path.
func NewFileStore(path string) *FileStore { return &FileStore{path: path} }

// Load implements Store.
func (fs *FileStore) Load() ([]byte, error) {
	b, err := os.ReadFile(fs.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotExist
		}
		return nil, err
	}
	return b, nil
}

// Save implements Store.
func (fs *FileStore) Save(data []byte) error {
	dir := filepath.Dir(fs.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".omniproxy-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), fs.path)
}

// Secret reference identifiers for ServerProfile fields stored in SecretStore.
const (
	RefPassword      = "password"
	RefUUID          = "uuid"
	RefSSHPrivateKey = "ssh.privatekey"
)

// Engine is the Configuration Engine. It owns the in-memory config state and
// synchronizes it with the encrypted store on every mutation.
type Engine struct {
	store   Store
	secrets secret.Store
	key     []byte
	logger  *log.Logger

	mu  sync.RWMutex
	cfg configDoc
}

// configDoc is the on-disk structure (before encryption). Servers are stored
// sanitized: credential values live in SecretStore, referenced by SecretRefs.
type configDoc struct {
	Version  int                `json:"version"`
	Settings models.AppSettings `json:"settings"`
	Servers  []*persistedServer `json:"servers,omitempty"`
}

type persistedServer struct {
	*models.ServerProfile
	SecretRefs []string `json:"secretRefs,omitempty"`
}

// New opens (or initializes) the configuration engine. A corrupt or tampered
// store is backed up and reset to defaults with a warning rather than failing
// startup (PRD §9: tamper protection; resilient recovery).
func New(store Store, secrets secret.Store, logger *log.Logger) (*Engine, error) {
	if store == nil || secrets == nil || logger == nil {
		return nil, errors.New("config: nil dependency")
	}
	key, err := secret.GetOrCreateDataKey(secrets, "")
	if err != nil {
		return nil, fmt.Errorf("config: data key: %w", err)
	}
	e := &Engine{store: store, secrets: secrets, key: key, logger: logger}
	if err := e.load(); err != nil {
		return nil, err
	}
	return e, nil
}

// Settings returns the current application settings.
func (e *Engine) Settings() models.AppSettings {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cfg.Settings
}

// UpdateSettings validates/normalizes and persists settings.
func (e *Engine) UpdateSettings(s models.AppSettings) (models.AppSettings, error) {
	s.Normalize()
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cfg.Settings = s
	if err := e.saveLocked(); err != nil {
		return models.AppSettings{}, err
	}
	e.logger.Infof("config", "settings updated")
	return s, nil
}

// ListServers returns clones of all server profiles with secrets restored.
func (e *Engine) ListServers() []*models.ServerProfile {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]*models.ServerProfile, 0, len(e.cfg.Servers))
	for _, ps := range e.cfg.Servers {
		out = append(out, ps.ServerProfile.Clone())
	}
	return out
}

// GetServer returns a clone of one profile, or ErrServerNotFound.
func (e *Engine) GetServer(id string) (*models.ServerProfile, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, ps := range e.cfg.Servers {
		if ps.ID == id {
			return ps.ServerProfile.Clone(), nil
		}
	}
	return nil, ErrServerNotFound
}

// AddServer validates and persists a new profile, returning its assigned id.
func (e *Engine) AddServer(p *models.ServerProfile) (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	cp := p.Clone()
	now := time.Now().UTC()
	if cp.ID == "" {
		cp.ID = models.NewID()
	}
	cp.CreatedAt, cp.UpdatedAt = now, now

	e.cfg.Servers = append(e.cfg.Servers, &persistedServer{ServerProfile: cp})
	if err := e.saveLocked(); err != nil {
		return "", err
	}
	e.logger.Redactor().Add(cp.Password, cp.UUID, cp.SSH.PrivateKey)
	e.logger.Infof("config", "added server %q (%s)", cp.Name, cp.ID)
	return cp.ID, nil
}

// UpdateServer fully replaces an existing profile (full replace semantics).
func (e *Engine) UpdateServer(p *models.ServerProfile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	var found *persistedServer
	for _, ps := range e.cfg.Servers {
		if ps.ID == p.ID {
			found = ps
			break
		}
	}
	if found == nil {
		return ErrServerNotFound
	}

	// Drop superseded secrets before re-storing the new values on save.
	for _, ref := range secretRefsFor(found.ServerProfile) {
		_ = e.secrets.Delete(secretStoreKey(p.ID, ref))
	}

	cp := p.Clone()
	cp.CreatedAt = found.ServerProfile.CreatedAt
	cp.UpdatedAt = time.Now().UTC()
	found.ServerProfile = cp

	if err := e.saveLocked(); err != nil {
		return err
	}
	e.logger.Redactor().Add(cp.Password, cp.UUID, cp.SSH.PrivateKey)
	e.logger.Infof("config", "updated server %q", cp.Name)
	return nil
}

// DeleteServer removes a profile and its stored secrets.
func (e *Engine) DeleteServer(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	idx := -1
	for i, ps := range e.cfg.Servers {
		if ps.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrServerNotFound
	}

	for _, ref := range secretRefsFor(e.cfg.Servers[idx].ServerProfile) {
		_ = e.secrets.Delete(secretStoreKey(id, ref))
	}
	e.cfg.Servers = append(e.cfg.Servers[:idx], e.cfg.Servers[idx+1:]...)
	if err := e.saveLocked(); err != nil {
		return err
	}
	e.logger.Infof("config", "deleted server %s", id)
	return nil
}

// load reads, decrypts, and restores the configuration from the store.
func (e *Engine) load() error {
	raw, err := e.store.Load()
	if err != nil {
		if errors.Is(err, ErrNotExist) {
			e.cfg = configDoc{Version: currentVersion, Settings: models.DefaultAppSettings()}
			return e.saveLocked()
		}
		return err
	}

	plain, err := secret.Open(e.key, raw)
	if err != nil {
		e.recoverCorrupt(err)
		return nil
	}

	var doc configDoc
	if err := json.Unmarshal(plain, &doc); err != nil {
		e.recoverCorrupt(err)
		return nil
	}

	doc.Settings.Normalize()
	for _, ps := range doc.Servers {
		if err := e.restoreSecrets(ps); err != nil {
			return fmt.Errorf("config: restore server %s: %w", ps.ID, err)
		}
	}
	e.cfg = doc
	e.logger.Infof("config", "loaded %d server profile(s)", len(doc.Servers))
	return nil
}

// recoverCorrupt backs up the tampered/corrupt store and resets to defaults.
func (e *Engine) recoverCorrupt(cause error) {
	if fs, ok := e.store.(*FileStore); ok {
		bak := fmt.Sprintf("%s.corrupt.%d", fs.path, time.Now().Unix())
		if err := os.Rename(fs.path, bak); err != nil {
			e.logger.Warnf("config", "could not back up corrupt store: %v", err)
		} else {
			e.logger.Warnf("config", "backed up corrupt store to %s", bak)
		}
	}
	e.logger.Warnf("config", "configuration corrupt or tampered (%v); resetting to defaults", cause)
	e.cfg = configDoc{Version: currentVersion, Settings: models.DefaultAppSettings()}
	if err := e.saveLocked(); err != nil {
		e.logger.Errorf("config", "could not write default configuration: %v", err)
	}
}

// saveLocked persists the in-memory config, externalizing secrets. Caller holds e.mu.
func (e *Engine) saveLocked() error {
	doc := configDoc{
		Version:  currentVersion,
		Settings: e.cfg.Settings,
	}
	doc.Settings.Normalize()

	doc.Servers = make([]*persistedServer, 0, len(e.cfg.Servers))
	for _, ps := range e.cfg.Servers {
		san, err := e.sanitize(ps)
		if err != nil {
			return err
		}
		doc.Servers = append(doc.Servers, san)
	}

	plain, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	sealed, err := secret.Seal(e.key, plain)
	if err != nil {
		return err
	}
	if err := e.store.Save(sealed); err != nil {
		return fmt.Errorf("config: persist: %w", err)
	}
	return nil
}

// sanitize produces the persisted representation, moving credential values into
// SecretStore. Caller holds e.mu.
func (e *Engine) sanitize(ps *persistedServer) (*persistedServer, error) {
	cp := ps.ServerProfile.Clone()
	refs := secretRefsFor(ps.ServerProfile)
	for _, ref := range refs {
		key := secretStoreKey(cp.ID, ref)
		if err := e.secrets.Set(key, secretValueFor(ps.ServerProfile, ref)); err != nil {
			return nil, fmt.Errorf("config: store secret %s: %w", ref, err)
		}
	}
	cp.Password, cp.UUID, cp.SSH.PrivateKey = "", "", ""
	return &persistedServer{ServerProfile: cp, SecretRefs: refs}, nil
}

// restoreSecrets fills credential values from SecretStore and registers them
// with the logger redactor. Caller holds e.mu.
func (e *Engine) restoreSecrets(ps *persistedServer) error {
	for _, ref := range ps.SecretRefs {
		v, err := e.secrets.Get(secretStoreKey(ps.ID, ref))
		if err != nil {
			if errors.Is(err, secret.ErrNotFound) {
				e.logger.Warnf("config", "missing stored secret %q for server %s; skipping", ref, ps.ID)
				continue
			}
			return err
		}
		setSecretValue(ps.ServerProfile, ref, v)
	}
	e.logger.Redactor().Add(ps.Password, ps.UUID, ps.SSH.PrivateKey)
	return nil
}

func secretRefsFor(p *models.ServerProfile) []string {
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

func secretStoreKey(serverID, ref string) string {
	return "omniproxy.server." + serverID + "." + ref
}

func secretValueFor(p *models.ServerProfile, ref string) string {
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

func setSecretValue(p *models.ServerProfile, ref, v string) {
	switch ref {
	case RefPassword:
		p.Password = v
	case RefUUID:
		p.UUID = v
	case RefSSHPrivateKey:
		p.SSH.PrivateKey = v
	}
}
