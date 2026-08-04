package models

// Theme is the app theme selection (system-following by default).
type Theme string

const (
	ThemeSystem Theme = "system"
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
)

// LogLevel is a leveled logging severity shared by the logger and AppSettings.
type LogLevel string

const (
	LevelTrace LogLevel = "trace"
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// Valid reports whether l is a known level.
func (l LogLevel) Valid() bool {
	_, ok := map[LogLevel]bool{
		LevelTrace: true, LevelDebug: true, LevelInfo: true, LevelWarn: true, LevelError: true,
	}[l]
	return ok
}

// Rank returns a numeric ordering (trace=0 .. error=4).
func (l LogLevel) Rank() int {
	switch l {
	case LevelTrace:
		return 0
	case LevelDebug:
		return 1
	case LevelInfo:
		return 2
	case LevelWarn:
		return 3
	case LevelError:
		return 4
	default:
		return 2 // unknown levels behave as info
	}
}

// IPv6Mode selects how IPv6 is handled on the tunnel (PRD §3.1; see
// docs/platform-notes.md §IPv6). The default is PreferIPv4: IPv6 stays fully
// captured by the TUN (no leak), but DNS prefers A records so v4-only upstream
// paths (e.g. fast.com measurement servers) are not stalled behind IPv6.
type IPv6Mode string

const (
	// IPv6ModeAuto leaves the engine's default behavior (no override).
	IPv6ModeAuto IPv6Mode = "auto"
	// IPv6ModePreferIPv4 keeps IPv6 captured but prefers A over AAAA answers.
	IPv6ModePreferIPv4 IPv6Mode = "prefer_ipv4"
	// IPv6ModeDisable blocks IPv6 end-to-end: AAAA queries get empty NOERROR
	// replies and any IPv6 packet reaching the tunnel is dropped.
	IPv6ModeDisable IPv6Mode = "disable_ipv6"
	// IPv6ModeEnable prefers IPv6 addresses when the upstream path supports
	// them.
	IPv6ModeEnable IPv6Mode = "enable_ipv6"
)

// Valid reports whether m is a known IPv6 mode.
func (m IPv6Mode) Valid() bool {
	switch m {
	case IPv6ModeAuto, IPv6ModePreferIPv4, IPv6ModeDisable, IPv6ModeEnable:
		return true
	default:
		return false
	}
}

// AppSettings holds user-level and app-level preferences (PRD §8).
// Behavior fields (AutoConnect, StartWithSystem) are persisted in the MVP and
// honored in later phases; AdvancedModeEnabled is always false until Phase 2.
type AppSettings struct {
	Theme                Theme          `json:"theme"`
	ConnectionMode       ConnectionMode `json:"connectionMode"`
	AutoConnect          bool           `json:"autoConnect"`
	StartWithSystem      bool           `json:"startWithSystem"`
	AdvancedModeEnabled  bool           `json:"advancedModeEnabled"`
	NotificationsEnabled bool           `json:"notificationsEnabled"`
	LogLevel             LogLevel       `json:"logLevel"`
	IPv6Mode             IPv6Mode       `json:"ipv6Mode"`
}

// DefaultAppSettings returns the out-of-the-box settings.
func DefaultAppSettings() AppSettings {
	return AppSettings{
		Theme:                ThemeSystem,
		ConnectionMode:       ModeVPN,
		NotificationsEnabled: true,
		LogLevel:             LevelInfo,
		IPv6Mode:             IPv6ModePreferIPv4,
	}
}

// Normalize clamps invalid values to safe defaults in place.
func (s *AppSettings) Normalize() {
	switch s.Theme {
	case ThemeSystem, ThemeLight, ThemeDark:
	default:
		s.Theme = ThemeSystem
	}
	if !s.ConnectionMode.Valid() {
		s.ConnectionMode = ModeVPN
	}
	if !s.LogLevel.Valid() {
		s.LogLevel = LevelInfo
	}
	if !s.IPv6Mode.Valid() {
		s.IPv6Mode = IPv6ModePreferIPv4
	}
}
