package store

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"omniproxy/core/log"
	"omniproxy/core/models"
	"omniproxy/core/secret"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newTestRepo(t *testing.T) (*SQLiteServerRepository, *secret.InMemory) {
	t.Helper()
	secrets := secret.NewInMemory()
	repo := NewSQLiteServerRepository(openTestDB(t), secrets)
	return repo, secrets
}

func sampleVLESS() *models.ServerProfile {
	return &models.ServerProfile{
		Name:     "Frankfurt",
		Protocol: models.ProtocolVLESS,
		Address:  "fra1.example.com",
		Port:     443,
		UUID:     "11111111-2222-4333-8444-555555555555",
		Password: "hunter2-secret",
		TLS:      models.TLSConfig{Enabled: true, ServerName: "fra1.example.com"},
		Enabled:  true,
		Group:    "Work",
		Tags:     []string{"eu", "wireguard-tag"},
	}
}

func sampleSSH() *models.ServerProfile {
	return &models.ServerProfile{
		Name:          "Bastion",
		Protocol:      models.ProtocolSSH,
		Address:       "bastion.example.com",
		Port:          22,
		SSH:           models.SSHConfig{User: "ubuntu", PrivateKey: "-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----"},
		Username:      "ubuntu",
		Enabled:       true,
		LastLatencyMS: 42,
	}
}

func TestMigrateAppliesOnceAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.SQLDB().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("schema_migrations: %v", err)
	}
	if n != len(migrations) {
		t.Fatalf("expected %d migrations, got %d", len(migrations), n)
	}
	db.Close()

	if _, err := Open(path); err != nil {
		t.Fatalf("reopen: %v", err)
	}
}

func TestCreateGetRoundTrip(t *testing.T) {
	repo, secrets := newTestRepo(t)

	id, err := repo.Create(sampleVLESS())
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected assigned id")
	}

	got, err := repo.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	want := sampleVLESS()
	if got.Name != want.Name || got.UUID != want.UUID || got.Password != want.Password {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got, want)
	}
	if !got.Enabled || got.Group != "Work" || !reflect.DeepEqual(got.Tags, want.Tags) {
		t.Fatalf("management fields not round-tripped: %+v", got)
	}
	if !got.TLS.Enabled || got.TLS.ServerName != want.TLS.ServerName {
		t.Fatalf("tls not round-tripped: %+v", got.TLS)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not set: %+v", got)
	}

	// Secrets must live in the secret store, not in SQLite.
	if secrets.Len() == 0 {
		t.Fatal("expected secrets in the secret store")
	}
	var storedUUID, storedPassword string
	err = repo.db.SQLDB().QueryRow(
		`SELECT uuid, password FROM servers WHERE id = ?`, id,
	).Scan(&storedUUID, &storedPassword)
	if err != nil {
		t.Fatal(err)
	}
	if storedUUID != "" || storedPassword != "" {
		t.Fatalf("secrets leaked into sqlite: uuid=%q password=%q", storedUUID, storedPassword)
	}
}

func TestCreateSetsTimestampsAndEnabledPersists(t *testing.T) {
	repo, _ := newTestRepo(t)
	id, err := repo.Create(sampleVLESS())
	if err != nil {
		t.Fatal(err)
	}
	got, _ := repo.Get(id)
	if !got.Enabled {
		t.Fatal("enabled should persist true")
	}
}

func TestUpdateFullReplace(t *testing.T) {
	repo, secrets := newTestRepo(t)
	id, err := repo.Create(sampleVLESS())
	if err != nil {
		t.Fatal(err)
	}
	created, _ := repo.Get(id)

	updated := sampleVLESS()
	updated.ID = id
	updated.Name = "Frankfurt 2"
	updated.Password = "" // drop the password secret
	updated.Group = "Home"
	updated.Enabled = false
	updated.Tags = nil
	if err := repo.Update(updated); err != nil {
		t.Fatal(err)
	}

	got, _ := repo.Get(id)
	if got.Name != "Frankfurt 2" || got.Enabled || got.Group != "Home" || len(got.Tags) != 0 {
		t.Fatalf("update not applied: %+v", got)
	}
	if !got.CreatedAt.Equal(created.CreatedAt) {
		t.Fatal("created_at must be preserved on update")
	}
	if got.UpdatedAt.Before(created.UpdatedAt) {
		t.Fatal("updated_at must bump on update")
	}
	if got.UUID != sampleVLESS().UUID {
		t.Fatal("uuid secret should be preserved across update")
	}
	// Dropped password secret must be removed from the secret store.
	if v, err := secrets.Get(SecretStoreKey(id, RefPassword)); !errors.Is(err, secret.ErrNotFound) || v != "" {
		t.Fatalf("password secret not dropped: %q %v", v, err)
	}
}

