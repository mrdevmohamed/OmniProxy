package engine

import (
	"context"
	"sync"

	"github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing/service"
)

// Engine wraps a sing-box instance for one tunnel run.
type Engine struct {
	mu       sync.Mutex
	box      *box.Box
	ctx      context.Context
	cancel   context.CancelFunc
	logSink  LogSink
	platform adapter.PlatformInterface
}

// New returns an idle engine. Start may be called multiple times (one run at a time).
func New(logSink LogSink) *Engine {
	return &Engine{logSink: logSink}
}

// SetPlatformInterface injects a platform hook (e.g. Android VpnService fd
// provider). Must be called before Start. Nil disables platform integration.
func (e *Engine) SetPlatformInterface(pi adapter.PlatformInterface) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.platform = pi
}

func (e *Engine) currentPlatform() adapter.PlatformInterface {
	if e.platform != nil {
		return e.platform
	}
	return nil
}

// Start builds the sing-box configuration and starts the tunnel.
// Start fails if an engine is already running.
func (e *Engine) Start(opts Options) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.box != nil {
		return errAlreadyRunning
	}
	built, err := buildOptions(opts)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	ctx = newContext(ctx)
	ctx = service.ContextWith[adapter.PlatformInterface](ctx, e.currentPlatform())
	sb, err := box.New(box.Options{
		Options:           built,
		Context:           ctx,
		PlatformLogWriter: platformLogWriter{sink: e.logSink},
	})
	if err != nil {
		cancel()
		return err
	}
	if err := sb.Start(); err != nil {
		_ = sb.Close()
		cancel()
		return err
	}
	e.ctx, e.cancel, e.box = ctx, cancel, sb
	return nil
}

// Close stops the tunnel. It is safe to call when not running.
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.box == nil {
		return nil
	}
	err := e.box.Close()
	if e.cancel != nil {
		e.cancel()
	}
	e.cancel, e.box, e.ctx = nil, nil, nil
	return err
}

// Running reports whether a tunnel is currently started.
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.box != nil
}
