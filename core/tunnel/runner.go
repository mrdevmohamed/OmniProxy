package tunnel

import (
	"omniproxy/core/log"
	"omniproxy/core/models"
	"omniproxy/engine"
)

// Runner owns the engine lifecycle. In-process on every platform in Phase 1;
// Linux VPN mode swaps this for a socket client to the privileged helper (M6).
type Runner interface {
	Start(opts engine.Options) error
	Stop() error
	Running() bool
}

// PlatformSetter is implemented by runners that accept a sing-box platform
// interface. Android injects the VpnService TUN fd this way before VPN-mode
// Start; other platforms leave the engine on its noop platform.
type PlatformSetter interface {
	SetPlatform(engine.Platform)
}

// InProcessRunner runs the sing-box engine directly in this process.
type InProcessRunner struct {
	eng    *engine.Engine
	logger *log.Logger
}

// NewInProcessRunner returns a runner wrapping a fresh engine.
func NewInProcessRunner(logger *log.Logger) *InProcessRunner {
	return &InProcessRunner{eng: engine.New(engineLogSink{logger: logger}), logger: logger}
}

// SetPlatform implements PlatformSetter, injecting the platform interface (e.g.
// Android FdTunPlatform) before Start.
func (r *InProcessRunner) SetPlatform(pi engine.Platform) { r.eng.SetPlatformInterface(pi) }

// Start implements Runner.
func (r *InProcessRunner) Start(opts engine.Options) error {
	r.logger.Infof("tunnel", "starting tunnel (%s)", opts.Mode)
	return r.eng.Start(opts)
}

// Stop implements Runner.
func (r *InProcessRunner) Stop() error { return r.eng.Close() }

// Running implements Runner.
func (r *InProcessRunner) Running() bool { return r.eng.Running() }

// engineLogSink adapts the core logger to the engine log interface. The logger
// redacts every message before it reaches any sink or subscriber.
type engineLogSink struct{ logger *log.Logger }

func (s engineLogSink) WriteMessage(level engine.Level, message string) {
	if s.logger == nil {
		return
	}
	s.logger.Log(engineToModelLevel(level), "engine", message, nil)
}

// engineToModelLevel maps an engine log level onto the models enum.
func engineToModelLevel(level engine.Level) models.LogLevel {
	switch level {
	case engine.LevelTrace:
		return models.LevelTrace
	case engine.LevelDebug:
		return models.LevelDebug
	case engine.LevelWarn:
		return models.LevelWarn
	case engine.LevelError, engine.LevelFatal, engine.LevelPanic:
		return models.LevelError
	default:
		return models.LevelInfo
	}
}

// modelToEngineLevel maps the models log level onto the engine enum.
func modelToEngineLevel(lv models.LogLevel) engine.Level {
	switch lv {
	case models.LevelTrace:
		return engine.LevelTrace
	case models.LevelDebug:
		return engine.LevelDebug
	case models.LevelWarn:
		return engine.LevelWarn
	case models.LevelError:
		return engine.LevelError
	default:
		return engine.LevelInfo
	}
}
