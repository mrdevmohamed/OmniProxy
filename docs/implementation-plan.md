# OmniProxy — Phase 1 (MVP) Implementation Plan

**Source of truth:** `PRD.md` · **Companion specs:** `docs/api-contract.md`, `docs/platform-notes.md`
**Status:** Phase 1 in progress (Milestone 6 of 9)

## 1. Goals & scope

Phase 1 MVP per PRD §11: app shell on Android/Windows/Linux, server CRUD + import/export (single server, no chaining), basic connect/disconnect to one server at a time, embedded `sing-box` Go module, Dashboard.

**In scope**
- Flutter shell, Material 3, dark/light/system theme, responsive (mobile bottom nav / desktop nav rail).
- Go core: models, Config Engine, Server Manager, Tunnel Manager (single-server), VPN Service, structured logging.
- `sing-box` as a pinned Go module on **all platforms** (never shelled out).
- Dashboard: status / current server / live duration / connect-disconnect only (speed/ping/traffic deferred to Phase 2).
- Server import/export: `.onnproxy` file + pasted shareable link (QR deferred).
- Android: VpnService (TUN) default + local-proxy mode (SOCKS5/HTTP on loopback), both hosted in a foreground service with a persistent notification.
- Security applied from the start (see §6).

**Explicitly out of scope (Phase 2/3 — TODOs only, no implementation):** routing manager, tunnel chain builder, monitoring/stats, Advanced Mode, accounts, cloud sync, subscriptions, remote management.

## 2. Decisions (confirmed)

| Decision | Choice |
|---|---|
| Platform order | Linux E2E → Android E2E → Windows (code-complete, untested on this host) |
| Linux TUN privileges | Privileged helper (root via `pkexec`) **hosts the engine** for VPN mode (embedded sing-box module); core drives it over a Unix socket (JSON). Not fd-passing — TUN setup + auto-route need CAP_NET_ADMIN in the engine process (see platform-notes) |
| Git | `git init`; one commit per milestone |
| Dashboard MVP | Status/server/duration only |
| Import/export | `.onnproxy` file + pasted link; QR deferred |
| Engine pin | `github.com/sagernet/sing-box v1.13.15` (stable) |
| Android modes | VPN (VpnService) default + optional local-proxy mode |
| State management | Riverpod |
| Latency test | TCP-dial via injectable tester in core (sing-box url-test/group delay is a Phase 2 enhancement — `adapter.Outbound` has no `Delay()` in v1.13.15) |
| Bridge flavor | Android: MethodChannel over gomobile bind; Linux/Windows: `dart:ffi` into c-shared lib |
| Android emulator | `run_emu` (alias for `emulator -avd light_emulator`) |

## 3. Repo structure

```
OmniProxy/
├── PRD.md · AGENTS.md · README.md
├── go.work                      # workspace: core + engine
├── core/                        # Go core — platform-independent
│   ├── go.mod · core.go         # facade wiring components together
│   ├── models/                  # ServerProfile, VPNSession, AppSettings, LogEntry
│   ├── config/                  # Config Engine: validate, persist settings, encrypt-at-rest
│   ├── store/                   # SQLite persistence + schema migrations + server repository
│   ├── server/                  # Server Manager: CRUD, import/export, favorites, latency, links
│   ├── tunnel/                  # Tunnel Manager (single-server; chain = Phase 2 TODO)
│   ├── vpn/                     # VPN Service: state machine, reconnect/backoff, sessions
│   ├── log/                     # leveled structured logger + credential redaction
│   ├── secret/                  # SecretStore interface; Linux/Windows via go-keyring
│   └── api/                     # shared API contract (types + method names)
├── engine/                      # sing-box wrapper module (pinned v1.13.15)
│   ├── go.mod
│   ├── engine.go                # Engine: New / Start / Close, log sink
│   ├── config.go                # typed config → option.Options (TUN | mixed inbound)
│   ├── registry.go              # minimal protocol registrations (keeps module lean)
│   ├── platform.go              # PlatformInterface noop + injectable hook (Android VpnService fd)
│   └── log.go                   # Level + LogSink → sing-box PlatformWriter
├── app/                         # Flutter UI
│   ├── lib/
│   │   ├── main.dart
│   │   ├── app/                 # AppRoot, router, theme
│   │   ├── core/                # ApiClient (single contract) + bridge transports
│   │   │   └── bridge/          # bridge_android / bridge_linux / bridge_windows
│   │   ├── state/               # Riverpod providers
│   │   └── features/            # dashboard/, servers/, settings/
│   ├── android/                 # Kotlin: MethodChannel host, VpnProxyService, VpnService
│   ├── linux/ · windows/        # FFI glue
├── docs/
│   ├── api-contract.md          # canonical bridge contract (created M1)
│   ├── implementation-plan.md   # this document
│   └── platform-notes.md        # TUN/privileges/limitations per platform
└── tools/                       # build scripts: c-shared, gomobile bind, helper, wintun
```

