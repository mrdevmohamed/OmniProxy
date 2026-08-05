package vpn

import (
	"errors"
	"sync"
	"testing"
	"time"

	"omniproxy/core/api"
	"omniproxy/core/log"
	"omniproxy/core/models"
)

type fakeResolver struct {
	profiles map[string]*models.ServerProfile
	err      error
}

func (r fakeResolver) Resolve(id string) (*models.ServerProfile, error) {
	if r.err != nil {
		return nil, r.err
	}
	p, ok := r.profiles[id]
	if !ok {
		return nil, errors.New("vpn: unknown server")
	}
	return p.Clone(), nil
}

type fakeTunneler struct {
	mu         sync.Mutex
	failures   int // number of upcoming Start calls to fail
	startErr   error
	startCalls int
	stopCalls  int
}

func (t *fakeTunneler) Start(_ *models.ServerProfile, _ models.ConnectionMode) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.startCalls++
	if t.startErr != nil {
		return t.startErr
	}
	if t.failures > 0 {
		t.failures--
		return errors.New("tunnel: connection refused")
	}
	return nil
}

func (t *fakeTunneler) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopCalls++
	return nil
}

func (t *fakeTunneler) counts() (int, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.startCalls, t.stopCalls
}

type collector struct {
	mu     sync.Mutex
	events []api.Event
}

func (c *collector) SendEvent(e api.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *collector) states() []models.ConnectionState {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []models.ConnectionState
	for _, e := range c.events {
		if d, ok := e.Data.(api.StateResponse); ok {
			out = append(out, d.State)
		}
	}
	return out
}

type testPolicy struct {
	delay time.Duration
	max   int
}

func (p testPolicy) Backoff(int) time.Duration { return p.delay }
func (p testPolicy) MaxAttempts() int          { return p.max }

func testServer(id string) *models.ServerProfile {
	return &models.ServerProfile{ID: id, Name: "srv", Address: "10.0.0.1", Port: 443}
}

func newService(t *testing.T, tun Tunneler) (*Service, *collector) {
	t.Helper()
	col := &collector{}
	bus := api.NewEventBus()
	bus.Subscribe(col)
	svc := NewService(
		fakeResolver{profiles: map[string]*models.ServerProfile{"srv-1": testServer("srv-1")}},
		tun,
		log.NewNopLogger(),
		bus,
	)
	return svc, col
}

func waitState(t *testing.T, svc *Service, want models.ConnectionState) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if svc.State() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("state %q not reached; last: %q", want, svc.State())
}

func TestConnectHappyPath(t *testing.T) {
	tun := &fakeTunneler{}
	svc, col := newService(t, tun)
	svc.SetAutoReconnect(false)

	if err := svc.Connect("srv-1", models.ModeVPN); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if got := svc.State(); got != models.StateConnecting {
		t.Fatalf("state after connect = %s", got)
	}
	waitState(t, svc, models.StateConnected)

	sess := svc.Session()
	if sess == nil || sess.ServerID != "srv-1" || sess.Mode != models.ModeVPN {
		t.Fatalf("unexpected session: %+v", sess)
	}
	if !svc.RunningTo("srv-1") || svc.RunningTo("other") {
		t.Fatal("RunningTo returned wrong result")
	}
	last := sess.StatusHistory[len(sess.StatusHistory)-1].State
	if last != models.StateConnected {
		t.Fatalf("last status = %s", last)
	}
	if got := col.states(); got[len(got)-1] != models.StateConnected {
		t.Fatalf("expected connected event, got %v", got)
	}

	if err := svc.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	waitState(t, svc, models.StateDisconnected)
	sess = svc.Session()
	if sess == nil || sess.EndedAt == nil || sess.Running() {
		t.Fatalf("session should be ended: %+v", sess)
	}
	if starts, stops := tun.counts(); starts != 1 || stops != 1 {
		t.Fatalf("expected 1 start and 1 stop, got %d/%d", starts, stops)
	}
}

