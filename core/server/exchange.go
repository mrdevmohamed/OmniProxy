package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"omniproxy/core/models"
)

// Envelope is the shared, cross-platform interchange format for server
// profiles (PRD §3.4). A single export is one envelope; a bulk export is a JSON
// array of envelopes. Import accepts either.
type Envelope struct {
	Format  string                `json:"format"`
	Version int                   `json:"version"`
	Server  *models.ServerProfile `json:"server,omitempty"`
}

const (
	// EnvelopeFormat is the interchange format identifier.
	EnvelopeFormat = "onnproxy"
	// EnvelopeVersion is the current interchange version.
	EnvelopeVersion = 1
)

// ExportServers serializes the selected profiles (empty ids = all) into the
// onnproxy format. Returns an error when there is nothing to export.
func (m *Manager) ExportServers(ids []string) (string, error) {
	servers := m.engine.ListServers()
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

	envs := make([]Envelope, len(servers))
	for i, s := range servers {
		envs[i] = Envelope{Format: EnvelopeFormat, Version: EnvelopeVersion, Server: s}
	}

	if len(envs) == 1 {
		b, err := json.Marshal(envs[0])
		return string(b), err
	}
	b, err := json.Marshal(envs)
	return string(b), err
}

// ImportError reports one failed profile during a bulk import.
type ImportError struct {
	Index   int    `json:"index,omitempty"`
	Message string `json:"message"`
}

// ImportServers parses an onnproxy blob (single envelope or array) or a
// multi-line payload of native share links (vmess:// vless:// ss:// trojan://
// socks5:// http://), assigns fresh ids (imports never collide with existing
// profiles), and persists each valid profile. Returns added/failed counts and
// per-item errors; a blob that is neither valid onnproxy nor a parseable link
// is reported as one failed item.
func (m *Manager) ImportServers(data string) (added int, failed int, errs []ImportError, err error) {
	profiles, errs := ParseImportData(data)
	failed = len(errs)
	for i, p := range profiles {
		p.ID = ""
		p.CreatedAt, p.UpdatedAt = time.Time{}, time.Time{}
		p.LastLatencyMS = 0
		p.LastTestedAt = nil
		if _, aerr := m.engine.AddServer(p); aerr != nil {
			failed++
			errs = append(errs, ImportError{Index: i, Message: aerr.Error()})
			continue
		}
		added++
	}
	return added, failed, errs, nil
}

// ParseEnvelopes parses either a single Envelope object or a JSON array of
// envelopes into server profiles. Strict on format/version, tolerant on layout.
func ParseEnvelopes(data []byte) ([]*models.ServerProfile, error) {
	trim := bytes.TrimSpace(data)
	if len(trim) == 0 {
		return nil, errors.New("import data is empty")
	}

	if trim[0] == '[' {
		var envs []Envelope
		if err := json.Unmarshal(trim, &envs); err != nil {
			return nil, fmt.Errorf("invalid onnproxy blob: %w", err)
		}
		out := make([]*models.ServerProfile, 0, len(envs))
		for i := range envs {
			if err := validateEnvelope(&envs[i]); err != nil {
				return nil, fmt.Errorf("envelope %d: %w", i, err)
			}
			if envs[i].Server == nil {
				return nil, fmt.Errorf("envelope %d: missing server", i)
			}
			out = append(out, envs[i].Server)
		}
		return out, nil
	}

	var env Envelope
	if err := json.Unmarshal(trim, &env); err != nil {
		return nil, fmt.Errorf("invalid onnproxy blob: %w", err)
	}
	if err := validateEnvelope(&env); err != nil {
		return nil, err
	}
	if env.Server == nil {
		return nil, errors.New("envelope: missing server")
	}
	return []*models.ServerProfile{env.Server}, nil
}

func validateEnvelope(e *Envelope) error {
	if e.Format != EnvelopeFormat {
		return fmt.Errorf("unsupported format %q", e.Format)
	}
	if e.Version != EnvelopeVersion {
		return fmt.Errorf("unsupported version %d", e.Version)
	}
	return nil
}
