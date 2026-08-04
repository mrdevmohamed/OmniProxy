package api

import (
	"encoding/json"
	"errors"
	"testing"

	"omniproxy/core/models"
)

var errStubNotFound = errors.New("stub: not found")

type stubHandler struct{}

func (stubHandler) GetVersion() GetVersionResponse {
	return GetVersionResponse{Version: "1.0.0", EngineVersion: "v1.13.15", Platform: "linux"}
}
func (stubHandler) ListServers(req ServerListRequest) ServerListResponse {
	return ServerListResponse{Servers: []*models.ServerProfile{{Name: req.Search + ":" + string(req.Sort)}}}
}
func (stubHandler) GetServer(id string) (*models.ServerProfile, error) {
	if id == "missing" {
		return nil, errStubNotFound
	}
	return &models.ServerProfile{ID: id, Name: "stub"}, nil
}
func (stubHandler) AddServer(s *models.ServerProfile) (IDResponse, error) {
	return IDResponse{ID: "new"}, nil
}
func (stubHandler) UpdateServer(s *models.ServerProfile) (*models.ServerProfile, error) {
	return s, nil
}
func (stubHandler) DeleteServer(id string) error { return nil }
func (stubHandler) DuplicateServer(id string) (IDResponse, error) {
	if id == "missing" {
		return IDResponse{}, errStubNotFound
	}
	return IDResponse{ID: id + "-copy"}, nil
}
func (stubHandler) ImportServers(req ImportRequest) (ImportResponse, error) {
	return ImportResponse{Added: 1, Failed: 0}, nil
}
func (stubHandler) ExportServers(req ExportRequest) (ExportResponse, error) {
	return ExportResponse{Format: "onnproxy", Blob: "{}"}, nil
}
func (stubHandler) TestServerLatency(id string) (LatencyResponse, error) {
	return LatencyResponse{LatencyMS: 42}, nil
}
func (stubHandler) Connect(req ConnectRequest) (StateResponse, error) {
	return StateResponse{State: models.StateConnecting}, nil
}
func (stubHandler) Disconnect() (StateResponse, error) {
	return StateResponse{State: models.StateDisconnected}, nil
}
func (stubHandler) ConnectionState() StateResponse {
	return StateResponse{State: models.StateDisconnected}
}
func (stubHandler) GetLogs(req LogsRequest) (LogsResponse, error) { return LogsResponse{}, nil }
func (stubHandler) GetSettings() (SettingsResponse, error) {
	return SettingsResponse{Settings: models.DefaultAppSettings()}, nil
}
func (stubHandler) UpdateSettings(req SettingsRequest) (SettingsResponse, error) {
	return SettingsResponse{Settings: req.Settings}, nil
}
func (stubHandler) Subscribe(req SubscribeRequest) error { return nil }
func (stubHandler) Unsubscribe() error                   { return nil }

func newTestDispatcher() *Dispatcher {
	return &Dispatcher{
		Handler: stubHandler{},
		MapError: func(err error) *Error {
			if errors.Is(err, errStubNotFound) {
				return &Error{Code: ErrCodeNotFound, Message: err.Error()}
			}
			return nil
		},
	}
}

func decodeResponse(t *testing.T, raw []byte) Response {
	t.Helper()
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v\n%s", err, raw)
	}
	return resp
}

func TestDispatchGetVersion(t *testing.T) {
	resp := decodeResponse(t, newTestDispatcher().Dispatch(MethodGetVersion, []byte("{}")))
	if !resp.OK || resp.Error != nil {
		t.Fatalf("unexpected envelope: %+v", resp)
	}
	m := resp.Data.(map[string]any)
	if m["version"] != "1.0.0" || m["engineVersion"] != "v1.13.15" || m["platform"] != "linux" {
		t.Fatalf("unexpected version data: %v", m)
	}
}

func TestDispatchUnknownMethod(t *testing.T) {
	resp := decodeResponse(t, newTestDispatcher().Dispatch("bogus", []byte("{}")))
	if resp.OK || resp.Error == nil || resp.Error.Code != ErrCodeInvalidArgument {
		t.Fatalf("unexpected envelope: %+v", resp)
	}
}