func TestConnectBusy(t *testing.T) {
	tun := &fakeTunneler{}
	svc, _ := newService(t, tun)
	svc.SetAutoReconnect(false)
	if err := svc.Connect("srv-1", models.ModeVPN); err != nil {
		t.Fatal(err)
	}
	if err := svc.Connect("srv-1", models.ModeVPN); !errors.Is(err, ErrBusy) {
		t.Fatalf("expected ErrBusy, got %v", err)
	}
	_ = svc.Disconnect()
}

func TestConnectUnknownServer(t *testing.T) {
	svc, _ := newService(t, &fakeTunneler{})
	if err := svc.Connect("nope", models.ModeVPN); err == nil {
		t.Fatal("expected resolve error")
	}
	if svc.State() != models.StateDisconnected {
		t.Fatalf("state should stay disconnected, got %s", svc.State())
	}
}

func TestConnectFailsNoRetry(t *testing.T) {
	tun := &fakeTunneler{startErr: errors.New("tunnel: boom")}
	svc, _ := newService(t, tun)
	svc.SetAutoReconnect(false)
	if err := svc.Connect("srv-1", models.ModeProxy); err != nil {
		t.Fatal(err)
	}
	waitState(t, svc, models.StateError)
	sess := svc.Session()
	if sess == nil || sess.Error == nil || sess.Error.Code != api.ErrCodeEngine {
		t.Fatalf("expected session error code %s: %+v", api.ErrCodeEngine, sess)
	}
	if sess.EndedAt == nil {
		t.Fatal("session should be ended")
	}
}

func TestConnectRetriesThenSucceeds(t *testing.T) {
	tun := &fakeTunneler{failures: 2}
	svc, _ := newService(t, tun)
	svc.SetAutoReconnect(true)
	svc.SetRetryPolicy(testPolicy{delay: time.Millisecond, max: 5})

	if err := svc.Connect("srv-1", models.ModeVPN); err != nil {
		t.Fatal(err)
	}
	waitState(t, svc, models.StateConnected)

	sess := svc.Session()
	seen := map[models.ConnectionState]bool{}
	for _, sc := range sess.StatusHistory {
		seen[sc.State] = true
	}
	if !seen[models.StateReconnecting] || !seen[models.StateConnected] {
		t.Fatalf("expected reconnecting and connected in history: %+v", sess.StatusHistory)
	}
	_ = svc.Disconnect()
}

func TestConnectExhaustsRetries(t *testing.T) {
	tun := &fakeTunneler{startErr: errors.New("tunnel: always down")}
	svc, _ := newService(t, tun)
	svc.SetAutoReconnect(true)
	svc.SetRetryPolicy(testPolicy{delay: time.Millisecond, max: 2})

	if err := svc.Connect("srv-1", models.ModeVPN); err != nil {
		t.Fatal(err)
	}
	waitState(t, svc, models.StateError)
	if starts, _ := tun.counts(); starts < 3 {
		t.Fatalf("expected at least 3 start attempts, got %d", starts)
	}
}

