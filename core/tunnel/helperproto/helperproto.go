// Package helperproto defines the JSON wire protocol between the unprivileged
// core (client) and the privileged omniproxy-helper (server), which hosts the
// sing-box engine for VPN mode (docs/platform-notes.md §Linux). Messages are
// newline-delimited JSON over a Unix stream socket.
//
// The types live in their own package so both sides share them without an
// import cycle: core/tunnel (client) and core/tunnel/helperhost (server).
package helperproto

import "omniproxy/engine"

// ClientMessage is sent by the core to the helper. Options carries the full
// engine.Options so the helper never re-derives engine configuration itself
// (the shared engine module is the single builder).
type ClientMessage struct {
	Type    string        `json:"type"` // connect | disconnect | ping | quit
	Seq     uint64        `json:"seq,omitempty"`
	Options engine.Options `json:"options,omitempty"`
}

// ServerMessage is sent by the helper to the core.
type ServerMessage struct {
	Type  string          `json:"type"` // response | event
	Seq   uint64          `json:"seq,omitempty"`
	OK    bool            `json:"ok,omitempty"`
	State string          `json:"state,omitempty"` // connected | disconnected
	Error string          `json:"error,omitempty"`
	Event *HelperLogEvent `json:"event,omitempty"`
}

// HelperLogEvent carries one engine log line from the helper.
type HelperLogEvent struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}
