package tunnel

import (
	"context"
	"errors"
	"net"
	"strconv"
	"testing"
	"time"

	"omniproxy/core/log"
	"omniproxy/core/models"
	"omniproxy/engine"
)

type fakeRunner struct {
	startErr error
	started  bool
	starts   []engine.Options
	stops    int
}

func (r *fakeRunner) Start(opts engine.Options) error {
	if r.startErr != nil {
		return r.startErr
	}
	r.started = true
	r.starts = append(r.starts, opts)
	return nil
}

func (r *fakeRunner) Stop() error {
	r.started = false
	r.stops++
	return nil
}

func (r *fakeRunner) Running() bool { return r.started }

func testProfile() *models.ServerProfile {
	return &models.ServerProfile{
		ID:       "srv-1",
		Name:     "test",
		Protocol: models.ProtocolVLESS,
		Address:  "10.0.0.1",
		Port:     443,
		UUID:     "11111111-1111-4111-8111-111111111111",
		TLS:      models.TLSConfig{Enabled: true, ServerName: "example.com"},
	}
}

func TestBuildEngineOptionsModes(t *testing.T) {
	p := testProfile()

	vpn := BuildEngineOptions(p, models.ModeVPN, models.IPv6ModePreferIPv4, engine.LevelInfo, "")
	if vpn.Mode != engine.ModeVPN {
		t.Fatalf("expected vpn mode, got %s", vpn.Mode)
	}
	if vpn.Outbound.Protocol != engine.ProtocolVLESS {
		t.Fatalf("expected vless, got %s", vpn.Outbound.Protocol)
	}
	if vpn.Outbound.TLS == nil || vpn.Outbound.TLS.Insecure || vpn.Outbound.TLS.ServerName != "example.com" {
		t.Fatalf("unexpected tls: %+v", vpn.Outbound.TLS)
	}
	if vpn.CacheFilePath != "" {
		t.Fatalf("cache file should be empty, got %q", vpn.CacheFilePath)
	}

	proxy := BuildEngineOptions(p, models.ModeProxy, models.IPv6ModeDisable, engine.LevelDebug, "/tmp/cache.db")
	if proxy.Mode != engine.ModeProxy {
		t.Fatalf("expected proxy mode, got %s", proxy.Mode)
	}
	if proxy.LogLevel != engine.LevelDebug {
		t.Fatalf("expected debug level, got %d", proxy.LogLevel)
	}
	if proxy.CacheFilePath != "/tmp/cache.db" {
		t.Fatalf("unexpected cache file %q", proxy.CacheFilePath)
	}
}

func TestBuildEngineOptionsProtocols(t *testing.T) {
	cases := []struct {
		proto  models.Protocol
		want   engine.Protocol
		mutate func(p *models.ServerProfile)
		check  func(ob engine.Outbound) bool
	}{
		{proto: models.ProtocolSOCKS5, want: engine.ProtocolSOCKS, mutate: func(p *models.ServerProfile) {
			p.Username = "user"
			p.Password = "pass"
		}, check: func(ob engine.Outbound) bool { return ob.Username == "user" && ob.Password == "pass" }},
		{proto: models.ProtocolHTTP, want: engine.ProtocolHTTP, mutate: func(p *models.ServerProfile) {
			p.Username = "user"
		}, check: func(ob engine.Outbound) bool { return ob.Username == "user" }},
		{proto: models.ProtocolShadowsocks, want: engine.ProtocolShadowsocks, mutate: func(p *models.ServerProfile) {
			p.Cipher = "aes-128-gcm"
			p.Password = "pass"
		}, check: func(ob engine.Outbound) bool { return ob.Cipher == "aes-128-gcm" && ob.Password == "pass" }},
		{proto: models.ProtocolVMess, want: engine.ProtocolVMess, check: func(ob engine.Outbound) bool { return ob.UUID != "" }},
		{proto: models.ProtocolSSH, want: engine.ProtocolSSH, mutate: func(p *models.ServerProfile) {
			p.SSH = models.SSHConfig{User: "root", PrivateKey: "PEM"}
			p.Password = "secret"
		}, check: func(ob engine.Outbound) bool {
			return ob.Username == "root" && ob.SSH != nil && ob.SSH.PrivateKey == "PEM"
		}},
	}
	for _, c := range cases {
		p := testProfile()
		p.Protocol = c.proto
		if c.mutate != nil {
			c.mutate(p)
		}
		ob := BuildEngineOptions(p, models.ModeProxy, models.IPv6ModePreferIPv4, engine.LevelInfo, "").Outbound
		if ob.Protocol != c.want {
			t.Errorf("%s: want protocol %s, got %s", c.proto, c.want, ob.Protocol)
		}
		if !c.check(ob) {
			t.Errorf("%s: field mapping failed: %+v", c.proto, ob)
		}
	}
}

func TestBuildEngineOptionsTLSDisabled(t *testing.T) {
	p := testProfile()
	p.TLS.Enabled = false
	ob := BuildEngineOptions(p, models.ModeProxy, models.IPv6ModePreferIPv4, engine.LevelInfo, "").Outbound
	if ob.TLS != nil {
		t.Fatalf("tls should be nil when disabled, got %+v", ob.TLS)
	}
}

