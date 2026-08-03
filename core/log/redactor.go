package log

import (
	"regexp"
	"strings"
	"sync"
)

const redactionMarker = "[REDACTED]"

// Redactor replaces registered secrets inside log messages before they reach
// any sink or subscriber. Registration is additive and case-insensitive;
// secrets shorter than 3 characters are ignored to avoid mangling common words.
type Redactor struct {
	mu sync.RWMutex
	re *regexp.Regexp
}

// NewRedactor returns an empty redactor (matches nothing).
func NewRedactor() *Redactor {
	return &Redactor{re: regexp.MustCompile("a^")}
}

// Add registers secrets for redaction.
func (r *Redactor) Add(secrets ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var parts []string
	seen := make(map[string]bool)
	for _, s := range secrets {
		s = strings.TrimSpace(s)
		if len(s) < 3 {
			continue
		}
		low := strings.ToLower(s)
		if seen[low] {
			continue
		}
		seen[low] = true
		parts = append(parts, regexp.QuoteMeta(s))
	}
	if len(parts) == 0 {
		return
	}
	r.re = regexp.MustCompile("(?i)" + strings.Join(parts, "|"))
}

// Redact replaces every registered secret in s with the redaction marker.
func (r *Redactor) Redact(s string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.re.ReplaceAllString(s, redactionMarker)
}
