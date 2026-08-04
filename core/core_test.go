package core

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"omniproxy/core/api"
	"omniproxy/core/log"
	"omniproxy/core/models"
	"omniproxy/core/secret"
	"omniproxy/engine"
)

type fakeRunner struct {
	mu       sync.Mutex
	startErr error
	starts   int
}

func (r *fakeRunner) Start(engine.Options) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.starts++
	return r.startErr
}
func (r *fakeRunner) Stop() error   { return nil }
func (r *fakeRunner) Running() bool { return false }

func newTestFacade(t *testing.T, runner *fakeRunner) *Facade {
	t.Helper()
	f, err := New(Config{
		Platform:    "linux",
		DataDir:     t.TempDir(),
		LogLevel:    models.LevelDebug,
		SecretStore: secret.NewInMemory(),
		Logger:      log.NewNopLogger(),
		Runner:      runner,
	})
	if err != nil {
		t.Fatalf("core.New: %v", err)
	}
	return f
}

// request dispatches a method with body and returns the decoded envelope.
func request(t *testing.T, f *Facade, method, body string) api.Response {
	t.Helper()
	var resp api.Response
	if err := json.Unmarshal(f.Dispatch(method, []byte(body)), &resp); err != nil {
		t.Fatalf("dispatch %s: invalid response JSON: %v", method, err)
	}
	return resp
}

func mustOK(t *testing.T, resp api.Response) {
	t.Helper()
	if !resp.OK {
		t.Fatalf("expected ok response, got %+v", resp)
	}
}

func serverBody(name string) string {
	return `{"server":{"name":"` + name + `","protocol":"vless","address":"10.0.0.1","port":443,"uuid":"11111111-1111-4111-8111-111111111111","enabled":true}}`
}

func waitState(t *testing.T, f *Facade, want models.ConnectionState) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp := request(t, f, api.MethodGetConnectionState, "{}")
		st, _ := resp.Data.(map[string]any)
		if st["state"] == string(want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	resp := request(t, f, api.MethodGetConnectionState, "{}")
	t.Fatalf("state %q not reached; got %v", want, resp.Data)
}

func TestFacadeVersion(t *testing.T) {
	f := newTestFacade(t, &fakeRunner{})
	resp := request(t, f, api.MethodGetVersion, "{}")
	mustOK(t, resp)
	m := resp.Data.(map[string]any)
	if m["version"] != Version || m["engineVersion"] != EngineVersion || m["platform"] != "linux" {
		t.Fatalf("unexpected version: %v", m)
	}
}

func TestFacadeServerCRUD(t *testing.T) {
	f := newTestFacade(t, &fakeRunner{})

	resp := request(t, f, api.MethodAddServer, serverBody("alpha"))
	mustOK(t, resp)
	id := resp.Data.(map[string]any)["id"].(string)
	if id == "" {
		t.Fatal("expected assigned id")
	}

	resp = request(t, f, api.MethodListServers, "{}")
	mustOK(t, resp)
	if servers := resp.Data.(map[string]any)["servers"].([]any); len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}

	resp = request(t, f, api.MethodGetServer, `{"id":"`+id+`"}`)
	mustOK(t, resp)
	if name := resp.Data.(map[string]any)["name"]; name != "alpha" {
		t.Fatalf("unexpected name: %v", name)
	}

	resp = request(t, f, api.MethodUpdateServer, `{"server":{"id":"`+id+`","name":"beta","protocol":"vless","address":"10.0.0.2","port":8443,"uuid":"11111111-1111-4111-8111-111111111111","enabled":true}}`)
	mustOK(t, resp)
	if name := resp.Data.(map[string]any)["name"]; name != "beta" {
		t.Fatalf("update did not persist: %v", resp.Data)
	}

	resp = request(t, f, api.MethodDeleteServer, `{"id":"`+id+`"}`)
	mustOK(t, resp)
	resp = request(t, f, api.MethodGetServer, `{"id":"`+id+`"}`)
	if resp.OK || resp.Error.Code != api.ErrCodeNotFound {
		t.Fatalf("expected not_found after delete, got %+v", resp)
	}
}

func TestFacadeServerValidation(t *testing.T) {
	f := newTestFacade(t, &fakeRunner{})
	resp := request(t, f, api.MethodAddServer, `{"server":{"name":"","protocol":"vless","address":"","port":0}}`)
	if resp.OK || resp.Error.Code != api.ErrCodeValidation {
		t.Fatalf("expected validation_failed, got %+v", resp)
	}
}

