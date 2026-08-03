// Helper protocol wire types, re-exported from helperproto so package tunnel
// keeps a stable local surface. Socket path helpers live here too.
package tunnel

import (
	"os"
	"path/filepath"

	"omniproxy/core/tunnel/helperproto"
)

// Re-exported wire types (defined once in helperproto).
type (
	ClientMessage  = helperproto.ClientMessage
	ServerMessage  = helperproto.ServerMessage
	HelperLogEvent = helperproto.HelperLogEvent
)

// helperSocketName is the control socket file name under
// $XDG_RUNTIME_DIR/omniproxy/.
const helperSocketName = "helper.sock"

// HelperSocketPath returns the default control socket path, creating the
// parent directory when possible.
func HelperSocketPath() (string, error) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = "/tmp"
	}
	dir := filepath.Join(runtimeDir, "omniproxy")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, helperSocketName), nil
}
