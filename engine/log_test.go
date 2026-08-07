package engine

import (
	"testing"

	sblog "github.com/sagernet/sing-box/log"
)

// TestLevelFromBox guards the sing-box -> engine level mapping. The two enums
// have reversed orderings, so a direct numeric cast would turn INFO into WARN
// and ERROR into INFO.
func TestLevelFromBox(t *testing.T) {
	cases := []struct {
		box  sblog.Level
		want Level
	}{
		{sblog.LevelPanic, LevelPanic},
		{sblog.LevelFatal, LevelFatal},
		{sblog.LevelError, LevelError},
		{sblog.LevelWarn, LevelWarn},
		{sblog.LevelInfo, LevelInfo},
		{sblog.LevelDebug, LevelDebug},
		{sblog.LevelTrace, LevelTrace},
	}
	for _, c := range cases {
		if got := levelFromBox(c.box); got != c.want {
			t.Errorf("levelFromBox(%d) = %v, want %v", c.box, got, c.want)
		}
	}
}

func TestLevelFromBoxDefault(t *testing.T) {
	if got := levelFromBox(sblog.Level(0xFF)); got != LevelInfo {
		t.Errorf("levelFromBox(unknown) = %v, want LevelInfo", got)
	}
}

// TestPlatformLogWriterForwards ensures the adapter also forwards the level
// through the mapping and passes messages through verbatim.
func TestPlatformLogWriterForwards(t *testing.T) {
	got := make([]struct {
		level   Level
		message string
	}, 0, 1)
	w := platformLogWriter{sink: logSinkFunc(func(l Level, m string) {
		got = append(got, struct {
			level   Level
			message string
		}{l, m})
	})}
	w.WriteMessage(sblog.LevelError, "boom")
	if len(got) != 1 {
		t.Fatalf("expected 1 write, got %d", len(got))
	}
	if got[0].level != LevelError {
		t.Errorf("forwarded level = %v, want LevelError", got[0].level)
	}
	if got[0].message != "boom" {
		t.Errorf("forwarded message = %q, want %q", got[0].message, "boom")
	}
}

// logSinkFunc adapts a func to LogSink for tests.
type logSinkFunc func(level Level, message string)

func (f logSinkFunc) WriteMessage(level Level, message string) { f(level, message) }
