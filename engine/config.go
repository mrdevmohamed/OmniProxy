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
	ProtocolSOCKS       Protocol = "socks"
	ProtocolHTTP        Protocol = "http"
	ProtocolSSH         Protocol = "ssh"
)

// TLSSettings configures TLS for outbounds that support it.
type TLSSettings struct {
	ServerName string
	Insecure   bool
	ALPN       []string
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
	Password string // socks / http / shadowsocks password
	Cipher   string // shadowsocks method, e.g. aes-128-gcm
	UUID     string // vless / vmess
	Flow     string // vless flow, e.g. xtls-rprx-vision
	Security string // vmess security: auto | none | aes-128-gcm | chacha20-poly1305

	TLS *TLSSettings
	SSH *SSHSettings
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
				InboundOptions: option.InboundOptions{
					SniffEnabled: true,
				},
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
		return option.Outbound{
			Type: C.TypeVLESS,
			Tag:  "proxy",
			Options: &option.VLESSOutboundOptions{
				ServerOptions:               server,
				UUID:                        ob.UUID,
				Flow:                        ob.Flow,
				OutboundTLSOptionsContainer: tlsContainer(ob.TLS),
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
				OutboundTLSOptionsContainer: tlsContainer(ob.TLS),
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
	return option.OutboundTLSOptionsContainer{
		TLS: &option.OutboundTLSOptions{
			Enabled:    true,
			ServerName: tlsCfg.ServerName,
			Insecure:   tlsCfg.Insecure,
			ALPN:       badoption.Listable[string](tlsCfg.ALPN),
		},
	}
}
