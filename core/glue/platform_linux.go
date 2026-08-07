//go:build linux

package main

import (
	"omniproxy/core/log"
	"omniproxy/core/tunnel"
)

// platformName is the platform reported by getVersion (core/core.go:216).
func platformName() string { return "linux" }

// newRunner selects the tunnel runner. Linux routes VPN mode to the privileged
// omniproxy-helper (pkexec, Unix socket) and runs proxy mode in-process
// (docs/platform-notes.md §Linux).
func newRunner(logger *log.Logger, helperPath string) tunnel.Runner {
	return tunnel.NewHelperAwareRunner(logger, tunnel.NewHelperSpawner(helperPath))
}
