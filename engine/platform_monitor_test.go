//go:build linux || android

package engine

import (
	"testing"

	"github.com/sagernet/sing/common/control"
)

func TestPlatformInterfaceMonitorPassiveContract(t *testing.T) {
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

func TestPlatformInterfaceMonitorCallbacks(t *testing.T) {
	m := newPlatformInterfaceMonitor()

	var calls int
	elem := m.RegisterCallback(func(_ *control.Interface, _ int) { calls++ })

	m.RegisterCallback(func(_ *control.Interface, _ int) { calls++ })
	// A passive monitor never emits; both callbacks stay registered and
	// unregister cleanly without firing.
	m.UnregisterCallback(elem)
	m.UnregisterCallback(elem)
	if calls != 0 {
		t.Fatalf("callbacks fired %d times, want 0", calls)
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
}
