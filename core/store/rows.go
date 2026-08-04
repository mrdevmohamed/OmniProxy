package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"omniproxy/core/models"
)

// serverColumnList is the canonical column order of the servers table. The
// SELECT/INSERT/UPDATE paths and scanServer all rely on this exact order.
var serverColumnList = []string{
	"id",
	"name",
	"protocol",
	"address",
	"port",
	"username",
	"password",
	"cipher",
	"uuid",
	"flow",
	"security",
	"global_padding",
	"authenticated_length",
	"packet_encoding",
	"tls_enabled",
	"tls_allow_insecure",
	"tls_server_name",
	"tls_alpn",
	"tls_fingerprint",
	"transport_type",
	"transport_path",
	"transport_host",
	"transport_max_early_data",
	"transport_early_data_header_name",
	"ssh_user",
	"ssh_host_key",
	"ssh_private_key",
	"reality_enabled",
	"reality_public_key",
	"reality_short_id",
	"reality_spider_x",
	"enabled",
	"tags",
	"group_name",
	"favorite",
	"last_latency_ms",
	"last_tested_at",
	"created_at",
	"updated_at",
	"secret_refs",
}

var serverColumns = strings.Join(serverColumnList, ", ")

type scanner interface {
	Scan(dest ...any) error
}

// serverRow is the raw persisted form plus the stored secret refs, which are
// not part of the public profile.
type serverRow struct {
	profile   *models.ServerProfile
	secretRef []string
}

// scanServer reads one row in serverColumnList order.
func scanServer(s scanner) (*serverRow, error) {
	var (
		id, name, protocol, address                      string
		port                                             int
		username, password, cipher                       string
		uuid, flow, security                             string
		globalPadding                                    int
		authenticatedLength                              int
		packetEncoding                                   string
		tlsEnabled, tlsAllowInsecure                     int
		tlsServerName, tlsALPN                           string
		tlsFingerprint                                   string
		transportType, transportPath, transportHost      string
		transportMaxEarlyData                            int
		transportEarlyDataHeaderName                     string
		sshUser, sshHostKey, sshPrivateKey               string
		realityEnabled                                   int
		realityPublicKey, realityShortID, realitySpiderX string
		enabled, favorite                                int
		tags, groupName                                  string
		lastLatencyMS                                    int
		lastTestedAt                                     string
		createdAt, updatedAt                             string
		secretRefs                                       string
	)

	err := s.Scan(
		&id, &name, &protocol, &address, &port,
		&username, &password, &cipher, &uuid, &flow, &security,
		&globalPadding, &authenticatedLength, &packetEncoding,
		&tlsEnabled, &tlsAllowInsecure, &tlsServerName, &tlsALPN, &tlsFingerprint,
		&transportType, &transportPath, &transportHost, &transportMaxEarlyData, &transportEarlyDataHeaderName,
		&sshUser, &sshHostKey, &sshPrivateKey,
		&realityEnabled, &realityPublicKey, &realityShortID, &realitySpiderX,
		&enabled, &tags, &groupName, &favorite,
		&lastLatencyMS, &lastTestedAt,
		&createdAt, &updatedAt, &secretRefs,
	)
	if err != nil {
		return nil, err
	}

	p := &models.ServerProfile{
		ID:                  id,
		Name:                name,
		Protocol:            models.Protocol(protocol),
		Address:             address,
		Port:                port,
		Username:            username,
		Password:            password,
		Cipher:              cipher,
		UUID:                uuid,
		Flow:                flow,
		Security:            security,
		GlobalPadding:       globalPadding != 0,
		AuthenticatedLength: authenticatedLength != 0,
		PacketEncoding:      packetEncoding,
		TLS: models.TLSConfig{
			Enabled:       tlsEnabled != 0,
			AllowInsecure: tlsAllowInsecure != 0,
			ServerName:    tlsServerName,
			Fingerprint:   tlsFingerprint,
		},
		SSH: models.SSHConfig{
			User:       sshUser,
			HostKey:    sshHostKey,
			PrivateKey: sshPrivateKey,
		},
		Enabled:       enabled != 0,
		Group:         groupName,
		Favorite:      favorite != 0,
		LastLatencyMS: lastLatencyMS,
	}

	if tlsALPN != "" && tlsALPN != "[]" {
		if err := json.Unmarshal([]byte(tlsALPN), &p.TLS.ALPN); err != nil {
			return nil, fmt.Errorf("store: parse tls_alpn for %s: %w", id, err)
		}
	}
	if transportType != "" || transportPath != "" || transportHost != "" {
		p.Transport = &models.TransportConfig{
			Type:                models.TransportType(transportType),
			Path:                transportPath,
			Host:                transportHost,
			MaxEarlyData:        uint32(transportMaxEarlyData),
			EarlyDataHeaderName: transportEarlyDataHeaderName,
		}
	}
	if realityEnabled != 0 || realityPublicKey != "" || realityShortID != "" || realitySpiderX != "" {
		p.Reality = &models.RealityConfig{
			Enabled:   realityEnabled != 0,
			PublicKey: realityPublicKey,
			ShortID:   realityShortID,
			SpiderX:   realitySpiderX,
		}
	}
	if tags != "" && tags != "[]" {
		if err := json.Unmarshal([]byte(tags), &p.Tags); err != nil {
			return nil, fmt.Errorf("store: parse tags for %s: %w", id, err)
		}
	}
	if lastTestedAt != "" {
		t, err := time.Parse(time.RFC3339, lastTestedAt)
		if err != nil {
			return nil, fmt.Errorf("store: parse last_tested_at for %s: %w", id, err)
		}
		p.LastTestedAt = &t
	}
	if createdAt != "" {
		p.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, fmt.Errorf("store: parse created_at for %s: %w", id, err)
		}
	}
	if updatedAt != "" {
		p.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("store: parse updated_at for %s: %w", id, err)
		}
	}

	refs := []string{}
	if secretRefs != "" && secretRefs != "[]" {
		if err := json.Unmarshal([]byte(secretRefs), &refs); err != nil {
			return nil, fmt.Errorf("store: parse secret_refs for %s: %w", id, err)
		}
	}
	return &serverRow{profile: p, secretRef: refs}, nil
}

