// Package server implements the Server Manager (PRD §7.1): CRUD, favorites,
// import/export, and latency testing for server profiles.
package server

import (
	"context"
	"errors"
	"time"

	"omniproxy/core/config"
	"omniproxy/core/log"
	"omniproxy/core/models"
)

// ErrLatencyUnavailable is returned when no latency tester is wired up.
var ErrLatencyUnavailable = errors.New("server: latency testing unavailable")

// LatencyTester measures round-trip latency to a server. Implemented by the
// tunnel/engine layer (sing-box outbound Delay); injectable for tests.
type LatencyTester interface {
	TestLatency(ctx context.Context, profile *models.ServerProfile) (int, error)
}

// Manager is the Server Manager facade over the Config Engine.
type Manager struct {
	engine  *config.Engine
	logger  *log.Logger
	latency LatencyTester
}

// New returns a Manager backed by the given config engine.
func New(engine *config.Engine, logger *log.Logger) *Manager {
	return &Manager{engine: engine, logger: logger}
}

// SetLatencyTester wires the engine-backed latency tester (called at core init).
func (m *Manager) SetLatencyTester(t LatencyTester) { m.latency = t }

// ListServers returns all profiles (clones; secrets restored).
func (m *Manager) ListServers() []*models.ServerProfile {
	return m.engine.ListServers()
}

// GetServer returns one profile, or config.ErrServerNotFound.
func (m *Manager) GetServer(id string) (*models.ServerProfile, error) {
	return m.engine.GetServer(id)
}

// AddServer validates and persists a new profile, returning its id.
func (m *Manager) AddServer(p *models.ServerProfile) (string, error) {
	return m.engine.AddServer(p)
}

// UpdateServer replaces an existing profile (full replace).
func (m *Manager) UpdateServer(p *models.ServerProfile) error {
	return m.engine.UpdateServer(p)
}

// DeleteServer removes a profile and its stored secrets.
func (m *Manager) DeleteServer(id string) error {
	return m.engine.DeleteServer(id)
}

// SetFavorite toggles the favorite flag on a profile.
func (m *Manager) SetFavorite(id string, fav bool) error {
	p, err := m.engine.GetServer(id)
	if err != nil {
		return err
	}
	p.Favorite = fav
	return m.engine.UpdateServer(p)
}

// TestLatency measures and persists latency for one server. Requires a wired
// LatencyTester (see SetLatencyTester).
func (m *Manager) TestLatency(ctx context.Context, id string) (int, error) {
	p, err := m.engine.GetServer(id)
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
	if err := m.engine.UpdateServer(p); err != nil {
		return 0, err
	}
	m.logger.Infof("server", "latency %s = %dms", p.Name, ms)
	return ms, nil
}
