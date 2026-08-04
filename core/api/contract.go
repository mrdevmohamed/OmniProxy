// Package api is the shared Flutter ↔ Go bridge contract (docs/api-contract.md):
// method names, request/response schemas, error codes, and the event envelope.
// Every bridge transport implements exactly this contract, never its own.
package api

import "omniproxy/core/models"

// Method names (contract §3). Transports must not add, remove, or reinterpret.
const (
	MethodGetVersion         = "getVersion"
	MethodListServers        = "listServers"
	MethodGetServer          = "getServer"
	MethodAddServer          = "addServer"
	MethodUpdateServer       = "updateServer"
	MethodDeleteServer       = "deleteServer"
	MethodDuplicateServer    = "duplicateServer"
	MethodImportServers      = "importServers"
	MethodExportServers      = "exportServers"
	MethodTestServerLatency  = "testServerLatency"
	MethodConnect            = "connect"
	MethodDisconnect         = "disconnect"
	MethodGetConnectionState = "getConnectionState"
	MethodGetLogs            = "getLogs"
	MethodGetSettings        = "getSettings"
	MethodUpdateSettings     = "updateSettings"
	MethodSubscribe          = "subscribe"
	MethodUnsubscribe        = "unsubscribe"
)

// Event types (contract §4).
const (
	EventStateChanged  = "stateChanged"
	EventLogAppended   = "logAppended"
	EventLatencyTested = "latencyTested"
)

// Error codes (contract §3).
const (
	ErrCodeNotFound        = "not_found"
	ErrCodeInvalidArgument = "invalid_argument"
	ErrCodeValidation      = "validation_failed"
	ErrCodeEngine          = "engine_error"
	ErrCodeBusy            = "busy"
	ErrCodeConnected       = "connected"
	ErrCodeUnauthorized    = "unauthorized"
	ErrCodeInternal        = "internal"
)

// Error is the canonical error object (contract §1).
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Response is the canonical response envelope (contract §1).
type Response struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error *Error `json:"error,omitempty"`
}

// --- Requests ---

// IDRequest carries a server/session id.
type IDRequest struct {
	ID string `json:"id"`
}

// IDResponse is the response for addServer.
type IDResponse struct {
	ID string `json:"id"`
}

// ServerRequest wraps a ServerProfile for add/update.
type ServerRequest struct {
	Server *models.ServerProfile `json:"server"`
}

// ImportSource describes how import data was obtained (file/link/clipboard);
// core only consumes the blob itself.
type ImportSource struct {
	Kind string `json:"kind"`
	Data string `json:"data"`
}

// ImportRequest is the importServers request.
type ImportRequest struct {
	Source ImportSource `json:"source"`
}

// ImportError reports one failed profile during a bulk import.
type ImportError struct {
	Index   int    `json:"index,omitempty"`
	Message string `json:"message"`
}

// ImportResponse is the importServers response.
type ImportResponse struct {
	Added  int           `json:"added"`
	Failed int           `json:"failed"`
	Errors []ImportError `json:"errors"`
}

// ExportRequest selects profiles to export (empty = all) and the output
// format. Format defaults to ExportFormatEnvelope when empty.
type ExportRequest struct {
	IDs    []string `json:"ids"`
	Format string   `json:"format"`
}

// Export formats supported by exportServers.
const (
	// ExportFormatEnvelope is the OmniProxy envelope (importServers compatible).
	ExportFormatEnvelope = "onnproxy"
	// ExportFormatLinks is newline-separated native share links
	// (vless://, vmess://, ss://, trojan://, socks5://, http://).
	ExportFormatLinks = "links"
)

// ExportResponse is the exportServers response.
type ExportResponse struct {
	Format string `json:"format"`
	Blob   string `json:"blob"`
}

// ServerSort orders listServers results.
type ServerSort string

// Supported listServers sort orders. The zero value sorts by name.
const (
	ServerSortName      ServerSort = "name"
	ServerSortUpdatedAt ServerSort = "updatedAt"
	ServerSortLatency   ServerSort = "latency"
)

// ServerListRequest is the listServers request. All fields are optional; the
// zero value returns every server ordered by name.
type ServerListRequest struct {
	Search   string     `json:"search"`
	Protocol string     `json:"protocol"`
	Group    string     `json:"group"`
	Enabled  *bool      `json:"enabled"`
	Favorite *bool      `json:"favorite"`
	Sort     ServerSort `json:"sort"`
}

// DuplicateRequest is the duplicateServer request.
type DuplicateRequest struct {
	ID string `json:"id"`
}

// LatencyRequest is the testServerLatency request.
type LatencyRequest struct {
	ID string `json:"id"`
}

// LatencyResponse is the testServerLatency response.
type LatencyResponse struct {
	LatencyMS int `json:"latencyMs"`
}

// LatencyTestedEventData is the latencyTested event payload.
type LatencyTestedEventData struct {
	ID        string `json:"id"`
	LatencyMS int    `json:"latencyMs"`
}

// ConnectRequest is the connect request. Mode may be empty (defaults to
// settings.connectionMode).
type ConnectRequest struct {
	ServerID string                `json:"serverId"`
	Mode     models.ConnectionMode `json:"mode"`
}

// StateResponse is the getConnectionState response and the stateChanged event
// payload. Session is null when no session is live.
type StateResponse struct {
	State   models.ConnectionState `json:"state"`
	Session *models.VPNSession     `json:"session"`
}

// LogsRequest is the getLogs request (both fields optional).
type LogsRequest struct {
	AfterSeq uint64 `json:"afterSeq"`
	Limit    int    `json:"limit"`
}

// LogsResponse is the getLogs response.
type LogsResponse struct {
	Logs []models.LogEntry `json:"logs"`
}

// SettingsRequest is the updateSettings request.
type SettingsRequest struct {
	Settings models.AppSettings `json:"settings"`
}

// SettingsResponse is the getSettings/updateSettings response.
type SettingsResponse struct {
	Settings models.AppSettings `json:"settings"`
}

// SubscribeRequest selects the event types to deliver.
type SubscribeRequest struct {
	Events []string `json:"events"`
}

// --- Responses ---

// GetVersionResponse is the getVersion response.
type GetVersionResponse struct {
	Version       string `json:"version"`
	EngineVersion string `json:"engineVersion"`
	Platform      string `json:"platform"`
}

// ServerListResponse is the listServers response.
type ServerListResponse struct {
	Servers []*models.ServerProfile `json:"servers"`
}
