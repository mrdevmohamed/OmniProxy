package models

import (
	"fmt"
	"time"
)

// ConnectionState is the VPN connection lifecycle state (PRD: connection states).
type ConnectionState string

const (
	StateDisconnected ConnectionState = "disconnected"
	StateConnecting   ConnectionState = "connecting"
	StateConnected    ConnectionState = "connected"
	StateReconnecting ConnectionState = "reconnecting"
	StateError        ConnectionState = "error"
)

// ConnectionMode selects how the tunnel is exposed to the platform.
type ConnectionMode string

const (
	// ModeVPN routes traffic through a system VPN/TUN interface (default).
	ModeVPN ConnectionMode = "vpn"
	// ModeProxy exposes a local SOCKS5/HTTP proxy on loopback, no TUN.
	ModeProxy ConnectionMode = "proxy"
)

// Valid reports whether m is a known mode.
func (m ConnectionMode) Valid() bool { return m == ModeVPN || m == ModeProxy }

// ParseConnectionMode parses a mode string, tolerating unknown values by
// returning ModeVPN and an error.
func ParseConnectionMode(s string) (ConnectionMode, error) {
	switch ConnectionMode(s) {
	case ModeVPN, ModeProxy:
		return ConnectionMode(s), nil
	default:
		return ModeVPN, fmt.Errorf("unknown connection mode %q", s)
	}
}

// StatusChange is a single entry in a session's status history.
type StatusChange struct {
	State ConnectionState `json:"state"`
	At    time.Time       `json:"at"`
}

// SessionError is a structured, user-facing connection error.
type SessionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// VPNSession tracks one connection session (PRD §8). Bytes up/down are
// populated by the Phase 2 monitoring layer; they are 0 in the MVP.
type VPNSession struct {
	ID            string         `json:"id"`
	ServerID      string         `json:"serverId"`
	Mode          ConnectionMode `json:"mode"`
	StartedAt     time.Time      `json:"startedAt"`
	EndedAt       *time.Time     `json:"endedAt,omitempty"`
	BytesUp       uint64         `json:"bytesUp"`
	BytesDown     uint64         `json:"bytesDown"`
	StatusHistory []StatusChange `json:"statusHistory"`
	Error         *SessionError  `json:"error,omitempty"`
}

// Duration returns the elapsed session duration.
func (s *VPNSession) Duration() time.Duration {
	if s.EndedAt != nil {
		return s.EndedAt.Sub(s.StartedAt)
	}
	return time.Since(s.StartedAt)
}

// AddState appends a status change and records the error, if any.
func (s *VPNSession) AddState(state ConnectionState) {
	s.StatusHistory = append(s.StatusHistory, StatusChange{State: state, At: time.Now().UTC()})
	if state == StateError {
		s.Error = &SessionError{Code: "state_error", Message: string(state)}
	}
}

// Running reports whether the session is live (connecting/connected/reconnecting).
func (s *VPNSession) Running() bool {
	if len(s.StatusHistory) == 0 {
		return false
	}
	st := s.StatusHistory[len(s.StatusHistory)-1].State
	return st == StateConnecting || st == StateConnected || st == StateReconnecting
}
