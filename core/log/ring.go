package log

import (
	"sync"

	"omniproxy/core/models"
)

// RingBuffer keeps the most recent log entries for the UI log viewer, with
// monotonically increasing sequence numbers for incremental pulls (getLogs).
type RingBuffer struct {
	mu      sync.Mutex
	entries []models.LogEntry
	max     int
	seq     uint64
}

// NewRingBuffer returns a buffer holding at most max entries.
func NewRingBuffer(max int) *RingBuffer {
	if max < 1 {
		max = 1
	}
	return &RingBuffer{max: max}
}

// Append adds an entry, assigning its sequence number, and drops the oldest
// entry when the buffer is full.
func (rb *RingBuffer) Append(e models.LogEntry) uint64 {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.seq++
	e.Seq = rb.seq
	rb.entries = append(rb.entries, e)
	if len(rb.entries) > rb.max {
		drop := len(rb.entries) - rb.max
		kept := make([]models.LogEntry, 0, rb.max)
		kept = append(kept, rb.entries[drop:]...)
		rb.entries = kept
	}
	return e.Seq
}

// Since returns up to limit entries with Seq > afterSeq, oldest first.
func (rb *RingBuffer) Since(afterSeq uint64, limit int) []models.LogEntry {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if limit <= 0 || limit > rb.max {
		limit = rb.max
	}
	out := make([]models.LogEntry, 0, limit)
	for _, e := range rb.entries {
		if e.Seq <= afterSeq {
			continue
		}
		out = append(out, e)
		if len(out) >= limit {
			break
		}
	}
	return out
}
