package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"omniproxy/core/models"
)

// Native share-link import (PRD §3.4): vmess://, vless://, ss://, trojan://,
// socks5:// and http:// links, the formats VPN users get from their providers.
// The visual builders in the app generate the sing-box configuration from the
// resulting profile; nothing here touches the engine directly.

// ErrUnsupportedLink is wrapped by parse errors for unrecognized schemes.
var ErrUnsupportedLink = errors.New("server: unsupported link")

// ParseImportData parses either an onnproxy envelope blob (single object or
// array, the previous import format) or a multi-line payload of native share
// links. Envelope JSON is strict; link parsing is per-line and lenient. The
// returned errors carry per-item failures (indices refer to lines).
func ParseImportData(data string) ([]*models.ServerProfile, []ImportError) {
	trim := strings.TrimSpace(data)
	if trim == "" {
		return nil, []ImportError{{Message: "import data is empty"}}
	}
	if trim[0] == '{' || trim[0] == '[' {
		profiles, err := ParseEnvelopes([]byte(trim))
		if err != nil {
			return nil, []ImportError{{Message: err.Error()}}
		}
		return profiles, nil
	}

	var profiles []*models.ServerProfile
	var errs []ImportError
	for i, line := range strings.Split(trim, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line[0] == '{' {
			envs, err := ParseEnvelopes([]byte(line))
			if err != nil {
				errs = append(errs, ImportError{Index: i, Message: err.Error()})
				continue
			}
			profiles = append(profiles, envs...)
			continue
		}
		p, err := ParseLink(line)
		if err != nil {
			errs = append(errs, ImportError{Index: i, Message: err.Error()})
			continue
		}
		profiles = append(profiles, p)
	}
	return profiles, errs
}

// ParseLink parses a single native share link into a server profile.
func ParseLink(link string) (*models.ServerProfile, error) {
	trimmed := strings.TrimSpace(link)
	if strings.HasPrefix(trimmed, "vmess://") {
		return parseVMess(trimmed)
	}
	if strings.HasPrefix(trimmed, "vless://") {
		return parseVLess(trimmed)
	}
	if strings.HasPrefix(trimmed, "ss://") {
		return parseShadowsocks(trimmed)
	}
	if strings.HasPrefix(trimmed, "trojan://") {
		return parseTrojan(trimmed)
	}
	if strings.HasPrefix(trimmed, "socks5://") || strings.HasPrefix(trimmed, "socks://") {
		return parseUserPass(trimmed, models.ProtocolSOCKS5)
	}
	if strings.HasPrefix(trimmed, "http://") {
		return parseUserPass(trimmed, models.ProtocolHTTP)
	}
	return nil, fmt.Errorf("%w: %q", ErrUnsupportedLink, schemeOf(trimmed))
}

func schemeOf(link string) string {
	if i := strings.Index(link, "://"); i > 0 {
		return link[:i]
	}
	return "unknown"
}

