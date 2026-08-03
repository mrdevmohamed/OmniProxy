package log

import (
	"strings"
	"testing"

	"omniproxy/core/models"
)

func TestLoggerLevelFiltering(t *testing.T) {
	l := NewLogger(models.LevelInfo)
	l.Errorf("vpn", "connect failed")
	l.Infof("vpn", "connected")
	l.Debugf("vpn", "detail")

	entries := l.LogsAfter(0, 100)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries at info level, got %d", len(entries))
	}
	if entries[0].Message != "connect failed" || entries[1].Message != "connected" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

func TestLoggerSequencingAndLimit(t *testing.T) {
	l := NewNopLogger()
	for i := 0; i < 5; i++ {
		l.Infof("vpn", "msg %d", i)
	}
	all := l.LogsAfter(0, 100)
	if len(all) != 5 {
		t.Fatalf("expected 5, got %d", len(all))
	}
	if all[0].Seq != 1 || all[4].Seq != 5 {
		t.Fatalf("bad sequence numbers: %+v", all)
	}
	after := l.LogsAfter(3, 100)
	if len(after) != 2 || after[0].Seq != 4 {
		t.Fatalf("since(3) should return seq 4,5: %+v", after)
	}
	limited := l.LogsAfter(0, 2)
	if len(limited) != 2 {
		t.Fatalf("limit failed: %d", len(limited))
	}
}

func TestRingBufferDropsOldest(t *testing.T) {
	rb := NewRingBuffer(3)
	for i := 0; i < 5; i++ {
		rb.Append(models.LogEntry{Message: string(rune('a' + i))})
	}
	entries := rb.Since(0, 100)
	if len(entries) != 3 {
		t.Fatalf("expected 3 retained, got %d", len(entries))
	}
	if entries[0].Message != "c" {
		t.Fatalf("expected oldest dropped, got first=%q", entries[0].Message)
	}
}

func TestLoggerRedactsMessageAndContext(t *testing.T) {
	l := NewNopLogger()
	l.Redactor().Add("SuperSecretToken")
	l.Log(models.LevelInfo, "vpn", "hello SuperSecretToken", map[string]any{
		"token":  "SuperSecretToken",
		"port":   443,
		"nested": map[string]any{"key": "SuperSecretToken"},
	})
	entries := l.LogsAfter(0, 1)
	if len(entries) != 1 {
		t.Fatal("expected one entry")
	}
	e := entries[0]
	if strings.Contains(e.Message, "SuperSecretToken") {
		t.Fatalf("message leaked: %q", e.Message)
	}
	if !strings.Contains(e.Context["token"].(string), "[REDACTED]") {
		t.Fatalf("context leaked: %+v", e.Context)
	}
	if nested := e.Context["nested"].(map[string]any); strings.Contains(nested["key"].(string), "SuperSecretToken") {
		t.Fatalf("nested context leaked: %+v", e.Context)
	}
}
