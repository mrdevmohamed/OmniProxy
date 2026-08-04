package tunnel

import (
	"errors"
	"sync"

	"omniproxy/core/log"
	"omniproxy/core/models"
	"omniproxy/engine"
)

// ErrAlreadyRunning is returned when a start is attempted while a tunnel is
// active.
var ErrAlreadyRunning = errors.New("tunnel: already running")

// Manager runs the tunnel for the active server profile. Single-server only
// (chaining is a Phase 2 seam, see implementation-plan §7).
type Manager struct {
	runner Runner
	logger *log.Logger

	mu        sync.Mutex
	profile   *models.ServerProfile
	mode      models.ConnectionMode
	logLevel  engine.Level
	cacheFile string
	ipv6Mode  models.IPv6Mode
}

// NewManager returns a Manager driving the given runner.
func NewManager(runner Runner, logger *log.Logger) *Manager {
	return &Manager{runner: runner, logger: logger, logLevel: engine.LevelInfo, ipv6Mode: models.IPv6ModePreferIPv4}
}

// SetLogLevel sets the engine log level for subsequent runs.
func (m *Manager) SetLogLevel(lv models.LogLevel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logLevel = modelToEngineLevel(lv)
}

// SetCacheFilePath sets the engine cache DB path for subsequent runs.
func (m *Manager) SetCacheFilePath(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cacheFile = path
}

// SetIPv6Mode sets the IPv6 handling mode for subsequent runs.
func (m *Manager) SetIPv6Mode(mode models.IPv6Mode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !mode.Valid() {
		mode = models.IPv6ModePreferIPv4
	}
	m.ipv6Mode = mode
}

// Start begins the tunnel for profile, failing with ErrAlreadyRunning when a
// tunnel is already active. The profile is cloned; the caller's copy is never
// retained or mutated.
func (m *Manager) Start(p *models.ServerProfile, mode models.ConnectionMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.profile != nil {
		return ErrAlreadyRunning
	}
	cp := p.Clone()
	opts := BuildEngineOptions(cp, mode, m.ipv6Mode, m.logLevel, m.cacheFile)
	if err := m.runner.Start(opts); err != nil {
		m.logger.Errorf("tunnel", "start failed: %v", err)
		return err
	}
	m.profile = cp
	m.mode = mode
	m.logger.Infof("tunnel", "started %s tunnel to %s:%d (ipv6 mode %s)", mode, cp.Address, cp.Port, m.ipv6Mode)
	return nil
}

// Stop ends the current run. Idempotent; safe when idle.
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.profile == nil {
		return nil
	}
	err := m.runner.Stop()
	m.profile, m.mode = nil, ""
	m.logger.Infof("tunnel", "stopped")
	return err
}

// Active reports whether a tunnel is running.
func (m *Manager) Active() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.profile != nil
}

// Server returns a clone of the active profile, or nil when idle.
func (m *Manager) Server() *models.ServerProfile {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.profile.Clone()
}

// Mode returns the active connection mode, or "" when idle.
func (m *Manager) Mode() models.ConnectionMode {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mode
}