// serverValues returns the row values in serverColumnList order. p must
// already be sanitized (credential fields blank).
func serverValues(p *models.ServerProfile, refs []string) ([]any, error) {
	alpn, err := json.Marshal(p.TLS.ALPN)
	if err != nil {
		return nil, err
	}
	if string(alpn) == "null" {
		alpn = []byte("[]")
	}
	tags, err := json.Marshal(p.Tags)
	if err != nil {
		return nil, err
	}
	if string(tags) == "null" {
		tags = []byte("[]")
	}
	refsJSON, err := json.Marshal(refs)
	if err != nil {
		return nil, err
	}

	transportType, transportPath, transportHost := "", "", ""
	var transportMaxEarlyData int
	var transportEarlyDataHeaderName string
	if p.Transport != nil {
		transportType = string(p.Transport.Type)
		transportPath = p.Transport.Path
		transportHost = p.Transport.Host
		transportMaxEarlyData = int(p.Transport.MaxEarlyData)
		transportEarlyDataHeaderName = p.Transport.EarlyDataHeaderName
	}

	realityEnabled := 0
	realityPublicKey, realityShortID, realitySpiderX := "", "", ""
	if p.Reality != nil {
		realityEnabled = boolInt(p.Reality.Enabled)
		realityPublicKey = p.Reality.PublicKey
		realityShortID = p.Reality.ShortID
		realitySpiderX = p.Reality.SpiderX
	}

	lastTestedAt := ""
	if p.LastTestedAt != nil {
		lastTestedAt = p.LastTestedAt.UTC().Format(time.RFC3339)
	}

	return []any{
		p.ID,
		p.Name,
		string(p.Protocol),
		p.Address,
		p.Port,
		p.Username,
		p.Password,
		p.Cipher,
		p.UUID,
		p.Flow,
		p.Security,
		boolInt(p.GlobalPadding),
		boolInt(p.AuthenticatedLength),
		p.PacketEncoding,
		boolInt(p.TLS.Enabled),
		boolInt(p.TLS.AllowInsecure),
		p.TLS.ServerName,
		string(alpn),
		p.TLS.Fingerprint,
		transportType,
		transportPath,
		transportHost,
		transportMaxEarlyData,
		transportEarlyDataHeaderName,
		p.SSH.User,
		p.SSH.HostKey,
		p.SSH.PrivateKey,
		realityEnabled,
		realityPublicKey,
		realityShortID,
		realitySpiderX,
		boolInt(p.Enabled),
		string(tags),
		p.Group,
		boolInt(p.Favorite),
		p.LastLatencyMS,
		lastTestedAt,
		p.CreatedAt.UTC().Format(time.RFC3339),
		p.UpdatedAt.UTC().Format(time.RFC3339),
		string(refsJSON),
	}, nil
}

func updateClause() string {
	var cols []string
	for _, c := range serverColumnList {
		if c == "id" {
			continue
		}
		cols = append(cols, c+" = ?")
	}
	return strings.Join(cols, ", ")
}

// querySecretRefs reads the stored refs for a row without restoring values.
func (r *SQLiteServerRepository) querySecretRefs(id string) ([]string, error) {
	var raw string
	err := r.db.SQLDB().QueryRow("SELECT secret_refs FROM servers WHERE id = ?", id).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrServerNotFound
		}
		return nil, err
	}
	refs := []string{}
	if raw != "" && raw != "[]" {
		if err := json.Unmarshal([]byte(raw), &refs); err != nil {
			return nil, fmt.Errorf("store: parse secret_refs for %s: %w", id, err)
		}
	}
	return refs, nil
}
