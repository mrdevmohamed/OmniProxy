//go:build windows

package main

import (
	"omniproxy/core/log"
	"omniproxy/core/tunnel"
)

// platformName is the platform reported by getVersion (core/core.go:216).
func platformName() string { return "windows" }

// newRunner selects the tunnel runner. Windows runs the engine in-process for
// both modes: there is no privileged helper, and wintun (via sing-tun) creates
// the adapter inside the engine process (docs/platform-notes.md §Windows).
func newRunner(logger *log.Logger, _ string) tunnel.Runner {
	return tunnel.NewInProcessRunner(logger)
}