// parseVMess handles the v2rayN format: vmess://base64url(JSON).
func parseVMess(link string) (*models.ServerProfile, error) {
	payload := strings.TrimPrefix(link, "vmess://")
	// Some clients emit vmess://<b64uuid>@host:port?... (SIP002-ish). Detect an
	// '@' before any decoding succeeds and treat it like vless without the flow.
	if strings.Contains(payload, "@") {
		return parseVLess(strings.Replace(link, "vmess://", "vless://", 1))
	}
	raw, err := decodeBase64(strings.TrimSpace(payload))
	if err != nil {
		return nil, fmt.Errorf("vmess: invalid base64 payload: %w", err)
	}
	var vm struct {
		PS             string `json:"ps"`
		Add            string `json:"add"`
		Port           any    `json:"port"`
		ID             string `json:"id"`
		Security       string `json:"scy"`
		Network        string `json:"net"`
		Host           string `json:"host"`
		Path           string `json:"path"`
		TLS            string `json:"tls"`
		SNI            string `json:"sni"`
		FP             string `json:"fp"`
		ALPN           string `json:"alpn"`
		AllowInsecure  bool   `json:"allowInsecure"`
		GlobalPadding  bool   `json:"global_padding"`
		PacketEncoding string `json:"packetEncoding"`
	}
	if err := json.Unmarshal(raw, &vm); err != nil {
		return nil, fmt.Errorf("vmess: payload is not JSON: %w", err)
	}
	if strings.TrimSpace(vm.Add) == "" || strings.TrimSpace(vm.ID) == "" {
		return nil, errors.New("vmess: missing address or uuid")
	}
	port, err := portValue(vm.Port)
	if err != nil {
		return nil, err
	}
	if vm.TLS == "reality" {
		return nil, errors.New("vmess: reality is not supported yet")
	}
	transport, terr := transportFromNetwork(vm.Network, vm.Host, vm.Path)
	if terr != nil {
		return nil, terr
	}
	tls, terr := tlsFromFields(vm.TLS == "tls", vm.SNI, vm.Host, vm.FP, vm.ALPN, vm.AllowInsecure)
	if terr != nil {
		return nil, terr
	}
	return newProfile(models.ProtocolVMess, vm.PS, vm.Add, port, &models.ServerProfile{
		UUID:           strings.TrimSpace(vm.ID),
		Transport:      transport,
		TLS:            *tls,
		GlobalPadding:  vm.GlobalPadding,
		PacketEncoding: vm.PacketEncoding,
		Security:       vm.Security,
	})
}

// parseVLess handles vless://uuid@host:port?params#name.
func parseVLess(link string) (*models.ServerProfile, error) {
	rest := strings.TrimPrefix(link, "vless://")
	name, rest := splitFragment(rest)
	u, err := url.Parse("omniproxy://" + rest)
	if err != nil {
		return nil, fmt.Errorf("vless: %w", err)
	}
	if strings.TrimSpace(u.Host) == "" || strings.TrimSpace(u.User.Username()) == "" {
		return nil, errors.New("vless: missing host or uuid")
	}
	q := u.Query()
	if enc := q.Get("encryption"); enc != "" && enc != "none" {
		return nil, fmt.Errorf("vless: unsupported encryption %q (only none)", enc)
	}
	if sec := q.Get("security"); sec == "reality" {
		return nil, errors.New("vless: reality is not supported yet")
	}
	transport, err := transportFromNetwork(q.Get("type"), q.Get("host"), q.Get("path"))
	if err != nil {
		return nil, err
	}
	tls, err := tlsFromFields(q.Get("security") == "tls", q.Get("sni"), q.Get("host"), q.Get("fp"), q.Get("alpn"), allowInsecure(q.Get("allowInsecure")))
	if err != nil {
		return nil, err
	}
	return newProfile(models.ProtocolVLESS, name, u.Hostname(), portOr(u.Port(), "443"), &models.ServerProfile{
		UUID:           u.User.Username(),
		Flow:           q.Get("flow"),
		Transport:      transport,
		TLS:            *tls,
		PacketEncoding: q.Get("packetEncoding"),
	})
}

