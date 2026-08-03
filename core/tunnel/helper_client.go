package tunnel

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"omniproxy/core/log"
	"omniproxy/core/models"
	"omniproxy/engine"
)

// HelperSpawner starts the privileged helper process. Implementations must be
// safe for concurrent use.
type HelperSpawner interface {
	Spawn(socketPath string) (*exec.Cmd, error)
}

// HelperSpawnFunc adapts a function to HelperSpawner.
type HelperSpawnFunc func(socketPath string) (*exec.Cmd, error)

// Spawn implements HelperSpawner.
func (f HelperSpawnFunc) Spawn(socketPath string) (*exec.Cmd, error) { return f(socketPath) }

const (
	helperSpawnTimeout     = 15 * time.Second
	helperConnectTimeout   = 30 * time.Second
	helperDisconnectTimeout = 5 * time.Second
)

// NewHelperSpawner returns the default spawner: pkexec running the bundled
// omniproxy-helper. When helperPath is empty, OMNIPROXY_HELPER is tried; when
// set, the helper is launched directly (no pkexec) so tests can exercise the
// socket protocol unprivileged.
func NewHelperSpawner(helperPath string) HelperSpawner {
	return HelperSpawnFunc(func(socketPath string) (*exec.Cmd, error) {
		bin := helperPath
		direct := false
		if bin == "" {
			if env := os.Getenv("OMNIPROXY_HELPER"); env != "" {
				bin, direct = env, true
			} else {
				bin = "omniproxy-helper"
			}
		}
		args := []string{"--socket", socketPath}
		if direct {
			return exec.Command(bin, args...), nil
		}
		return exec.Command("pkexec", append([]string{bin}, args...)...), nil
	})
}

// HelperRunner routes tunnel runs per mode: proxy mode runs the engine
// in-process; VPN mode drives the privileged helper over its control socket
// (docs/platform-notes.md §Linux). It is the Linux bridge's runner seam.
type HelperRunner struct {
	logger  *log.Logger
	spawner HelperSpawner
	inProc  *InProcessRunner

	mu     sync.Mutex
	client *helperClient
	cmd    *exec.Cmd
}

// NewHelperAwareRunner returns a mode-aware runner. spawner may be nil to use
// the default pkexec spawner.
func NewHelperAwareRunner(logger *log.Logger, spawner HelperSpawner) *HelperRunner {
	if spawner == nil {
		spawner = NewHelperSpawner("")
	}
	return &HelperRunner{logger: logger, spawner: spawner, inProc: NewInProcessRunner(logger)}
}

// Start implements Runner.
func (r *HelperRunner) Start(opts engine.Options) error {
	if opts.Mode == engine.ModeProxy {
		return r.inProc.Start(opts)
	}
	client, err := r.ensureHelper()
	if err != nil {
		return err
	}
	if err := client.connect(opts); err != nil {
		return err
	}
	r.logger.Infof("tunnel", "vpn engine running in privileged helper")
	return nil
}

// Stop implements Runner.
func (r *HelperRunner) Stop() error {
	if r.inProc.Running() {
		return r.inProc.Stop()
	}
	r.mu.Lock()
	client := r.client
	r.mu.Unlock()
	if client != nil {
		if err := client.disconnect(); err != nil {
			r.logger.Warnf("tunnel", "helper disconnect: %v", err)
			return err
		}
		r.logger.Infof("tunnel", "vpn engine stopped in privileged helper")
	}
	return nil
}