func TestReconnectWhileConnected(t *testing.T) {
	tun := &fakeTunneler{}
	svc, _ := newService(t, tun)
	svc.SetAutoReconnect(false)
	if err := svc.Connect("srv-1", models.ModeVPN); err != nil {
		t.Fatal(err)
	}
	waitState(t, svc, models.StateConnected)

	tun.mu.Lock()
	tun.failures = 1
	tun.mu.Unlock()
	svc.SetAutoReconnect(true)
	svc.SetRetryPolicy(testPolicy{delay: time.Millisecond, max: 3})

	if err := svc.Reconnect(); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	// Reconnect stops and re-starts: one failed attempt, then success via
	// auto-reconnect. Poll rather than waitState, since the state returns to
	// Connected before the start counter catches up.
	deadline := time.Now().Add(3 * time.Second)
	for {
		starts, _ := tun.counts()
		if starts >= 3 && svc.State() == models.StateConnected {
			break
		}
		if time.Now().After(deadline) {
			starts, _ := tun.counts()
			t.Fatalf("reconnect did not re-establish: starts=%d state=%s", starts, svc.State())
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestDisconnectIdle(t *testing.T) {
	svc, _ := newService(t, &fakeTunneler{})
	if err := svc.Disconnect(); err != nil {
		t.Fatalf("disconnect while idle: %v", err)
	}
}

func TestDisconnectFromError(t *testing.T) {
	tun := &fakeTunneler{startErr: errors.New("boom")}
	svc, _ := newService(t, tun)
	svc.SetAutoReconnect(false)
	_ = svc.Connect("srv-1", models.ModeVPN)
	waitState(t, svc, models.StateError)
	if err := svc.Disconnect(); err != nil {
		t.Fatalf("disconnect from error: %v", err)
	}
	if svc.State() != models.StateDisconnected {
		t.Fatalf("expected disconnected, got %s", svc.State())
	}
}

type blockingTunneler struct {
	mu         sync.Mutex
	startedA   chan struct{}
	releaseA   chan struct{}
	startCalls int
	stopCalls  int
}

func newBlockingTunneler() *blockingTunneler {
	return &blockingTunneler{startedA: make(chan struct{}), releaseA: make(chan struct{})}
}

func (t *blockingTunneler) Start(_ *models.ServerProfile, _ models.ConnectionMode) error {
	t.mu.Lock()
	t.startCalls++
	first := t.startCalls == 1
	t.mu.Unlock()
	if first {
		close(t.startedA)
		<-t.releaseA
	}
	return nil
}

func (t *blockingTunneler) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopCalls++
	return nil
}

func (t *blockingTunneler) waitFirstStart(tt *testing.T) {
	tt.Helper()
	select {
	case <-t.startedA:
	case <-time.After(3 * time.Second):
		tt.Fatal("first Start never began")
	}
}

func (t *blockingTunneler) counts() (int, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.startCalls, t.stopCalls
}

// stickyTunneler models an engine that "comes up" while a Start is in flight and
// stays up until explicitly stopped: the first Start blocks until released and then
// reports success (the tunnel starts routing traffic), later Starts fail with
// "already running" while the tunnel is up. This simulates a force-disconnected
// session whose engine keeps running, or a helper engine that starts after its
// connect response timed out.
type stickyTunneler struct {
	mu         sync.Mutex
	startedA   chan struct{}
	releaseA   chan struct{}
	up         bool
	startCalls int
	stopCalls  int
}

func newStickyTunneler() *stickyTunneler {
	return &stickyTunneler{startedA: make(chan struct{}), releaseA: make(chan struct{})}
}

func (t *stickyTunneler) Start(_ *models.ServerProfile, _ models.ConnectionMode) error {
	t.mu.Lock()
	t.startCalls++
	first := t.startCalls == 1
	up := t.up
	t.mu.Unlock()
	if first {
		close(t.startedA)
		<-t.releaseA
		// The engine comes up and keeps routing traffic — nothing stopped it.
		t.mu.Lock()
		t.up = true
		t.mu.Unlock()
		return nil
	}
	if up {
		return errors.New("tunnel: already running")
	}
	return nil
}

func (t *stickyTunneler) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopCalls++
	t.up = false
	return nil
}

func (t *stickyTunneler) counts() (int, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.startCalls, t.stopCalls
}

func (t *stickyTunneler) waitUp(tt *testing.T) {
	tt.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		t.mu.Lock()
		up := t.up
		t.mu.Unlock()
		if up {
			return
		}
		if time.Now().After(deadline) {
			tt.Fatal("orphaned tunnel never came up")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestNoFalseErrorWhenTunnelLeftRunning reproduces the bug where the UI reports
// Disconnected/Error while the tunnel is actually up and passing traffic: a
// disconnect during a slow connect leaves the engine running, and the next
// connect then fails with "already running" on every retry until it gives up in
// Error. The service must stop the leftover tunnel before retrying, so the
// retried start succeeds and the session lands in Connected.
func TestNoFalseErrorWhenTunnelLeftRunning(t *testing.T) {
	old := disconnectTimeout
	disconnectTimeout = 40 * time.Millisecond
	defer func() { disconnectTimeout = old }()

	tun := newStickyTunneler()
	svc, _ := newService(t, tun)
	svc.SetAutoReconnect(true)
	svc.SetRetryPolicy(testPolicy{delay: time.Millisecond, max: 3})

	if err := svc.Connect("srv-1", models.ModeVPN); err != nil {
		t.Fatalf("connect A: %v", err)
	}
	select {
	case <-tun.startedA:
	case <-time.After(3 * time.Second):
		t.Fatal("first Start never began")
	}

	// Disconnect while the connect is still in flight forces a disconnect;
	// the in-flight Start completes afterwards and leaves the tunnel up.
	if err := svc.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if got := svc.State(); got != models.StateDisconnected {
		t.Fatalf("state after disconnect = %s, want %s", got, models.StateDisconnected)
	}
	close(tun.releaseA)
	tun.waitUp(t)
	waitState(t, svc, models.StateDisconnected)

	// Reconnect: the first attempt hits "already running" from the orphaned
	// tunnel. With the fix the service stops it and retries cleanly; without
	// it the retries all fail and the service parks in a false Error.
	if err := svc.Connect("srv-1", models.ModeVPN); err != nil {
		t.Fatalf("connect B: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if svc.State() == models.StateConnected {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if got := svc.State(); got != models.StateConnected {
		t.Fatalf("state after reconnect = %s, want %s (false Error while tunnel was up)", got, models.StateConnected)
	}

	starts, stops := tun.counts()
	if starts < 3 {
		t.Fatalf("expected the retried start to run, got starts=%d", starts)
	}
	if stops < 2 {
		t.Fatalf("expected the leftover tunnel to be stopped, got stops=%d", stops)
	}
	_ = svc.Disconnect()
	waitState(t, svc, models.StateDisconnected)
}

// TestConnectAfterForceDisconnect races a connect against a loop still blocked
// inside Start. The stale loop must neither clobber the new session's state nor
// stop its tunnel.
func TestConnectAfterForceDisconnect(t *testing.T) {
	old := disconnectTimeout
	disconnectTimeout = 40 * time.Millisecond
	defer func() { disconnectTimeout = old }()

	tun := newBlockingTunneler()
	svc, _ := newService(t, tun)
	svc.SetAutoReconnect(false)

	if err := svc.Connect("srv-1", models.ModeVPN); err != nil {
		t.Fatalf("connect A: %v", err)
	}
	tun.waitFirstStart(t)

	if err := svc.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if got := svc.State(); got != models.StateDisconnected {
		t.Fatalf("state after disconnect = %s, want %s", got, models.StateDisconnected)
	}

	if err := svc.Connect("srv-1", models.ModeVPN); err != nil {
		t.Fatalf("connect B: %v", err)
	}
	close(tun.releaseA)

	waitState(t, svc, models.StateConnected)
	if sess := svc.Session(); sess == nil || sess.ServerID != "srv-1" {
		t.Fatalf("session after reconnect: %+v", sess)
	}
	starts, stops := tun.counts()
	if starts < 2 {
		t.Fatalf("expected B to start the tunnel, got starts=%d", starts)
	}
	if stops != 1 {
		t.Fatalf("expected exactly one Stop (from force-disconnect), got %d", stops)
	}
	_ = svc.Disconnect()
	waitState(t, svc, models.StateDisconnected)
}
