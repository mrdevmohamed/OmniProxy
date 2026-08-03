// Package mobile is the gomobile bind entry point for Android
// (docs/api-contract.md §5.1). The Kotlin host calls these functions through
// the generated `.aar`; events are buffered in a ring and drained on a poller
// thread, then forwarded to Dart over MethodChannel("com.omniproxy/events").
//
// Pure transport plus the platform seam the Android VpnService needs: SetTunFd
// hands the TUN file descriptor to sing-box before a VPN-mode connect.
//
// Build (from repo root): go run golang.org/x/mobile/cmd/gomobile bind \
//
//	-tags with_gvisor -target=android -javapkg=com.omniproxy.bind \
//	-o app/android/app/libs/omniproxy.aar ./core/mobile
package mobile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"omniproxy/core"
	"omniproxy/core/api"
	"omniproxy/core/internal/ring"
	"omniproxy/core/log"
	"omniproxy/core/models"
	"omniproxy/core/secret"
	"omniproxy/core/tunnel"
	"omniproxy/engine"
)

// eventRingCap mirrors the FFI ring: small enough to stay fresh, large enough
// to carry a connect burst (stateChanged + logAppended).
const eventRingCap = 512

var (
	mu      sync.Mutex
	facade  *core.Facade
	runner  *tunnel.InProcessRunner
	tunPlat *engine.FdTunPlatform
	events  = ring.New(eventRingCap)
)

// initConfig mirrors the configJSON passed to Init. DataDir is the app's
// files dir; LogLevel the initial logger level.
type initConfig struct {
	DataDir  string `json:"dataDir"`
	LogLevel string `json:"logLevel"`
}

// Init builds the facade. configJSON carries {dataDir, logLevel}. Safe to call
// again (replaces the previous facade); returns an error on failure.
//
// TODO(M9 security review): the Android SecretStore is a throwaway InMemory
// store until then — the at-rest data key and credential refs do not survive a
// process restart. Wire the Android Keystore data-key handoff there; see
// docs/platform-notes.md §Android.
func Init(configJSON string) error {
	mu.Lock()
	defer mu.Unlock()
	cfg := initConfig{LogLevel: string(models.LevelInfo)}
	if configJSON != "" {
		if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
			return err
		}
	}
	level := models.LogLevel(cfg.LogLevel)
	if !level.Valid() {
		level = models.LevelInfo
	}
	dataDir := cfg.DataDir
	if dataDir == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		dataDir = filepath.Join(dir, "omniproxy")
	}

	logger := log.NewLogger(level)
	runner = tunnel.NewInProcessRunner(logger)
	tunPlat = engine.NewFdTunPlatform()

	f, err := core.New(core.Config{
		Platform:    "android",
		DataDir:     dataDir,
		LogLevel:    level,
		Logger:      logger,
		Runner:      runner,
		SecretStore: secret.NewInMemory(),
	})
	if err != nil {
		return err
	}
	if facade != nil {
		_ = facade.Close()
	}
	facade = f
	facade.SetEventSink(api.FuncEventSink(events.Push))
	return nil
}

// SetTunFd stores the VpnService TUN file descriptor for the next VPN-mode
// connect and points the runner at the fd platform.
func SetTunFd(fd int32) {
	mu.Lock()
	defer mu.Unlock()
	if tunPlat == nil || runner == nil {
		return
	}
	tunPlat.SetFd(int(fd))
	runner.SetPlatform(tunPlat)
}

// Request dispatches a bridge method and returns the canonical response JSON
// ({ok, data|error}).
func Request(method, requestJSON string) string {
	mu.Lock()
	defer mu.Unlock()
	if facade == nil {
		return `{"ok":false,"error":{"code":"internal","message":"core not initialized"}}`
	}
	return string(facade.Dispatch(method, []byte(requestJSON)))
}

// PollEvents returns the buffered bridge events as a JSON array ([] when empty).
func PollEvents() string {
	batch := events.Drain()
	if len(batch) == 0 {
		return "[]"
	}
	b, err := json.Marshal(batch)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// Shutdown closes the facade and releases the tunnel.
func Shutdown() error {
	mu.Lock()
	defer mu.Unlock()
	if facade != nil {
		err := facade.Close()
		facade = nil
		return err
	}
	return nil
}
