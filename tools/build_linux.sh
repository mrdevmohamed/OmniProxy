#!/usr/bin/env bash
# Build the Linux bridge artifacts (docs/implementation-plan.md M6):
#   - libomniproxy.so     c-shared Go core + engine for dart:ffi (app)
#   - omniproxy-helper    privileged VPN-mode daemon (launched via pkexec)
# Helper hosts the sing-box engine for VPN mode (TUN needs CAP_NET_ADMIN in the
# engine process); the core drives it over a Unix socket (JSON). VPN mode also
# requires the userspace gVisor TUN stack, so the helper is built with the same
# with_gvisor tag the Android AAR uses (GOMOD_TAGS, Makefile). Default here so
# the script stays correct when run standalone; `make linux-core` overrides it.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${1:-$ROOT/core/out}"
TAGS="${GOMOD_TAGS:-with_gvisor}"

echo "==> building libomniproxy.so (c-shared core+engine)"
( cd "$ROOT/core" && go build -buildmode=c-shared -o "$OUT/libomniproxy.so" ./glue )

echo "==> building omniproxy-helper (pkexec VPN helper, tags: $TAGS)"
( cd "$ROOT/core" && go build -tags "$TAGS" -o "$OUT/omniproxy-helper" ./cmd/omniproxy-helper )

echo "==> artifacts in $OUT"
ls -la "$OUT"
