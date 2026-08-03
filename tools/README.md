# tools/

Per-platform build scripts. Each is filled in during its milestone; the table below is the contract.

| Script | Produces | Milestone | Notes |
|---|---|---|---|
| `build_linux.sh` | `libomniproxy.so` (c-shared core+engine) + `omniproxy-helper` (pkexec TUN helper) | M6 | helper created via `sing-tun`; fd passed over Unix socket |
| `build_android.sh` | `omniproxy.aar` via `gomobile bind` (tags `with_gvisor`) | M7 | linked by Gradle; Kotlin channel host + `VpnProxyService`/`VpnService` |
| `build_windows.sh` | `omniproxy.dll` (c-shared) + bundled `wintun.dll` | M8 | cross-compiled from this host; untested here |
| `build_engine_check.sh` | — | M1+ | `go build ./... && go vet ./...` for `core/` and `engine/` |
