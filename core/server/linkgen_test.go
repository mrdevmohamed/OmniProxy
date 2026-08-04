package server

import (
	"reflect"
	"strings"
	"testing"

	"omniproxy/core/models"
)

func wsTransport(path, host string) *models.TransportConfig {
	return &models.TransportConfig{Type: models.TransportWS, Path: path, Host: host}
}

func roundTripProfile(t *testing.T, p *models.ServerProfile) {
	t.Helper()
	link, err := Link(p)
	if err != nil {
		t.Fatalf("%s: Link: %v", p.Protocol, err)
	}
	got, err := ParseLink(link)
	if err != nil {
		t.Fatalf("%s: parse %q: %v", p.Protocol, link, err)
	}

	if got.Protocol != p.Protocol || got.Address != p.Address || got.Port != p.Port {
		t.Fatalf("%s: addr/port/proto mismatch: got %s %s:%d want %s %s:%d (link %q)",
			p.Protocol, got.Protocol, got.Address, got.Port, p.Protocol, p.Address, p.Port, link)
	}
	if got.UUID != p.UUID {
		t.Fatalf("%s: uuid mismatch: %q vs %q", p.Protocol, got.UUID, p.UUID)
	}
	if got.Password != p.Password {
		t.Fatalf("%s: password mismatch: %q vs %q", p.Protocol, got.Password, p.Password)
	}
	if got.Cipher != p.Cipher {
		t.Fatalf("%s: cipher mismatch: %q vs %q", p.Protocol, got.Cipher, p.Cipher)
	}
	if got.Username != p.Username {
		t.Fatalf("%s: username mismatch: %q vs %q", p.Protocol, got.Username, p.Username)
	}
	if got.Security != p.Security {
		t.Fatalf("%s: security mismatch: %q vs %q", p.Protocol, got.Security, p.Security)
	}
	if got.Flow != p.Flow {
		t.Fatalf("%s: flow mismatch: %q vs %q", p.Protocol, got.Flow, p.Flow)
	}
	if got.PacketEncoding != p.PacketEncoding {
		t.Fatalf("%s: packetEncoding mismatch: %q vs %q", p.Protocol, got.PacketEncoding, p.PacketEncoding)
	}
	if got.GlobalPadding != p.GlobalPadding || got.AuthenticatedLength != p.AuthenticatedLength {
		t.Fatalf("%s: wire options mismatch", p.Protocol)
	}
	if got.TLS.Enabled != p.TLS.Enabled || got.TLS.AllowInsecure != p.TLS.AllowInsecure {
		t.Fatalf("%s: tls flags mismatch: %+v vs %+v", p.Protocol, got.TLS, p.TLS)
	}
	if p.TLS.Enabled && got.TLS.ServerName != p.TLS.ServerName {
		t.Fatalf("%s: sni mismatch: %q vs %q", p.Protocol, got.TLS.ServerName, p.TLS.ServerName)
	}
	if !reflect.DeepEqual(got.TLS.ALPN, p.TLS.ALPN) {
		t.Fatalf("%s: alpn mismatch: %v vs %v", p.Protocol, got.TLS.ALPN, p.TLS.ALPN)
	}
	if got.TLS.Fingerprint != p.TLS.Fingerprint {
		t.Fatalf("%s: fp mismatch: %q vs %q", p.Protocol, got.TLS.Fingerprint, p.TLS.Fingerprint)
	}
	if !transportEqual(got.Transport, p.Transport) {
		t.Fatalf("%s: transport mismatch: %+v vs %+v", p.Protocol, got.Transport, p.Transport)
	}
	if got.Name != p.Name {
		t.Fatalf("%s: name mismatch: %q vs %q", p.Protocol, got.Name, p.Name)
	}
}

func transportEqual(a, b *models.TransportConfig) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func TestLinkRoundTrip(t *testing.T) {
	cases := []*models.ServerProfile{
		{
			Name: "Nagwa VLESS WS", Protocol: models.ProtocolVLESS,
			Address: "www.nagwa.com", Port: 443, UUID: "11111111-2222-3333-4444-555555555555",
			Flow: "xtls-rprx-vision", PacketEncoding: "xudp",
			TLS:       models.TLSConfig{Enabled: true, ServerName: "www.nagwa.com.edge.dpdns.org", Fingerprint: "chrome", ALPN: []string{"h2", "http/1.1"}},
			Transport: wsTransport("/vpnjantit", "www.nagwa.com.edge.dpdns.org"),
		},
		{
			Name: "Plain", Protocol: models.ProtocolVLESS,
			Address: "cp.example.com", Port: 80, UUID: "11111111-2222-3333-4444-555555555555",
		},
		{
			Name: "VMess WS", Protocol: models.ProtocolVMess,
			Address: "www.nagwa.com", Port: 443, UUID: "11111111-2222-3333-4444-555555555555",
			Security: "auto", GlobalPadding: true, PacketEncoding: "xudp",
			TLS:       models.TLSConfig{Enabled: true, ServerName: "www.nagwa.com", Fingerprint: "chrome"},
			Transport: wsTransport("/ws", "cdn.example.com"),
		},
		{
			Name: "SS", Protocol: models.ProtocolShadowsocks,
			Address: "cp.example.com", Port: 8388, Cipher: "aes-128-gcm", Password: "sec ret : x",
		},
		{
			Name: "Trojan WS", Protocol: models.ProtocolTrojan,
			Address: "cp.example.com", Port: 443, Password: "trojan:pass@1",
			TLS:       models.TLSConfig{Enabled: true, ServerName: "cp.example.com", AllowInsecure: true},
			Transport: wsTransport("/ws", "cp.example.com"),
		},
		{
			Name: "Socks", Protocol: models.ProtocolSOCKS5,
			Address: "cp.example.com", Port: 1080, Username: "user", Password: "pass",
		},
		{
			Name: "HTTP", Protocol: models.ProtocolHTTP,
			Address: "cp.example.com", Port: 8080, Username: "user", Password: "p a:ss",
		},
	}
	for _, p := range cases {
		roundTripProfile(t, p)
	}
}

func TestLinkUnsupportedProtocols(t *testing.T) {
	if _, err := Link(&models.ServerProfile{Protocol: models.ProtocolSSH, Address: "h", Port: 22, SSH: models.SSHConfig{User: "u"}}); err == nil {
		t.Fatal("ssh must not be exported as a share link")
	}
}

func TestExportLinks(t *testing.T) {
	m := newTestManager(t)
	id, _ := m.AddServer(&models.ServerProfile{
		Name: "A", Protocol: models.ProtocolVLESS, Address: "a.example.com", Port: 443,
		UUID: "11111111-2222-3333-4444-555555555555", Enabled: true,
	})
	links, err := m.ExportLinks([]string{id})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(links, "vless://") {
		t.Fatalf("expected vless link, got %q", links)
	}
	// The exported link re-imports into a fresh manager.
	m2 := newTestManager(t)
	added, failed, errs, err := m2.ImportServers(links)
	if err != nil || failed != 0 || added != 1 {
		t.Fatalf("reimport: added=%d failed=%d errs=%v err=%v", added, failed, errs, err)
	}
}

func TestExportLinksRejectsSSH(t *testing.T) {
	m := newTestManager(t)
	_, _ = m.AddServer(&models.ServerProfile{
		Name: "S", Protocol: models.ProtocolSSH, Address: "h", Port: 22,
		SSH: models.SSHConfig{User: "u", PrivateKey: "k"}, Enabled: true,
	})
	if _, err := m.ExportLinks(nil); err == nil {
		t.Fatal("expected error exporting ssh as links")
	}
}
