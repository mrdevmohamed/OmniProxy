package server

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"omniproxy/core/models"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func vmessLink(t *testing.T, doc map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return "vmess://" + base64.RawURLEncoding.EncodeToString(raw)
}

func TestParseVMessWebSocket(t *testing.T) {
	link := vmessLink(t, map[string]any{
		"v": "2", "ps": "Nagwa WS", "add": "www.nagwa.com", "port": 443,
		"id": "11111111-2222-3333-4444-555555555555", "aid": "0",
		"net": "ws", "type": "none",
		"host": "www.nagwa.com.edge.dpdns.org", "path": "/vpnjantit",
		"tls": "tls", "sni": "www.nagwa.com.edge.dpdns.org",
		"fp": "chrome", "alpn": "h2,http/1.1", "scy": "auto",
		"global_padding": true, "packetEncoding": "xudp",
	})
	p, err := ParseLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if p.Protocol != models.ProtocolVMess {
		t.Fatalf("protocol = %s", p.Protocol)
	}
	if p.Address != "www.nagwa.com" || p.Port != 443 {
		t.Fatalf("address/port = %s:%d", p.Address, p.Port)
	}
	if p.UUID != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("uuid = %q", p.UUID)
	}
	if p.Security != "auto" {
		t.Fatalf("security = %q", p.Security)
	}
	if p.GlobalPadding != true || p.PacketEncoding != "xudp" {
		t.Fatalf("padding=%v packet=%q", p.GlobalPadding, p.PacketEncoding)
	}
	if p.Transport == nil || p.Transport.Type != models.TransportWS {
		t.Fatalf("transport = %+v", p.Transport)
	}
	if p.Transport.Path != "/vpnjantit" || p.Transport.Host != "www.nagwa.com.edge.dpdns.org" {
		t.Fatalf("ws path/host = %q %q", p.Transport.Path, p.Transport.Host)
	}
	if !p.TLS.Enabled || p.TLS.ServerName != "www.nagwa.com.edge.dpdns.org" {
		t.Fatalf("tls = %+v", p.TLS)
	}
	if p.TLS.Fingerprint != "chrome" {
		t.Fatalf("fingerprint = %q", p.TLS.Fingerprint)
	}
	if len(p.TLS.ALPN) != 2 {
		t.Fatalf("alpn = %v", p.TLS.ALPN)
	}
	if p.Name != "Nagwa WS" {
		t.Fatalf("name = %q", p.Name)
	}
}

func TestParseVMessNoTLS(t *testing.T) {
	link := vmessLink(t, map[string]any{
		"v": "2", "ps": "Plain", "add": "cp.example.com", "port": 80,
		"id": "11111111-2222-3333-4444-555555555555", "net": "ws",
		"host": "cp.example.com", "path": "/ws", "tls": "", "scy": "none",
	})
	p, err := ParseLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if p.TLS.Enabled {
		t.Fatalf("tls should be disabled, got %+v", p.TLS)
	}
	if p.Security != "none" {
		t.Fatalf("security = %q", p.Security)
	}
}

func TestParseVLessWebSocket(t *testing.T) {
	link := "vless://11111111-2222-3333-4444-555555555555@www.nagwa.com:443" +
		"?type=ws&security=tls&path=%2Fvpnjantit&host=www.nagwa.com.edge.dpdns.org" +
		"&sni=www.nagwa.com.edge.dpdns.org&fp=chrome&alpn=h2,http%2F1.1" +
		"&flow=xtls-rprx-vision&packetEncoding=xudp#Nagwa%20VLESS"
	p, err := ParseLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if p.Protocol != models.ProtocolVLESS || p.UUID == "" {
		t.Fatalf("vless uuid/protocol: %s %q", p.Protocol, p.UUID)
	}
	if p.Flow != "xtls-rprx-vision" {
		t.Fatalf("flow = %q", p.Flow)
	}
	if p.PacketEncoding != "xudp" {
		t.Fatalf("packetEncoding = %q", p.PacketEncoding)
	}
	if p.Transport == nil || p.Transport.Path != "/vpnjantit" {
		t.Fatalf("transport = %+v", p.Transport)
	}
	if p.TLS.ServerName != "www.nagwa.com.edge.dpdns.org" {
		t.Fatalf("tls servername = %q", p.TLS.ServerName)
	}
	if p.Name != "Nagwa VLESS" {
		t.Fatalf("name = %q", p.Name)
	}
}