func TestUpdateNotFound(t *testing.T) {
	repo, _ := newTestRepo(t)
	p := sampleVLESS()
	p.ID = "does-not-exist"
	if err := repo.Update(p); !errors.Is(err, ErrServerNotFound) {
		t.Fatalf("expected ErrServerNotFound, got %v", err)
	}
}

func TestDeleteRemovesRowAndSecrets(t *testing.T) {
	repo, secrets := newTestRepo(t)
	id, err := repo.Create(sampleVLESS())
	if err != nil {
		t.Fatal(err)
	}
	if secrets.Len() != 2 { // uuid + password
		t.Fatalf("expected 2 stored secrets, got %d", secrets.Len())
	}
	if err := repo.Delete(id); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(id); !errors.Is(err, ErrServerNotFound) {
		t.Fatalf("expected ErrServerNotFound, got %v", err)
	}
	if secrets.Len() != 0 {
		t.Fatalf("secrets not removed: %v", secrets.Keys())
	}
}

func TestDeleteNotFound(t *testing.T) {
	repo, _ := newTestRepo(t)
	if err := repo.Delete("nope"); !errors.Is(err, ErrServerNotFound) {
		t.Fatalf("expected ErrServerNotFound, got %v", err)
	}
}

func TestCreateValidation(t *testing.T) {
	repo, _ := newTestRepo(t)

	p := sampleVLESS()
	p.UUID = ""
	if _, err := repo.Create(p); !errors.Is(err, models.ErrValidation) {
		t.Fatalf("expected validation error for missing uuid, got %v", err)
	}

	p = sampleVLESS()
	p.Reality = &models.RealityConfig{Enabled: true, PublicKey: "abc"}
	if _, err := repo.Create(p); !errors.Is(err, models.ErrValidation) {
		t.Fatalf("expected validation error for reality enabled, got %v", err)
	}

	// Reserved reality fields may be stored while disabled.
	p = sampleVLESS()
	p.Reality = &models.RealityConfig{PublicKey: "abc", ShortID: "0123"}
	if _, err := repo.Create(p); err != nil {
		t.Fatalf("disabled reality should be allowed: %v", err)
	}
}

func TestListFilter(t *testing.T) {
	repo, _ := newTestRepo(t)
	vl := sampleVLESS()
	vl.Enabled = true
	vl.Group = "Work"
	ss := sampleSSH()
	ss.Enabled = true
	ss.Group = "Home"

	disabled := sampleVLESS()
	disabled.Name = "Frankfurt Off"
	disabled.Enabled = false
	disabled.Group = "Work"

	for _, p := range []*models.ServerProfile{vl, ss, disabled} {
		if _, err := repo.Create(p); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name  string
		query Query
		wantN int
	}{
		{"all", Query{}, 3},
		{"search name", Query{Search: "frank"}, 2},
		{"search address", Query{Search: "bastion"}, 1},
		{"search group", Query{Search: "work"}, 2},
		{"group exact", Query{Group: "Work"}, 2},
		{"protocol", Query{Protocol: "ssh"}, 1},
		{"enabled", Query{Enabled: boolPtr(true)}, 2},
		{"disabled", Query{Enabled: boolPtr(false)}, 1},
		{"favorite", Query{Favorite: boolPtr(true)}, 0},
	}
	for _, tc := range cases {
		got, err := repo.List(tc.query)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(got) != tc.wantN {
			t.Fatalf("%s: expected %d, got %d (%v)", tc.name, tc.wantN, len(got), names(got))
		}
	}
}

