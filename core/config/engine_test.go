package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"omniproxy/core/log"
	"omniproxy/core/models"
	"omniproxy/core/secret"
)

func newTestEngine(t *testing.T, dir string) *Engine {
	t.Helper()
	store := NewFileStore(filepath.Join(dir, "config.bin"))
	secrets := secret.NewInMemory()
	e, err := New(store, secrets, log.NewNopLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return e
}

func sampleVLESS() *models.ServerProfile {
	return &models.ServerProfile{
		Name:     "Frankfurt",
		Protocol: models.ProtocolVLESS,
		Address:  "fra1.example.com",
		Port:     443,
		UUID:     "11111111-2222-4333-8444-555555555555",
		TLS:      models.TLSConfig{Enabled: true, ServerName: "fra1.example.com"},
	}
}

func TestAddListGetDelete(t *testing.T) {
	e := newTestEngine(t, t.TempDir())
	id, err := e.AddServer(sampleVLESS())
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected an id")
	}
	got, err := e.GetServer(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Frankfurt" || got.UUID != sampleVLESS().UUID {
		t.Fatalf("unexpected profile: %+v", got)
	}
	servers := e.ListServers()
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}
	if err := e.DeleteServer(id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.GetServer(id); !errors.Is(err, ErrServerNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(filepath.Join(dir, "config.bin"))
	secrets := secret.NewInMemory()

	e1, err := New(store, secrets, log.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}
	id, err := e1.AddServer(sampleVLESS())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e1.UpdateSettings(models.AppSettings{Theme: models.ThemeDark}); err != nil {
		t.Fatal(err)
	}

	e2, err := New(store, secrets, log.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}
	got, err := e2.GetServer(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.UUID != sampleVLESS().UUID {
		t.Fatalf("secret not restored after reload: %+v", got)
	}
	if e2.Settings().Theme != models.ThemeDark {
		t.Fatalf("settings not restored: %+v", e2.Settings())
	}
}

func TestSecretsNotPersistedInPlaintext(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(filepath.Join(dir, "config.bin"))
	secrets := secret.NewInMemory()

	e, err := New(store, secrets, log.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.AddServer(sampleVLESS()); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "config.bin"))
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{sampleVLESS().UUID, sampleVLESS().Name, "fra1.example.com"} {
		if strings.Contains(string(raw), needle) {
			t.Fatalf("plaintext leak of %q in store", needle)
		}
	}
	if len(raw) == 0 {
		t.Fatal("store should not be empty")
	}
}

func TestUpdateClearsSupersededSecrets(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(filepath.Join(dir, "config.bin"))
	secrets := secret.NewInMemory()
	e, err := New(store, secrets, log.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}
	id, err := e.AddServer(sampleVLESS())
	if err != nil {
		t.Fatal(err)
	}

	// Change protocol to one with no credential fields; the UUID secret must
	// be removed from secure storage.
	upd := &models.ServerProfile{
		ID:        id,
		Name:      "Frankfurt",
		Protocol:  models.ProtocolSOCKS5,
		Address:   "fra1.example.com",
		Port:      1080,
		Username:  "user",
		Password:  "socks-pass",
		CreatedAt: sampleVLESS().CreatedAt,
	}
	if err := e.UpdateServer(upd); err != nil {
		t.Fatal(err)
	}
	refs := secretRefsFor(mustGet(t, e, id))
	if len(refs) != 1 || refs[0] != RefPassword {
		t.Fatalf("expected only password ref after protocol change, got %v", refs)
	}
	if s := secrets.Len(); s != 2 { // data key + socks-pass
		t.Fatalf("expected 2 entries in store, got %d", s)
	}

	// Now clear the password too (socks5 allows empty creds).
	upd.Password = ""
	if err := e.UpdateServer(upd); err != nil {
		t.Fatal(err)
	}
	refs = secretRefsFor(mustGet(t, e, id))
	if len(refs) != 0 {
		t.Fatalf("expected no secret refs after clearing password, got %v", refs)
	}
	if s := secrets.Len(); s != 1 { // only the data key remains
		t.Fatalf("expected only data key in store, got %d entries", s)
	}
}

func TestDeleteClearsSecrets(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(filepath.Join(dir, "config.bin"))
	secrets := secret.NewInMemory()
	e, err := New(store, secrets, log.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}
	id, err := e.AddServer(sampleVLESS())
	if err != nil {
		t.Fatal(err)
	}
	if err := e.DeleteServer(id); err != nil {
		t.Fatal(err)
	}
	if s := secrets.Len(); s != 1 {
		t.Fatalf("expected only data key left, got %d entries", s)
	}
}

func TestCorruptionRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.bin")
	store := NewFileStore(path)
	secrets := secret.NewInMemory()

	e1, err := New(store, secrets, log.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e1.AddServer(sampleVLESS()); err != nil {
		t.Fatal(err)
	}

	// Tamper with the sealed blob.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)/2] ^= 0xff
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	e2, err := New(store, secrets, log.NewNopLogger())
	if err != nil {
		t.Fatalf("tampered store should recover, got %v", err)
	}
	if len(e2.ListServers()) != 0 {
		t.Fatal("expected empty config after recovery")
	}
	// The corrupt file must have been backed up, not silently lost.
	matches, err := filepath.Glob(path + ".corrupt.*")
	if err != nil || len(matches) == 0 {
		t.Fatalf("expected corrupt backup file, got %v (%v)", matches, err)
	}
}

func TestAddValidation(t *testing.T) {
	e := newTestEngine(t, t.TempDir())
	bad := sampleVLESS()
	bad.Port = 70000
	if _, err := e.AddServer(bad); !errors.Is(err, models.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
	unknown := sampleVLESS()
	unknown.Protocol = "tunnelbears"
	if _, err := e.AddServer(unknown); !errors.Is(err, models.ErrValidation) {
		t.Fatalf("expected validation error for unknown protocol, got %v", err)
	}
}

func TestUpdateSettingsNormalize(t *testing.T) {
	e := newTestEngine(t, t.TempDir())
	got, err := e.UpdateSettings(models.AppSettings{Theme: "neon"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != models.ThemeSystem {
		t.Fatalf("invalid theme should normalize to system, got %q", got.Theme)
	}
}

func mustGet(t *testing.T, e *Engine, id string) *models.ServerProfile {
	t.Helper()
	p, err := e.GetServer(id)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
