// Package helperhost is the privileged Linux VPN-mode helper server
// (docs/platform-notes.md §Linux). It owns the sing-box engine lifecycle and
// the TUN interface for VPN mode, serving a single control client over a Unix
// socket. Embedded by core/cmd/omniproxy-helper (the thin pkexec entry point)
// and exercised directly by tests — it is never the sing-box CLI.
package helperhost

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"

	"omniproxy/core/tunnel/helperproto"
	"omniproxy/engine"
)

// Run serves the helper protocol on socketPath: one control connection per
// helper lifetime. When the connection closes (app exit, crash, or explicit
// quit) the tunnel is torn down and Run returns, so no orphaned root-owned TUN
// survives the app.
func Run(socketPath string) error {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		return err
	}
	_ = os.Remove(socketPath)
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	defer l.Close()
	if err := os.Chmod(socketPath, 0o600); err != nil {
		return err
	}

	conn, err := l.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()

	h := &Host{enc: json.NewEncoder(conn)}
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		var msg helperproto.ClientMessage
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "connect":
			h.Connect(msg)
		case "disconnect":
			h.Disconnect(msg.Seq)
		case "ping":
			h.Ping(msg.Seq)
		case "quit":
			h.StopEngine()
			return nil
		}
	}
	h.StopEngine()
	return nil
}

// Host owns the engine lifecycle for the single served connection.
type Host struct {
	mu  sync.Mutex // engine state
	wmu sync.Mutex // serializes writes to the socket
	enc *json.Encoder
	eng *engine.Engine
}

func (h *Host) respond(seq uint64, ok bool, state, errMsg string) {
	h.wmu.Lock()
	defer h.wmu.Unlock()
	_ = h.enc.Encode(helperproto.ServerMessage{
		Type:  "response",
		Seq:   seq,
		OK:    ok,
		State: state,
		Error: errMsg,
	})
}

// Connect starts the engine for the requested options. Options carries the
// full engine.Options so the helper never re-derives engine configuration
// itself (the shared engine module is the single builder).
func (h *Host) Connect(msg helperproto.ClientMessage) {
	h.mu.Lock()
	if h.eng != nil {
		h.mu.Unlock()
		h.respond(msg.Seq, false, "", "tunnel already running")
		return
	}
	h.mu.Unlock()

	eng := engine.New(logSink{h})
	err := eng.Start(msg.Options)

	h.mu.Lock()
	defer h.mu.Unlock()
	if err != nil {
		h.respond(msg.Seq, false, "", err.Error())
		return
	}
	if h.eng != nil {
		_ = eng.Close()
		h.respond(msg.Seq, false, "", "tunnel already running")
		return
	}
	h.eng = eng
	h.respond(msg.Seq, true, "connected", "")
}

// Disconnect stops the engine, if running. Idempotent.
func (h *Host) Disconnect(seq uint64) {
	h.mu.Lock()
	eng := h.eng
	h.eng = nil
	h.mu.Unlock()
	if eng == nil {
		h.respond(seq, true, "disconnected", "")
		return
	}
	if err := eng.Close(); err != nil {
		h.respond(seq, false, "", err.Error())
		return
	}
	h.respond(seq, true, "disconnected", "")
}

// Ping reports the current engine state.
func (h *Host) Ping(seq uint64) {
	h.mu.Lock()
	state := "disconnected"
	if h.eng != nil {
		state = "connected"
	}
	h.mu.Unlock()
	h.respond(seq, true, state, "")
}

// StopEngine stops the engine, if running.
func (h *Host) StopEngine() {
	h.mu.Lock()
	eng := h.eng
	h.eng = nil
	h.mu.Unlock()
	if eng != nil {
		_ = eng.Close()
	}
}

// logSink forwards engine log lines to the control client as events; the core
// routes them into its redacting logger.
type logSink struct{ h *Host }

func (s logSink) WriteMessage(level engine.Level, message string) {
	s.h.wmu.Lock()
	defer s.h.wmu.Unlock()
	_ = s.h.enc.Encode(helperproto.ServerMessage{
		Type:  "event",
		Event: &helperproto.HelperLogEvent{Level: level.String(), Message: message},
	})
}

// IsRunning reports engine state; used by tests.
func (h *Host) IsRunning() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.eng != nil
}
