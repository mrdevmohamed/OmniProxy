package api

import (
	"bytes"
	"encoding/json"
	"errors"
)

// Dispatcher routes a request by method name onto the Handler and builds the
// canonical response envelope. MapError translates a domain error into the
// contract's error codes (core provides it).
type Dispatcher struct {
	Handler
	MapError func(err error) *Error
}

// Dispatch processes method with requestJSON and returns the envelope JSON.
func (d *Dispatcher) Dispatch(method string, requestJSON []byte) []byte {
	resp := d.dispatch(method, requestJSON)
	b, err := json.Marshal(resp)
	if err != nil {
		b, _ = json.Marshal(Response{OK: false, Error: &Error{Code: ErrCodeInternal, Message: "internal: encode response"}})
	}
	return b
}

func (d *Dispatcher) dispatch(method string, requestJSON []byte) Response {
	decode := func(v any) error {
		if len(bytes.TrimSpace(requestJSON)) == 0 {
			return nil
		}
		return json.Unmarshal(requestJSON, v)
	}
	invalid := func(err error) Response {
		return Response{OK: false, Error: &Error{Code: ErrCodeInvalidArgument, Message: err.Error()}}
	}

	switch method {
	case MethodGetVersion:
		return d.ok(d.GetVersion())
	case MethodListServers:
		return d.ok(d.ListServers())
	case MethodGetServer:
		var req IDRequest
		if err := decode(&req); err != nil {
			return invalid(err)
		}
		return d.result(d.GetServer(req.ID))
	case MethodAddServer:
		var req ServerRequest
		if err := decode(&req); err != nil {
			return invalid(err)
		}
		if req.Server == nil {
			return invalid(errors.New("server is required"))
		}
		return d.result(d.AddServer(req.Server))
	case MethodUpdateServer:
		var req ServerRequest
		if err := decode(&req); err != nil {
			return invalid(err)
		}
		if req.Server == nil {
			return invalid(errors.New("server is required"))
		}
		return d.result(d.UpdateServer(req.Server))
	case MethodDeleteServer:
		var req IDRequest
		if err := decode(&req); err != nil {
			return invalid(err)
		}
		return d.result(nil, d.DeleteServer(req.ID))
	case MethodImportServers:
		var req ImportRequest
		if err := decode(&req); err != nil {
			return invalid(err)
		}
		return d.result(d.ImportServers(req))
	case MethodExportServers:
		var req ExportRequest
		if err := decode(&req); err != nil {
			return invalid(err)
		}
		return d.result(d.ExportServers(req))
	case MethodTestServerLatency:
		var req LatencyRequest
		if err := decode(&req); err != nil {
			return invalid(err)
		}
		return d.result(d.TestServerLatency(req.ID))
	case MethodConnect:
		var req ConnectRequest
		if err := decode(&req); err != nil {
			return invalid(err)
		}
		return d.result(d.Connect(req))
	case MethodDisconnect:
		return d.result(d.Disconnect())
	case MethodGetConnectionState:
		return d.ok(d.ConnectionState())
	case MethodGetLogs:
		var req LogsRequest
		if err := decode(&req); err != nil {
			return invalid(err)
		}
		return d.result(d.GetLogs(req))
	case MethodGetSettings:
		return d.result(d.GetSettings())
	case MethodUpdateSettings:
		var req SettingsRequest
		if err := decode(&req); err != nil {
			return invalid(err)
		}
		return d.result(d.UpdateSettings(req))
	case MethodSubscribe:
		var req SubscribeRequest
		if err := decode(&req); err != nil {
			return invalid(err)
		}
		return d.result(nil, d.Subscribe(req))
	case MethodUnsubscribe:
		return d.result(nil, d.Unsubscribe())
	default:
		return invalid(errors.New("unknown method " + method))
	}
}

func (d *Dispatcher) ok(data any) Response { return Response{OK: true, Data: data} }

func (d *Dispatcher) result(a any, err error) Response {
	if err != nil {
		return d.fail(err)
	}
	return Response{OK: true, Data: a}
}

func (d *Dispatcher) fail(err error) Response {
	if d.MapError != nil {
		if e := d.MapError(err); e != nil {
			return Response{OK: false, Error: e}
		}
	}
	return Response{OK: false, Error: &Error{Code: ErrCodeInternal, Message: err.Error()}}
}
