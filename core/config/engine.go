// Package config implements the Configuration Engine (PRD §7.1): validation and
// persistence of application settings. The on-disk document is encrypted at
// rest (AES-256-GCM) under a data key held in OS secure storage (PRD §9).
//
// Server profiles are persisted by the store package (SQLite); the encrypted
// settings blob may still contain a legacy "servers" section from pre-migration
// builds. LoadLegacyServers reads that section once so core can import the
// profiles into the server repository.
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
	"omniproxy/core/store"
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

// Engine is the Configuration Engine. It owns the in-memory settings state and
// synchronizes it with the encrypted store on every mutation.
type Engine struct {
	store   Store
	secrets secret.Store
	key     []byte
	logger  *log.Logger

	mu  sync.RWMutex
	cfg configDoc
}

// configDoc is the on-disk structure (before encryption).
type configDoc struct {
	Version  int                `json:"version"`
	Settings models.AppSettings `json:"settings"`
}

// legacyDoc is the pre-migration on-disk structure (v1) that also embedded
// server profiles. It is read only by LoadLegacyServers during the one-time
// migration to repository-backed server persistence.
type legacyDoc struct {
	Version  int                `json:"version"`
	Settings models.AppSettings `json:"settings"`
	Servers  []*persistedServer `json:"servers,omitempty"`
}

type persistedServer struct {
	*models.ServerProfile
	SecretRefs []string `json:"secretRefs,omitempty"`
}

// LoadLegacyServers reads a pre-migration configuration blob and returns the
// server profiles embedded in it, with credentials restored from SecretStore
// and registered with the logger redactor. It returns nil when no blob (or no
// legacy servers) exist. Callers that find a corrupt blob should log and skip
// the migration; config.New will reset the store to defaults on next load.
func LoadLegacyServers(store Store, secrets secret.Store, logger *log.Logger) ([]*models.ServerProfile, error) {
	if store == nil || secrets == nil || logger == nil {
		return nil, errors.New("config: nil dependency")
	}
	key, err := secret.GetOrCreateDataKey(secrets, "")
	if err != nil {
		return nil, fmt.Errorf("config: data key: %w", err)
	}
	raw, err := store.Load()
	if err != nil {
		if errors.Is(err, ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	plain, err := secret.Open(key, raw)
	if err != nil {
		return nil, fmt.Errorf("config: legacy blob: %w", err)
	}
	var doc legacyDoc
	if err := json.Unmarshal(plain, &doc); err != nil {
		return nil, fmt.Errorf("config: legacy blob: %w", err)
	}
	out := make([]*models.ServerProfile, 0, len(doc.Servers))
	for _, ps := range doc.Servers {
		if err := restoreSecrets(secrets, logger, ps); err != nil {
			return nil, err
		}
		out = append(out, ps.ServerProfile.Clone())
	}
	if len(doc.Servers) > 0 {
		logger.Infof("config", "found %d legacy server profile(s) to migrate", len(doc.Servers))
	}
	return out, nil
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

// load reads, decrypts, and normalizes the settings from the store.
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
	e.cfg = doc
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

// saveLocked persists the in-memory settings. Caller holds e.mu.
func (e *Engine) saveLocked() error {
	doc := configDoc{
		Version:  currentVersion,
		Settings: e.cfg.Settings,
	}
	doc.Settings.Normalize()

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

// restoreSecrets fills a legacy profile's credential values from SecretStore and
// registers them with the logger redactor.
func restoreSecrets(secrets secret.Store, logger *log.Logger, ps *persistedServer) error {
	for _, ref := range ps.SecretRefs {
		v, err := secrets.Get(store.SecretStoreKey(ps.ID, ref))
		if err != nil {
			if errors.Is(err, secret.ErrNotFound) {
				logger.Warnf("config", "missing stored secret %q for server %s; skipping", ref, ps.ID)
				continue
			}
			return err
		}
		setSecretValue(ps.ServerProfile, ref, v)
	}
	logger.Redactor().Add(ps.Password, ps.UUID, ps.SSH.PrivateKey)
	return nil
}

func setSecretValue(p *models.ServerProfile, ref, v string) {
	switch ref {
	case store.RefPassword:
		p.Password = v
	case store.RefUUID:
		p.UUID = v
	case store.RefSSHPrivateKey:
		p.SSH.PrivateKey = v
	}
}
