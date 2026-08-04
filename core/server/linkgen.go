package server

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"omniproxy/core/models"
	"omniproxy/core/store"
)

// Native share-link export (PRD §3.4): Link serializes a profile into the
// vless:// vmess:// ss:// trojan:// socks5:// http:// format that other VPN
// clients understand. Every output round-trips through ParseLink. Protocols
// without a share-link representation (ssh) are rejected rather than silently
// dropped.

// Link returns the native share-link string for one profile.
func Link(p *models.ServerProfile) (string, error) {
	switch p.Protocol {
	case models.ProtocolVLESS:
		return linkVLESS(p)
	case models.ProtocolVMess:
		return linkVMess(p)
	case models.ProtocolShadowsocks:
		return linkShadowsocks(p)
	case models.ProtocolTrojan:
		return linkTrojan(p)
	case models.ProtocolSOCKS5, models.ProtocolHTTP:
		return linkUserPass(p)
	default:
		return "", fmt.Errorf("server: cannot export %s as a share link", p.Protocol)
	}
}

// ExportLinks serializes the selected profiles (empty ids = all) into one
// newline-separated share-link payload.
func (m *Manager) ExportLinks(ids []string) (string, error) {
	servers, err := m.repo.List(store.Query{})
	if err != nil {
		return "", err
	}
	if len(ids) > 0 {
		want := make(map[string]bool, len(ids))
		for _, id := range ids {
			want[id] = true
		}
		filtered := servers[:0]
		for _, s := range servers {
			if want[s.ID] {
				filtered = append(filtered, s)
			}
		}
		servers = filtered
	}
	if len(servers) == 0 {
		return "", errors.New("server: nothing to export")
	}

	var b strings.Builder
	for _, s := range servers {
		link, err := Link(s)
		if err != nil {
			return "", err
		}
		b.WriteString(link)
		b.WriteByte('\n')
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// streamQuery builds the shared transport/TLS query params. Only WebSocket is
// encoded; a plain TCP profile omits the transport params entirely.
func streamQuery(p *models.ServerProfile) (url.Values, error) {
	q := url.Values{}
	if p.TLS.Enabled {
		q.Set("security", "tls")
	} else {
		q.Set("security", "none")
	}
	if p.TLS.ServerName != "" {
		q.Set("sni", p.TLS.ServerName)
	}
	if len(p.TLS.ALPN) > 0 {
		q.Set("alpn", strings.Join(p.TLS.ALPN, ","))
	}
	if p.TLS.Fingerprint != "" {
		q.Set("fp", p.TLS.Fingerprint)
	}
	if p.TLS.AllowInsecure {
		q.Set("allowInsecure", "1")
	}
	if p.Transport != nil {
		switch p.Transport.Type {
		case models.TransportTCP:
		case models.TransportWS:
			q.Set("type", "ws")
			if p.Transport.Path != "" {
				q.Set("path", p.Transport.Path)
			}
			if p.Transport.Host != "" {
				q.Set("host", p.Transport.Host)
			}
		default:
			return nil, fmt.Errorf("server: cannot export %s transport as a share link", p.Transport.Type)
		}
	}
	if p.PacketEncoding != "" {
		q.Set("packetEncoding", p.PacketEncoding)
	}
	return q, nil
}

func linkVLESS(p *models.ServerProfile) (string, error) {
	q, err := streamQuery(p)
	if err != nil {
		return "", err
	}
	q.Set("encryption", "none")
	if p.Flow != "" {
		q.Set("flow", p.Flow)
	}
	u := &url.URL{Scheme: "vless", User: url.User(p.UUID), Host: hostPort(p)}
	u.RawQuery = q.Encode()
	u.Fragment = p.Name
	return u.String(), nil
}

func linkVMess(p *models.ServerProfile) (string, error) {
	q, err := streamQuery(p)
	if err != nil {
		return "", err
	}
	security := p.Security
	if security == "" {
		security = "auto"
	}
	q.Set("encryption", security)
	if p.GlobalPadding {
		q.Set("global_padding", "1")
	}
	if p.AuthenticatedLength {
		q.Set("authenticated_length", "1")
	}
	u := &url.URL{Scheme: "vmess", User: url.User(p.UUID), Host: hostPort(p)}
	u.RawQuery = q.Encode()
	u.Fragment = p.Name
	return u.String(), nil
}

func linkShadowsocks(p *models.ServerProfile) (string, error) {
	userInfo := base64.RawURLEncoding.EncodeToString([]byte(p.Cipher + ":" + p.Password))
	u := &url.URL{Scheme: "ss", User: url.User(userInfo), Host: hostPort(p), Fragment: p.Name}
	return u.String(), nil
}

func linkTrojan(p *models.ServerProfile) (string, error) {
	q, err := streamQuery(p)
	if err != nil {
		return "", err
	}
	u := &url.URL{Scheme: "trojan", User: url.UserPassword(p.Password, ""), Host: hostPort(p), Fragment: p.Name}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func linkUserPass(p *models.ServerProfile) (string, error) {
	scheme := "socks5"
	if p.Protocol == models.ProtocolHTTP {
		scheme = "http"
	}
	u := &url.URL{Scheme: scheme, Host: hostPort(p), Fragment: p.Name}
	if p.Username != "" {
		if p.Password != "" {
			u.User = url.UserPassword(p.Username, p.Password)
		} else {
			u.User = url.User(p.Username)
		}
	}
	return u.String(), nil
}

func hostPort(p *models.ServerProfile) string {
	return net.JoinHostPort(p.Address, strconv.Itoa(p.Port))
}