func TestFacadeImportExport(t *testing.T) {
	f := newTestFacade(t, &fakeRunner{})
	mustOK(t, request(t, f, api.MethodAddServer, serverBody("alpha")))
	mustOK(t, request(t, f, api.MethodAddServer, serverBody("beta")))

	resp := request(t, f, api.MethodExportServers, "{}")
	mustOK(t, resp)
	blob := resp.Data.(map[string]any)["blob"].(string)
	if blob == "" {
		t.Fatal("expected non-empty blob")
	}

	// Import the blob into a fresh facade.
	f2 := newTestFacade(t, &fakeRunner{})
	resp = request(t, f2, api.MethodImportServers, `{"source":{"kind":"file","data":`+jsonString(blob)+`}}`)
	mustOK(t, resp)
	m := resp.Data.(map[string]any)
	if m["added"].(float64) != 2 {
		t.Fatalf("expected 2 added, got %v", m)
	}

	resp = request(t, f2, api.MethodImportServers, `{"source":{"kind":"link","data":"garbage"}}`)
	mustOK(t, resp) // malformed blob reported as failed, not an error
	if m := resp.Data.(map[string]any); m["failed"].(float64) != 1 {
		t.Fatalf("expected 1 failed, got %v", m)
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestFacadeDuplicateServer(t *testing.T) {
	f := newTestFacade(t, &fakeRunner{})
	resp := request(t, f, api.MethodAddServer, serverBody("alpha"))
	mustOK(t, resp)
	id := resp.Data.(map[string]any)["id"].(string)

	resp = request(t, f, api.MethodDuplicateServer, `{"id":"`+id+`"}`)
	mustOK(t, resp)
	dupID := resp.Data.(map[string]any)["id"].(string)
	if dupID == id || dupID == "" {
		t.Fatalf("expected fresh id, got %q (original %q)", dupID, id)
	}

	// Duplicated profile has a copy name and works standalone.
	resp = request(t, f, api.MethodGetServer, `{"id":"`+dupID+`"}`)
	mustOK(t, resp)
	if name := resp.Data.(map[string]any)["name"]; name != "alpha (copy)" {
		t.Fatalf("unexpected copy name: %v", name)
	}

	resp = request(t, f, api.MethodDuplicateServer, `{"id":"nope"}`)
	if resp.OK || resp.Error.Code != api.ErrCodeNotFound {
		t.Fatalf("expected not_found, got %+v", resp)
	}
}

func TestFacadeListServersQuery(t *testing.T) {
	f := newTestFacade(t, &fakeRunner{})
	mustOK(t, request(t, f, api.MethodAddServer, serverBody("Alpha")))
	mustOK(t, request(t, f, api.MethodAddServer, serverBody("Bravo")))

	// Disable the first server.
	alphaID := ""
	{
		resp := request(t, f, api.MethodListServers, "{}")
		mustOK(t, resp)
		for _, s := range resp.Data.(map[string]any)["servers"].([]any) {
			sm := s.(map[string]any)
			if sm["name"] == "Alpha" {
				alphaID = sm["id"].(string)
			}
		}
	}
	mustOK(t, request(t, f, api.MethodUpdateServer,
		`{"server":{"id":"`+alphaID+`","name":"Alpha","protocol":"vless","address":"10.0.0.1","port":443,"uuid":"11111111-1111-4111-8111-111111111111","enabled":false}}`))

	resp := request(t, f, api.MethodListServers, `{"enabled":true,"sort":"name"}`)
	mustOK(t, resp)
	servers := resp.Data.(map[string]any)["servers"].([]any)
	if len(servers) != 1 || servers[0].(map[string]any)["name"] != "Bravo" {
		t.Fatalf("enabled filter failed: %v", servers)
	}

	resp = request(t, f, api.MethodListServers, `{"search":"alp"}`)
	mustOK(t, resp)
	servers = resp.Data.(map[string]any)["servers"].([]any)
	if len(servers) != 1 || servers[0].(map[string]any)["name"] != "Alpha" {
		t.Fatalf("search filter failed: %v", servers)
	}
}

func TestFacadeExportLinks(t *testing.T) {
	f := newTestFacade(t, &fakeRunner{})
	resp := request(t, f, api.MethodAddServer, serverBody("alpha"))
	mustOK(t, resp)
	id := resp.Data.(map[string]any)["id"].(string)

	resp = request(t, f, api.MethodExportServers, `{"format":"links","ids":["`+id+`"]}`)
	mustOK(t, resp)
	blob := resp.Data.(map[string]any)["blob"].(string)
	if blob == "" || blob[:6] != "vless:" {
		t.Fatalf("expected a vless link, got %q", blob)
	}

	// Unknown format is a validation error.
	resp = request(t, f, api.MethodExportServers, `{"format":"bogus"}`)
	if resp.OK || resp.Error.Code != api.ErrCodeValidation {
		t.Fatalf("expected validation_failed for bad format, got %+v", resp)
	}
}

func TestFacadeSettings(t *testing.T) {
	f := newTestFacade(t, &fakeRunner{})
	resp := request(t, f, api.MethodGetSettings, "{}")
	mustOK(t, resp)
	settings := resp.Data.(map[string]any)["settings"].(map[string]any)
	if settings["theme"] != "system" {
		t.Fatalf("unexpected defaults: %v", settings)
	}

	resp = request(t, f, api.MethodUpdateSettings, `{"settings":{"theme":"dark","connectionMode":"proxy","logLevel":"debug"}}`)
	mustOK(t, resp)
	settings = resp.Data.(map[string]any)["settings"].(map[string]any)
	if settings["theme"] != "dark" || settings["connectionMode"] != "proxy" {
		t.Fatalf("settings not persisted: %v", settings)
	}
}

func TestFacadeConnectDisconnect(t *testing.T) {
	runner := &fakeRunner{}
	f := newTestFacade(t, runner)
	mustOK(t, request(t, f, api.MethodAddServer, serverBody("alpha")))
	id := request(t, f, api.MethodListServers, "{}").Data.(map[string]any)["servers"].([]any)[0].(map[string]any)["id"].(string)

	resp := request(t, f, api.MethodConnect, `{"serverId":"`+id+`","mode":"proxy"}`)
	mustOK(t, resp)
	if st := resp.Data.(map[string]any)["state"]; st != "connecting" {
		t.Fatalf("connect response state = %v", st)
	}
	waitState(t, f, models.StateConnected)

	// deleteServer while connected must be rejected.
	resp = request(t, f, api.MethodDeleteServer, `{"id":"`+id+`"}`)
	if resp.OK || resp.Error.Code != api.ErrCodeConnected {
		t.Fatalf("expected connected error, got %+v", resp)
	}
	resp = request(t, f, api.MethodUpdateServer, `{"server":{"id":"`+id+`","name":"alpha","protocol":"vless","address":"10.0.0.1","port":443,"uuid":"11111111-1111-4111-8111-111111111111","enabled":true}}`)
	if resp.OK || resp.Error.Code != api.ErrCodeConnected {
		t.Fatalf("expected connected error on update, got %+v", resp)
	}

	resp = request(t, f, api.MethodDisconnect, "{}")
	mustOK(t, resp)
	waitState(t, f, models.StateDisconnected)

	// Now delete is allowed.
	resp = request(t, f, api.MethodDeleteServer, `{"id":"`+id+`"}`)
	mustOK(t, resp)
}

func TestFacadeConnectUnknownServer(t *testing.T) {
	f := newTestFacade(t, &fakeRunner{})
	resp := request(t, f, api.MethodConnect, `{"serverId":"nope","mode":"proxy"}`)
	if resp.OK || resp.Error.Code != api.ErrCodeNotFound {
		t.Fatalf("expected not_found, got %+v", resp)
	}
}

func TestFacadeConnectDisabled(t *testing.T) {
	f := newTestFacade(t, &fakeRunner{})
	resp := request(t, f, api.MethodAddServer,
		`{"server":{"name":"off","protocol":"vless","address":"10.0.0.1","port":443,"uuid":"11111111-1111-4111-8111-111111111111","enabled":false}}`)
	mustOK(t, resp)
	id := resp.Data.(map[string]any)["id"].(string)

	resp = request(t, f, api.MethodConnect, `{"serverId":"`+id+`","mode":"proxy"}`)
	if resp.OK || resp.Error.Code != api.ErrCodeValidation {
		t.Fatalf("expected validation_failed for disabled server, got %+v", resp)
	}
}

func TestFacadeEvents(t *testing.T) {
	runner := &fakeRunner{}
	f := newTestFacade(t, runner)

	var mu sync.Mutex
	var got []api.Event
	f.SetEventSink(api.FuncEventSink(func(e api.Event) {
		mu.Lock()
		got = append(got, e)
		mu.Unlock()
	}))

	mustOK(t, request(t, f, api.MethodSubscribe, `{"events":["stateChanged","logAppended"]}`))
	mustOK(t, request(t, f, api.MethodAddServer, serverBody("alpha")))

	mu.Lock()
	count := len(got)
	mu.Unlock()
	if count == 0 {
		t.Fatal("expected logAppended events after subscribe")
	}
	hasLog := false
	mu.Lock()
	for _, e := range got {
		if e.Type == api.EventLogAppended {
			hasLog = true
		}
	}
	mu.Unlock()
	if !hasLog {
		t.Fatal("expected a logAppended event")
	}

	// Unsubscribe: further activity must not reach the sink.
	mustOK(t, request(t, f, api.MethodUnsubscribe, "{}"))
	got = nil
	mustOK(t, request(t, f, api.MethodAddServer, serverBody("gamma")))
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	count = len(got)
	mu.Unlock()
	if count != 0 {
		t.Fatalf("expected no events after unsubscribe, got %d", count)
	}
}

func TestFacadeLogs(t *testing.T) {
	f := newTestFacade(t, &fakeRunner{})
	mustOK(t, request(t, f, api.MethodAddServer, serverBody("alpha")))
	resp := request(t, f, api.MethodGetLogs, "{}")
	mustOK(t, resp)
	logs := resp.Data.(map[string]any)["logs"].([]any)
	if len(logs) == 0 {
		t.Fatal("expected log entries")
	}
}

func TestFacadeUnknownMethod(t *testing.T) {
	f := newTestFacade(t, &fakeRunner{})
	resp := request(t, f, "nope", "{}")
	if resp.OK || resp.Error.Code != api.ErrCodeInvalidArgument {
		t.Fatalf("expected invalid_argument, got %+v", resp)
	}
}