## 4. Bridge API contract

Defined once in `docs/api-contract.md` and implemented identically over every transport:
`getVersion` · `listServers` (query params: search/protocol/group/enabled/favorite/sort) · `getServer` · `addServer` · `updateServer` · `deleteServer` · `duplicateServer` · `importServers` · `exportServers` (onnproxy envelope or native share links) · `testServerLatency` · `connect({serverId, mode})` · `disconnect` · `getConnectionState` · `getLogs` · `getSettings` / `updateSettings` · `subscribe` / `unsubscribe` (events: `stateChanged`, `logAppended`, `latencyTested`).

Bridges are **pure transport only** — no platform business logic (PRD §7.2).

### Server persistence (SQLite + SecretStore)

Server profiles are persisted in a local SQLite database (`omniproxy.db`, schema managed by versioned migrations in `core/store`). Credential values are **never** stored in SQLite rows — they are externalized to OS-native secure storage under per-server keys (`omniproxy.server.<id>.<ref>`) via the same `SecretStore` used for the at-rest encryption key. The repository (`core/store/server_repository.go`) is the seam for any future column-level encryption. The encrypted settings blob (`config/`) holds settings only; on first startup after this change, `config.LoadLegacyServers` imports any server profiles still embedded in a pre-migration blob into the repository. Duplicate/enable/disable/favorite/search/filter/sort/group are managed in the repository; the bridge surfaces them via `listServers` query params, `duplicateServer`, and the `enabled`/`favorite`/`group` fields on `ServerProfile`.

## 5. Milestones (commit at each)

