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

func newService(t *testing.T, tun *fakeTunneler) (*Service, *collector) {
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
