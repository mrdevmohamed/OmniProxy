//go:build linux || android

package engine

import (
	"net"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing/common/control"
)

func TestPlatformInterfaceMonitorContract(t *testing.T) {
	m := newPlatformInterfaceMonitor()

	if err := m.Start(); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}
	if got := m.DefaultInterface(); got != nil {
		t.Fatalf("DefaultInterface() = %v, want nil", got)
	}
	if m.OverrideAndroidVPN() {
		t.Fatal("OverrideAndroidVPN() = true, want false")
	}
	if m.AndroidVPNEnabled() {
		t.Fatal("AndroidVPNEnabled() = true, want false")
	}

	m.RegisterMyInterface("tun0")
	m.RegisterMyInterface("tun0")
	if got := m.MyInterfaces(); len(got) != 1 || got[0] != "tun0" {
		t.Fatalf("MyInterfaces() = %v, want [tun0] (deduplicated)", got)
	}
}

func TestPlatformInterfaceMonitorUpdateDefaultInterface(t *testing.T) {
	m := newPlatformInterfaceMonitor()

	var updated *control.Interface
	m.RegisterCallback(func(iif *control.Interface, _ int) { updated = iif })

	// Without a network manager the interface is stored as a sparse record.
	m.UpdateDefaultInterface("wlan0", 5)
	got := m.DefaultInterface()
	if got == nil || got.Name != "wlan0" || got.Index != 5 {
		t.Fatalf("DefaultInterface() = %v, want {wlan0, 5}", got)
	}
	if updated == nil || updated.Name != "wlan0" {
		t.Fatalf("callback received %v, want wlan0", updated)
	}

	// Re-pushing the same interface does not fire the callback again.
	updated = nil
	m.UpdateDefaultInterface("wlan0", 5)
	if updated != nil {
		t.Fatalf("callback fired for an unchanged default interface: %v", updated)
	}

	// An empty push clears the default interface.
	m.UpdateDefaultInterface("", -1)
	if got := m.DefaultInterface(); got != nil {
		t.Fatalf("DefaultInterface() = %v, want nil after clear", got)
	}
	if updated != nil {
		t.Fatalf("callback received %v after clear, want nil", updated)
	}
}

func TestFdTunPlatformMonitorNonNil(t *testing.T) {
	p := NewFdTunPlatform()
	if !p.UsePlatformDefaultInterfaceMonitor() {
		t.Fatal("UsePlatformDefaultInterfaceMonitor() = false, want true")
	}
	mon := p.CreateDefaultInterfaceMonitor(nil)
	if mon == nil {
		t.Fatal("CreateDefaultInterfaceMonitor() = nil, want non-nil (sing-box dereferences it)")
	}
	if !p.UsePlatformNetworkInterfaces() {
		t.Fatal("UsePlatformNetworkInterfaces() = false, want true")
	}
}

func TestFdTunPlatformNetworkInterfacesFromHost(t *testing.T) {
	p := NewFdTunPlatform()

	// Before the host pushes anything the list is empty and error-free (no
	// netlink: net.Interfaces() is forbidden on some Android sandboxes).
	ifaces, err := p.NetworkInterfaces()
	if err != nil {
		t.Fatalf("NetworkInterfaces() = %v, want nil err", err)
	}
	if len(ifaces) != 0 {
		t.Fatalf("NetworkInterfaces() = %d entries, want 0 before host push", len(ifaces))
	}

	// The Kotlin host feeds the physical interfaces through the Java API.
	p.SetNetworkInterfaces([]adapter.NetworkInterface{
		{Interface: control.Interface{Index: 2, Name: "wlan0", Flags: net.FlagUp | net.FlagRunning}, Type: C.InterfaceTypeOther},
		{Interface: control.Interface{Index: 1, Name: "lo", Flags: net.FlagUp | net.FlagRunning}, Type: C.InterfaceTypeOther},
	})
	ifaces, err = p.NetworkInterfaces()
	if err != nil {
		t.Fatalf("NetworkInterfaces() = %v, want nil err", err)
	}
	if len(ifaces) != 2 || ifaces[0].Name != "wlan0" || ifaces[0].Index != 2 {
		t.Fatalf("NetworkInterfaces() = %v, want [wlan0/2 lo/1]", ifaces)
	}
	if ifaces[0].Type != C.InterfaceTypeOther {
		t.Fatalf("NetworkInterfaces()[0].Type = %v, want InterfaceTypeOther", ifaces[0].Type)
	}
}
