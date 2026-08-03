package models

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrValidation wraps validation failures on models.
var ErrValidation = errors.New("validation failed")

// Protocol is a supported outbound protocol. Protocol support comes from the
// engine (sing-box); this enum is the intersection of what the engine provides
// and what the UI exposes in Phase 1.
type Protocol string

const (
	ProtocolVLESS       Protocol = "vless"
	ProtocolVMess       Protocol = "vmess"
	ProtocolShadowsocks Protocol = "shadowsocks"
	ProtocolTrojan      Protocol = "trojan"
	ProtocolSOCKS5      Protocol = "socks5"
	ProtocolHTTP        Protocol = "http"
	ProtocolSSH         Protocol = "ssh"
)

// SupportedProtocols lists protocols accepted by Validate; extensible as the
// engine adds protocol support without UI rework.
var SupportedProtocols = []Protocol{
	ProtocolVLESS,
	ProtocolVMess,
	ProtocolShadowsocks,
	ProtocolTrojan,
	ProtocolSOCKS5,
	ProtocolHTTP,
	ProtocolSSH,
}

// TransportType identifies a stream transport for the vless/vmess/trojan
// outbounds. The empty value means plain TCP.
type TransportType string

const (
	TransportTCP TransportType = ""
	TransportWS  TransportType = "ws"
)

// TransportConfig holds stream-transport settings (Phase 1: WebSocket). The
// shape mirrors the sing-box `transport` option and the onnproxy envelope.
type TransportConfig struct {
	Type                TransportType `json:"type,omitempty"`
	Path                string        `json:"path,omitempty"`
	Host                string        `json:"host,omitempty"`
	MaxEarlyData        uint32        `json:"maxEarlyData,omitempty"`
	EarlyDataHeaderName string        `json:"earlyDataHeaderName,omitempty"`
}

// TLSConfig holds TLS settings for a server profile. Certificate validation is
// on by default; AllowInsecure may only be enabled in Advanced Mode with a
// user-visible warning (Phase 2).
type TLSConfig struct {
	Enabled       bool     `json:"enabled"`
	AllowInsecure bool     `json:"allowInsecure"`
	ServerName    string   `json:"serverName,omitempty"`
	ALPN          []string `json:"alpn,omitempty"`
	Fingerprint   string   `json:"fingerprint,omitempty"`
}

// SSHConfig holds SSH-tunnel-specific settings.
type SSHConfig struct {
	User       string `json:"user,omitempty"`
	HostKey    string `json:"hostKey,omitempty"`
	PrivateKey string `json:"privateKey,omitempty"` // PEM; stored via SecretStore, never in logs
}

// ServerProfile is a single configured server/protocol endpoint (PRD §8).
// Credential fields (Password, UUID, SSH.PrivateKey) are secrets: the config
// layer stores them in OS-native secure storage, never plaintext on disk, and
// the logger redacts them.
type ServerProfile struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Protocol       Protocol         `json:"protocol"`
	Address        string           `json:"address"`
	Port           int              `json:"port"`
	Username       string           `json:"username,omitempty"`
	Password       string           `json:"password,omitempty"`
	Cipher         string           `json:"cipher,omitempty"`
	UUID           string           `json:"uuid,omitempty"`
	Flow           string           `json:"flow,omitempty"`
	Security       string           `json:"security,omitempty"`
	TLS            TLSConfig        `json:"tls"`
	SSH            SSHConfig        `json:"ssh"`
	Transport      *TransportConfig `json:"transport,omitempty"`
	GlobalPadding  bool             `json:"globalPadding,omitempty"`
	PacketEncoding string           `json:"packetEncoding,omitempty"`
	Favorite       bool             `json:"favorite"`
	LastLatencyMS  int              `json:"lastLatencyMs"`
	LastTestedAt   *time.Time       `json:"lastTestedAt,omitempty"`
	CreatedAt      time.Time        `json:"createdAt"`
	UpdatedAt      time.Time        `json:"updatedAt"`
}

// Clone returns a deep copy of the profile.
func (p *ServerProfile) Clone() *ServerProfile {
	if p == nil {
		return nil
	}
	cp := *p
	if p.TLS.ALPN != nil {
		cp.TLS.ALPN = append([]string(nil), p.TLS.ALPN...)
	}
	if p.Transport != nil {
		t := *p.Transport
		cp.Transport = &t
	}
	if p.LastTestedAt != nil {
		t := *p.LastTestedAt
		cp.LastTestedAt = &t
	}
	return &cp
}

// Validate checks structural requirements per protocol. It deliberately keeps
// checks loose (e.g. shadowsocks cipher is required but not enumerated) because
// deep protocol validation belongs to the engine, not the core.
func (p *ServerProfile) Validate() error {
	field := func(msg string) error { return fmt.Errorf("%w: %s", ErrValidation, msg) }

	if strings.TrimSpace(p.Name) == "" {
		return field("name is required")
	}
	if strings.TrimSpace(p.Address) == "" {
		return field("address is required")
	}
	if p.Port < 1 || p.Port > 65535 {
		return field("port must be in 1..65535")
	}
	switch p.Protocol {
	case ProtocolVLESS, ProtocolVMess:
		if strings.TrimSpace(p.UUID) == "" {
			return field("uuid is required for " + string(p.Protocol))
		}
	case ProtocolShadowsocks:
		if p.Password == "" {
			return field("password is required for shadowsocks")
		}
		if p.Cipher == "" {
			return field("cipher is required for shadowsocks")
		}
	case ProtocolTrojan:
		if p.Password == "" {
			return field("password is required for trojan")
		}
	case ProtocolSSH:
		if p.SSH.User == "" {
			return field("ssh user is required")
		}
		if p.SSH.PrivateKey == "" && p.Password == "" {
			return field("ssh requires a private key or password")
		}
	case ProtocolSOCKS5, ProtocolHTTP:
	default:
		return field("unsupported protocol " + string(p.Protocol))
	}
	if p.Transport != nil {
		switch p.Transport.Type {
		case TransportTCP, TransportWS:
		default:
			return field("unsupported transport " + string(p.Transport.Type))
		}
	}
	return nil
}
