package engine

import (
	"net/netip"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

// Mode selects the inbound type for a run.
type Mode string

const (
	// ModeVPN runs a TUN inbound. Requires privileges: root helper on Linux,
	// VpnService fd on Android, Wintun admin rights on Windows.
	ModeVPN Mode = "vpn"
	// ModeProxy runs a loopback SOCKS5+HTTP mixed inbound. No privileges.
	ModeProxy Mode = "proxy"
)

// Protocol is the upstream server protocol (Phase 1: one server, no chaining).
type Protocol string

const (
	ProtocolVLESS       Protocol = "vless"
	ProtocolVMess       Protocol = "vmess"
	ProtocolShadowsocks Protocol = "shadowsocks"
	ProtocolTrojan      Protocol = "trojan"
	ProtocolSOCKS       Protocol = "socks"
	ProtocolHTTP        Protocol = "http"
	ProtocolSSH         Protocol = "ssh"
)

// Transport is the outbound stream transport for protocols that support one
// (vless / vmess / trojan). The empty value means a plain TCP connection.
type Transport string

const (
	TransportTCP Transport = ""
	TransportWS  Transport = "ws"
)

// TransportSettings configures an outbound transport. Only the WebSocket
// transport is wired to sing-box today; gRPC / HTTPUpgrade are parser seams.
type TransportSettings struct {
	Type                Transport
	Path                string // ws path, e.g. /vpnjantit
	Host                string // ws Host header
	MaxEarlyData        uint32
	EarlyDataHeaderName string
}

// TLSSettings configures TLS for outbounds that support it.
type TLSSettings struct {
	ServerName  string
	Insecure    bool
	ALPN        []string
	Fingerprint string // utls fingerprint, e.g. "chrome"
}

// SSHSettings configures the SSH outbound.
type SSHSettings struct {
	User           string
	Password       string
	PrivateKey     string
	PrivateKeyPass string
	HostKey        string
}

// Outbound describes the single upstream server.
type Outbound struct {
	Protocol Protocol
	Address  string
	Port     uint16

	Username string // socks / http / ssh user
	Password string // socks / http / shadowsocks / trojan password
	Cipher   string // shadowsocks method, e.g. aes-128-gcm
	UUID     string // vless / vmess
	Flow     string // vless flow, e.g. xtls-rprx-vision
	Security string // vmess security: auto | none | aes-128-gcm | chacha20-poly1305

	// GlobalPadding, AuthenticatedLength and PacketEncoding are VMess
	// wire/transport options.
	GlobalPadding       bool
	AuthenticatedLength bool
	PacketEncoding      string // "xudp", "packet" or ""

	TLS       *TLSSettings
	SSH       *SSHSettings
	Transport *TransportSettings
}

// ProxyOptions configures the ModeProxy inbound.
type ProxyOptions struct {
	Listen string // default "127.0.0.1"
	Port   uint16 // default 1080
}

// TunOptions configures the ModeVPN inbound.
type TunOptions struct {
	InterfaceName string         // default "omniproxy"
	MTU           uint32         // default 1500
	Address       []netip.Prefix // default 10.0.0.1/24 (v4) + fd00::1/64 (v6)
	AutoRoute     bool           // default true
	Stack         string         // default "mixed"
	StrictRoute   bool           // default false
}

const (
	defaultProxyListen = "127.0.0.1"
	defaultProxyPort   = uint16(1080)
	defaultTunName     = "omniproxy"
	defaultMTU         = uint32(1500)
	defaultStack       = "mixed"

	// VPN-mode DNS defaults. Client queries resolve through the proxy (no DNS
	// leak); queries triggered by outbound dialing (the proxy server's own
	// domain) resolve over the real network to avoid a chicken-and-egg loop.
	dnsServerProxyTag = "dns-proxy"
	dnsServerLocalTag = "dns-local"
	dnsRemoteAddress  = "8.8.8.8"
	dnsLocalAddress   = "223.5.5.5"
)

// Options configures a single engine run.
type Options struct {
	Mode     Mode
	LogLevel Level
	Outbound Outbound
	Proxy    ProxyOptions
	Tun      TunOptions
	// CacheFilePath, when non-empty, points the sing-box cache DB to a real
	// directory (default is "cache.db" in the working directory).
	CacheFilePath string
}

func buildOptions(opts Options) (option.Options, error) {
	if err := opts.validate(); err != nil {
		return option.Options{}, err
	}
	outbounds := []option.Outbound{
		{
			Type:    C.TypeDirect,
			Tag:     "direct",
			Options: &option.DirectOutboundOptions{},
		},
		buildOutbound(opts.Outbound),
	}
	o := option.Options{
		Log: &option.LogOptions{
			Level:     opts.LogLevel.String(),
			Timestamp: true,
		},
		Outbounds: outbounds,
		Route: &option.RouteOptions{
			Final: "proxy",
		},
	}
	if opts.Mode == ModeVPN {
		// TUN traffic is opaque to sing-box without a DNS module; without one,
		// client DNS queries fall through to the proxy outbound as raw UDP and
		// are answered by whatever resolver the server forwards them to. Add a
		// DNS module and hijack DNS at the router so queries are answered
		// locally (through the proxy) and resolved domains are cached.
		o.DNS = buildDNSOptions()
		o.Route.Rules = append([]option.Rule{dnsHijackRule()}, o.Route.Rules...)
		// The tunnel's own sockets (dns-local bootstrap, the proxy server
		// connection) must bypass the TUN or their traffic loops back into the
		// tunnel. On Android the platform hook protects them (VpnService
		// protect()); on other platforms sing-box binds direct dials to the
		// default interface.
		o.Route.AutoDetectInterface = true
	}
	if opts.CacheFilePath != "" {
		o.Experimental = &option.ExperimentalOptions{
			CacheFile: &option.CacheFileOptions{
				Enabled: true,
				Path:    opts.CacheFilePath,
			},
		}
	}
	switch opts.Mode {
	case ModeVPN:
		o.Inbounds = []option.Inbound{{
			Type: C.TypeTun,
			Tag:  "tun",
			Options: &option.TunInboundOptions{
				InterfaceName: opts.Tun.interfaceName(),
				MTU:           opts.Tun.mtu(),
				Address:       opts.Tun.addresses(),
				AutoRoute:     opts.Tun.autoRoute(),
				Stack:         opts.Tun.stack(),
				StrictRoute:   opts.Tun.StrictRoute,
			},
		}}
	case ModeProxy:
		listen := opts.Proxy.Listen
		if listen == "" {
			listen = defaultProxyListen
		}
		port := opts.Proxy.Port
		if port == 0 {
			port = defaultProxyPort
		}
		addr := badoption.Addr(netip.MustParseAddr(listen))
		o.Inbounds = []option.Inbound{{
			Type: C.TypeMixed,
			Tag:  "mixed",
			Options: &option.HTTPMixedInboundOptions{
				ListenOptions: option.ListenOptions{
					Listen:     &addr,
					ListenPort: port,
				},
			},
		}}
	}
	return o, nil
}

func (o Options) validate() error {
	if o.Mode != ModeVPN && o.Mode != ModeProxy {
		return errInvalidMode(o.Mode)
	}
	if o.Outbound.Address == "" {
		return errEmptyServerAddress
	}
	if o.Outbound.Port == 0 {
		return errEmptyServerPort
	}
	return nil
}

func (t TunOptions) interfaceName() string {
	if t.InterfaceName == "" {
		return defaultTunName
	}
	return t.InterfaceName
}

func (t TunOptions) mtu() uint32 {
	if t.MTU == 0 {
		return defaultMTU
	}
	return t.MTU
}

func (t TunOptions) stack() string {
	if t.Stack == "" {
		return defaultStack
	}
	return t.Stack
}

func (t TunOptions) autoRoute() bool {
	return t.AutoRoute || !t.StrictRoute
}

func (t TunOptions) addresses() badoption.Listable[netip.Prefix] {
	if len(t.Address) > 0 {
		return badoption.Listable[netip.Prefix](t.Address)
	}
	return badoption.Listable[netip.Prefix]{
		netip.MustParsePrefix("10.0.0.1/24"),
		netip.MustParsePrefix("fd00::1/64"),
	}
}

func buildOutbound(ob Outbound) option.Outbound {
	server := option.ServerOptions{Server: ob.Address, ServerPort: ob.Port}
	switch ob.Protocol {
	case ProtocolVLESS:
		var packetEncoding *string
		if ob.PacketEncoding != "" {
			pe := ob.PacketEncoding
			packetEncoding = &pe
		}
		return option.Outbound{
			Type: C.TypeVLESS,
			Tag:  "proxy",
			Options: &option.VLESSOutboundOptions{
				ServerOptions:               server,
				UUID:                        ob.UUID,
				Flow:                        ob.Flow,
				PacketEncoding:              packetEncoding,
				OutboundTLSOptionsContainer: tlsContainer(ob.TLS),
				Transport:                   buildTransport(ob.Transport),
			},
		}
	case ProtocolVMess:
		security := ob.Security
		if security == "" {
			security = "auto"
		}
		return option.Outbound{
			Type: C.TypeVMess,
			Tag:  "proxy",
			Options: &option.VMessOutboundOptions{
				ServerOptions:               server,
				UUID:                        ob.UUID,
				Security:                    security,
				GlobalPadding:               ob.GlobalPadding,
				AuthenticatedLength:         ob.AuthenticatedLength,
				PacketEncoding:              ob.PacketEncoding,
				OutboundTLSOptionsContainer: tlsContainer(ob.TLS),
				Transport:                   buildTransport(ob.Transport),
			},
		}
	case ProtocolTrojan:
		return option.Outbound{
			Type: C.TypeTrojan,
			Tag:  "proxy",
			Options: &option.TrojanOutboundOptions{
				ServerOptions:               server,
				Password:                    ob.Password,
				OutboundTLSOptionsContainer: tlsContainer(ob.TLS),
				Transport:                   buildTransport(ob.Transport),
			},
		}
	case ProtocolShadowsocks:
		return option.Outbound{
			Type: C.TypeShadowsocks,
			Tag:  "proxy",
			Options: &option.ShadowsocksOutboundOptions{
				ServerOptions: server,
				Method:        ob.Cipher,
				Password:      ob.Password,
			},
		}
	case ProtocolSOCKS:
		return option.Outbound{
			Type: C.TypeSOCKS,
			Tag:  "proxy",
			Options: &option.SOCKSOutboundOptions{
				ServerOptions: server,
				Version:       "5",
				Username:      ob.Username,
				Password:      ob.Password,
			},
		}
	case ProtocolHTTP:
		return option.Outbound{
			Type: C.TypeHTTP,
			Tag:  "proxy",
			Options: &option.HTTPOutboundOptions{
				ServerOptions:               server,
				Username:                    ob.Username,
				Password:                    ob.Password,
				OutboundTLSOptionsContainer: tlsContainer(ob.TLS),
			},
		}
	case ProtocolSSH:
		sshOpts := option.SSHOutboundOptions{
			ServerOptions: server,
			User:          ob.Username,
			Password:      ob.Password,
		}
		if ob.SSH != nil {
			if ob.SSH.PrivateKey != "" {
				sshOpts.PrivateKey = badoption.Listable[string]{ob.SSH.PrivateKey}
			}
			sshOpts.PrivateKeyPassphrase = ob.SSH.PrivateKeyPass
			if ob.SSH.HostKey != "" {
				sshOpts.HostKey = badoption.Listable[string]{ob.SSH.HostKey}
			}
		}
		return option.Outbound{
			Type:    C.TypeSSH,
			Tag:     "proxy",
			Options: &sshOpts,
		}
	default:
		return option.Outbound{Type: C.TypeDirect, Tag: "proxy", Options: &option.DirectOutboundOptions{}}
	}
}

func tlsContainer(tlsCfg *TLSSettings) option.OutboundTLSOptionsContainer {
	if tlsCfg == nil {
		return option.OutboundTLSOptionsContainer{}
	}
	tlsOpts := &option.OutboundTLSOptions{
		Enabled:    true,
		ServerName: tlsCfg.ServerName,
		Insecure:   tlsCfg.Insecure,
		ALPN:       badoption.Listable[string](tlsCfg.ALPN),
	}
	if tlsCfg.Fingerprint != "" {
		tlsOpts.UTLS = &option.OutboundUTLSOptions{Fingerprint: tlsCfg.Fingerprint}
	}
	return option.OutboundTLSOptionsContainer{TLS: tlsOpts}
}

// buildDNSOptions returns the VPN-mode DNS configuration. Client queries use
// the default (dns-proxy) server so lookups travel through the tunnel; queries
// issued while dialing an outbound (e.g. resolving the proxy server's domain)
// use dns-local (empty-direct default dialer) to avoid a resolution loop.
// reverse_mapping lets the router answer PTR lookups for tunneled addresses.
func buildDNSOptions() *option.DNSOptions {
	return &option.DNSOptions{
		RawDNSOptions: option.RawDNSOptions{
			Servers: []option.DNSServerOptions{
				{
					Type: C.DNSTypeUDP,
					Tag:  dnsServerProxyTag,
					Options: &option.RemoteDNSServerOptions{
						RawLocalDNSServerOptions: option.RawLocalDNSServerOptions{
							DialerOptions: option.DialerOptions{Detour: "proxy"},
						},
						DNSServerAddressOptions: option.DNSServerAddressOptions{
							Server: dnsRemoteAddress,
						},
					},
				},
				{
					// No detour: DNS dialers default to an empty-direct dialer
					// (sing-box v1.12+ semantics), so this resolves over the
					// real network without a detour loop. An explicit detour to
					// the empty "direct" outbound is rejected by sing-box.
					Type: C.DNSTypeUDP,
					Tag:  dnsServerLocalTag,
					Options: &option.RemoteDNSServerOptions{
						DNSServerAddressOptions: option.DNSServerAddressOptions{
							Server: dnsLocalAddress,
						},
					},
				},
			},
			Rules: []option.DNSRule{
				{
					Type: C.RuleTypeDefault,
					DefaultOptions: option.DefaultDNSRule{
						RawDefaultDNSRule: option.RawDefaultDNSRule{
							Outbound: badoption.Listable[string]{"any"},
						},
						DNSRuleAction: option.DNSRuleAction{
							Action: C.RuleActionTypeRoute,
							RouteOptions: option.DNSRouteActionOptions{
								Server: dnsServerLocalTag,
							},
						},
					},
				},
			},
			Final:          dnsServerProxyTag,
			ReverseMapping: true,
			DNSClientOptions: option.DNSClientOptions{
				IndependentCache: true,
			},
		},
	}
}

// dnsHijackRule diverts DNS traffic arriving at the TUN inbound to the DNS
// module instead of forwarding it as ordinary UDP through the proxy.
func dnsHijackRule() option.Rule {
	return option.Rule{
		Type: C.RuleTypeDefault,
		DefaultOptions: option.DefaultRule{
			RawDefaultRule: option.RawDefaultRule{
				Protocol: badoption.Listable[string]{C.ProtocolDNS},
			},
			RuleAction: option.RuleAction{
				Action: C.RuleActionTypeHijackDNS,
			},
		},
	}
}

// buildTransport maps outbound transport settings onto sing-box options. Only
// the WebSocket transport is supported; any other non-empty transport yields nil
// (plain TCP), and unrecognised transports are surfaced at validation time by
// the core rather than here.
func buildTransport(t *TransportSettings) *option.V2RayTransportOptions {
	if t == nil || t.Type != TransportWS {
		return nil
	}
	ws := option.V2RayWebsocketOptions{
		Path:                t.Path,
		MaxEarlyData:        t.MaxEarlyData,
		EarlyDataHeaderName: t.EarlyDataHeaderName,
	}
	if t.Host != "" {
		ws.Headers = badoption.HTTPHeader{
			"Host": badoption.Listable[string]{t.Host},
		}
	}
	return &option.V2RayTransportOptions{
		Type:             C.V2RayTransportTypeWebsocket,
		WebsocketOptions: ws,
	}
}
