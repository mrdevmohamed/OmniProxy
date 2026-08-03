package engine

import (
	"net/netip"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/logger"
)

// Platform is the sing-box platform interface hook, re-exported so the core can
// inject a platform without importing sing-box directly. Android supplies the
// VpnService TUN fd through it (see FdTunPlatform); other platforms leave it
// nil and sing-box uses the noopPlatform behavior.
type Platform = adapter.PlatformInterface

// noopPlatform is a PlatformInterface that opts out of every platform hook.
// sing-box then creates the TUN device itself (Linux root helper, Windows
// Wintun). Android overrides this in the bridge layer to supply the
// VpnService file descriptor.
type noopPlatform struct{}

var _ adapter.PlatformInterface = (*noopPlatform)(nil)

func (noopPlatform) Initialize(networkManager adapter.NetworkManager) error { return nil }

func (noopPlatform) UsePlatformAutoDetectInterfaceControl() bool { return false }
func (noopPlatform) AutoDetectInterfaceControl(fd int) error     { return nil }

func (noopPlatform) UsePlatformInterface() bool { return false }
func (noopPlatform) OpenInterface(options *tun.Options, platformOptions option.TunPlatformOptions) (tun.Tun, error) {
	return nil, nil
}

func (noopPlatform) UsePlatformDefaultInterfaceMonitor() bool { return false }
func (noopPlatform) CreateDefaultInterfaceMonitor(logger logger.Logger) tun.DefaultInterfaceMonitor {
	return nil
}

func (noopPlatform) UsePlatformNetworkInterfaces() bool { return false }
func (noopPlatform) NetworkInterfaces() ([]adapter.NetworkInterface, error) {
	return nil, nil
}

func (noopPlatform) UnderNetworkExtension() bool              { return false }
func (noopPlatform) NetworkExtensionIncludeAllNetworks() bool { return false }
func (noopPlatform) ClearDNSCache()                           {}
func (noopPlatform) RequestPermissionForWIFIState() error     { return nil }
func (noopPlatform) ReadWIFIState() adapter.WIFIState         { return adapter.WIFIState{} }
func (noopPlatform) SystemCertificates() []string             { return nil }

func (noopPlatform) UsePlatformConnectionOwnerFinder() bool { return false }
func (noopPlatform) FindConnectionOwner(request *adapter.FindConnectionOwnerRequest) (*adapter.ConnectionOwner, error) {
	return nil, nil
}

func (noopPlatform) UsePlatformWIFIMonitor() bool { return false }

func (noopPlatform) UsePlatformNotification() bool { return false }
func (noopPlatform) SendNotification(notification *adapter.Notification) error {
	return nil
}

func (noopPlatform) MyInterfaceAddress() []netip.Addr { return nil }
