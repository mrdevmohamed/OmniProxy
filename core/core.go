// Package core is the OmniProxy Go core. It wires the Config Engine, Server
// Manager, Tunnel Manager, and VPN Service behind the shared bridge API
// (docs/api-contract.md). Transports talk to the facade only; no platform
// business logic lives here.
package core

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"omniproxy/core/api"
	"omniproxy/core/config"
	"omniproxy/core/log"
	"omniproxy/core/models"
	"omniproxy/core/secret"
	"omniproxy/core/server"
	"omniproxy/core/tunnel"
	"omniproxy/core/vpn"
)

const (
	// Version is the app version reported by getVersion.
	Version = "1.0.0"
	// EngineVersion is the pinned sing-box version reported by getVersion.
	EngineVersion = "v1.13.15"
)

const configFileName = "omniproxy.conf"

// errConnected is returned when an operation requires a disconnected state.
var errConnected = errors.New("core: server is connected; disconnect first")

// Config configures the facade. DataDir is the platform config directory.
// SecretStore/Store/Logger/Runner are optional seams for tests.
type Config struct {
	Platform    string
	DataDir     string
	LogLevel    models.LogLevel
	SecretStore secret.Store
	Store       config.Store
	Logger      *log.Logger
	Runner      tunnel.Runner
}

// Facade implements api.Handler and routes bridge requests (Dispatch).
type Facade struct {
	cfg     Config
	logger  *log.Logger
	config  *config.Engine
	servers *server.Manager
	tunnel  *tunnel.Manager
	vpn     *vpn.Service
	bus     *api.EventBus
	api.Dispatcher

	subMu      sync.Mutex
	subscribed bool
	eventSet   map[string]bool
	sink       api.EventSink
}

// New builds the facade: logger, config engine (encrypted at rest), server
// manager with TCP-dial latency tester, in-process tunnel manager, and the
// VPN service. Log entries are published to the event bus for logAppended.
func New(cfg Config) (*Facade, error) {
	if cfg.DataDir == "" {
		return nil, errors.New("core: DataDir is required")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = log.NewLogger(cfg.LogLevel)
	}

	secrets := cfg.SecretStore
	if secrets == nil {
		secrets = secret.NewKeyring("omniproxy")
	}
	store := cfg.Store
	if store == nil {
		store = config.NewFileStore(filepath.Join(cfg.DataDir, configFileName))
	}

	cfgEngine, err := config.New(store, secrets, logger)
	if err != nil {
		return nil, err
	}

	bus := api.NewEventBus()
	f := &Facade{
		cfg:     cfg,
		logger:  logger,
		config:  cfgEngine,
		servers: server.New(cfgEngine, logger),
		bus:     bus,
	}

	runner := cfg.Runner
	if runner == nil {
		runner = tunnel.NewInProcessRunner(logger)
	}
	f.tunnel = tunnel.NewManager(runner, logger)
	f.tunnel.SetCacheFilePath(filepath.Join(cfg.DataDir, "cache.db"))
	f.tunnel.SetLogLevel(cfg.LogLevel)

	f.servers.SetLatencyTester(tunnel.NewLatencyTester(0, logger))

	f.vpn = vpn.NewService(f, f.tunnel, logger, bus)

	logger.AddSink(logSink{f})
	bus.Subscribe(facadeSink{f})

	f.Dispatcher = api.Dispatcher{Handler: f, MapError: f.MapError}
	return f, nil
}

// Dispatch routes a bridge request by method name; it is the embedded
// api.Dispatcher promoted onto the facade.

// SetEventSink registers the bridge event callback. Called by transports at
// init; may be re-registered at any time.
func (f *Facade) SetEventSink(sink api.EventSink) {
	f.subMu.Lock()
	defer f.subMu.Unlock()
	f.sink = sink
}

// MapError translates domain errors onto the contract's error codes.
func (f *Facade) MapError(err error) *api.Error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, config.ErrServerNotFound):
		return &api.Error{Code: api.ErrCodeNotFound, Message: err.Error()}
	case errors.Is(err, models.ErrValidation):
		return &api.Error{Code: api.ErrCodeValidation, Message: err.Error()}
	case errors.Is(err, vpn.ErrBusy), errors.Is(err, tunnel.ErrAlreadyRunning):
		return &api.Error{Code: api.ErrCodeBusy, Message: err.Error()}
	case errors.Is(err, errConnected):
		return &api.Error{Code: api.ErrCodeConnected, Message: err.Error()}
	case errors.Is(err, server.ErrLatencyUnavailable):
		return &api.Error{Code: api.ErrCodeEngine, Message: err.Error()}
	default:
		return &api.Error{Code: api.ErrCodeInternal, Message: err.Error()}
	}
}

// Resolve implements vpn.ProfileResolver.
func (f *Facade) Resolve(serverID string) (*models.ServerProfile, error) {
	return f.servers.GetServer(serverID)
}

// --- api.Handler ---

// GetVersion implements api.Handler.
func (f *Facade) GetVersion() api.GetVersionResponse {
	return api.GetVersionResponse{Version: Version, EngineVersion: EngineVersion, Platform: f.cfg.Platform}
}

// ListServers implements api.Handler.
func (f *Facade) ListServers() api.ServerListResponse {
	return api.ServerListResponse{Servers: f.servers.ListServers()}
}

// GetServer implements api.Handler.
func (f *Facade) GetServer(id string) (*models.ServerProfile, error) {
	return f.servers.GetServer(id)
}