func TestParseVLessNoTLS(t *testing.T) {
	link := "vless://11111111-2222-3333-4444-555555555555@cp.example.com:443" +
		"?encryption=none&type=ws&security=none&path=%2Fws&host=cp.example.com"
	p, err := ParseLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if p.TLS.Enabled {
		t.Fatalf("tls should be disabled: %+v", p.TLS)
	}
}

func TestParseVLessEncryptionAuto(t *testing.T) {
	link := "vless://ae6ee62c-8ec6-11f1-9ed8-674a19ddf00b@www.nagwa.com:80" +
		"?global_padding=true&authenticated_length=false&encryption=auto&security=none" +
		"&fp=&type=ws&path=/vpnjantit&host=www.nagwa.com.ru2.mrmohamed.dpdns.org#ru2"
	p, err := ParseLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if p.Protocol != models.ProtocolVLESS || p.Port != 80 {
		t.Fatalf("protocol/port = %s:%d", p.Protocol, p.Port)
	}
	if p.TLS.Enabled {
		t.Fatalf("tls should be disabled: %+v", p.TLS)
	}
	if p.Transport == nil || p.Transport.Type != models.TransportWS ||
		p.Transport.Path != "/vpnjantit" || p.Transport.Host != "www.nagwa.com.ru2.mrmohamed.dpdns.org" {
		t.Fatalf("transport = %+v", p.Transport)
	}
	if p.Name != "ru2" {
		t.Fatalf("name = %q", p.Name)
	}
}

func TestParseVmessSIP002(t *testing.T) {
	link := "vmess://ae6ee62c-8ec6-11f1-9ed8-674a19ddf00b@www.nagwa.com:80" +
		"?global_padding=true&authenticated_length=false&encryption=auto&security=none" +
		"&fp=&type=ws&path=/vpnjantit&host=www.nagwa.com.ru2.mrmohamed.dpdns.org#ru2"
	p, err := ParseLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if p.Protocol != models.ProtocolVMess {
		t.Fatalf("scheme vmess:// with uuid@host is a vmess link; got protocol %q", p.Protocol)
	}
	if p.UUID != "ae6ee62c-8ec6-11f1-9ed8-674a19ddf00b" || p.Port != 80 {
		t.Fatalf("uuid/port = %q:%d", p.UUID, p.Port)
	}
	if p.Security != "auto" {
		t.Fatalf("vmess security = %q, want auto", p.Security)
	}
	if p.TLS.Enabled {
		t.Fatalf("tls should be disabled: %+v", p.TLS)
	}
	if !p.GlobalPadding || p.AuthenticatedLength {
		t.Fatalf("globalPadding=%v authenticatedLength=%v", p.GlobalPadding, p.AuthenticatedLength)
	}
	if p.Transport == nil || p.Transport.Host != "www.nagwa.com.ru2.mrmohamed.dpdns.org" ||
		p.Transport.Path != "/vpnjantit" {
		t.Fatalf("transport = %+v", p.Transport)
	}
	if p.Name != "ru2" {
		t.Fatalf("name = %q", p.Name)
	}
}

func TestParseShadowsocksSIP002(t *testing.T) {
	p, err := ParseLink("ss://" + b64("aes-128-gcm:secret") + "@cp.example.com:8388#Frankfurt")
	if err != nil {
		t.Fatal(err)
	}
	if p.Protocol != models.ProtocolShadowsocks || p.Cipher != "aes-128-gcm" || p.Password != "secret" {
		t.Fatalf("ss profile = %+v", p)
	}
	if p.Address != "cp.example.com" || p.Port != 8388 {
		t.Fatalf("addr/port = %s:%d", p.Address, p.Port)
	}
	if p.Name != "Frankfurt" {
		t.Fatalf("name = %q", p.Name)
	}
}

func TestParseShadowsocksLegacy(t *testing.T) {
	p, err := ParseLink("ss://" + b64("chacha20-ietf-poly1305:pass@cp.example.com:8388") + "#Legacy")
	if err != nil {
		t.Fatal(err)
	}
	if p.Cipher != "chacha20-ietf-poly1305" || p.Password != "pass" {
		t.Fatalf("ss legacy = %+v", p)
	}
	if p.Name != "Legacy" {
		t.Fatalf("name = %q", p.Name)
	}
}

