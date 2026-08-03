# AGENTS.md

## Project

OmniProxy: a commercial cross-platform VPN client for **Android, Windows, Linux**.
Flutter UI (single codebase) + Go core (controller/logic) + **sing-box** networking engine.
`PRD.md` is the source of truth for product behavior; read it before implementing features.
`docs/` holds the engineering plan and bridge spec — read the relevant docs before implementing or changing behavior.

## Docs (read before implementing)

- `docs/implementation-plan.md` — Phase 1 (MVP) plan: repo layout, confirmed decisions, milestones, scope boundaries.
- `docs/api-contract.md` — canonical Flutter ↔ Go bridge contract (method names, request/response schemas). Every bridge transport implements exactly this contract.
- `docs/platform-notes.md` — per-platform notes: TUN/privileges, native glue, platform limitations.

If the plan and the PRD disagree, ask the user rather than guessing.
If a change touches the bridge contract or the plan, update the docs in the same change.

## Architecture (planned layout)

Three layers — protocols are always delegated to the engine, never reimplemented:

- `app/` — Flutter UI. Material 3, dark/light/system theme, responsive (mobile single-column, desktop multi-pane). Dashboard is the default landing screen; Advanced Mode is opt-in and hidden from the default flow.
- `core/` — Go: VPN service, config engine, server manager, tunnel manager, structured logging. Compiled natively per platform.
- `engine/` — sing-box, pinned to a known-good version per release (upstream is actively developed; breaking changes possible).

Flutter ↔ Go bridges are **pure transport only** — no platform-specific business logic:
- Android: Go compiled via gomobile/cgo, invoked through platform channels or FFI; `VpnService` lives in the native layer.
- Windows/Linux: native DLL/shared library or background daemon via FFI and/or IPC (Unix socket on Linux).
- One shared internal API contract (method names, request/response schemas) implemented identically across all bridges.

## Networking & VPN rules

- Never implement VPN/tunnel protocols or crypto manually. Integrate a proven engine: `sing-box` (project-chosen), `xray-core`, or mature Go networking libraries.
- Protocol support comes from the engine, not from us: VLESS, VMess, Shadowsocks, SOCKS5, HTTP proxy, SSH tunneling, TLS handling, DNS resolution.
- No custom implementations for: networking, encryption, parsing, serialization, auth, DB operations, file handling, async/concurrency, CLI, UI components.

## Library usage policy

- Standard library first; then mature, actively maintained third-party libraries with stable APIs and compatible licenses.
- Justify each new dependency before adding it; keep dependencies minimal. Follow official docs and recommended usage patterns; don't duplicate trusted library functionality.
- Go: prefer stdlib + established pkg.go.dev packages; avoid hand-rolled networking, crypto, HTTP, JSON, or concurrency utilities.

## Product rules that are easy to miss (from PRD)

- Users never hand-edit JSON. The visual routing/tunnel builders generate the `sing-box` configuration automatically.
- Identical feature set across platforms; any platform limitation must be surfaced in the UI, never silently degraded.
- Shared config format so profiles export/import cleanly across platforms.
- Credentials only in OS-native secure storage (Android Keystore, Windows Credential Manager/DPAPI, Linux Secret Service/libsecret); never plaintext. Encrypt sensitive config at rest.
- Logs must never contain credentials, private keys, or raw traffic content.
- Certificate validation on by default; bypass only in Advanced Mode with an explicit user-visible warning.
- Android: persistent non-dismissible notification while connected; auto-reconnect on network change/drop; battery/doze-mode handling.
- Connection states: Disconnected / Connecting / Connected / Reconnecting / Error.

## MVP scope gate (Phase 1)

App shell (3 platforms) + server CRUD/import/export + single-server connect (no chaining) + sing-box integration + dashboard.
Advanced routing, tunnel chaining, stats are Phase 2 — do not build ahead of scope during MVP work.

## Repo status

Initialized in Milestone 1. See `docs/implementation-plan.md` for established build/lint/test commands and commit conventions.