// AddServer implements api.Handler.
func (f *Facade) AddServer(s *models.ServerProfile) (api.IDResponse, error) {
	id, err := f.servers.AddServer(s)
	return api.IDResponse{ID: id}, err
}

// UpdateServer implements api.Handler (rejected while connected to it).
func (f *Facade) UpdateServer(s *models.ServerProfile) (*models.ServerProfile, error) {
	if f.vpn.RunningTo(s.ID) {
		return nil, errConnected
	}
	if err := f.servers.UpdateServer(s); err != nil {
		return nil, err
	}
	return f.servers.GetServer(s.ID)
}

// DeleteServer implements api.Handler (rejected while connected to it).
func (f *Facade) DeleteServer(id string) error {
	if f.vpn.RunningTo(id) {
		return errConnected
	}
	return f.servers.DeleteServer(id)
}

// ImportServers implements api.Handler.
func (f *Facade) ImportServers(req api.ImportRequest) (api.ImportResponse, error) {
	added, failed, errs, err := f.servers.ImportServers(req.Source.Data)
	if err != nil {
		return api.ImportResponse{}, err
	}
	out := api.ImportResponse{Added: added, Failed: failed}
	for _, e := range errs {
		out.Errors = append(out.Errors, api.ImportError{Index: e.Index, Message: e.Message})
	}
	return out, nil
}

// ExportServers implements api.Handler.
func (f *Facade) ExportServers(req api.ExportRequest) (api.ExportResponse, error) {
	blob, err := f.servers.ExportServers(req.IDs)
	if err != nil {
		return api.ExportResponse{}, err
	}
	return api.ExportResponse{Format: server.EnvelopeFormat, Blob: blob}, nil
}

// TestServerLatency implements api.Handler and emits a latencyTested event.
func (f *Facade) TestServerLatency(id string) (api.LatencyResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ms, err := f.servers.TestLatency(ctx, id)
	if err != nil {
		return api.LatencyResponse{}, err
	}
	f.bus.Publish(api.Event{Type: api.EventLatencyTested, Data: api.LatencyTestedEventData{ID: id, LatencyMS: ms}})
	return api.LatencyResponse{LatencyMS: ms}, nil
}

// Connect implements api.Handler. Mode defaults to settings.connectionMode.
func (f *Facade) Connect(req api.ConnectRequest) (api.StateResponse, error) {
	if req.Mode == "" {
		req.Mode = f.config.Settings().ConnectionMode
	}
	if err := f.vpn.Connect(req.ServerID, req.Mode); err != nil {
		return api.StateResponse{}, err
	}
	return f.ConnectionState(), nil
}

// Disconnect implements api.Handler.
func (f *Facade) Disconnect() (api.StateResponse, error) {
	if err := f.vpn.Disconnect(); err != nil {
		return api.StateResponse{}, err
	}
	return f.ConnectionState(), nil
}

// ConnectionState implements api.Handler.
func (f *Facade) ConnectionState() api.StateResponse {
	state, sess := f.vpn.ConnectionState()
	return api.StateResponse{State: state, Session: sess}
}

// GetLogs implements api.Handler.
func (f *Facade) GetLogs(req api.LogsRequest) (api.LogsResponse, error) {
	if req.Limit <= 0 || req.Limit > 500 {
		req.Limit = 200
	}
	return api.LogsResponse{Logs: f.logger.LogsAfter(req.AfterSeq, req.Limit)}, nil
}

// GetSettings implements api.Handler.
func (f *Facade) GetSettings() (api.SettingsResponse, error) {
	return api.SettingsResponse{Settings: f.config.Settings()}, nil
}

// UpdateSettings implements api.Handler, applying the log level immediately.
func (f *Facade) UpdateSettings(req api.SettingsRequest) (api.SettingsResponse, error) {
	s, err := f.config.UpdateSettings(req.Settings)
	if err != nil {
		return api.SettingsResponse{}, err
	}
	f.logger.SetLevel(s.LogLevel)
	f.tunnel.SetLogLevel(s.LogLevel)
	return api.SettingsResponse{Settings: s}, nil
}

// Subscribe implements api.Handler, enabling event delivery to the registered
// sink for the requested event types.
func (f *Facade) Subscribe(req api.SubscribeRequest) error {
	f.subMu.Lock()
	defer f.subMu.Unlock()
	f.subscribed = true
	f.eventSet = make(map[string]bool)
	for _, t := range req.Events {
		switch t {
		case api.EventStateChanged, api.EventLogAppended, api.EventLatencyTested:
			f.eventSet[t] = true
		}
	}
	return nil
}

// Unsubscribe implements api.Handler, disabling event delivery.
func (f *Facade) Unsubscribe() error {
	f.subMu.Lock()
	defer f.subMu.Unlock()
	f.subscribed = false
	f.eventSet = nil
	return nil
}

// facadeSink gates event delivery on the subscription state.
type facadeSink struct{ f *Facade }

func (s facadeSink) SendEvent(e api.Event) {
	f := s.f
	f.subMu.Lock()
	deliver := f.subscribed && f.sink != nil
	if deliver && len(f.eventSet) > 0 {
		deliver = f.eventSet[e.Type]
	}
	sink := f.sink
	f.subMu.Unlock()
	if deliver {
		sink.SendEvent(e)
	}
}

// logSink publishes every redacted log entry as a logAppended event.
type logSink struct{ f *Facade }

func (s logSink) Write(entry models.LogEntry) {
	s.f.bus.Publish(api.Event{Type: api.EventLogAppended, Data: entry})
}
