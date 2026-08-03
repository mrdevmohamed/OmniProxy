// Package tunnel implements the Tunnel Manager (PRD §7.1): it owns the engine
// lifecycle for the active server profile and maps core models onto engine
// options. Phase 1 is strictly single-server (no chaining); chains are a
// Phase 2 seam.
package tunnel

import (
	"omniproxy/core/models"
	"omniproxy/engine"
)

// BuildEngineOptions maps a validated server profile and connection mode onto
// engine options. logLevel maps the app log level onto the engine; cacheFile,
// when non-empty, points the engine cache DB at a real path.
func BuildEngineOptions(p *models.ServerProfile, mode models.ConnectionMode, logLevel engine.Level, cacheFile string) engine.Options {
	opts := engine.Options{
		LogLevel:      logLevel,
		CacheFilePath: cacheFile,
		Outbound:      buildOutbound(p),
	}
	switch mode {
	case models.ModeProxy:
		opts.Mode = engine.ModeProxy
	default:
		opts.Mode = engine.ModeVPN
	}
	return opts
}

func buildOutbound(p *models.ServerProfile) engine.Outbound {
	ob := engine.Outbound{
		Address:        p.Address,
		Port:           uint16(p.Port),
		Username:       p.Username,
		Password:       p.Password,
		Cipher:         p.Cipher,
		UUID:           p.UUID,
		Flow:           p.Flow,
		Security:       p.Security,
		GlobalPadding:  p.GlobalPadding,
		PacketEncoding: p.PacketEncoding,
		Transport:      transportIfEnabled(p.Transport),
	}
	if p.AuthenticatedLength {
		ob.AuthenticatedLength = true
	}
	switch p.Protocol {
	case models.ProtocolVLESS:
		ob.Protocol = engine.ProtocolVLESS
		ob.TLS = tlsIfEnabled(p)
	case models.ProtocolVMess:
		ob.Protocol = engine.ProtocolVMess
		ob.TLS = tlsIfEnabled(p)
	case models.ProtocolShadowsocks:
		ob.Protocol = engine.ProtocolShadowsocks
	case models.ProtocolTrojan:
		ob.Protocol = engine.ProtocolTrojan
		ob.TLS = tlsIfEnabled(p)
	case models.ProtocolSOCKS5:
		ob.Protocol = engine.ProtocolSOCKS
	case models.ProtocolHTTP:
		ob.Protocol = engine.ProtocolHTTP
		ob.TLS = tlsIfEnabled(p)
	case models.ProtocolSSH:
		ob.Protocol = engine.ProtocolSSH
		ob.Username = p.SSH.User
		ob.SSH = &engine.SSHSettings{
			PrivateKey: p.SSH.PrivateKey,
			HostKey:    p.SSH.HostKey,
		}
	}
	return ob
}

func transportIfEnabled(t *models.TransportConfig) *engine.TransportSettings {
	if t == nil {
		return nil
	}
	return &engine.TransportSettings{
		Type:                engine.Transport(t.Type),
		Path:                t.Path,
		Host:                t.Host,
		MaxEarlyData:        t.MaxEarlyData,
		EarlyDataHeaderName: t.EarlyDataHeaderName,
	}
}

func tlsIfEnabled(p *models.ServerProfile) *engine.TLSSettings {
	if !p.TLS.Enabled {
		return nil
	}
	return &engine.TLSSettings{
		ServerName:  p.TLS.ServerName,
		Insecure:    p.TLS.AllowInsecure,
		ALPN:        p.TLS.ALPN,
		Fingerprint: p.TLS.Fingerprint,
	}
}
