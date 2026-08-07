package engine

import (
	sblog "github.com/sagernet/sing-box/log"
)

// Level mirrors the sing-box log levels (1:1).
type Level int

const (
	LevelTrace Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
	LevelPanic
)

// String returns the sing-box config level name.
func (l Level) String() string {
	switch l {
	case LevelTrace:
		return "trace"
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	case LevelFatal:
		return "fatal"
	case LevelPanic:
		return "panic"
	default:
		return "info"
	}
}

// LogSink receives engine log messages. The core implements it with its
// redacting logger so credentials never reach logs.
type LogSink interface {
	WriteMessage(level Level, message string)
}

// platformLogWriter adapts a LogSink to sing-box's log.PlatformWriter.
type platformLogWriter struct {
	sink LogSink
}

func (w platformLogWriter) WriteMessage(level sblog.Level, message string) {
	if w.sink == nil {
		return
	}
	w.sink.WriteMessage(levelFromBox(level), message)
}

// levelFromBox maps a sing-box log level onto the engine level. The two
// enums are ordered in opposite directions (sing-box: Panic=0 .. Trace=6;
// engine: Trace=0 .. Panic=6), so a direct numeric cast would swap every
// severity — an engine ERROR line would surface as INFO and vice versa.
func levelFromBox(level sblog.Level) Level {
	switch level {
	case sblog.LevelPanic:
		return LevelPanic
	case sblog.LevelFatal:
		return LevelFatal
	case sblog.LevelError:
		return LevelError
	case sblog.LevelWarn:
		return LevelWarn
	case sblog.LevelInfo:
		return LevelInfo
	case sblog.LevelDebug:
		return LevelDebug
	case sblog.LevelTrace:
		return LevelTrace
	default:
		return LevelInfo
	}
}
