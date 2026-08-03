package log

import (
	"strings"
	"testing"
)

func TestRedactorReplacesSecrets(t *testing.T) {
	r := NewRedactor()
	r.Add("p@ssw0rd!secret")
	out := r.Redact("connecting to server with password p@ssw0rd!secret now")
	if strings.Contains(out, "p@ssw0rd!secret") {
		t.Fatalf("secret leaked: %q", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("expected marker in %q", out)
	}
}

func TestRedactorCaseInsensitive(t *testing.T) {
	r := NewRedactor()
	r.Add("SecretUUIDvalue")
	out := r.Redact("token is secretuuidvalue but also SECRETUUIDVALUE")
	if strings.Contains(strings.ToLower(out), "secretuuidvalue") {
		t.Fatalf("case-insensitive leak: %q", out)
	}
}

func TestRedactorIgnoresShortAndDuplicateSecrets(t *testing.T) {
	r := NewRedactor()
	r.Add("ab", "the-secret", "THE-SECRET")
	out := r.Redact("keep ab plain, redact the-secret here")
	if !strings.Contains(out, "ab") {
		t.Fatalf("short secret should be ignored: %q", out)
	}
	if strings.Contains(out, "the-secret") {
		t.Fatalf("secret leaked: %q", out)
	}
	if strings.Count(out, "[REDACTED]") != 1 {
		t.Fatalf("duplicate secret should redact once: %q", out)
	}
}
