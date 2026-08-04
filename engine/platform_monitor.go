//go:build linux || android

package engine

import (
	"sync"

	"github.com/sagernet/sing-box/adapter"
	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/control"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
	"github.com/sagernet/sing/common/x/list"
)

// platformInterfaceMonitor is the tun.DefaultInterfaceMonitor for the Android
// VpnService flow, modeled after libbox.platformDefaultInterfaceMonitor.
//
// The VpnService owns the TUN device and its routing, and Android forbids
// watching the physical network through netlink, so the Kotlin host reports the
// physical default interface from ConnectivityManager (Bridge's
// DefaultNetworkMonitor) through UpdateDefaultInterface. The value is stored
// and sing-box's registered callbacks fire so the tunnel re-evaluates its
// routes (and reconnects) on network change.
//
// The monitor also keeps the network manager's interface list populated: with
// Route.AutoDetectInterface set, the default dialer picks the interfaces it
// dials through from networkManager.NetworkInterfaces() (sing-box
// common/dialer/default.go) and fails with "no available network interface"
// when that list is empty.
type platformInterfaceMonitor struct {
	access           sync.Mutex
	logger           logger.Logger
	networkManager   adapter.NetworkManager
	callbacks        list.List[tun.DefaultInterfaceUpdateCallback]
	myInterfaces     []string
	defaultInterface *control.Interface
}

var _ tun.DefaultInterfaceMonitor = (*platformInterfaceMonitor)(nil)

func newPlatformInterfaceMonitor() *platformInterfaceMonitor {
	return &platformInterfaceMonitor{}
}

func (m *platformInterfaceMonitor) setLogger(logger logger.Logger) {
	m.access.Lock()
	defer m.access.Unlock()
	m.logger = logger
}

// setNetworkManager wires the network manager so updates can re-populate its
// interface list. Only available from the platform Initialize call, which
// sing-box performs after CreateDefaultInterfaceMonitor.
func (m *platformInterfaceMonitor) setNetworkManager(networkManager adapter.NetworkManager) {
	m.access.Lock()
	defer m.access.Unlock()
	m.networkManager = networkManager
}

// refreshInterfaces re-populates the network manager's interface list. Called
// on Start and on every UpdateDefaultInterface so the default dialer always has
// the current physical networks to choose from.
func (m *platformInterfaceMonitor) refreshInterfaces() {
	m.access.Lock()
	networkManager := m.networkManager
	logger := m.logger
	m.access.Unlock()
	if networkManager == nil {
		return
	}
	if err := networkManager.UpdateInterfaces(); err != nil && logger != nil {
		logger.Error(E.Cause(err, "update interfaces"))
	}
}

func (m *platformInterfaceMonitor) Start() error {
	m.refreshInterfaces()
	return nil
}

func (m *platformInterfaceMonitor) Close() error { return nil }

func (m *platformInterfaceMonitor) DefaultInterface() *control.Interface {
	m.access.Lock()
	defer m.access.Unlock()
	return m.defaultInterface
}

// UpdateDefaultInterface is called by the Kotlin host when ConnectivityManager
// reports a new physical default network (and on Start through the platform's
// cached value). It re-populates the network manager's interface list, stores
// the new default interface and notifies sing-box's registered callbacks. An
// empty name or a negative index clears the default interface.
func (m *platformInterfaceMonitor) UpdateDefaultInterface(interfaceName string, interfaceIndex int) {
	m.refreshInterfaces()
	m.access.Lock()
	defer m.access.Unlock()

	var newInterface *control.Interface
	if interfaceName != "" && interfaceIndex >= 0 {
		networkManager := m.networkManager
		if networkManager != nil {
			newInterface, _ = networkManager.InterfaceFinder().ByIndex(interfaceIndex)
		}
		if newInterface == nil {
			newInterface = &control.Interface{Index: interfaceIndex, Name: interfaceName}
		}
	}
	if m.defaultInterface != nil && newInterface != nil &&
		m.defaultInterface.Index == newInterface.Index && m.defaultInterface.Name == newInterface.Name {
		return
	}
	m.defaultInterface = newInterface
	callbacks := m.callbacks.Array()
	for _, callback := range callbacks {
		callback(newInterface, 0)
	}
}

func (m *platformInterfaceMonitor) OverrideAndroidVPN() bool { return false }

func (m *platformInterfaceMonitor) AndroidVPNEnabled() bool { return false }

func (m *platformInterfaceMonitor) RegisterCallback(callback tun.DefaultInterfaceUpdateCallback) *list.Element[tun.DefaultInterfaceUpdateCallback] {
	m.access.Lock()
	defer m.access.Unlock()
	return m.callbacks.PushBack(callback)
}

func (m *platformInterfaceMonitor) UnregisterCallback(element *list.Element[tun.DefaultInterfaceUpdateCallback]) {
	m.access.Lock()
	defer m.access.Unlock()
	m.callbacks.Remove(element)
}

func (m *platformInterfaceMonitor) RegisterMyInterface(interfaceName string) {
	m.access.Lock()
	defer m.access.Unlock()
	for _, name := range m.myInterfaces {
		if name == interfaceName {
			return
		}
	}
	m.myInterfaces = append(m.myInterfaces, interfaceName)
}

func (m *platformInterfaceMonitor) MyInterfaces() []string {
	m.access.Lock()
	defer m.access.Unlock()
	return append([]string(nil), m.myInterfaces...)
}