func TestDispatchInvalidJSON(t *testing.T) {
	resp := decodeResponse(t, newTestDispatcher().Dispatch(MethodGetServer, []byte("{not json")))
	if resp.OK || resp.Error == nil || resp.Error.Code != ErrCodeInvalidArgument {
		t.Fatalf("unexpected envelope: %+v", resp)
	}
}

func TestDispatchErrorMapping(t *testing.T) {
	resp := decodeResponse(t, newTestDispatcher().Dispatch(MethodGetServer, []byte(`{"id":"missing"}`)))
	if resp.OK || resp.Error == nil || resp.Error.Code != ErrCodeNotFound {
		t.Fatalf("unexpected envelope: %+v", resp)
	}
}

func TestDispatchEmptyBodyAllowed(t *testing.T) {
	resp := decodeResponse(t, newTestDispatcher().Dispatch(MethodGetVersion, nil))
	if !resp.OK {
		t.Fatalf("empty body should be tolerated: %+v", resp)
	}
}

func TestDispatchConnectDefaults(t *testing.T) {
	resp := decodeResponse(t, newTestDispatcher().Dispatch(MethodConnect, []byte(`{"serverId":"abc"}`)))
	if !resp.OK {
		t.Fatalf("unexpected envelope: %+v", resp)
	}
	m := resp.Data.(map[string]any)
	if m["state"] != "connecting" {
		t.Fatalf("unexpected state: %v", m)
	}
}

func TestDispatchListServersQuery(t *testing.T) {
	resp := decodeResponse(t, newTestDispatcher().Dispatch(MethodListServers, []byte(`{"search":"fra","sort":"updatedAt"}`)))
	if !resp.OK {
		t.Fatalf("unexpected envelope: %+v", resp)
	}
	servers := resp.Data.(map[string]any)["servers"].([]any)
	got := servers[0].(map[string]any)["name"]
	if got != "fra:updatedAt" {
		t.Fatalf("query params not forwarded: %v", got)
	}
	// Empty body must still work (defaults).
	resp = decodeResponse(t, newTestDispatcher().Dispatch(MethodListServers, nil))
	if !resp.OK {
		t.Fatalf("empty listServers body failed: %+v", resp)
	}
}

func TestDispatchDuplicateServer(t *testing.T) {
	resp := decodeResponse(t, newTestDispatcher().Dispatch(MethodDuplicateServer, []byte(`{"id":"abc"}`)))
	if !resp.OK {
		t.Fatalf("unexpected envelope: %+v", resp)
	}
	if resp.Data.(map[string]any)["id"] != "abc-copy" {
		t.Fatalf("unexpected duplicate data: %v", resp.Data)
	}

	resp = decodeResponse(t, newTestDispatcher().Dispatch(MethodDuplicateServer, []byte(`{"id":"missing"}`)))
	if resp.OK || resp.Error == nil || resp.Error.Code != ErrCodeNotFound {
		t.Fatalf("unexpected envelope: %+v", resp)
	}
}

func TestDispatchExportServersFormat(t *testing.T) {
	resp := decodeResponse(t, newTestDispatcher().Dispatch(MethodExportServers, []byte(`{"format":"links","ids":["a"]}`)))
	if !resp.OK {
		t.Fatalf("unexpected envelope: %+v", resp)
	}
	m := resp.Data.(map[string]any)
	if m["format"] != "onnproxy" {
		t.Fatalf("stub ignores format, envelope format should be onnproxy: %v", m)
	}
}

func TestEventBus(t *testing.T) {
	bus := NewEventBus()
	var got []Event
	sub := bus.Subscribe(FuncEventSink(func(e Event) { got = append(got, e) }))

	bus.Publish(Event{Type: EventStateChanged, Data: "x"})
	bus.Publish(Event{Type: EventLogAppended, Data: "y"})
	if len(got) != 2 {
		t.Fatalf("expected 2 deliveries, got %d", len(got))
	}
	if got[0].Type != EventStateChanged || got[1].Type != EventLogAppended {
		t.Fatalf("unexpected events: %+v", got)
	}

	bus.Unsubscribe(sub)
	bus.Publish(Event{Type: EventStateChanged, Data: "z"})
	if len(got) != 2 {
		t.Fatalf("events should not be delivered after unsubscribe")
	}

	bus.Unsubscribe(sub) // idempotent, no panic
}
