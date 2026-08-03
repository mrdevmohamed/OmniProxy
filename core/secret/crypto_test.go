package secret

import (
	"bytes"
	"errors"
	"testing"
)

func TestInMemoryCRUD(t *testing.T) {
	s := NewInMemory()
	if _, err := s.Get("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := s.Set("k", "v"); err != nil {
		t.Fatal(err)
	}
	v, err := s.Get("k")
	if err != nil || v != "v" {
		t.Fatalf("got %q, %v", v, err)
	}
	if err := s.Delete("missing"); err != nil {
		t.Fatalf("delete missing should be nil, got %v", err)
	}
	if err := s.Delete("k"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestSealOpenRoundTrip(t *testing.T) {
	key, err := NewDataKey()
	if err != nil {
		t.Fatal(err)
	}
	ct, err := Seal(key, []byte("sensitive payload"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ct, []byte("sensitive")) {
		t.Fatal("ciphertext leaks plaintext")
	}
	pt, err := Open(key, ct)
	if err != nil {
		t.Fatal(err)
	}
	if string(pt) != "sensitive payload" {
		t.Fatalf("round trip mismatch: %q", pt)
	}
}

func TestOpenDetectsTampering(t *testing.T) {
	key, _ := NewDataKey()
	ct, _ := Seal(key, []byte("payload"))
	ct[len(ct)-1] ^= 0xff
	if _, err := Open(key, ct); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("expected ErrCorrupt on tamper, got %v", err)
	}
}

func TestOpenWrongKey(t *testing.T) {
	key1, _ := NewDataKey()
	key2, _ := NewDataKey()
	ct, _ := Seal(key1, []byte("payload"))
	if _, err := Open(key2, ct); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("expected ErrCorrupt on wrong key, got %v", err)
	}
}

func TestGetOrCreateDataKeyStable(t *testing.T) {
	s := NewInMemory()
	k1, err := GetOrCreateDataKey(s, "")
	if err != nil {
		t.Fatal(err)
	}
	k2, err := GetOrCreateDataKey(s, "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(k1, k2) {
		t.Fatal("data key must be stable across calls")
	}
	if len(k1) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(k1))
	}
}
