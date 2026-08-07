# Platform Notes

Per-platform details for the Flutter ↔ Go bridge, TUN/privileges, and native glue. Read before touching any platform-specific code.

## Cross-platform

- One internal API contract (`docs/api-contract.md`) on every platform; bridges are pure transport.
- Shared `.onnproxy` export format so profiles move between platforms cleanly.
- Platform limitations must be surfaced in the UI, never silently degraded.
- **Logging:** the logger's minimum level starts from the init config (`logLevel`, platform default `info`) and is re-applied at runtime whenever `updateSettings` changes `AppSettings.logLevel`. The Logs screen is a live view over `logAppended` events with a `getLogs` backfill (deduped by `seq`); core redacts credentials/keys/raw traffic before they reach the ring.

## Android

- **Service model:** a foreground `VpnProxyService` hosts the Go core in both connection modes and shows a persistent, non-dismissible notification while connected (PRD §3.1).
  - **VPN mode (default):** an `android.net.VpnService` establishes the TUN; the resulting `ParcelFileDescriptor` fd is handed to the Go core, which feeds it to sing-box's TUN stack (build with `with_gvisor`).
  - **Proxy mode:** no VpnService/permission flow; sing-box serves a SOCKS5+HTTP mixed inbound on `127.0.0.1:<port>`; the local address/port is surfaced in the UI.
- **Go core packaging:** `gomobile bind` → `.aar` placed under `app/android/` and linked by Gradle. Kotlin `MainActivity`/`VpnProxyService` forward `MethodChannel("com.omniproxy/bridge")` calls into the bindings; events return via `MethodChannel("com.omniproxy/events")`.
- **TUN monitor:** the Android platform hook (`FdTunPlatform`) returns a **passive** default-interface monitor. sing-box requires a non-nil monitor whenever a platform interface is present (`route/network.go`), and netlink-based monitors are banned on Android (`sing-tun: netlink is banned by google`). The VpnService owns the TUN device and its routing, so the monitor never reports a default interface and never emits updates. Verified against sing-box v1.13.15.
- **TUN inbound:** the TUN inbound is built without legacy inbound fields (e.g. `sniff`), which sing-box 1.11 deprecated and 1.13 removed — the option validator rejects them at start. Use rule actions instead (Phase 2).
- **Credentials:** Android Keystore via the `SecretStore` interface (implemented in the native/bridge layer, not in core).
- **Android specifics to honor (PRD):** auto-reconnect on network change/drop, doze-mode/battery guidance, background execution limits — test across OEM skins.
- **Emulator:** `run_emu` (alias for `emulator -avd light_emulator`). VpnService TUN works on the emulator.

## IPv6

IPv6 handling is decided at the engine layer, so behavior is identical on every
platform. `AppSettings.ipv6Mode` (see `docs/api-contract.md` §2.3, default
`prefer_ipv4`) maps onto the sing-box DNS domain strategy and route rules in
`engine/config.go`:

| Mode | DNS strategy | Route rules |
|---|---|---|
| `auto` | (none — as-is) | — |
| `prefer_ipv4` | `prefer_ipv4` | — |
| `disable_ipv6` | `ipv4_only` | `ip_version: 6` → `block` outbound |
| `enable_ipv6` | `prefer_ipv6` | — |

- **The TUN always keeps its IPv6 address and `::/0` capture** (Android
  `VpnService.Builder` adds `fd00::1/64` + `::/0`; the engine TUN inbound
  defaults to `10.0.0.1/24` + `fd00::1/64`). IPv6 can therefore never fall out
  onto the physical interface, even in `disable_ipv6` — "disabling" IPv6 only
  stops it being *used*, it is never *leaked*.
- **Why `prefer_ipv4` is the default:** with the default (as-is) strategy,
  apps receive AAAA answers and dial IPv6 first; when the upstream path to IPv6
  destinations is slow or broken (a common cause of stalled `fast.com` speed
  tests through a v4-only relay), connections hang. `prefer_ipv4` keeps IPv6
  captured and usable for IPv6-only sites while preferring the working IPv4
  path.
- **`disable_ipv6` details:** the `ipv4_only` DNS strategy answers AAAA queries
  with an empty NOERROR reply (`dns/client.go`), so clients never learn IPv6
  addresses; any literal IPv6 dial that still reaches the tunnel is refused by
  the block outbound. `block` is registered in the engine's protocol registry
  (`engine/registry.go`).