func TestParseTrojanWebSocket(t *testing.T) {
	p, err := ParseLink("trojan://trojan-pass@cp.example.com:443?type=ws&path=%2Fws&host=cp.example.com&sni=cp.example.com&fp=chrome#Trojan")
	if err != nil {
		t.Fatal(err)
	}
	if p.Protocol != models.ProtocolTrojan || p.Password != "trojan-pass" {
		t.Fatalf("trojan = %+v", p)
	}
	if !p.TLS.Enabled {
		t.Fatalf("trojan should force tls: %+v", p.TLS)
	}
	if p.Transport == nil || p.Transport.Host != "cp.example.com" {
		t.Fatalf("transport = %+v", p.Transport)
	}
}

func TestParseSocksAndHTTP(t *testing.T) {
	socks, err := ParseLink("socks5://user:pass@cp.example.com:1080#Socks")
	if err != nil {
		t.Fatal(err)
	}
	if socks.Protocol != models.ProtocolSOCKS5 || socks.Username != "user" || socks.Password != "pass" {
		t.Fatalf("socks = %+v", socks)
	}
	http, err := ParseLink("http://user:pass@cp.example.com:8080#HTTP")
	if err != nil {
		t.Fatal(err)
	}
	if http.Protocol != models.ProtocolHTTP || http.Port != 8080 {
		t.Fatalf("http = %+v", http)
	}
}

func TestParseLinkRejectsUnsupported(t *testing.T) {
	for _, link := range []string{
		"wireguard://cp.example.com:51820#WG",
		"not a link at all",
		"",
	} {
		if _, err := ParseLink(link); err == nil {
			t.Fatalf("want error for %q", link)
		}
	}
}

func TestParseLinkRejectsUnsupportedTransport(t *testing.T) {
	link := vmessLink(t, map[string]any{
		"v": "2", "ps": "GRPC", "add": "cp.example.com", "port": 443,
		"id": "11111111-2222-3333-4444-555555555555", "net": "grpc",
		"host": "cp.example.com", "path": "service", "tls": "tls",
	})
	if _, err := ParseLink(link); err == nil || !strings.Contains(err.Error(), "grpc") {
		t.Fatalf("want grpc rejection, got %v", err)
	}
}

func TestParseLinkRejectsReality(t *testing.T) {
	link := "vless://11111111-2222-3333-4444-555555555555@cp.example.com:443?security=reality&pbk=x&sid=y"
	if _, err := ParseLink(link); err == nil || !strings.Contains(err.Error(), "reality") {
		t.Fatalf("want reality rejection, got %v", err)
	}
}

func TestParseImportDataMixed(t *testing.T) {
	vm := vmessLink(t, map[string]any{
		"v": "2", "ps": "One", "add": "a.example.com", "port": 443,
		"id": "11111111-2222-3333-4444-555555555555", "net": "ws",
		"host": "a.example.com", "path": "/ws", "tls": "tls", "sni": "a.example.com",
	})
	blob := strings.Join([]string{
		vm,
		"vless://11111111-2222-3333-4444-555555555555@b.example.com:443?type=tcp&security=none#Two",
		"garbage line",
		"ss://" + b64("aes-128-gcm:secret") + "@c.example.com:8388#Three",
		"",
	}, "\n")
	profiles, errs := ParseImportData(blob)
	if len(profiles) != 3 {
		t.Fatalf("want 3 profiles, got %d", len(profiles))
	}
	if len(errs) != 1 {
		t.Fatalf("want 1 error, got %d: %v", len(errs), errs)
	}
	if profiles[2].Name != "Three" {
		t.Fatalf("third profile = %+v", profiles[2])
	}
}

func TestManagerImportLinks(t *testing.T) {
	m := newTestManager(t)
	blob := "vmess://" + b64(`{"v":"2","ps":"A","add":"a.example.com","port":443,"id":"11111111-2222-3333-4444-555555555555","net":"tcp","tls":""}`) +
		"\n" + "broken"
	added, failed, errs, err := m.ImportServers(blob)
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 || failed != 1 {
		t.Fatalf("added=%d failed=%d errs=%v", added, failed, errs)
	}
	if len(m.ListServers()) != 1 {
		t.Fatalf("want 1 stored server, got %d", len(m.ListServers()))
	}
}
