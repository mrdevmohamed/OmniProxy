// Package main (core/glue) is the c-shared entry point for the Linux/Windows
// FFI bridge (docs/api-contract.md §5.2). It exports the canonical C ABI —
// omniproxy_init / omniproxy_request / omniproxy_poll_events /
// omniproxy_shutdown / omniproxy_free_string — forwarding requests to the core
// facade and buffering events for the transport to drain.
//
// Events are polled, not pushed: a native callback into the Dart isolate
// deadlocks when the isolate is blocked inside a synchronous request call that
// itself publishes an event (the cgo thread cannot hand the callback off while
// the isolate is inside the FFI call). So the glue keeps a bounded ring that
// Dart drains with a short timer. Pure transport: no platform business logic.
//
// Build: go build -buildmode=c-shared -o libomniproxy.so ./core/glue
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	"omniproxy/core"
	"omniproxy/core/api"
	"omniproxy/core/internal/ring"
	"omniproxy/core/log"
	"omniproxy/core/models"
)

func main() {}

var (
	coreMu sync.Mutex
	facade *core.Facade
	events = ring.New(eventRingCap)
)

// eventRingCap bounds buffered events between polls (stateChanged, logAppended,
// latencyTested). Oldest entries are dropped first; stateChanged is frequently
// superseded, so a small cap keeps delivery fresh without loss of context.
const eventRingCap = 512

// initConfig mirrors the optional config_json passed to omniproxy_init.
// DataDir is the platform config directory; LogLevel the initial logger level;
// HelperPath the location of the omniproxy-helper binary (VPN mode, Linux only).
type initConfig struct {
	DataDir    string `json:"dataDir"`
	LogLevel   string `json:"logLevel"`
	HelperPath string `json:"helperPath"`
}

// defaultDataDir falls back to the OS config directory:
// $XDG_CONFIG_HOME/omniproxy (~/.config/omniproxy) on Linux, %APPDATA%\OmniProxy
// on Windows (os.UserConfigDir is per-platform).
func defaultDataDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return ".omniproxy"
	}
	return filepath.Join(base, "omniproxy")
}

//export omniproxy_init
func omniproxy_init(configJSON *C.char) C.int {
	cfg := initConfig{LogLevel: string(models.LevelInfo)}
	if configJSON != nil {
		raw := C.GoString(configJSON)
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
				return -1
			}
		}
	}
	level := models.LogLevel(cfg.LogLevel)
	if !level.Valid() {
		level = models.LevelInfo
	}
	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = defaultDataDir()
	}

	logger := log.NewLogger(level)
	runner := newRunner(logger, cfg.HelperPath)
	f, err := core.New(core.Config{
		Platform: platformName(),
		DataDir:  dataDir,
		LogLevel: level,
		Logger:   logger,
		Runner:   runner,
	})
	if err != nil {
		return -1
	}

	coreMu.Lock()
	defer coreMu.Unlock()
	if facade != nil {
		_ = facade.Close()
	}
	facade = f
	facade.SetEventSink(api.FuncEventSink(enqueueEvent))
	return 0
}

//export omniproxy_request
func omniproxy_request(method *C.char, requestJSON *C.char) *C.char {
	coreMu.Lock()
	defer coreMu.Unlock()
	if facade == nil {
		return cString(mustJSON(api.Response{OK: false, Error: &api.Error{Code: api.ErrCodeInternal, Message: "core not initialized"}}))
	}
	m := ""
	if method != nil {
		m = C.GoString(method)
	}
	var req []byte
	if requestJSON != nil {
		req = []byte(C.GoString(requestJSON))
	}
	return cString(string(facade.Dispatch(m, req)))
}

//export omniproxy_poll_events
func omniproxy_poll_events() *C.char {
	batch := events.Drain()
	if len(batch) == 0 {
		return cString("[]")
	}
	return cString(mustJSON(batch))
}

// enqueueEvent buffers one bridge event for the next poll. Runs on the core's
// publisher goroutine; never blocks on the Dart side.
func enqueueEvent(e api.Event) { events.Push(e) }

//export omniproxy_shutdown
func omniproxy_shutdown() {
	coreMu.Lock()
	defer coreMu.Unlock()
	if facade != nil {
		_ = facade.Close()
		facade = nil
	}
	events.Drain()
}

//export omniproxy_free_string
func omniproxy_free_string(s *C.char) {
	if s != nil {
		C.free(unsafe.Pointer(s))
	}
}

func cString(s string) *C.char { return C.CString(s) }

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"ok":false,"error":{"code":"internal","message":"encode error"}}`
	}
	return string(b)
}
