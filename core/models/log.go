package models

import "time"

// LogEntry is one structured log line (PRD §8). Message is already redacted by
// the logger before it reaches subscribers or sinks.
type LogEntry struct {
	Seq       uint64         `json:"seq"`
	Timestamp time.Time      `json:"timestamp"`
	Level     LogLevel       `json:"level"`
	Component string         `json:"component"`
	Message   string         `json:"message"`
	Context   map[string]any `json:"context,omitempty"`
}
