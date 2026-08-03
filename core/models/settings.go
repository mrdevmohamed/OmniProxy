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
}

// DefaultAppSettings returns the out-of-the-box settings.
func DefaultAppSettings() AppSettings {
	return AppSettings{
		Theme:                ThemeSystem,
		ConnectionMode:       ModeVPN,
		NotificationsEnabled: true,
		LogLevel:             LevelInfo,
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
}
