//go:build android

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
//
// Android-only: SetTunFd feeds the VpnService TUN fd through the engine's
// Linux/Android FdTunPlatform hook.
package mobile

import (
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sync"

	"github.com/sagernet/sing-box/adapter"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing/common/control"
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

// SocketProtector is implemented by the Kotlin host. It protects a socket fd
// from being routed into the VpnService TUN, keeping the tunnel's own traffic
// (DNS bootstrap, the proxy server connection) on the physical network.
type SocketProtector interface {
	Protect(fd int32) bool
}

// SetSocketProtector registers the VpnService protect callback. Must be set
// before a VPN-mode connect (the VpnService holds the instance). Passing nil
// clears it.
func SetSocketProtector(p SocketProtector) {
	mu.Lock()
	defer mu.Unlock()
	if tunPlat == nil {
		return
	}
	if p != nil {
		tunPlat.SetProtectFunc(func(fd int) error {
			if p.Protect(int32(fd)) {
				return nil
			}
			return errors.New("mobile: VpnService.protect rejected fd")
		})
	} else {
		tunPlat.SetProtectFunc(nil)
	}
}

// SetDefaultInterface records the physical default network interface reported
// by the Android ConnectivityManager (Bridge's DefaultNetworkMonitor). The
// VpnService routes 0.0.0.0/0 into the TUN, so sing-box cannot watch the real
// default network through netlink; the Kotlin host feeds it here (mirrors
// libbox.platformDefaultInterfaceMonitor.UpdateDefaultInterface). An empty name
// and index -1 clear the default. Must be called before a VPN-mode connect.
func SetDefaultInterface(interfaceName string, interfaceIndex int32) {
	mu.Lock()
	defer mu.Unlock()
	if tunPlat == nil {
		return
	}
	tunPlat.UpdateDefaultInterface(interfaceName, int(interfaceIndex))
}

// pushedInterface is one entry of the JSON array accepted by
// SetNetworkInterfaces. It mirrors java.net.NetworkInterface: name, index,
// mtu, up, and the interface's address prefixes as "ip/prefixlen" strings.
type pushedInterface struct {
	Name      string   `json:"name"`
	Index     int      `json:"index"`
	MTU       int      `json:"mtu"`
	Up        bool     `json:"up"`
	Addresses []string `json:"addresses"`
}

// SetNetworkInterfaces records the physical interface list reported by the
// Kotlin host (java.net.NetworkInterface enumeration). Go's net.Interfaces()
// opens a NETLINK_ROUTE socket, which the Android app sandbox denies on some
// devices, so the host enumerates interfaces through the Java API and feeds
// them here. sing-box's network manager serves them back to the default dialer
// (route/network.go UpdateInterfaces -> NetworkInterfaces). Must be called
// before a VPN-mode connect.
func SetNetworkInterfaces(interfacesJSON string) {
	mu.Lock()
	defer mu.Unlock()
	if tunPlat == nil {
		return
	}
	var pushed []pushedInterface
	if interfacesJSON != "" {
		if err := json.Unmarshal([]byte(interfacesJSON), &pushed); err != nil {
			return
		}
	}
	interfaces := make([]adapter.NetworkInterface, 0, len(pushed))
	for _, it := range pushed {
		flags := net.Flags(0)
		if it.Up {
			flags = net.FlagUp | net.FlagRunning
		}
		var addresses []netip.Prefix
		for _, raw := range it.Addresses {
			if prefix, err := netip.ParsePrefix(raw); err == nil {
				addresses = append(addresses, prefix)
			}
		}
		interfaces = append(interfaces, adapter.NetworkInterface{
			Interface: control.Interface{
				Index:     it.Index,
				MTU:       it.MTU,
				Name:      it.Name,
				Flags:     flags,
				Addresses: addresses,
			},
			Type: C.InterfaceTypeOther,
		})
	}
	tunPlat.SetNetworkInterfaces(interfaces)
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
