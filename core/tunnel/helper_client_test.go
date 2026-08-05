package tunnel

import (
	"bufio"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"omniproxy/core/log"
	"omniproxy/core/models"
	"omniproxy/engine"
)

// fakeHelper is a minimal in-process server speaking the helper wire protocol
// (newline-delimited JSON). It mimics cmd/omniproxy-helper behavior for
// connect/disconnect/ping without requiring privileges.
type fakeHelper struct {
	ln      net.Listener
	started bool
	closed  chan struct{}
	pings   atomic.Int64
}

func startFakeHelper(t *testing.T, dir string) *fakeHelper {
	t.Helper()
	path := filepath.Join(dir, "helper.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	h := &fakeHelper{ln: ln, closed: make(chan struct{})}
	go h.serve()
	return h
}

func (h *fakeHelper) serve() {
	defer close(h.closed)
	for {
		conn, err := h.ln.Accept()
		if err != nil {
			return
		}
		h.handle(conn)
		return
	}
}

func (h *fakeHelper) handle(conn net.Conn) {
	defer conn.Close()
	enc := json.NewEncoder(conn)
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		var msg ClientMessage
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "connect":
			h.started = true
			_ = enc.Encode(ServerMessage{Type: "response", Seq: msg.Seq, OK: true, State: "connected"})
			_ = enc.Encode(ServerMessage{Type: "event", Event: &HelperLogEvent{Level: "info", Message: "fake engine started"}})
		case "disconnect":
			h.started = false
			_ = enc.Encode(ServerMessage{Type: "response", Seq: msg.Seq, OK: true, State: "disconnected"})
		case "ping":
			h.pings.Add(1)
			state := "disconnected"
			if h.started {
				state = "connected"
			}
			_ = enc.Encode(ServerMessage{Type: "response", Seq: msg.Seq, OK: true, State: state})
		case "quit":
			return
		}
	}
}

func TestHelperClientConnectDisconnect(t *testing.T) {
	h := startFakeHelper(t, t.TempDir())
	defer func() {
		h.ln.Close()
		<-h.closed
	}()

	c, err := dialHelper(h.ln.Addr().String(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.close(false)

	if err := c.connect(engine.Options{Mode: engine.ModeVPN}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if !c.running {
		t.Fatal("expected running after connect")
	}
	if err := c.disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if c.running {
		t.Fatal("expected not running after disconnect")
	}
}

func TestHelperClientConnectError(t *testing.T) {
	ln, err := net.Listen("unix", filepath.Join(t.TempDir(), "err.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		enc := json.NewEncoder(conn)
		sc := bufio.NewScanner(conn)
		if sc.Scan() {
			var msg ClientMessage
			_ = json.Unmarshal(sc.Bytes(), &msg)
			_ = enc.Encode(ServerMessage{Type: "response", Seq: msg.Seq, OK: false, Error: "tunnel already running"})
		}
	}()

	c, err := dialHelper(ln.Addr().String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.close(false)
	err = c.connect(engine.Options{Mode: engine.ModeVPN})
	if err == nil || !strings.Contains(err.Error(), "tunnel already running") {
		t.Fatalf("expected helper error, got %v", err)
	}
}

func TestHelperClientEventForwarding(t *testing.T) {
	h := startFakeHelper(t, t.TempDir())
	defer func() {
		h.ln.Close()
		<-h.closed
	}()

	logger := log.NewLogger(models.LevelDebug)
	c, err := dialHelper(h.ln.Addr().String(), logger)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.close(false)

	if err := c.connect(engine.Options{Mode: engine.ModeVPN}); err != nil {
		t.Fatalf("connect: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, e := range logger.LogsAfter(0, 20) {
			if strings.Contains(e.Message, "fake engine started") {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("engine log event was not forwarded to the core logger")
}

func TestHelperClientDialRejectsNonexistentSocket(t *testing.T) {
	if _, err := dialHelper(filepath.Join(t.TempDir(), "nope.sock"), nil); err == nil {
		t.Fatal("expected dial error")
	}
}

func TestHelperSocketPath(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	p, err := HelperSocketPath()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(p, filepath.Join("omniproxy", "helper.sock")) {
		t.Fatalf("unexpected socket path %q", p)
	}
}

func TestHelperClientNotifiesLostOnDeath(t *testing.T) {
	ln, err := net.Listen("unix", filepath.Join(t.TempDir(), "death.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		// The helper process dies: the server side just goes away.
		conn.Close()
	}()

	c, err := dialHelper(ln.Addr().String(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.close(false)

	select {
	case <-c.lost:
	case <-time.After(2 * time.Second):
		t.Fatal("expected the lost channel to fire when the helper connection dropped")
	}
	if c.wasIntentional() {
		t.Fatal("a dropped connection should be classified as loss, not intentional teardown")
	}
}

func TestHelperClientCloseDoesNotSignalLoss(t *testing.T) {
	h := startFakeHelper(t, t.TempDir())
	defer func() {
		h.ln.Close()
		<-h.closed
	}()

	c, err := dialHelper(h.ln.Addr().String(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	select {
	case <-c.lost:
		t.Fatal("lost channel fired before any close")
	default:
	}

	// Deliberate teardown (quit) must not be reported as loss.
	if err := c.close(true); err != nil {
		t.Fatalf("close: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	select {
	case <-c.lost:
		t.Fatal("deliberate close must not signal loss")
	default:
	}
	if !c.wasIntentional() {
		t.Fatal("close(true) should be classified as intentional teardown")
	}
}

func TestHelperClientSendsKeepalivePing(t *testing.T) {
	origInterval := helperPingInterval
	helperPingInterval = 50 * time.Millisecond
	defer func() { helperPingInterval = origInterval }()

	h := startFakeHelper(t, t.TempDir())
	defer func() {
		h.ln.Close()
		<-h.closed
	}()

	c, err := dialHelper(h.ln.Addr().String(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.close(false)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if h.pings.Load() >= 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected periodic ping from the client keepalive")
}

func TestHelperClientDetectsWedgedHelper(t *testing.T) {
	origInterval := helperPingInterval
	origTimeout := helperPingTimeout
	helperPingInterval = 20 * time.Millisecond
	helperPingTimeout = 100 * time.Millisecond
	defer func() {
		helperPingInterval = origInterval
		helperPingTimeout = origTimeout
	}()

	ln, err := net.Listen("unix", filepath.Join(t.TempDir(), "wedged.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Accept but never respond: simulates a wedged helper.
		buf := make([]byte, 4096)
		for {
			if _, err := conn.Read(buf); err != nil {
				return
			}
		}
	}()

	c, err := dialHelper(ln.Addr().String(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.close(false)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !c.alive() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected the client to drop the wedged helper connection")
}