// parseShadowsocks handles ss:// SIP002 (method:password b64) and the legacy
// ss://base64(method:password@host:port) forms.
func parseShadowsocks(link string) (*models.ServerProfile, error) {
	rest := strings.TrimPrefix(link, "ss://")
	name, rest := splitFragment(rest)
	query := ""
	if i := strings.IndexByte(rest, '?'); i >= 0 {
		query = rest[i+1:]
		rest = rest[:i]
	}
	userInfo := ""
	if i := strings.LastIndexByte(rest, '@'); i >= 0 {
		userInfo, rest = rest[:i], rest[i+1:]
	} else {
		// legacy: whole remaining part (host:port) is inside the base64 blob
		decoded, err := decodeBase64(rest)
		if err != nil {
			return nil, fmt.Errorf("ss: invalid base64 payload: %w", err)
		}
		return parseShadowsocksFull(name, query, string(decoded))
	}
	methodPass, err := decodeBase64(userInfo)
	if err != nil {
		return nil, fmt.Errorf("ss: invalid method:password encoding: %w", err)
	}
	host, port, ok := strings.Cut(rest, ":")
	if !ok {
		return nil, errors.New("ss: missing host:port")
	}
	portNum, err := parsePort(port)
	if err != nil {
		return nil, err
	}
	mp := strings.SplitN(string(methodPass), ":", 2)
	if len(mp) != 2 || mp[0] == "" || mp[1] == "" {
		return nil, errors.New("ss: expected method:password")
	}
	if strings.Contains(query, "plugin=") {
		return nil, errors.New("ss: plugins are not supported yet")
	}
	return newProfile(models.ProtocolShadowsocks, name, host, portNum, &models.ServerProfile{
		Cipher:   mp[0],
		Password: mp[1],
	})
}

func parseShadowsocksFull(name, query, full string) (*models.ServerProfile, error) {
	host, rest, ok := strings.Cut(full, "@")
	if !ok {
		return nil, errors.New("ss: expected method:password@host:port")
	}
	mp := strings.SplitN(host, ":", 2)
	if len(mp) != 2 || mp[0] == "" || mp[1] == "" {
		return nil, errors.New("ss: expected method:password")
	}
	addr, port, ok := strings.Cut(rest, ":")
	if !ok {
		return nil, errors.New("ss: missing host:port")
	}
	portNum, err := parsePort(port)
	if err != nil {
		return nil, err
	}
	return newProfile(models.ProtocolShadowsocks, name, addr, portNum, &models.ServerProfile{
		Cipher:   mp[0],
		Password: mp[1],
	})
}

// parseTrojan handles trojan://password@host:port?params#name.
func parseTrojan(link string) (*models.ServerProfile, error) {
	rest := strings.TrimPrefix(link, "trojan://")
	name, rest := splitFragment(rest)
	u, err := url.Parse("omniproxy://" + rest)
	if err != nil {
		return nil, fmt.Errorf("trojan: %w", err)
	}
	if strings.TrimSpace(u.Host) == "" || u.User == nil || u.User.Username() == "" {
		return nil, errors.New("trojan: missing host or password")
	}
	q := u.Query()
	transport, err := transportFromNetwork(q.Get("type"), q.Get("host"), q.Get("path"))
	if err != nil {
		return nil, err
	}
	// Trojan always rides TLS; sni falls back to the host.
	tls, err := tlsFromFields(true, q.Get("sni"), u.Hostname(), q.Get("fp"), q.Get("alpn"), allowInsecure(q.Get("allowInsecure")))
	if err != nil {
		return nil, err
	}
	return newProfile(models.ProtocolTrojan, name, u.Hostname(), portOr(u.Port(), "443"), &models.ServerProfile{
		Password:  u.User.Username(),
		Transport: transport,
		TLS:       *tls,
	})
}

// parseUserPass handles socks5:// and http:// links (user:pass@host:port#name).
func parseUserPass(link string, protocol models.Protocol) (*models.ServerProfile, error) {
	prefix := "socks5://"
	if strings.HasPrefix(link, "socks://") {
		prefix = "socks://"
	}
	if protocol == models.ProtocolHTTP {
		prefix = "http://"
	}
	rest := strings.TrimPrefix(link, prefix)
	name, rest := splitFragment(rest)
	u, err := url.Parse("omniproxy://" + rest)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", string(protocol), err)
	}
	if strings.TrimSpace(u.Host) == "" {
		return nil, errors.New(string(protocol) + ": missing host")
	}
	p := &models.ServerProfile{Address: u.Hostname()}
	if u.User != nil {
		p.Username = u.User.Username()
		if pw, ok := u.User.Password(); ok {
			p.Password = pw
		}
	}
	return newProfile(protocol, name, u.Hostname(), portOr(u.Port(), "1080"), p)
}

