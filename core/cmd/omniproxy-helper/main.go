// Command omniproxy-helper is the privileged Linux VPN-mode helper
// (docs/platform-notes.md §Linux). It is a thin pkexec entry point: the engine
// lifecycle and the control protocol live in core/tunnel/helperhost, which is
// exercised directly by tests. Launched via pkexec by the core, it owns the
// sing-box engine and the TUN interface for VPN mode — it is never the sing-box
// CLI.
package main

import (
	"flag"
	"fmt"
	"os"

	"omniproxy/core/tunnel/helperhost"
)

func main() {
	socket := flag.String("socket", "", "path to the helper control socket")
	flag.Parse()
	if *socket == "" {
		fmt.Fprintln(os.Stderr, "omniproxy-helper: --socket is required")
		os.Exit(2)
	}
	if err := helperhost.Run(*socket); err != nil {
		fmt.Fprintf(os.Stderr, "omniproxy-helper: %v\n", err)
		os.Exit(1)
	}
}