// Running implements Runner.
func (r *HelperRunner) Running() bool {
	if r.inProc.Running() {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.client != nil && r.client.running
}

// Close sends quit to the helper and reaps the process. Idempotent.
func (r *HelperRunner) Close() error {
	r.mu.Lock()
	client := r.client
	cmd := r.cmd
	r.client, r.cmd = nil, nil
	r.mu.Unlock()

	var errs []error
	if client != nil {
		if err := client.close(true); err != nil {
			errs = append(errs, err)
		}
	}
	if cmd != nil && cmd.Process != nil {
		done := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}
	if r.inProc.Running() {
		if err := r.inProc.Stop(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ensureHelper returns a live control client, spawning the helper on demand.
func (r *HelperRunner) ensureHelper() (*helperClient, error) {
	r.mu.Lock()
	if r.client != nil && r.client.alive() {
		c := r.client
		r.mu.Unlock()
		return c, nil
	}
	r.mu.Unlock()

	sock, err := HelperSocketPath()
	if err != nil {
		return nil, fmt.Errorf("privileged helper: %w", err)
	}
	if c, err := dialHelper(sock, r.logger); err == nil {
		r.mu.Lock()
		r.client = c
		r.mu.Unlock()
		return c, nil
	}

	cmd, err := r.spawner.Spawn(sock)
	if err != nil {
		return nil, fmt.Errorf("privileged helper: spawn: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("privileged helper: start: %w", err)
	}
	r.mu.Lock()
	r.cmd = cmd
	r.mu.Unlock()

	deadline := time.Now().Add(helperSpawnTimeout)
	for time.Now().Before(deadline) {
		if c, err := dialHelper(sock, r.logger); err == nil {
			r.mu.Lock()
			r.client = c
			r.mu.Unlock()
			return c, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil, errors.New("privileged helper: timed out waiting for the helper socket (pkexec cancelled?)")
}

// helperClient is the core's JSON-over-socket control client. Requests are
// correlated by sequence number; engine log events are forwarded to the core
// logger (which redacts and fans them out as logAppended events).
type helperClient struct {
	conn   net.Conn
	enc    *json.Encoder
	logger *log.Logger

	mu      sync.Mutex
	seqCounter uint64
	pending map[uint64]chan ServerMessage
	closed  bool
	running bool
}

func dialHelper(socketPath string, logger *log.Logger) (*helperClient, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, err
	}
	c := &helperClient{
		conn:    conn,
		enc:     json.NewEncoder(conn),
		logger:  logger,
		pending: make(map[uint64]chan ServerMessage),
	}
	go c.readLoop()
	return c, nil
}

func (c *helperClient) alive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed
}

func (c *helperClient) readLoop() {
	sc := bufio.NewScanner(c.conn)
	for sc.Scan() {
		var msg ServerMessage
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
			continue
		}
		if msg.Type == "event" {
			if msg.Event != nil && c.logger != nil {
				c.logger.Log(helperLevelToModel(msg.Event.Level), "engine", msg.Event.Message, nil)
			}
			continue
		}
		if msg.Type != "response" {
			continue
		}
		c.mu.Lock()
		if msg.OK && msg.State == "connected" {
			c.running = true
		}
		if msg.State == "disconnected" {
			c.running = false
		}
		ch := c.pending[msg.Seq]
		delete(c.pending, msg.Seq)
		c.mu.Unlock()
		if ch != nil {
			ch <- msg
		}
	}
	_ = c.close(false)
}

func (c *helperClient) nextSeq() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seqCounter++
	return c.seqCounter
}

func (c *helperClient) send(msg ClientMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("helper: connection closed")
	}
	return c.enc.Encode(msg)
}

func (c *helperClient) await(seq uint64, timeout time.Duration) (ServerMessage, error) {
	ch := make(chan ServerMessage, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ServerMessage{}, errors.New("helper: connection closed")
	}
	c.pending[seq] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, seq)
		c.mu.Unlock()
	}()
	select {
	case msg := <-ch:
		return msg, nil
	case <-time.After(timeout):
		return ServerMessage{}, errors.New("helper: request timed out")
	}
}

func (c *helperClient) connect(opts engine.Options) error {
	seq := c.nextSeq()
	if err := c.send(ClientMessage{Type: "connect", Seq: seq, Options: opts}); err != nil {
		return fmt.Errorf("privileged helper: %w", err)
	}
	resp, err := c.await(seq, helperConnectTimeout)
	if err != nil {
		return fmt.Errorf("privileged helper: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("privileged helper: %s", resp.Error)
	}
	return nil
}

func (c *helperClient) disconnect() error {
	seq := c.nextSeq()
	if err := c.send(ClientMessage{Type: "disconnect", Seq: seq}); err != nil {
		return fmt.Errorf("privileged helper: %w", err)
	}
	resp, err := c.await(seq, helperDisconnectTimeout)
	if err != nil {
		return fmt.Errorf("privileged helper: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("privileged helper: %s", resp.Error)
	}
	return nil
}

// close tears the connection down, optionally sending quit first. Idempotent.
func (c *helperClient) close(sendQuit bool) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.running = false
	pending := c.pending
	c.pending = nil
	c.mu.Unlock()

	var errs []error
	if sendQuit {
		if err := c.enc.Encode(ClientMessage{Type: "quit"}); err != nil {
			errs = append(errs, err)
		}
	}
	if err := c.conn.Close(); err != nil {
		errs = append(errs, err)
	}
	for _, ch := range pending {
		ch <- ServerMessage{Type: "response", OK: false, Error: "helper: connection closed"}
	}
	return errors.Join(errs...)
}

func helperLevelToModel(l string) models.LogLevel {
	return engineToModelLevel(helperLevelToEngine(l))
}

func helperLevelToEngine(l string) engine.Level {
	switch l {
	case "trace":
		return engine.LevelTrace
	case "debug":
		return engine.LevelDebug
	case "warn":
		return engine.LevelWarn
	case "error", "fatal", "panic":
		return engine.LevelError
	default:
		return engine.LevelInfo
	}
}