func TestBuildEngineOptionsIPv6Mode(t *testing.T) {
	p := testProfile()
	cases := []struct {
		in   models.IPv6Mode
		want engine.IPv6Mode
	}{
		{models.IPv6ModeAuto, engine.IPv6ModeAuto},
		{models.IPv6ModePreferIPv4, engine.IPv6ModePreferIPv4},
		{models.IPv6ModeDisable, engine.IPv6ModeDisable},
		{models.IPv6ModeEnable, engine.IPv6ModeEnable},
		{"bogus", engine.IPv6ModePreferIPv4},
		{"", engine.IPv6ModePreferIPv4},
	}
	for _, c := range cases {
		opts := BuildEngineOptions(p, models.ModeVPN, c.in, engine.LevelInfo, "")
		if opts.IPv6Mode != c.want {
			t.Errorf("ipv6 mode %q: want %s, got %s", c.in, c.want, opts.IPv6Mode)
		}
	}
}

func TestManagerSetIPv6ModeAppliesToStart(t *testing.T) {
	runner := &fakeRunner{}
	m := NewManager(runner, log.NewNopLogger())
	if err := m.Start(testProfile(), models.ModeVPN); err != nil {
		t.Fatal(err)
	}
	if got := runner.starts[0].IPv6Mode; got != engine.IPv6ModePreferIPv4 {
		t.Fatalf("default ipv6 mode should be prefer_ipv4, got %s", got)
	}
	m.Stop()

	m.SetIPv6Mode(models.IPv6ModeDisable)
	m.SetIPv6Mode("bogus") // invalid values fall back to prefer_ipv4
	if err := m.Start(testProfile(), models.ModeVPN); err != nil {
		t.Fatal(err)
	}
	if got := runner.starts[1].IPv6Mode; got != engine.IPv6ModePreferIPv4 {
		t.Fatalf("invalid ipv6 mode should fall back to prefer_ipv4, got %s", got)
	}
	m.Stop()

	m.SetIPv6Mode(models.IPv6ModeDisable)
	if err := m.Start(testProfile(), models.ModeVPN); err != nil {
		t.Fatal(err)
	}
	if got := runner.starts[2].IPv6Mode; got != engine.IPv6ModeDisable {
		t.Fatalf("want disable ipv6 mode, got %s", got)
	}
}

func TestManagerStartStop(t *testing.T) {
	runner := &fakeRunner{}
	m := NewManager(runner, log.NewNopLogger())
	p := testProfile()

	if m.Active() {
		t.Fatal("manager should start idle")
	}
	if err := m.Start(p, models.ModeVPN); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !m.Active() {
		t.Fatal("manager should be active after start")
	}
	if got := m.Mode(); got != models.ModeVPN {
		t.Fatalf("mode = %s", got)
	}
	if srv := m.Server(); srv == nil || srv.ID != p.ID {
		t.Fatalf("unexpected active server %+v", srv)
	}
	if len(runner.starts) != 1 {
		t.Fatalf("expected 1 start, got %d", len(runner.starts))
	}

	if err := m.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if m.Active() {
		t.Fatal("manager should be idle after stop")
	}
	if m.Server() != nil || m.Mode() != "" {
		t.Fatal("server/mode should be cleared after stop")
	}
}

func TestManagerStartRejectsDoubleRun(t *testing.T) {
	runner := &fakeRunner{}
	m := NewManager(runner, log.NewNopLogger())
	if err := m.Start(testProfile(), models.ModeVPN); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(testProfile(), models.ModeVPN); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("expected ErrAlreadyRunning, got %v", err)
	}
}

func TestManagerStopIdempotent(t *testing.T) {
	m := NewManager(&fakeRunner{}, log.NewNopLogger())
	if err := m.Stop(); err != nil {
		t.Fatalf("stop when idle should be a no-op, got %v", err)
	}
}

func TestManagerStartPropagatesError(t *testing.T) {
	runner := &fakeRunner{startErr: errors.New("engine: boom")}
	m := NewManager(runner, log.NewNopLogger())
	if err := m.Start(testProfile(), models.ModeVPN); err == nil {
		t.Fatal("expected start error")
	}
	if m.Active() {
		t.Fatal("manager should remain idle after failed start")
	}
}

func TestManagerClonesProfile(t *testing.T) {
	runner := &fakeRunner{}
	m := NewManager(runner, log.NewNopLogger())
	p := testProfile()
	if err := m.Start(p, models.ModeVPN); err != nil {
		t.Fatal(err)
	}
	p.Name = "mutated"
	if got := m.Server(); got.Name == "mutated" {
		t.Fatal("manager must not retain caller's profile")
	}
}

func TestLatencyTester(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	tester := NewLatencyTester(time.Second, log.NewNopLogger())
	ms, err := tester.TestLatency(context.Background(), &models.ServerProfile{Address: host, Port: port})
	if err != nil {
		t.Fatalf("latency: %v", err)
	}
	if ms < 1 {
		t.Fatalf("expected positive latency, got %d", ms)
	}
}

func TestLatencyTesterUnreachable(t *testing.T) {
	tester := NewLatencyTester(200*time.Millisecond, log.NewNopLogger())
	_, err := tester.TestLatency(context.Background(), &models.ServerProfile{Address: "127.0.0.1", Port: 1})
	if err == nil {
		t.Fatal("expected error for unreachable endpoint")
	}
}
