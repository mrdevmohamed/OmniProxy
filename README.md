# OmniProxy

Commercial cross-platform VPN client for **Android, Windows, Linux**.

- **UI:** Flutter (Material 3, single codebase)
- **Core:** Go (VPN controller, config, server/tunnel management, logging)
- **Engine:** sing-box (embedded Go module, pinned `v1.13.15` — never shelled out)

## Docs

- `PRD.md` — product source of truth (read before implementing features)
- `docs/implementation-plan.md` — Phase 1 plan, milestones, confirmed decisions
- `docs/api-contract.md` — canonical Flutter ↔ Go bridge contract (single source of truth for all transports)
- `docs/platform-notes.md` — per-platform TUN/privileges/native-glue notes

## Layout

| Path | Contents |
|---|---|
| `core/` | Go core (platform-independent business logic) |
| `engine/` | sing-box wrapper module (pinned) |
| `app/` | Flutter UI (`lib/`) + native glue (`android/`, `linux/`, `windows/`) |
| `tools/` | Build scripts (c-shared, gomobile bind, helper, wintun) |
| `docs/` | Engineering plan, bridge contract, platform notes |

## Prerequisites

- Go ≥ 1.26
- Flutter ≥ 3.44 (with Android SDK/NDK for Android, Linux desktop toolchain for Linux)
- Android: NDK + `golang.org/x/mobile/cmd/gomobile` (see `tools/`)

## Build / test / lint

```bash
# Go core + engine
cd core && go build ./... && go vet ./... && go test ./... && gofmt -l .
cd engine && go build ./... && go vet ./...

# Flutter app
cd app && flutter analyze && flutter test
cd app && flutter run -d linux          # desktop dev
cd app && flutter build linux           # release bundle
cd app && flutter build apk --debug     # android debug apk

# Platform artifacts (populated in later milestones)
tools/build_linux.sh    # libomniproxy.so (c-shared) + pkexec helper
tools/build_android.sh  # gomobile bind → .aar
tools/build_windows.sh  # omniproxy.dll + wintun.dll bundling
```

## Commit conventions

One commit per milestone (see `docs/implementation-plan.md` §5). Message style:
`m1: scaffold repo, docs, and go workspace` · `core: add config engine with encrypted persistence`.

## Status

Phase 1 in progress — see `docs/implementation-plan.md` for the current milestone.
