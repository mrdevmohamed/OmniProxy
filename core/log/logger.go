// Package log provides leveled, structured logging shared across all platforms
// (PRD §7.1). Every entry is redacted before it reaches a sink or subscriber:
// credentials, private keys, and raw traffic content must never appear in logs
// (PRD §9).
package log

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"omniproxy/core/models"
)

// Sink receives log entries. Implementations must be safe for concurrent use.
type Sink interface {
	Write(entry models.LogEntry)
}

// ConsoleSink writes human-readable lines to an io.Writer (default stderr).
type ConsoleSink struct {
	mu  sync.Mutex
	out io.Writer
}

// NewConsoleSink returns a sink writing to out.
func NewConsoleSink(out io.Writer) *ConsoleSink {
	if out == nil {
		out = os.Stderr
	}
	return &ConsoleSink{out: out}
}

// Write implements Sink.
func (s *ConsoleSink) Write(e models.LogEntry) {
	var b strings.Builder
	b.WriteString(e.Timestamp.UTC().Format(time.RFC3339))
	b.WriteByte(' ')
	b.WriteString(strings.ToUpper(string(e.Level)))
	b.WriteByte(' ')
	b.WriteString(e.Component)
	b.WriteByte(' ')
	b.WriteString(e.Message)
	if len(e.Context) > 0 {
		if ctx, err := json.Marshal(e.Context); err == nil {
			b.WriteByte(' ')
			b.Write(ctx)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	io.WriteString(s.out, b.String()+"\n")
}

// Logger is the core logger: leveled, redacted, with an in-memory ring buffer
// that feeds the bridge getLogs method.
type Logger struct {
	mu       sync.RWMutex
	level    models.LogLevel
	redactor *Redactor
	ring     *RingBuffer
	sinks    []Sink
}

// NewLogger returns a logger at the given level. With no sinks it writes to
// stderr via ConsoleSink.
func NewLogger(level models.LogLevel, sinks ...Sink) *Logger {
	if len(sinks) == 0 {
		sinks = []Sink{NewConsoleSink(os.Stderr)}
	}
	return &Logger{
		level:    normalizeLevel(level),
		redactor: NewRedactor(),
		ring:     NewRingBuffer(1000),
		sinks:    sinks,
	}
}

// NewNopLogger returns a logger with no sinks (tests, silent subsystems).
func NewNopLogger() *Logger {
	return &Logger{
		level:    models.LevelTrace,
		redactor: NewRedactor(),
		ring:     NewRingBuffer(1000),
	}
}

func normalizeLevel(l models.LogLevel) models.LogLevel {
	if l.Valid() {
		return l
	}
	return models.LevelInfo
}

// SetLevel changes the minimum emitted level.
func (l *Logger) SetLevel(lv models.LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = normalizeLevel(lv)
}

// Level returns the current minimum emitted level.
func (l *Logger) Level() models.LogLevel {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.level
}

// Redactor exposes the logger's secret registry so subsystems can register
// secrets as they are created/loaded.
func (l *Logger) Redactor() *Redactor { return l.redactor }

// AddSink registers an additional sink (e.g. the event bus bridge). Entries
// already redacted.
func (l *Logger) AddSink(s Sink) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sinks = append(l.sinks, s)
}

// LogsAfter returns up to limit redacted entries with Seq > afterSeq.
func (l *Logger) LogsAfter(afterSeq uint64, limit int) []models.LogEntry {
	return l.ring.Since(afterSeq, limit)
}

func (l *Logger) log(lv models.LogLevel, component, message string, ctx map[string]any) {
	l.mu.RLock()
	level := l.level
	l.mu.RUnlock()
	if lv.Rank() < level.Rank() {
		return
	}

	message = l.redactor.Redact(stripANSI(message))
	if len(ctx) > 0 {
		rc := make(map[string]any, len(ctx))
		for k, v := range ctx {
			rc[k] = redactValue(l.redactor, v)
		}
		ctx = rc
	}

	entry := models.LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     lv,
		Component: component,
		Message:   message,
		Context:   ctx,
	}
	// RingBuffer.Append assigns the sequence number to its own copy and
	// returns it; propagate it onto the entry so sinks (the event bridge)
	// emit logAppended events that carry a valid Seq for incremental pulls.
	entry.Seq = l.ring.Append(entry)

	l.mu.RLock()
	sinks := append([]Sink(nil), l.sinks...)
	l.mu.RUnlock()
	for _, s := range sinks {
		s.Write(entry)
	}
}

func redactValue(r *Redactor, v any) any {
	switch t := v.(type) {
	case string:
		return r.Redact(t)
	case map[string]any:
		for k, vv := range t {
			t[k] = redactValue(r, vv)
		}
		return t
	case []any:
		for i, vv := range t {
			t[i] = redactValue(r, vv)
		}
		return t
	default:
		return v
	}
}

// Log emits a structured entry.
func (l *Logger) Log(lv models.LogLevel, component, message string, ctx map[string]any) {
	l.log(lv, component, message, ctx)
}

// Tracef emits a trace-level message.
func (l *Logger) Tracef(component, format string, args ...any) {
	l.log(models.LevelTrace, component, fmt.Sprintf(format, args...), nil)
}

// Debugf emits a debug-level message.
func (l *Logger) Debugf(component, format string, args ...any) {
	l.log(models.LevelDebug, component, fmt.Sprintf(format, args...), nil)
}

// Infof emits an info-level message.
func (l *Logger) Infof(component, format string, args ...any) {
	l.log(models.LevelInfo, component, fmt.Sprintf(format, args...), nil)
}

// Warnf emits a warn-level message.
func (l *Logger) Warnf(component, format string, args ...any) {
	l.log(models.LevelWarn, component, fmt.Sprintf(format, args...), nil)
}

// Errorf emits an error-level message.
func (l *Logger) Errorf(component, format string, args ...any) {
	l.log(models.LevelError, component, fmt.Sprintf(format, args...), nil)
}
