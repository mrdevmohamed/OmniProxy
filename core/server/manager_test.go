package server

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"omniproxy/core/config"
	"omniproxy/core/log"
	"omniproxy/core/models"
	"omniproxy/core/secret"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	engine, err := config.New(
		config.NewFileStore(filepath.Join(t.TempDir(), "config.bin")),
		secret.NewInMemory(),
		log.NewNopLogger(),
	)
	if err != nil {
		t.Fatalf("config.New: %v", err)
	}
	return New(engine, log.NewNopLogger())
}

func vless(name string) *models.ServerProfile {
	return &models.ServerProfile{
		Name:     name,
		Protocol: models.ProtocolVLESS,
		Address:  "sv.example.com",
		Port:     443,
		UUID:     "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
	}
}

func TestCRUD(t *testing.T) {
	m := newTestManager(t)
	id, err := m.AddServer(vless("A"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.ListServers()) != 1 {
		t.Fatal("expected 1 server")
	}
	got, err := m.GetServer(id)
	if err != nil || got.Name != "A" {
		t.Fatalf("get failed: %+v, %v", got, err)
	}
	got.Name = "A2"
	if err := m.UpdateServer(got); err != nil {
		t.Fatal(err)
	}
	got2, _ := m.GetServer(id)
	if got2.Name != "A2" {
		t.Fatalf("update failed: %+v", got2)
	}
	if err := m.DeleteServer(id); err != nil {
		t.Fatal(err)
	}
	if len(m.ListServers()) != 0 {
		t.Fatal("expected empty")
	}
}

func TestExportImportSingleRoundTrip(t *testing.T) {
	m := newTestManager(t)
	id, err := m.AddServer(vless("Export-Me"))
	if err != nil {
		t.Fatal(err)
	}
	blob, err := m.ExportServers([]string{id})
	if err != nil {
		t.Fatal(err)
	}

	m2 := newTestManager(t)
	added, failed, errs, err := m2.ImportServers(blob)
	if err != nil || failed != 0 || added != 1 {
		t.Fatalf("import failed: added=%d failed=%d errs=%v err=%v", added, failed, errs, err)
	}
	servers := m2.ListServers()
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}
	if servers[0].Name != "Export-Me" || servers[0].Protocol != models.ProtocolVLESS {
		t.Fatalf("round trip mismatch: %+v", servers[0])
	}
	if servers[0].ID == id {
		t.Fatal("import must assign a fresh id")
	}
}

func TestExportImportMultiple(t *testing.T) {
	m := newTestManager(t)
	a, _ := m.AddServer(vless("A"))
	b, _ := m.AddServer(vless("B"))
	blob, err := m.ExportServers([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}

	m2 := newTestManager(t)
	added, failed, _, err := m2.ImportServers(blob)
	if err != nil || added != 2 || failed != 0 {
		t.Fatalf("bulk import: added=%d failed=%d err=%v", added, failed, err)
	}
	if len(m2.ListServers()) != 2 {
		t.Fatal("expected 2 servers after bulk import")
	}
}

func TestImportInvalid(t *testing.T) {
	m := newTestManager(t)
	added, failed, errs, err := m.ImportServers("this is not json at all")
	if err != nil {
		t.Fatalf("invalid import should not return err, got %v", err)
	}
	if added != 0 || failed != 1 || len(errs) != 1 {
		t.Fatalf("unexpected import result: added=%d failed=%d errs=%v", added, failed, errs)
	}

	// Wrong format id.
	added, failed, _, _ = m.ImportServers(`{"format":"clash","version":1,"server":{}}`)
	if added != 0 || failed != 1 {
		t.Fatalf("wrong format should fail: added=%d failed=%d", added, failed)
	}

	// Valid envelope but invalid profile (missing uuid).
	added, failed, _, _ = m.ImportServers(`{"format":"onnproxy","version":1,"server":{"name":"x","protocol":"vless","address":"a","port":443}}`)
	if added != 0 || failed != 1 {
		t.Fatalf("invalid profile should fail: added=%d failed=%d", added, failed)
	}
}

func TestExportEmpty(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.ExportServers(nil); err == nil {
		t.Fatal("expected error exporting nothing")
	}
}

func TestFavorite(t *testing.T) {
	m := newTestManager(t)
	id, _ := m.AddServer(vless("Fav"))
	if err := m.SetFavorite(id, true); err != nil {
		t.Fatal(err)
	}
	got, _ := m.GetServer(id)
	if !got.Favorite {
		t.Fatal("favorite not set")
	}
}

type fakeTester struct{ ms int }

func (f fakeTester) TestLatency(context.Context, *models.ServerProfile) (int, error) {
	return f.ms, nil
}

func TestLatency(t *testing.T) {
	m := newTestManager(t)
	id, _ := m.AddServer(vless("Latency"))
	m.SetLatencyTester(fakeTester{ms: 42})

	if _, err := m.TestLatency(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	got, _ := m.GetServer(id)
	if got.LastLatencyMS != 42 {
		t.Fatalf("expected 42ms, got %d", got.LastLatencyMS)
	}
	if got.LastTestedAt == nil || time.Since(*got.LastTestedAt) > time.Minute {
		t.Fatalf("LastTestedAt not set: %+v", got.LastTestedAt)
	}
}

func TestLatencyUnavailable(t *testing.T) {
	m := newTestManager(t)
	id, _ := m.AddServer(vless("NoLatency"))
	if _, err := m.TestLatency(context.Background(), id); err == nil {
		t.Fatal("expected ErrLatencyUnavailable")
	}
}
