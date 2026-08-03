package engine

import (
	"errors"
	"sync"

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
	mu sync.Mutex
	fd int
}

// NewFdTunPlatform returns a platform interface that feeds the VpnService TUN
// fd into sing-box's tunnel stack.
func NewFdTunPlatform() *FdTunPlatform { return &FdTunPlatform{} }

// SetFd stores the VpnService TUN fd. It must be set before each VPN-mode
// Start; Start runs the tunnel using whatever fd is current.
func (p *FdTunPlatform) SetFd(fd int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fd = fd
}

func (p *FdTunPlatform) UsePlatformInterface() bool { return true }

// UsePlatformDefaultInterfaceMonitor reports true so sing-box uses the
// platform monitor (see CreateDefaultInterfaceMonitor) instead of the netlink
// monitor, which Android forbids.
func (p *FdTunPlatform) UsePlatformDefaultInterfaceMonitor() bool { return true }

// CreateDefaultInterfaceMonitor returns the passive platform monitor. The
// VpnService owns the TUN and its routes, so nothing is watched; the non-nil
// monitor is required by sing-box, which registers its callbacks with it
// unconditionally.
func (p *FdTunPlatform) CreateDefaultInterfaceMonitor(_ logger.Logger) tun.DefaultInterfaceMonitor {
	return newPlatformInterfaceMonitor()
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