func TestListSort(t *testing.T) {
	repo, _ := newTestRepo(t)
	for _, p := range []*models.ServerProfile{sampleSSH(), sampleVLESS()} {
		if _, err := repo.Create(p); err != nil {
			t.Fatal(err)
		}
	}

	byName, err := repo.List(Query{Sort: SortName, Order: "asc"})
	if err != nil {
		t.Fatal(err)
	}
	if byName[0].Name != "Bastion" || byName[1].Name != "Frankfurt" {
		t.Fatalf("expected Bastion, Frankfurt; got %v", names(byName))
	}

	byLatency, err := repo.List(Query{Sort: SortLatency, Order: "desc"})
	if err != nil {
		t.Fatal(err)
	}
	if byLatency[0].LastLatencyMS != 42 || byLatency[1].LastLatencyMS != 0 {
		t.Fatalf("untested servers should sort last: %v", names(byLatency))
	}
}

func TestRedactorRegistration(t *testing.T) {
	db := openTestDB(t)
	secrets := secret.NewInMemory()

	repo1 := NewSQLiteServerRepository(db, secrets)
	id, err := repo1.Create(sampleVLESS())
	if err != nil {
		t.Fatal(err)
	}

	// A fresh repo with a fresh logger simulates a restart: loaded secrets must
	// be re-registered with the new logger's redactor.
	lg := log.NewNopLogger()
	repo2 := NewSQLiteServerRepository(db, secrets)
	repo2.SetRedactor(lg.Redactor())
	if _, err := repo2.Get(id); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(lg.Redactor().Redact("value hunter2-secret here"), "[REDACTED]") {
		t.Fatal("loaded secret not registered with redactor")
	}
	if !strings.Contains(lg.Redactor().Redact("11111111-2222-4333-8444-555555555555"), "[REDACTED]") {
		t.Fatal("loaded uuid not registered with redactor")
	}
}

// TestRedactorCoversCredentialFields verifies that a full-profile round-trip
// (Create, then Get from a fresh repo, as after a restart) registers every
// credential-bearing field with the logger redactor — not just the primary
// secrets (password/UUID/private key) but the SSH host key, Reality material,
// and WS host/path that the audit flagged as coverage gaps.
func TestRedactorCoversCredentialFields(t *testing.T) {
	db := openTestDB(t)
	secrets := secret.NewInMemory()
	lg := log.NewNopLogger()

	prof := sampleVLESS()
	prof.Transport = &models.TransportConfig{
		Type: models.TransportWS,
		Path: "/ws-path-secret",
		Host: "ws-host-secret",
	}
	prof.SSH = models.SSHConfig{HostKey: "ssh-host-key-secret"}
	prof.Reality = &models.RealityConfig{
		Enabled:   false, // Validate rejects Enabled; storage/redaction must still work
		PublicKey: "reality-public-secret",
		ShortID:   "abc123",
		SpiderX:   "/spider-x-secret",
	}

	repo := NewSQLiteServerRepository(db, secrets)
	repo.SetRedactor(lg.Redactor())
	id, err := repo.Create(prof)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a restart: a fresh repo + fresh logger re-registers on Get.
	lg2 := log.NewNopLogger()
	repo2 := NewSQLiteServerRepository(db, secrets)
	repo2.SetRedactor(lg2.Redactor())
	if _, err := repo2.Get(id); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"hunter2-secret",                       // password
		"11111111-2222-4333-8444-555555555555", // uuid
		"/ws-path-secret",                      // transport path
		"ws-host-secret",                       // transport host
		"ssh-host-key-secret",                  // ssh host key
		"reality-public-secret",                // reality public key
		"abc123",                               // reality short id
		"/spider-x-secret",                     // reality spiderX
	}
	for _, s := range want {
		if !strings.Contains(lg2.Redactor().Redact("value "+s+" here"), "[REDACTED]") {
			t.Fatalf("credential-bearing field %q not registered with the redactor", s)
		}
	}
}

func TestSSHRoundTrip(t *testing.T) {
	repo, _ := newTestRepo(t)
	id, err := repo.Create(sampleSSH())
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.SSH.User != "ubuntu" || got.SSH.PrivateKey != sampleSSH().SSH.PrivateKey {
		t.Fatalf("ssh not round-tripped: %+v", got.SSH)
	}
}

func boolPtr(b bool) *bool { return &b }

func names(servers []*models.ServerProfile) []string {
	out := make([]string, 0, len(servers))
	for _, s := range servers {
		out = append(out, s.Name)
	}
	return out
}

var _ ServerRepository = (*SQLiteServerRepository)(nil)
