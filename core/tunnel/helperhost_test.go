package tunnel

import (
	"path/filepath"
	"testing"
	"time"

	"omniproxy/core/tunnel/helperhost"
	"omniproxy/engine"
)

// TestHelperHostEndToEnd runs the real helperhost server (the exact code the
// omniproxy-helper binary embeds) against the real helperClient, starting and
// stopping a live engine in proxy mode over the socket. VPN mode needs TUN
// privileges and is exercised manually with pkexec.
func TestHelperHostEndToEnd(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "helper.sock")
	done := make(chan error, 1)
	go func() { done <- helperhost.Run(sock) }()

	var c *helperClient
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var err error
		if c, err = dialHelper(sock, nil); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if c == nil {
		t.Fatal("helper server did not come up on the socket")
	}
	defer c.close(true)

	opts := engine.Options{
		Mode:     engine.ModeProxy,
		LogLevel: engine.LevelInfo,
		Outbound: engine.Outbound{Protocol: engine.ProtocolSOCKS, Address: "127.0.0.1", Port: 1},
		Proxy:    engine.ProxyOptions{Listen: "127.0.0.1", Port: 0},
	}
	if err := c.connect(opts); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if !c.running {
		t.Fatal("expected running after connect")
	}
	if err := c.disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if c.running {
		t.Fatal("expected idle after disconnect")
	}

	select {
	case err := <-done:
		t.Fatalf("helper server exited early: %v", err)
	default:
	}
}