// newProfile fills the shared identity fields and validates the result.
func newProfile(protocol models.Protocol, name, address string, port int, extra *models.ServerProfile) (*models.ServerProfile, error) {
	p := &models.ServerProfile{
		ID:       "",
		Name:     strings.TrimSpace(name),
		Protocol: protocol,
		Address:  strings.TrimSpace(address),
		Port:     port,
		TLS:      models.TLSConfig{},
	}
	if extra != nil {
		p.Username = extra.Username
		p.Password = extra.Password
		p.Cipher = extra.Cipher
		p.UUID = extra.UUID
		p.Flow = extra.Flow
		p.Security = extra.Security
		p.TLS = extra.TLS
		p.Transport = extra.Transport
		p.GlobalPadding = extra.GlobalPadding
		p.PacketEncoding = extra.PacketEncoding
	}
	if p.Name == "" {
		p.Name = fmt.Sprintf("%s @ %s:%d", string(protocol), p.Address, p.Port)
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", string(protocol), err)
	}
	return p, nil
}

// transportFromNetwork maps the share-link network/transport value onto the
// profile. Only WebSocket is supported in Phase 1; everything else is rejected
// with a clear error instead of silently degrading to plain TCP.
func transportFromNetwork(network, host, path string) (*models.TransportConfig, error) {
	switch strings.ToLower(strings.TrimSpace(network)) {
	case "", "tcp":
		if strings.TrimSpace(host) == "" && strings.TrimSpace(path) == "" {
			return nil, nil
		}
		return nil, errors.New("tcp links with host/path are malformed")
	case "ws", "websocket":
		return &models.TransportConfig{
			Type: models.TransportWS,
			Path: path,
			Host: host,
		}, nil
	case "grpc", "h2", "http", "httpupgrade", "quic":
		return nil, fmt.Errorf("%s transport is not supported yet", network)
	default:
		return nil, fmt.Errorf("unknown transport %q", network)
	}
}

// tlsFromFields builds TLS settings. serverName falls back to the ws host when
// no sni is present, which matches how Xray clients behave.
func tlsFromFields(enabled bool, sni, host, fingerprint, alpn string, insecure bool) (*models.TLSConfig, error) {
	if !enabled {
		return &models.TLSConfig{}, nil
	}
	serverName := strings.TrimSpace(sni)
	if serverName == "" {
		serverName = strings.TrimSpace(host)
	}
	var alpnList []string
	if alpn != "" {
		alpnList = splitComma(alpn)
	}
	return &models.TLSConfig{
		Enabled:       true,
		AllowInsecure: insecure,
		ServerName:    serverName,
		ALPN:          alpnList,
		Fingerprint:   fingerprint,
	}, nil
}

func splitFragment(rest string) (name, remainder string) {
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		frag := rest[i+1:]
		if decoded, err := url.QueryUnescape(frag); err == nil {
			frag = decoded
		}
		return strings.TrimSpace(frag), rest[:i]
	}
	return "", rest
}

func portValue(v any) (int, error) {
	switch t := v.(type) {
	case float64:
		return parsePort(strconv.FormatFloat(t, 'f', 0, 64))
	case string:
		return parsePort(t)
	case int:
		return parsePort(strconv.Itoa(t))
	default:
		return parsePort("0")
	}
}

func portOr(port, fallback string) int {
	if port == "" {
		return mustInt(fallback)
	}
	return mustInt(port)
}
func parsePort(s string) (int, error) {
	if strings.TrimSpace(s) == "" {
		return 0, errors.New("missing port")
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 1 || n > 65535 {
		return 0, fmt.Errorf("invalid port %q", s)
	}
	return n, nil
}

func mustInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func allowInsecure(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// decodeBase64 accepts standard and URL-safe base64, with or without padding.
func decodeBase64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	for _, enc := range []*base64.Encoding{
		base64.RawStdEncoding, base64.StdEncoding,
		base64.RawURLEncoding, base64.URLEncoding,
	} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, errors.New("invalid base64")
}
