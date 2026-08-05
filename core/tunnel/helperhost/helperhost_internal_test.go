package helperhost

import "testing"

func TestPkexecInvoker(t *testing.T) {
	t.Setenv("PKEXEC_UID", "")
	if _, ok := pkexecInvoker(); ok {
		t.Fatal("expected no invoker when PKEXEC_UID is unset")
	}

	t.Setenv("PKEXEC_UID", "1000")
	uid, ok := pkexecInvoker()
	if !ok || uid != 1000 {
		t.Fatalf("expected uid 1000, got %d (ok=%v)", uid, ok)
	}

	t.Setenv("PKEXEC_UID", "not-a-number")
	if _, ok := pkexecInvoker(); ok {
		t.Fatal("expected no invoker for malformed PKEXEC_UID")
	}
}
