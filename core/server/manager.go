// Package server implements the Server Manager (PRD §7.1): CRUD, management
// (favorites, enable/disable, duplicate), import/export, and latency testing for
// server profiles. Persistence is delegated to the store repository.
package server

import (
	"context"
	"errors"
	"time"

	"omniproxy/core/log"
	"omniproxy/core/models"
	"omniproxy/core/store"
)

// ErrLatencyUnavailable is returned when no latency tester is wired up.
var ErrLatencyUnavailable = errors.New("server: latency testing unavailable")

// ErrDisabled is returned when connecting to a disabled profile is attempted.
var ErrDisabled = errors.New("server: server is disabled")

// LatencyTester measures round-trip latency to a server. Implemented by the
// tunnel/engine layer (sing-box outbound Delay); injectable for tests.
type LatencyTester interface {
	TestLatency(ctx context.Context, profile *models.ServerProfile) (int, error)
}

// Manager is the Server Manager facade over the server repository.
type Manager struct {
	repo    store.ServerRepository
	logger  *log.Logger
	latency LatencyTester
}

// New returns a Manager backed by the given repository.
func New(repo store.ServerRepository, logger *log.Logger) *Manager {
	return &Manager{repo: repo, logger: logger}
}

// SetLatencyTester wires the engine-backed latency tester (called at core init).
func (m *Manager) SetLatencyTester(t LatencyTester) { m.latency = t }

// ListServers returns all profiles (clones; secrets restored).
func (m *Manager) ListServers() []*models.ServerProfile {
	out, _ := m.repo.List(store.Query{})
	return out
}

// ListServersQuery returns profiles matching q (secrets restored).
func (m *Manager) ListServersQuery(q store.Query) ([]*models.ServerProfile, error) {
	return m.repo.List(q)
}

// GetServer returns one profile, or store.ErrServerNotFound.
func (m *Manager) GetServer(id string) (*models.ServerProfile, error) {
	return m.repo.Get(id)
}

// AddServer validates and persists a new profile, returning its id.
func (m *Manager) AddServer(p *models.ServerProfile) (string, error) {
	id, err := m.repo.Create(p)
	if err != nil {
		return "", err
	}
	m.logger.Infof("server", "added server %q (%s)", p.Name, id)
	return id, nil
}

// UpdateServer replaces an existing profile (full replace).
func (m *Manager) UpdateServer(p *models.ServerProfile) error {
	if err := m.repo.Update(p); err != nil {
		return err
	}
	m.logger.Infof("server", "updated server %q", p.Name)
	return nil
}

// DeleteServer removes a profile and its stored secrets.
func (m *Manager) DeleteServer(id string) error {
	if err := m.repo.Delete(id); err != nil {
		return err
	}
	m.logger.Infof("server", "deleted server %s", id)
	return nil
}

// SetFavorite toggles the favorite flag on a profile.
func (m *Manager) SetFavorite(id string, fav bool) error {
	p, err := m.repo.Get(id)
	if err != nil {
		return err
	}
	p.Favorite = fav
	return m.repo.Update(p)
}

// SetEnabled toggles the enabled flag on a profile.
func (m *Manager) SetEnabled(id string, enabled bool) error {
	p, err := m.repo.Get(id)
	if err != nil {
		return err
	}
	p.Enabled = enabled
	if err := m.repo.Update(p); err != nil {
		return err
	}
	m.logger.Infof("server", "%s server %q", enabledStr(enabled), p.Name)
	return nil
}

// Duplicate clones an existing profile under a fresh id. The copy is enabled,
// not favorite, and carries no latency data.
func (m *Manager) Duplicate(id string) (string, error) {
	src, err := m.repo.Get(id)
	if err != nil {
		return "", err
	}
	cp := src.Clone()
	cp.ID = ""
	cp.CreatedAt, cp.UpdatedAt = time.Time{}, time.Time{}
	cp.Favorite = false
	cp.LastLatencyMS = 0
	cp.LastTestedAt = nil
	if cp.Name != "" {
		cp.Name = src.Name + " (copy)"
	}
	dupID, err := m.repo.Create(cp)
	if err != nil {
		return "", err
	}
	m.logger.Infof("server", "duplicated server %q (%s) -> %s", src.Name, src.ID, dupID)
	return dupID, nil
}

// TestLatency measures and persists latency for one server. Requires a wired
// LatencyTester (see SetLatencyTester).
func (m *Manager) TestLatency(ctx context.Context, id string) (int, error) {
	p, err := m.repo.Get(id)
	if err != nil {
		return 0, err
	}
	if m.latency == nil {
		return 0, ErrLatencyUnavailable
	}
	ms, err := m.latency.TestLatency(ctx, p)
	if err != nil {
		return 0, err
	}
	p.LastLatencyMS = ms
	now := time.Now().UTC()
	p.LastTestedAt = &now
	if err := m.repo.Update(p); err != nil {
		return 0, err
	}
	m.logger.Infof("server", "latency %s = %dms", p.Name, ms)
	return ms, nil
}

func enabledStr(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}
