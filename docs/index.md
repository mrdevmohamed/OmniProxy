# OmniProxy — Engineering Docs

OmniProxy is a commercial cross-platform VPN client for **Android, Windows, and
Linux**: a Flutter UI (`app/`), a Go controller core (`core/`), and the
**sing-box** networking engine (`engine/`). macOS and web are intentionally out
of scope.

## Product

- `PRD.md` (repo root) — product requirements (source of truth for behavior)
- [`docs/implementation-plan.md`](implementation-plan.md) — Phase 1 (MVP) plan, scope boundaries, decisions

## Pipeline

- [`docs/CI.md`](CI.md) — the GitHub Actions build, test, and release pipeline, end to end

## Architecture & design

- [`docs/ARCHITECTURE.md`](ARCHITECTURE.md) — layered architecture overview
- [`docs/PROXY_ARCHITECTURE.md`](PROXY_ARCHITECTURE.md) — proxy/routing design
- [`docs/api-contract.md`](api-contract.md) — canonical Flutter ↔ Go bridge contract
- [`docs/platform-notes.md`](platform-notes.md) — per-platform notes (TUN, privileges, native glue)
- [`docs/CLASS_DIAGRAMS.md`](CLASS_DIAGRAMS.md) · [`docs/SEQUENCE_DIAGRAMS.md`](SEQUENCE_DIAGRAMS.md) · [`docs/FLOWCHARTS.md`](FLOWCHARTS.md)

## Deep dives

- [`docs/GO_RUNTIME.md`](GO_RUNTIME.md) · [`docs/VPN_INTERNALS.md`](VPN_INTERNALS.md) · [`docs/NETWORK_FLOW.md`](NETWORK_FLOW.md) · [`docs/LOW_LEVEL.md`](LOW_LEVEL.md)
- [`docs/SINGBOX.md`](SINGBOX.md) — the pinned networking engine
- [`docs/SECURITY.md`](SECURITY.md) — security model, credential storage, redaction

## Platform guides

- [`docs/FLUTTER.md`](FLUTTER.md) · [`docs/FLUTTER_GO_FFI.md`](FLUTTER_GO_FFI.md)
- [`docs/ANDROID.md`](ANDROID.md) · [`docs/WINDOWS.md`](WINDOWS.md) · [`docs/LINUX.md`](LINUX.md)
- [`docs/PERFORMANCE.md`](PERFORMANCE.md) · [`docs/DEBUGGING.md`](DEBUGGING.md)