1. **Scaffold** — git init, `go.work`, module skeletons, `docs/api-contract.md`, README build/lint/test commands.
2. **Core: models + Config Engine + Server Manager + logging** — 4 Phase-1 models, encrypt-at-rest persistence, CRUD/import/export/favorites, redacting logger. Unit tests alongside.
3. **Engine module** — pinned v1.13.15; config builders (VLESS/VMess/SOCKS5/HTTP/SSH/Shadowsocks outbound; TUN + mixed-loopback inbounds), `Start`/`Close`, log-sink adapter, minimal protocol registry, optional platform hook; e2e routing test through a local SOCKS5 test server. *(Latency via core TCP-dial; URL-test deferred. VPN mode adds a DNS module (`dns-proxy` over the tunnel for client queries, `dns-local` over direct for outbound dialing) + a `hijack-dns` route rule so TUN DNS never leaks or loops; UDP DNS transport registered in the protocol registry.)*
4. **Core: Tunnel Manager + VPN Service + facade** — state machine (Disconnected/Connecting/Connected/Reconnecting/Error), connect/disconnect, reconnect w/ backoff, `VPNSession` tracking; core test suite green. *(Auto-reconnect retries failed starts up to the retry policy cap; `reconnect` re-establishes the live session. `deleteServer`/`updateServer` are rejected while connected to that server.)* **Check-in.**
5. **Flutter shell** — M3 theme, responsive nav, Dashboard + Server Manager + Settings (theme, connection mode) wired to a mocked `ApiClient`. **Check-in.** *(M5 submitted: `app/lib/` organized as `app/` (theme/router/AppRoot) + `core/` (contract models, `ApiClient`, `MockApiClient`, pure-transport bridge stubs for M6–M8) + `state/` (Riverpod 3 providers) + `features/` (dashboard/servers/settings). Dashboard = status/server/duration/connect only; server CRUD/import/export/latency/favorites via contract methods; settings persist via `updateSettings`. `flutter analyze` + `flutter test` + `flutter build linux` green. Riverpod 3 manual providers — no codegen.)*
6. **Linux bridge E2E** — c-shared lib + dart:ffi + pkexec helper; real connect/disconnect against a local test server. **Check-in.** *(M6 submitted: `core/glue` c-shared ABI (`omniproxy_init`/`omniproxy_request`/`omniproxy_poll_events`/`omniproxy_shutdown`/`omniproxy_free_string`); `core/tunnel/helper_client.go` helper-aware runner (proxy → in-process, VPN → pkexec helper via Unix socket JSON, seq-correlated) + `core/tunnel/helperhost` privileged engine-hosting server; `core/cmd/omniproxy-helper` pkexec entry point; `tools/build_linux.sh` → `core/out/{libomniproxy.so,omniproxy-helper}`; Dart `LinuxBridge` (dart:ffi, event polling — a native event callback deadlocks while the isolate is blocked in a synchronous FFI request) + full `ApiClient` over the transport; `linux/CMakeLists.txt` bundles both artifacts; E2E test drives a real SOCKS5 client → engine mixed inbound → SOCKS5 outbound → local SOCKS5 test server → echo, all green; `flutter analyze`/`flutter test`/`flutter build linux` green. `core` and `engine` test suites green.)*
7. **Android bridge E2E** — gomobile bind → `.aar`, Kotlin MethodChannel host, `VpnProxyService` + `VpnService` (TUN) and proxy mode, persistent notification; verify both modes on `light_emulator`. **Check-in.** *(M7 submitted: `core/tunnel/runner.go`/`manager.go` `PlatformSetter`, `core/mobile` gomobile entry (`SetTunFd`, ring-based event poll), Kotlin `Bridge.kt` MethodChannel host (HandlerThread 25 ms poll) + `MainActivity`/`OmniProxyVpnService`/`VpnProxyService`, `.aar` linked from `app/android/app/libs/` (gitignored). API 36: `setSession` (VpnService `setName` removed). `engine/platform_fd.go` `FdTunPlatform` feeds the VpnService TUN fd into sing-tun and returns a **passive** default-interface monitor (`engine/platform_monitor.go`) — sing-box nil-derefs without one and netlink monitors are banned on Android; TUN inbound also drops the legacy `sniff` field rejected by sing-box 1.13.0. Device E2E `integration_test/bridge_e2e_test.dart` drives proxy mode and VPN mode through the consent dialog (guarded by `--dart-define=OMNIPROXY_VPN_E2E=true`); both green on device `2eb95e94`. `flutter analyze`/`flutter test`/`flutter build apk --debug` green; `engine`/`core` suites green.)*
 8. **Windows bridge** — code-complete FFI + Wintun bundling; documented untested. *(M8 build validation: the core DLL now cross-compiles from the Linux host — `engine/tun_name.go`/`platform_fd.go`/`platform_monitor.go` gained `//go:build linux || android` (they use Linux-only ioctls/syscalls), and `core/mobile` is `//go:build android`. `make windows-core` produces `omniproxy.dll` + `wintun.dll` via mingw-w64; the Flutter Windows app bundle still requires a Windows host/CI.)* *(M8 bridge: `core/glue` parameterized per platform via build tags — `platform_{linux,windows}.go` select `Platform` (`getVersion`) and the tunnel runner (Windows: `NewInProcessRunner` for both modes, no pkexec/Unix socket; data-dir fallback via `os.UserConfigDir()` → `%APPDATA%\OmniProxy`); a `GOOS=windows` c-shared cross-build of the glue is part of `make go-check` (`go-check-windows`). `bridge_windows.dart` is a full dart:ffi transport (same 5-symbol ABI, `%APPDATA%` default, no helperPath); `client_factory` routes `Platform.isWindows` to it; `app/windows/CMakeLists.txt` installs `omniproxy.dll` + `wintun.dll` next to the exe. `.github/workflows/windows.yml` builds the core artifacts + `flutter build windows --release` on `windows-latest`. Runtime behavior on a real Windows host remains unverified.)*
 9. **Share links + WebSocket transport + Logs screen** — core imports vmess/vless/ss/trojan/socks/http links (incl. WS-based links), engine gains WS transport + uTLS fingerprint, UI adds a live Logs screen and richer server form. **Check-in.** *(M9 submitted: `engine/config.go` — `Transport`/`TransportSettings` (WS path/Host/maxEarlyData/earlyDataHeaderName), `TLSSettings.Fingerprint` (uTLS), `ProtocolTrojan`, outbound `GlobalPadding`/`PacketEncoding`, wired into VLESS/VMess/Trojan builders; `core/server/links.go` — `ParseImportData` (onnproxy JSON **or** multi-line share links) + `ParseLink` for `vmess://` (base64 JSON + SIP002 fallback), `vless://`, `ss://` (SIP002 + legacy), `trojan://`, `socks5/socks`, `http`; rejects grpc/h2/httpupgrade/quic + reality, per-line `ImportError`s; `ImportServers` dispatches through it. `core/models/server.go` — `TransportType`/`TransportConfig`, `TLSConfig.Fingerprint`, `flow`/`security`/`globalPadding`/`packetEncoding`, `ProtocolTrojan`; `core/tunnel/options.go` maps all into the engine config. Dart: `models.dart` mirrors the new fields; server form gained protocol-specific sections (flow/security/global-padding/packet-encoding), WS transport section, TLS fingerprint, trojan password; import dialog shows per-line errors. New `LogsScreen` (`features/logs/`) backed by `logsProvider` (`logAppended` stream + `getLogs` backfill, deduped by seq) with level filter/clear/refresh; Logs tab added to the shell; Settings gained a log-level control. `engine`/`core` suites green; `flutter analyze` + `flutter test` (7 widget tests incl. logs tab) green; widget tests fixed to scroll the taller server form.)*
 10. **Security review + polish + README + final commit.**

