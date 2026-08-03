package engine

import (
	"sync"

	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/control"
	"github.com/sagernet/sing/common/x/list"
)

// platformInterfaceMonitor is a passive tun.DefaultInterfaceMonitor for the
// Android VpnService flow. The VpnService owns the TUN device and its routing
// on the Android side, so there is nothing for sing-box to watch: this monitor
// never reports a default interface and never emits updates.
//
// It still has to be non-nil: when a platform interface is present sing-box
// calls CreateDefaultInterfaceMonitor unconditionally and registers its
// network-update callback and the TUN stack registers its my-interface names
// and route-update callback with the result (sing-box route/network.go,
// sing-tun tun_linux.go).
type platformInterfaceMonitor struct {
	access       sync.Mutex
	callbacks    list.List[tun.DefaultInterfaceUpdateCallback]
	myInterfaces []string
}

var _ tun.DefaultInterfaceMonitor = (*platformInterfaceMonitor)(nil)

func newPlatformInterfaceMonitor() *platformInterfaceMonitor {
	return &platformInterfaceMonitor{}
}

func (m *platformInterfaceMonitor) Start() error { return nil }

func (m *platformInterfaceMonitor) Close() error { return nil }

func (m *platformInterfaceMonitor) DefaultInterface() *control.Interface { return nil }

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
