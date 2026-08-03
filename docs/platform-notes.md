# Platform Notes

Per-platform details for the Flutter ↔ Go bridge, TUN/privileges, and native glue. Read before touching any platform-specific code.

## Cross-platform

- One internal API contract (`docs/api-contract.md`) on every platform; bridges are pure transport.
- Shared `.onnproxy` export format so profiles move between platforms cleanly.
- Platform limitations must be surfaced in the UI, never silently degraded.

## Android

- **Service model:** a foreground `VpnProxyService` hosts the Go core in both connection modes and shows a persistent, non-dismissible notification while connected (PRD §3.1).
  - **VPN mode (default):** an `android.net.VpnService` establishes the TUN; the resulting `ParcelFileDescriptor` fd is handed to the Go core, which feeds it to sing-box's TUN stack (build with `with_gvisor`).
  - **Proxy mode:** no VpnService/permission flow; sing-box serves a SOCKS5+HTTP mixed inbound on `127.0.0.1:<port>`; the local address/port is surfaced in the UI.
- **Go core packaging:** `gomobile bind` → `.aar` placed under `app/android/` and linked by Gradle. Kotlin `MainActivity`/`VpnProxyService` forward `MethodChannel("com.omniproxy/bridge")` calls into the bindings; events return via `MethodChannel("com.omniproxy/events")`.
- **Credentials:** Android Keystore via the `SecretStore` interface (implemented in the native/bridge layer, not in core).
- **Android specifics to honor (PRD):** auto-reconnect on network change/drop, doze-mode/battery guidance, background execution limits — test across OEM skins.
- **Emulator:** `run_emu` (alias for `emulator -avd light_emulator`). VpnService TUN works on the emulator.

## Linux

- **TUN privileges (design):** creating a TUN interface **and configuring routing/DNS** (auto-route via netlink, ip rules, systemd-resolved) requires root/CAP_NET_ADMIN in the process that hosts the engine. The core itself runs unprivileged, so a small privileged helper (also Go, same workspace, embeds the same `engine` module) is launched via `pkexec` on demand and **hosts the sing-box tunnel process** for VPN mode. The helper listens on a Unix socket (`$XDG_RUNTIME_DIR/omniproxy/helper.sock`); the core is a JSON-over-socket client that sends `connect`/`disconnect`/`state` and streams events back. This is the same "embedded sing-box, never shelled out" rule — the helper links sing-box as a Go module; it is not the sing-box CLI.
  - An early design considered the helper only *creating* the TUN fd and passing it to the unprivileged core via `SCM_RIGHTS`; this was rejected because sing-box's tun setup (addresses, `auto_route`, DNS) also needs `CAP_NET_ADMIN` in the engine process. Verified against sing-box v1.13.15.
  - Helper scope: authenticate (pkexec), own the engine lifecycle, configure TUN/routing. No tunnel *protocol* logic lives in the helper beyond what the shared `engine` module provides.
  - The core retries/waits for the helper with a timeout and reports `unauthorized` (PRD §3.3) with actionable UI text if pkexec is cancelled.
- **Proxy mode:** no privileges needed; the core runs the `engine` module in-process with the local mixed inbound on loopback.
- **Go core packaging:** `go build -buildmode=c-shared` → `libomniproxy.so`, loaded via `dart:ffi`. The helper is a separate binary under `tools/`.
- **Credentials:** Secret Service / libsecret (`go-keyring`).
- **Config dir:** `$XDG_CONFIG_HOME/omniproxy` (fallback `~/.config/omniproxy`); data/logs under `$XDG_DATA_HOME`/`$XDG_STATE_HOME`.
- **Desktop integration:** integrate with NetworkManager/systemd-resolved handling to avoid DNS/routing conflicts (Phase 1 scope: keep to sing-box defaults; revisit in Phase 2).

## Windows

- **Go core packaging:** `go build -buildmode=c-shared` → `omniproxy.dll`, loaded via `dart:ffi`; `wintun.dll` bundled next to the binary for TUN (sing-box/Wintun).
- **TUN:** Wintun driver. Elevated privileges for interface creation are handled per sing-box's Windows model; the FFI process is the app process.
- **Background service model (PRD §3.2):** a separate Windows service is a Phase 1.5+ concern. MVP uses the in-process DLL + FFI; note the limitation in the UI if applicable.
- **Credentials:** Windows Credential Manager / DPAPI (`go-keyring`).
- **Config dir:** `%APPDATA%\OmniProxy`.
- **NOT TESTABLE on the Linux dev host** — this platform is code-complete only; verify on a Windows machine or CI before release.

## Permission matrix (MVP)

| Action | Android | Linux | Windows |
|---|---|---|---|
| TUN interface | VpnService grant | pkexec helper (hosts engine) | Wintun (in-process) |
| Connect (VPN mode) | requires VpnService consent | requires helper success | requires wintun.dll present |
| Connect (proxy mode) | none | none | none |
| Secure storage | Keystore | libsecret | Credential Manager/DPAPI |
