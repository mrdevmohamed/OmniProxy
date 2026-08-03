package engine

import (
	"context"
	"net"
	"path/filepath"
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json"
)

func testContext() context.Context {
	return newContext(context.Background())
}

func TestBuildProxyConfig(t *testing.T) {
	built, err := buildOptions(Options{
		Mode:     ModeProxy,
		LogLevel: LevelInfo,
		Outbound: Outbound{
			Protocol: ProtocolSOCKS,
			Address:  "203.0.113.10",
			Port:     1080,
			Username: "user",
			Password: "pass",
		},
		Proxy: ProxyOptions{Listen: "127.0.0.1", Port: 9080},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Inbounds) != 1 {
		t.Fatalf("want 1 inbound, got %d", len(built.Inbounds))
	}
	inb := built.Inbounds[0]
	if inb.Type != C.TypeMixed {
		t.Fatalf("want mixed inbound, got %q", inb.Type)
	}
	mixed, ok := inb.Options.(*option.HTTPMixedInboundOptions)
	if !ok {
		t.Fatalf("unexpected options type %T", inb.Options)
	}
	if mixed.ListenPort != 9080 {
		t.Fatalf("want listen port 9080, got %d", mixed.ListenPort)
	}
	if built.Route == nil || built.Route.Final != "proxy" {
		t.Fatalf("route final should be proxy, got %+v", built.Route)
	}
	if len(built.Outbounds) != 2 {
		t.Fatalf("want direct+proxy outbounds, got %d", len(built.Outbounds))
	}
}

func TestBuildVPNConfig(t *testing.T) {
	built, err := buildOptions(Options{
		Mode:     ModeVPN,
		LogLevel: LevelInfo,
		Outbound: Outbound{
			Protocol: ProtocolVLESS,
			Address:  "203.0.113.10",
			Port:     443,
			UUID:     "11111111-2222-3333-4444-555555555555",
			Flow:     "xtls-rprx-vision",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	inb := built.Inbounds[0]
	if inb.Type != C.TypeTun {
		t.Fatalf("want tun inbound, got %q", inb.Type)
	}
	tunOpts, ok := inb.Options.(*option.TunInboundOptions)
	if !ok {
		t.Fatalf("unexpected options type %T", inb.Options)
	}
	if tunOpts.InterfaceName != "omniproxy" {
		t.Fatalf("unexpected tun name %q", tunOpts.InterfaceName)
	}
	if tunOpts.MTU != 1500 {
		t.Fatalf("unexpected mtu %d", tunOpts.MTU)
	}
	if !tunOpts.AutoRoute {
		t.Fatal("AutoRoute should default true")
	}
	if len(tunOpts.Address) != 2 {
		t.Fatalf("want default v4+v6 addresses, got %v", tunOpts.Address)
	}
	if built.DNS == nil {
		t.Fatal("VPN mode should configure a DNS module")
	}
	if len(built.DNS.Servers) != 2 {
		t.Fatalf("want 2 dns servers, got %d", len(built.DNS.Servers))
	}
	if built.DNS.Final != "dns-proxy" {
		t.Fatalf("want final dns server dns-proxy, got %q", built.DNS.Final)
	}
	if !built.DNS.ReverseMapping {
		t.Fatal("dns reverse_mapping should be enabled in VPN mode")
	}
	if built.Route == nil || len(built.Route.Rules) == 0 {
		t.Fatal("VPN mode should route DNS through the router")
	}
	if built.Route.Final != "proxy" {
		t.Fatalf("want route final proxy, got %q", built.Route.Final)
	}
}

func TestBuildVPNConfigMarshalsDNS(t *testing.T) {
	built, err := buildOptions(Options{
		Mode:     ModeVPN,
		LogLevel: LevelInfo,
		Outbound: Outbound{
			Protocol: ProtocolVLESS,
			Address:  "203.0.113.10",
			Port:     443,
			UUID:     "11111111-2222-3333-4444-555555555555",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := testContext()
	data, err := json.MarshalContext(ctx, built)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := json.UnmarshalExtendedContext[option.Options](ctx, data); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, data)
	}
}

func TestBuildProxyConfigHasNoDNS(t *testing.T) {
	built, err := buildOptions(Options{
		Mode:     ModeProxy,
		LogLevel: LevelInfo,
		Outbound: Outbound{
			Protocol: ProtocolSOCKS,
			Address:  "203.0.113.10",
			Port:     1080,
		},
		Proxy: ProxyOptions{Listen: "127.0.0.1", Port: 9080},
	})
	if err != nil {
		t.Fatal(err)
	}
	if built.DNS != nil {
		t.Fatalf("proxy mode should not configure a DNS module, got %+v", built.DNS)
	}
	if built.Route != nil && len(built.Route.Rules) != 0 {
		t.Fatalf("proxy mode should not add dns hijack rules, got %v", built.Route.Rules)
	}
}

func TestBuildTLS(t *testing.T) {
	built, err := buildOptions(Options{
		Mode:     ModeVPN,
		LogLevel: LevelDebug,
		Outbound: Outbound{
			Protocol: ProtocolVLESS,
			Address:  "203.0.113.10",
			Port:     443,
			UUID:     "11111111-2222-3333-4444-555555555555",
			TLS: &TLSSettings{
				ServerName: "edge.example.com",
				Insecure:   true,
				ALPN:       []string{"h2", "http/1.1"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ob := built.Outbounds[1]
	opts := ob.Options.(*option.VLESSOutboundOptions)
	if opts.TLS == nil || !opts.TLS.Enabled {
		t.Fatal("tls should be enabled")
	}
	if opts.TLS.ServerName != "edge.example.com" {
		t.Fatalf("unexpected server name %q", opts.TLS.ServerName)
	}
	if !opts.TLS.Insecure {
		t.Fatal("insecure flag lost")
	}
	if len(opts.TLS.ALPN) != 2 {
		t.Fatalf("unexpected alpn %v", opts.TLS.ALPN)
	}
}

func TestBuildWebSocketTransport(t *testing.T) {
	built, err := buildOptions(Options{
		Mode:     ModeProxy,
		LogLevel: LevelInfo,
		Outbound: Outbound{
			Protocol:       ProtocolVMess,
			Address:        "203.0.113.10",
			Port:           443,
			UUID:           "11111111-2222-3333-4444-555555555555",
			Security:       "auto",
			GlobalPadding:  true,
			PacketEncoding: "xudp",
			TLS: &TLSSettings{
				ServerName:  "www.example.com",
				Fingerprint: "chrome",
			},
			Transport: &TransportSettings{
				Type: TransportWS,
				Path: "/vpnjantit",
				Host: "www.nagwa.com.dpdns.org",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ob := built.Outbounds[1]
	if ob.Type != C.TypeVMess {
		t.Fatalf("want vmess outbound, got %q", ob.Type)
	}
	opts := ob.Options.(*option.VMessOutboundOptions)
	if opts.Transport == nil {
		t.Fatal("expected ws transport")
	}
	if opts.Transport.Type != C.V2RayTransportTypeWebsocket {
		t.Fatalf("want websocket transport, got %q", opts.Transport.Type)
	}
	ws := opts.Transport.WebsocketOptions
	if ws.Path != "/vpnjantit" {
		t.Fatalf("unexpected path %q", ws.Path)
	}
	hosts, ok := ws.Headers["Host"]
	if !ok || len(hosts) != 1 || hosts[0] != "www.nagwa.com.dpdns.org" {
		t.Fatalf("unexpected ws host header %v", ws.Headers)
	}
	if !opts.GlobalPadding {
		t.Fatal("global padding lost")
	}
	if opts.PacketEncoding != "xudp" {
		t.Fatalf("unexpected packet encoding %q", opts.PacketEncoding)
	}
	if opts.TLS == nil || opts.TLS.UTLS == nil || opts.TLS.UTLS.Fingerprint != "chrome" {
		t.Fatalf("expected utls fingerprint chrome, got %+v", opts.TLS)
	}
}

func TestBuildWebSocketTransportDisabled(t *testing.T) {
	built, err := buildOptions(Options{
		Mode:     ModeProxy,
		LogLevel: LevelInfo,
		Outbound: Outbound{
			Protocol: ProtocolVLESS,
			Address:  "203.0.113.10",
			Port:     443,
			UUID:     "11111111-2222-3333-4444-555555555555",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	opts := built.Outbounds[1].Options.(*option.VLESSOutboundOptions)
	if opts.Transport != nil {
		t.Fatalf("expected no transport, got %+v", opts.Transport)
	}
}

func TestBuildTrojanOutbound(t *testing.T) {
	built, err := buildOptions(Options{
		Mode:     ModeProxy,
		LogLevel: LevelInfo,
		Outbound: Outbound{
			Protocol: ProtocolTrojan,
			Address:  "203.0.113.10",
			Port:     443,
			Password: "trojan-pass",
			TLS:      &TLSSettings{ServerName: "edge.example.com"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ob := built.Outbounds[1]
	if ob.Type != C.TypeTrojan {
		t.Fatalf("want trojan outbound, got %q", ob.Type)
	}
	opts := ob.Options.(*option.TrojanOutboundOptions)
	if opts.Password != "trojan-pass" {
		t.Fatalf("unexpected password %q", opts.Password)
	}
}

func TestConfigJSONRoundTrip(t *testing.T) {
	built, err := buildOptions(Options{
		Mode:     ModeProxy,
		LogLevel: LevelInfo,
		Outbound: Outbound{
			Protocol: ProtocolShadowsocks,
			Address:  "203.0.113.10",
			Port:     8388,
			Cipher:   "aes-128-gcm",
			Password: "secret",
		},
		Proxy: ProxyOptions{Listen: "127.0.0.1", Port: 9080},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := testContext()
	data, err := json.MarshalContext(ctx, built)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	_, err = json.UnmarshalExtendedContext[option.Options](ctx, data)
	if err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, data)
	}
}

func TestValidateRejectsIncompleteOutbound(t *testing.T) {
	_, err := buildOptions(Options{Mode: ModeProxy, Outbound: Outbound{Address: "1.2.3.4"}})
	if err == nil {
		t.Fatal("want validation error for missing port")
	}
}

func TestEngineStartStopProxy(t *testing.T) {
	port, err := freePort()
	if err != nil {
		t.Fatal(err)
	}
	eng := New(nil)
	defer eng.Close()
	if err := eng.Start(Options{
		Mode:          ModeProxy,
		LogLevel:      LevelError,
		CacheFilePath: filepath.Join(t.TempDir(), "cache.db"),
		Outbound: Outbound{
			Protocol: ProtocolSOCKS,
			Address:  "127.0.0.1",
			Port:     9,
		},
		Proxy: ProxyOptions{Listen: "127.0.0.1", Port: port},
	}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !eng.Running() {
		t.Fatal("engine should be running")
	}
	if err := eng.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if eng.Running() {
		t.Fatal("engine should be stopped")
	}
}

func TestEngineStartRejectsDoubleRun(t *testing.T) {
	port, err := freePort()
	if err != nil {
		t.Fatal(err)
	}
	eng := New(nil)
	defer eng.Close()
	opts := Options{
		Mode:          ModeProxy,
		LogLevel:      LevelError,
		CacheFilePath: filepath.Join(t.TempDir(), "cache.db"),
		Outbound:      Outbound{Protocol: ProtocolSOCKS, Address: "127.0.0.1", Port: 9},
		Proxy:         ProxyOptions{Listen: "127.0.0.1", Port: port},
	}
	if err := eng.Start(opts); err != nil {
		t.Fatalf("first start: %v", err)
	}
	if err := eng.Start(opts); err == nil {
		t.Fatal("second start should fail")
	}
}

func freePort() (uint16, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return uint16(l.Addr().(*net.TCPAddr).Port), nil
}
