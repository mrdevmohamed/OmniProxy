# tools/

Per-platform build scripts. Each is filled in during its milestone; the table below is the contract.

| Script | Produces | Milestone | Notes |
|---|---|---|---|
| `build_linux.sh` | `libomniproxy.so` (c-shared core+engine) + `omniproxy-helper` (pkexec TUN helper, built `-tags with_gvisor`) | M6 | helper created via `sing-tun`; fd passed over Unix socket; the `with_gvisor` userspace TUN stack is required by the `mixed` stack (`GOMOD_TAGS`, Makefile) |
| `build_android.sh` | `omniproxy.aar` via `gomobile bind` (tags `with_gvisor`) | M7 | *(driven by `make aar` / `make build`)* |
| `build_windows.sh` | `omniproxy.dll` (c-shared) + bundled `wintun.dll` | M8 | *(driven by `make windows-core`, cross-compiled from the Linux host)* |
| `build_engine_check.sh` | — | M1+ | `go build ./... && go vet ./...` for `core/` and `engine/` *(driven by `make go-check`)* |

The repo-root `Makefile` is the entry point: `make check` / `make test` / `make build`
(Android AAR+APK, Linux bundle, Windows DLL) / `make build-native` / `make e2e-android`.
`build_android.sh`/`build_windows.sh`/`build_engine_check.sh` are the documented
contracts that the Makefile's inline recipes fulfill.