- **Logging** (all visible in the Logs screen at `info` or higher):
  - DNS answers are logged per record type by sing-box (`dns/client_log.go`,
    e.g. `exchanged A 1.2.3.4 …` / `exchanged AAAA …`).
  - The applied mode is logged by the Tunnel Manager at each start
    (`started vpn tunnel to <addr>:<port> (ipv6 mode <mode>)`).
  - IPv6 leak attempts in `disable_ipv6` mode are logged by the block outbound
    (`blocked connection to <dest>` / `blocked packet connection to <dest>`).

## Linux
- **TUN privileges (design):** creating a TUN interface **and configuring routing/DNS** (auto-route via netlink, ip rules, systemd-resolved) requires root/CAP_NET_ADMIN in the process that hosts the engine. The core itself runs unprivileged, so a small privileged helper (also Go, same workspace, embeds the same `engine` module) is launched via `pkexec` on demand and **hosts the sing-box tunnel process** for VPN mode. The helper listens on a Unix socket (`$XDG_RUNTIME_DIR/omniproxy/helper.sock`); the core is a JSON-over-socket client that sends `connect`/`disconnect`/`state` and streams events back. This is the same "embedded sing-box, never shelled out" rule — the helper links sing-box as a Go module; it is not the sing-box CLI.
  - An early design considered the helper only *creating* the TUN fd and passing it to the unprivileged core via `SCM_RIGHTS`; this was rejected because sing-box's tun setup (addresses, `auto_route`, DNS) also needs `CAP_NET_ADMIN` in the engine process. Verified against sing-box v1.13.15.
  - Helper scope: authenticate (pkexec), own the engine lifecycle, configure TUN/routing. No tunnel *protocol* logic lives in the helper beyond what the shared `engine` module provides.
  - The core retries/waits for the helper with a timeout and reports `unauthorized` (PRD §3.3) with actionable UI text if pkexec is cancelled.
- **Proxy mode:** no privileges needed; the core runs the `engine` module in-process with the local mixed inbound on loopback.
- **Go core packaging:** `go build -buildmode=c-shared` → `libomniproxy.so`, loaded via `dart:ffi`. The helper is a separate binary under `tools/`, built **with the `with_gvisor` tag** (`make linux-core` → `tools/build_linux.sh`); the `.so` is not tagged because it never hosts the TUN on Linux (VPN mode always delegates to the helper).
- **Credentials:** Secret Service / libsecret (`go-keyring`).
- **Config dir:** `$XDG_CONFIG_HOME/omniproxy` (fallback `~/.config/omniproxy`); data/logs under `$XDG_DATA_HOME`/`$XDG_STATE_HOME`.
- **Desktop integration:** integrate with NetworkManager/systemd-resolved handling to avoid DNS/routing conflicts (Phase 1 scope: keep to sing-box defaults; revisit in Phase 2).

## Windows

- **Go core packaging:** `go build -buildmode=c-shared` → `omniproxy.dll`, loaded via `dart:ffi`; `wintun.dll` bundled next to the binary for TUN (sing-box/Wintun).
- **TUN:** Wintun driver. Elevated privileges for interface creation are handled per sing-box's Windows model; the FFI process is the app process.
- **Bridge:** `bridge_windows.dart` (dart:ffi, `omniproxy.dll`) mirrors the Linux transport; `client_factory` routes Windows to it. No privileged helper — the engine runs in-process for both modes (`core/glue/platform_windows.go`). The glue reports `platform: "windows"` via `getVersion`.
- **Background service model (PRD §3.2):** a separate Windows service is a Phase 1.5+ concern. MVP uses the in-process DLL + FFI; note the limitation in the UI if applicable.
- **Credentials:** Windows Credential Manager / DPAPI (`go-keyring`).
- **Config dir:** `%APPDATA%\OmniProxy`.
- **Build:** core DLL cross-compiles from Linux (`make windows-core`); the Flutter bundle requires a Windows host or CI — `.github/workflows/windows.yml` builds both on `windows-latest`. The glue is cross-checked by `make go-check` (`go-check-windows`).
- **NOT TESTABLE on the Linux dev host** — implemented and CI-built, but runtime behavior (wintun elevation, VPN/proxy connect, Credential Manager) must be verified on a Windows machine or CI before release.

## Permission matrix (MVP)

| Action | Android | Linux | Windows |
|---|---|---|---|
| TUN interface | VpnService grant | pkexec helper (hosts engine) | Wintun (in-process) |
| Connect (VPN mode) | requires VpnService consent | requires helper success | requires wintun.dll present |
| Connect (proxy mode) | none | none | none |
| Secure storage | Keystore | libsecret | Credential Manager/DPAPI |