## 6. Security (applied from the start, per PRD §9)

- Credentials only in OS-native secure storage: Android Keystore, Windows DPAPI, Linux Secret Service (libsecret) via `SecretStore` interface.
- Config at rest encrypted (AES-256-GCM) under a data key held by `SecretStore`.
- Logs redact credentials/keys/raw traffic; never logged.
- Certificate validation on by default; bypass only behind explicit warnings (Advanced Mode). *Implemented in the MVP hardening pass:* the allow-insecure toggle is locked unless Advanced Mode is enabled and requires a confirmation dialog (`app/lib/features/servers/server_edit_screen.dart:333-354`; `app/lib/features/settings/settings_screen.dart:129-158`).

## 7. Phase 2/3 seams (TODO stubs only)

- `TunnelNode` / `TunnelChain` / `RoutingRule` model slots reserved in `api/` and `models/`.
- Tunnel Manager exposes a chain-construction seam (only the single-server path is implemented).
- Stats/monitoring hooks on `VPNSession`; routing/tunnel-builder surfaces gated by Advanced Mode (the Advanced toggle itself now exists and gates the cert bypass, §6).

## 8. Build / test / lint commands

See `README.md` for the full set. The repo-root `Makefile` drives it all:

```bash
make check           # go build/vet/gofmt (core + engine) + flutter analyze
make test            # go test (core + engine) + flutter test
make build           # Android release APK + Linux release bundle + Windows DLL
make build-native    # Android + Linux only
make e2e-android     # bridge E2E on a connected device (DEVICE=<id>)

# Underlying commands
(cd core && go build ./... && go vet ./... && go test ./...)
(cd engine && go build ./... && go vet ./...)

(cd app && flutter analyze && flutter test)
(cd app && flutter build linux --release)   # desktop
(cd app && flutter build apk --release)     # android
```
