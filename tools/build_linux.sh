#!/usr/bin/env bash
# Build the Linux bridge artifacts (docs/implementation-plan.md M6):
#   - libomniproxy.so     c-shared Go core + engine for dart:ffi (app)
#   - omniproxy-helper    privileged VPN-mode daemon (launched via pkexec)
# Helper hosts the sing-box engine for VPN mode (TUN needs CAP_NET_ADMIN in the
# engine process); the core drives it over a Unix socket (JSON).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${1:-$ROOT/core/out}"

echo "==> building libomniproxy.so (c-shared core+engine)"
( cd "$ROOT/core" && go build -buildmode=c-shared -o "$OUT/libomniproxy.so" ./glue )

echo "==> building omniproxy-helper (pkexec VPN helper)"
( cd "$ROOT/core" && go build -o "$OUT/omniproxy-helper" ./cmd/omniproxy-helper )

echo "==> artifacts in $OUT"
ls -la "$OUT"
