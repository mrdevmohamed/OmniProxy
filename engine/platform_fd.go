//go:build linux || android

package engine

import (
	"errors"
	"sync"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/logger"
	"golang.org/x/sys/unix"
)

// FdTunPlatform is the Android platform hook. The VpnService establishes the
// TUN device and hands its file descriptor to sing-box, which opens it directly
// instead of creating a TUN itself. Every other platform hook stays off.
type FdTunPlatform struct {
	noopPlatform
	mu                sync.Mutex
	fd                int
	protect           func(fd int) error
	networkManager    adapter.NetworkManager
	monitor           *platformInterfaceMonitor
	defaultName       string
	defaultIndex      int
	networkInterfaces []adapter.NetworkInterface
}

// NewFdTunPlatform returns a platform interface that feeds the VpnService TUN
// fd into sing-box's tunnel stack.
func NewFdTunPlatform() *FdTunPlatform { return &FdTunPlatform{} }

// Initialize is called by sing-box after the network manager exists but before
// it starts. It wires the network manager into the interface monitor so the
// monitor can re-populate the interface list (see platform_monitor.go) and
// seeds the monitor with the physical default interface cached by
// UpdateDefaultInterface.
func (p *FdTunPlatform) Initialize(networkManager adapter.NetworkManager) error {
	p.mu.Lock()
	p.networkManager = networkManager
	monitor := p.monitor
	defaultName := p.defaultName
	defaultIndex := p.defaultIndex
	p.mu.Unlock()
	if monitor != nil {
		monitor.setNetworkManager(networkManager)
		if defaultName != "" {
			monitor.UpdateDefaultInterface(defaultName, defaultIndex)
		}
	}
	return nil
}

// SetFd stores the VpnService TUN fd. It must be set before each VPN-mode
// Start; Start runs the tunnel using whatever fd is current.
func (p *FdTunPlatform) SetFd(fd int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fd = fd
}

// SetProtectFunc registers the Android VpnService.protect callback. The
// VpnService routes every app socket into the TUN (0.0.0.0/0), including the
// tunnel's own sockets; without protect() those sockets re-enter the TUN and
// loop (a DNS query for the outbound server domain resolves via dns-local,
// whose direct socket is captured by the TUN, hijacked into the proxy, which
// needs the same domain...). protect() marks a socket so the system sends its
// traffic over the physical network, breaking the loop.
func (p *FdTunPlatform) SetProtectFunc(protect func(fd int) error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.protect = protect
}

// UsePlatformAutoDetectInterfaceControl reports true once a protect func is
// registered. sing-box then routes direct dials (DNS bootstrap, the outbound
// server connection) through AutoDetectInterfaceControl instead of binding to
// an auto-detected interface (which our passive monitor cannot provide).
func (p *FdTunPlatform) UsePlatformAutoDetectInterfaceControl() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.protect != nil
}

// AutoDetectInterfaceControl protects the socket fd from the VpnService TUN.
func (p *FdTunPlatform) AutoDetectInterfaceControl(fd int) error {
	p.mu.Lock()
	protect := p.protect
	p.mu.Unlock()
	if protect == nil {
		return errors.New("engine: android socket protector not set")
	}
	return protect(fd)
}

func (p *FdTunPlatform) UsePlatformInterface() bool { return true }

// UsePlatformDefaultInterfaceMonitor reports true so sing-box uses the
// platform monitor (see CreateDefaultInterfaceMonitor) instead of the netlink
// monitor, which Android forbids.
func (p *FdTunPlatform) UsePlatformDefaultInterfaceMonitor() bool { return true }

// CreateDefaultInterfaceMonitor returns the platform monitor, which the Kotlin
// host feeds with the physical default interface (ConnectivityManager) through
// UpdateDefaultInterface. It stores the monitor on the platform so the engine
// can reach it before the engine Start.
func (p *FdTunPlatform) CreateDefaultInterfaceMonitor(logger logger.Logger) tun.DefaultInterfaceMonitor {
	p.mu.Lock()
	defer p.mu.Unlock()
	monitor := newPlatformInterfaceMonitor()
	monitor.setLogger(logger)
	p.monitor = monitor
	return monitor
}

// UsePlatformNetworkInterfaces reports true so the network manager pulls the
// interface list from NetworkInterfaces (sing-box route/network.go
// UpdateInterfaces) instead of the netlink finder, which Android forbids.
func (p *FdTunPlatform) UsePlatformNetworkInterfaces() bool { return true }

// SetNetworkInterfaces stores the physical interface list fed by the Kotlin
// host (java.net.NetworkInterface enumeration; see Bridge's
// enumerateNetworkInterfaces). Go's net.Interfaces() opens a NETLINK_ROUTE
// socket, which the Android app sandbox denies on some devices, so the host
// enumerates interfaces through the Java API instead. The list is served back
// to the network manager by NetworkInterfaces.
func (p *FdTunPlatform) SetNetworkInterfaces(interfaces []adapter.NetworkInterface) {
	p.mu.Lock()
	p.networkInterfaces = append([]adapter.NetworkInterface(nil), interfaces...)
	monitor := p.monitor
	p.mu.Unlock()
	if monitor != nil {
		monitor.refreshInterfaces()
	}
}

// NetworkInterfaces returns the interface list pushed by the Kotlin host. It
// never touches netlink. Type is unknown (the Java API does not classify the
// transport), so every interface is marked "other"; with the default network
// strategy the type is unused. The TUN itself is filtered out of the dial
// selection by name (see platform_monitor.go MyInterfaces) and by the Kotlin
// host (it never enumerates tun*).
func (p *FdTunPlatform) NetworkInterfaces() ([]adapter.NetworkInterface, error) {
	p.mu.Lock()
	result := append([]adapter.NetworkInterface(nil), p.networkInterfaces...)
	p.mu.Unlock()
	return result, nil
}

// UpdateDefaultInterface is the engine-facing entry point for the Kotlin
// ConnectivityManager push (see core/mobile SetDefaultInterface). The value is
// cached for the monitor that CreateDefaultInterfaceMonitor builds later and
// forwarded to the live monitor once the engine has started.
func (p *FdTunPlatform) UpdateDefaultInterface(interfaceName string, interfaceIndex int) {
	p.mu.Lock()
	p.defaultName = interfaceName
	p.defaultIndex = interfaceIndex
	monitor := p.monitor
	p.mu.Unlock()
	if monitor != nil {
		monitor.UpdateDefaultInterface(interfaceName, interfaceIndex)
	}
}

// OpenInterface implements adapter.PlatformInterface. The fd is duplicated (the
// tun device keeps its own descriptor) and handed to sing-tun.
func (p *FdTunPlatform) OpenInterface(options *tun.Options, _ option.TunPlatformOptions) (tun.Tun, error) {
	p.mu.Lock()
	fd := p.fd
	p.mu.Unlock()
	if fd <= 0 {
		return nil, errors.New("engine: android tun fd not set")
	}
	name, err := tunName(fd)
	if err != nil {
		return nil, err
	}
	options.Name = name
	if options.InterfaceMonitor != nil {
		options.InterfaceMonitor.RegisterMyInterface(name)
	}
	dupFd, err := unix.Dup(fd)
	if err != nil {
		return nil, err
	}
	options.FileDescriptor = dupFd
	return tun.New(*options)
}
