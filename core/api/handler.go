package api

import "omniproxy/core/models"

// Handler is the core-side surface of the bridge contract. The facade
// (omniproxy/core) implements it; every transport dispatches into it.
type Handler interface {
	GetVersion() GetVersionResponse
	ListServers() ServerListResponse
	GetServer(id string) (*models.ServerProfile, error)
	AddServer(s *models.ServerProfile) (IDResponse, error)
	UpdateServer(s *models.ServerProfile) (*models.ServerProfile, error)
	DeleteServer(id string) error
	ImportServers(req ImportRequest) (ImportResponse, error)
	ExportServers(req ExportRequest) (ExportResponse, error)
	TestServerLatency(id string) (LatencyResponse, error)
	Connect(req ConnectRequest) (StateResponse, error)
	Disconnect() (StateResponse, error)
	ConnectionState() StateResponse
	GetLogs(req LogsRequest) (LogsResponse, error)
	GetSettings() (SettingsResponse, error)
	UpdateSettings(req SettingsRequest) (SettingsResponse, error)
	Subscribe(req SubscribeRequest) error
	Unsubscribe() error
}
