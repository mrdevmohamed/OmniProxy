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
	mu      sync.RWMutex
	secrets map[string]string // lowercase secret -> last registered casing
	re      *regexp.Regexp
}

// NewRedactor returns an empty redactor (matches nothing).
func NewRedactor() *Redactor {
	return &Redactor{
		secrets: make(map[string]string),
		re:      regexp.MustCompile("a^"),
	}
}

// Add registers secrets for redaction. Registration accumulates: every secret
// added so far stays redacted (later calls never drop earlier ones).
func (r *Redactor) Add(secrets ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, s := range secrets {
		s = strings.TrimSpace(s)
		if len(s) < 3 {
			continue
		}
		r.secrets[strings.ToLower(s)] = s
	}
	r.re = r.compile()
}

func (r *Redactor) compile() *regexp.Regexp {
	if len(r.secrets) == 0 {
		return regexp.MustCompile("a^")
	}
	parts := make([]string, 0, len(r.secrets))
	for _, original := range r.secrets {
		parts = append(parts, regexp.QuoteMeta(original))
	}
	return regexp.MustCompile("(?i)" + strings.Join(parts, "|"))
}

// Redact replaces every registered secret in s with the redaction marker.
func (r *Redactor) Redact(s string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.re.ReplaceAllString(s, redactionMarker)
}
