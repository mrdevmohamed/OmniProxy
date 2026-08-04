package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"omniproxy/core/log"
	"omniproxy/core/models"
	"omniproxy/core/secret"
	"omniproxy/core/store"
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

func TestSettingsPersist(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(filepath.Join(dir, "config.bin"))
	secrets := secret.NewInMemory()

	e1, err := New(store, secrets, log.NewNopLogger())
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
	if e2.Settings().Theme != models.ThemeDark {
		t.Fatalf("settings not restored: %+v", e2.Settings())
	}
}

func TestDefaultSettings(t *testing.T) {
	e := newTestEngine(t, t.TempDir())
	if e.Settings().Theme != models.ThemeSystem {
		t.Fatalf("default theme should be system, got %q", e.Settings().Theme)
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

func TestUpdateSettingsIPv6ModeNormalize(t *testing.T) {
	e := newTestEngine(t, t.TempDir())

	got, err := e.UpdateSettings(models.AppSettings{IPv6Mode: "tunnelvision"})
	if err != nil {
		t.Fatal(err)
	}
	if got.IPv6Mode != models.IPv6ModePreferIPv4 {
		t.Fatalf("invalid ipv6 mode should normalize to prefer_ipv4, got %q", got.IPv6Mode)
	}

	for _, m := range []models.IPv6Mode{
		models.IPv6ModeAuto,
		models.IPv6ModePreferIPv4,
		models.IPv6ModeDisable,
		models.IPv6ModeEnable,
	} {
		got, err := e.UpdateSettings(models.AppSettings{IPv6Mode: m})
		if err != nil {
			t.Fatal(err)
		}
		if got.IPv6Mode != m {
			t.Fatalf("ipv6 mode %q should round-trip, got %q", m, got.IPv6Mode)
		}
	}

	// Defaults: a fresh engine starts with prefer_ipv4.
	e2 := newTestEngine(t, t.TempDir())
	if e2.Settings().IPv6Mode != models.IPv6ModePreferIPv4 {
		t.Fatalf("default ipv6 mode should be prefer_ipv4, got %q", e2.Settings().IPv6Mode)
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
	if _, err := e1.UpdateSettings(models.AppSettings{Theme: models.ThemeDark}); err != nil {
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
	if e2.Settings().Theme != models.ThemeSystem {
		t.Fatal("expected default settings after recovery")
	}
	// The corrupt file must have been backed up, not silently lost.
	matches, err := filepath.Glob(path + ".corrupt.*")
	if err != nil || len(matches) == 0 {
		t.Fatalf("expected corrupt backup file, got %v (%v)", matches, err)
	}
}

func TestLoadLegacyServers(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(filepath.Join(dir, "config.bin"))
	secrets := secret.NewInMemory()
	key, err := secret.GetOrCreateDataKey(secrets, "")
	if err != nil {
		t.Fatal(err)
	}

	id := models.NewID()
	legacy := legacyDoc{
		Version:  1,
		Settings: models.AppSettings{Theme: models.ThemeDark},
		Servers: []*persistedServer{
			{
				ServerProfile: &models.ServerProfile{
					ID:        id,
					Name:      "Frankfurt",
					Protocol:  models.ProtocolVLESS,
					Address:   "fra1.example.com",
					Port:      443,
					CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
				},
				SecretRefs: []string{store.RefUUID},
			},
		},
	}
	if err := secrets.Set(store.SecretStoreKey(id, store.RefUUID), sampleUUID); err != nil {
		t.Fatal(err)
	}

	plain, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := secret.Seal(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Save(sealed); err != nil {
		t.Fatal(err)
	}

	logger := log.NewNopLogger()
	got, err := LoadLegacyServers(fs, secrets, logger)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 legacy profile, got %d", len(got))
	}
	if got[0].ID != id || got[0].UUID != sampleUUID || got[0].Name != "Frankfurt" {
		t.Fatalf("profile not restored: %+v", got[0])
	}
	if !got[0].CreatedAt.Equal(legacy.Servers[0].CreatedAt) {
		t.Fatalf("created timestamp not preserved: %v vs %v", got[0].CreatedAt, legacy.Servers[0].CreatedAt)
	}

	// The settings-only engine must still load the same blob untouched.
	e, err := New(fs, secrets, logger)
	if err != nil {
		t.Fatal(err)
	}
	if e.Settings().Theme != models.ThemeDark {
		t.Fatalf("settings lost by settings-only load: %+v", e.Settings())
	}
}

func TestLoadLegacyServersSkipsMissingSecret(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(filepath.Join(dir, "config.bin"))
	secrets := secret.NewInMemory()
	key, err := secret.GetOrCreateDataKey(secrets, "")
	if err != nil {
		t.Fatal(err)
	}

	legacy := legacyDoc{
		Version: 1,
		Servers: []*persistedServer{
			{
				ServerProfile: &models.ServerProfile{
					ID:       models.NewID(),
					Name:     "NoSecret",
					Protocol: models.ProtocolVLESS,
					Address:  "example.com",
					Port:     443,
				},
				SecretRefs: []string{store.RefUUID},
			},
		},
	}
	plain, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := secret.Seal(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Save(sealed); err != nil {
		t.Fatal(err)
	}

	got, err := LoadLegacyServers(fs, secrets, log.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].UUID != "" {
		t.Fatalf("expected profile with empty uuid, got %+v", got)
	}
}

func TestLoadLegacyServersEmpty(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(filepath.Join(dir, "config.bin"))
	secrets := secret.NewInMemory()

	// Fresh store: no blob yet.
	got, err := LoadLegacyServers(fs, secrets, log.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no legacy profiles, got %d", len(got))
	}

	// Blob with no servers section.
	key, err := secret.GetOrCreateDataKey(secrets, "")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := json.Marshal(legacyDoc{Version: 1, Settings: models.DefaultAppSettings()})
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := secret.Seal(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Save(sealed); err != nil {
		t.Fatal(err)
	}
	got, err = LoadLegacyServers(fs, secrets, log.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no legacy profiles, got %d", len(got))
	}
}

var sampleUUID = "11111111-2222-4333-8444-555555555555"
