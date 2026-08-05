# Sequence Diagrams — OmniProxy Atlas

This document is one of three *atlas* diagrams that consolidate how OmniProxy actually
works end to end. It complements — it does not replace — the engineering docs:

- `docs/implementation-plan.md` — scope and milestones (this doc shows the MVP wiring).
- `docs/api-contract.md` — the canonical Flutter ↔ Go contract (method names and
  request/response schemas that every sequence here carries over the wire).
- `docs/FLOWCHARTS.md` — state machines and decision flows (decisions referenced here).
- `docs/CLASS_DIAGRAMS.md` — static structure of the participants used below.
- `docs/platform-notes.md` — per-platform constraints that shape these flows.

## Reading conventions

- Participants are numbered `[1] … [n]`; the number appears in the participant bar and
  is reused across diagrams so you can follow one role across many flows.
- Every diagram is anchored to the source that implements it (`path:line`). Line numbers
  point at the entry point named in the first message.
- Arrows: solid `->` = synchronous call / return, `->>` = async or cross-thread message,
  `-->>` = callback/event delivery.
- The "Why" paragraph after each diagram explains a design decision or tradeoff that the
  diagram makes visible. Where a decision is explained in another doc, it is referenced
  rather than repeated.

### Roles used across diagrams

| # | Role | Where |
|---|------|-------|
| 1 | UI shell (widgets) | `app/lib/app/app_root.dart` |
| 2 | Riverpod providers / notifiers | `app/lib/state/providers.dart` |
| 3 | `ApiClient` (bridge or mock) | `app/lib/core/api_client.dart`, `app/lib/core/client_factory.dart` |
| 4 | Bridge transport (platform) | `app/lib/core/bridge/bridge_linux.dart`, `bridge_android.dart` |
| 5 | Native glue (c-shared / gomobile) | `core/glue/glue.go`, `core/mobile/mobile.go` |
| 6 | Core `Facade` | `core/core.go` |
| 7 | Core sub-services | `core/vpn`, `core/server`, `core/tunnel`, `core/store`, `core/secret` |
| 8 | sing-box engine | `engine/engine.go` |
| 9 | Android host (Kotlin) | `app/android/.../omniproxy/*.kt` |

---

## 1. App startup — Linux (FFI transport)

```mermaid
sequenceDiagram
    participant UI as [1] UI shell
    participant P as [2] Providers
    participant F as [3] Client factory
    participant T as [4] LinuxBridge
    participant G as [5] Go glue (c-shared)
    participant C as [6] Core Facade

    UI->>UI: runApp(ProviderScope)  main.dart:14
    UI->>P: build apiClientProvider
    P->>F: buildApiClient()  client_factory.dart:20
    F->>F: platform == linux?
    F->>T: createLinuxTransport(dataDir, helperPath)  client_factory.dart:20
    T->>G: dlopen(libomniproxy.so)  bridge_linux.dart:27
    T->>G: omniproxy_init({dataDir,logLevel,helperPath})  bridge_linux.dart:44
    G->>C: core.New(Config)  glue.go:72, core.go:74
    C->>C: open omniproxy.db (WAL) + migrate  store/store.go
    C->>C: load omniproxy.conf (encrypted)  config/engine.go
    C->>C: GetOrCreateDataKey -> secret store  crypto.go:29
    C-->>G: init ok
    G-->>T: 0
    T-->>F: LinuxBridge ready
    F-->>P: BridgeApiClient  bridge_api_client.dart:9
    P-->>UI: apiClientProvider ready
    UI->>P: first paint (dashboard)
    P->>C: getSettings, listServers, getLogs (backfill)  providers.dart:33,53
    C-->>P: data
```

**Why.** The Dart side is pure transport: `client_factory.dart:12` picks a transport by
platform and nothing else (see `docs/FLOWCHARTS.md` § Client factory). Initialization is a
single synchronous C call so the isolate does not race the core; all later calls reuse the
same `Facade`. Data key creation is idempotent (`GetOrCreateDataKey`) so restarting the app
never rotates the at-rest key and existing ciphertext stays decryptable
(`core/secret/crypto.go:29`).

---

## 2. App startup — Android (MethodChannel transport)

```mermaid
sequenceDiagram
    participant A as [9] MainActivity
    participant B as [9] Bridge (Kotlin)
    participant G as [5] Go core (gomobile AAR)
    participant D as [4] AndroidBridge
    participant P as [2] Providers

    A->>A: configureFlutterEngine  MainActivity.kt:23
    A->>B: set events channel  Bridge.kt:194
    B->>G: ensureInit(appContext)  Bridge.kt:59
    B->>B: startDefaultNetworkMonitor  Bridge.kt:81
    B->>D: MethodChannel com.omniproxy/bridge  bridge_android.dart:17
    D-->>A: invokeMethod("getVersion") routed to handleCall  MainActivity.kt:37
    A->>G: executeRequest -> Go  Bridge.kt:213
    G-->>A: response
    A-->>D: result
    D-->>P: BridgeApiClient ready
```

**Why.** On Android the Flutter engine owns the lifecycle, so the bridge is installed from
`configureFlutterEngine` rather than lazily. `Bridge` starts its own `HandlerThread` poller
(`Bridge.kt:251`) that drains Go events every 25 ms and forwards them over the events
channel — the same polled-event model as Linux, because Go cannot safely push into the
Flutter engine thread (see diagram § 6).

---

## 3. VPN connect — Android (consent + TUN fd)

```mermaid
sequenceDiagram
    participant UI as [1] UI shell (dashboard)
    participant P as [2] ConnectionNotifier
    participant B as [3] BridgeApiClient
    participant D as [4] AndroidBridge
    participant A as [9] MainActivity
    participant S as [9] OmniProxyVpnService
    participant K as [9] Bridge.setTunFd
    participant G as [6/7] Core vpn + tunnel
    participant E as [8] Engine

    UI->>P: connect(serverId)  providers.dart:161
    P->>B: connect(serverId, mode: vpn)  providers.dart:164
    B->>D: request("connect", {...})  bridge_api_client.dart
    D->>A: invokeMethod("connect")  MainActivity.kt:37
    A->>A: handleConnect  MainActivity.kt:48
    A->>A: VpnService.prepare() -> consent
    alt no consent yet
        A->>S: startActivityForResult (system consent UI)
        S-->>A: grant -> continue
    end
    A->>S: startForegroundService(request)  MainActivity.kt:69
    S->>S: startForeground(DATA_SYNC notification)  OmniProxyVpnService.kt:15
    S->>S: establishTun() -> VpnService.Builder  OmniProxyVpnService.kt
    S->>K: setTunFd(fd)  Bridge.kt:198
    K->>G: SetTunFd -> engine.SetPlatformInterface(FdTunPlatform)  mobile.go, engine/platform_fd.go
    S->>G: executeRequest("connect", requestJson)  Bridge.kt:227
    G->>E: tunnel.Start(options) -> engine.Start  vpn/service.go:156
    E-->>G: running
    G-->>S: ok
    G-->>P: stateChanged: connected (via poller)  providers.dart:152
    P-->>UI: state == connected
```

**Why.** The TUN device is created by Android's `VpnService`, not by sing-box: Android
forbids apps from creating raw TUN interfaces, and the system must approve the network via
the consent flow before any traffic can be routed. The file descriptor is handed to Go,
which feeds it to sing-box through `FdTunPlatform` (`engine/platform_fd.go:33`); the engine
never opens its own TUN on this path. The `VpnService` (foreground, persistent
non-dismissible notification per PRD §3.1) also hosts the core for the lifetime of the
tunnel so the tunnel survives screen lock.

---

## 4. VPN connect — Linux (privileged helper)

```mermaid
sequenceDiagram
    participant P as [2] ConnectionNotifier
    participant T as [4] LinuxBridge
    participant G as [6] Core Facade
    participant S as [7] vpn.Service
    participant M as [7] tunnel.Manager
    participant R as [7] HelperAwareRunner
    participant H as [7] omniproxy-helper (root)
    participant E as [8] Engine (in helper)

    P->>T: request("connect", {serverId, mode: vpn})
    T->>G: omniproxy_request("connect")  glue.go:115
    G->>S: Connect(serverId, vpn)  vpn/service.go:156
    S->>M: Start(profile, vpn)  tunnel/manager.go:62
    M->>R: Start(options)  helper_client.go:83
    R->>R: mode == vpn ? spawn helper : in-process  helper_client.go:84
    R->>H: pkexec omniproxy-helper --socket XDG_RUNTIME_DIR/omniproxy/helper.sock  helper_client.go:41
    H->>H: helperhost.Run -> listen Unix socket  helperhost.go:24
    R->>H: connect (seq, engine.Options)  helperproto.go
    H->>H: Host.Connect: open TUN (root), apply options  helperhost.go:90
    H->>E: engine.Start(options)  engine/engine.go:44
    E-->>H: running
    H-->>R: response {ok, state: connected}
    R-->>M: ok
    M-->>S: ok
    S-->>G: running
    G-->>P: stateChanged: connected (polled event)
```

**Why.** Linux TUN creation plus auto-routing needs `CAP_NET_ADMIN`, so the engine runs
inside a short-lived root helper process instead of the app process (this split and its
rationale are specified in `docs/implementation-plan.md` § Linux privileges). The helper is
single-connection: one control socket, one tunnel, clean teardown on socket close or
`quit` so no orphaned root TUN survives a crash (`helperhost.go:146`). Timeouts: spawn 15s,
connect 30s, disconnect 5s (`helper_client.go`). Proxy mode never needs root and stays
in-process (diagram § 5).

---

## 5. Proxy-mode connect — in-process (no root, no helper)

```mermaid
sequenceDiagram
    participant P as [2] ConnectionNotifier
    participant T as [4] LinuxBridge / AndroidBridge
    participant G as [6] Core Facade
    participant S as [7] vpn.Service
    participant M as [7] tunnel.Manager
    participant R as [7] HelperAwareRunner / InProcessRunner
    participant E as [8] Engine

    P->>T: request("connect", {serverId, mode: proxy})
    T->>G: omniproxy_request("connect")
    G->>S: Connect(serverId, proxy)  vpn/service.go:156
    S->>M: Start(profile, proxy)  tunnel/manager.go:62
    M->>R: Start(options)  helper_client.go:83
    R->>R: mode == proxy -> InProcessRunner  runner.go:31
    R->>E: engine.Start(options)  engine/engine.go:44
    E->>E: mixed inbound 127.0.0.1:<port> (SOCKS5+HTTP)  registry.go
    E-->>R: running
    R-->>M: ok
    S-->>G: running
    G-->>P: stateChanged: connected
```

**Why.** Proxy mode exposes a local `mixed` inbound (SOCKS5 + HTTP on one port), so no
privileges, no `VpnService`, and no helper are needed. On Android this still requires a
foreground service to keep the engine alive in background — `VpnProxyService`
(`VpnProxyService.kt:14`), which starts the notification and stays `START_STICKY`. On
Windows this same in-process path is expected to run the engine inside the app
(`docs/platform-notes.md`).

---

## 6. Event delivery — publish to ring to UI

```mermaid
sequenceDiagram
    participant E as [8] Engine / core services
    participant B as [7] api.EventBus
    participant G as [5] Go glue / gomobile
    participant R as [5] EventRing (cap 512)
    participant P as [4] Poller (Linux timer / Android HandlerThread)
    participant D as [4] Transport event stream
    participant U as [2] Providers -> UI

    E->>B: Publish(stateChanged | logAppended)  events.go:66
    B->>G: facadeSink.SendEvent -> enqueueEvent  core.go:407, glue.go:143
    G->>R: Push(event)  ring.go:29
    alt ring full
        R->>R: drop oldest (cap 512)
    end
    loop every 15ms (Linux) / 25ms (Android)
        P->>G: omniproxy_poll_events / pollOnce  glue.go:133, Bridge.kt:266
        G->>R: Drain()  ring.go:39
        R-->>P: []events
        P->>D: stream event
        D->>U: connectionProvider._onEvent / logsProvider._onEvent  providers.dart:152,207
    end
```

**Why.** Events are **polled, never pushed**. A native callback into Dart can deadlock when
the Flutter isolate is blocked inside a synchronous FFI call that happens to publish an
event — the callback cannot run until the call returns. Polling on a short timer keeps the
C ABI trivially serializable and makes both transports behave identically (Linux poll
every 15 ms, Android every 25 ms). The ring is deliberately small (512) and drops the
oldest on overflow rather than blocking a producer (see `docs/FLOWCHARTS.md` § Event ring).

---

## 7. Disconnect and shutdown

```mermaid
sequenceDiagram
    participant UI as [1] UI shell
    participant P as [2] ConnectionNotifier
    participant T as [4] Transport
    participant G as [6] Core Facade
    participant S as [7] vpn.Service
    participant M as [7] tunnel.Manager
    participant R as [7] Runner
    participant E as [8] Engine
    participant A as [9] Android host

    UI->>P: disconnect()  providers.dart:167
    P->>T: request("disconnect")
    T->>G: omniproxy_request("disconnect") / channel
    G->>S: Disconnect()  vpn/service.go:213
    S->>M: Stop()  tunnel/manager.go:85
    M->>R: Stop()
    alt in-process / proxy
        R->>E: engine.Close()  engine.go:76
    else helper (VPN, Linux)
        R->>H: disconnect (seq)  helper_client.go:328
        H->>E: StopEngine()  helperhost.go:146
        H-->>R: response {ok}
        R->>H: close / quit -> helper exits, socket removed
    end
    S-->>G: state: disconnected
    G-->>P: stateChanged: disconnected
    P-->>UI: state == disconnected
    Note over A: Android: MainActivity.stopConnectionHosts  MainActivity.kt:92
    Note over A: Android: stopService, notification cleared
```

**Why.** Disconnect is a graceful two-phase: the state machine (diagram § 8 of
FLOWCHARTS) moves to `disconnected` only after the runner has fully stopped the engine. On
the helper path, teardown is symmetric to setup — the client sends `disconnect`, then
`quit`, and the helper exits, dropping the TUN. Android additionally tears down the
foreground hosts in `stopConnectionHosts` because the system service (not the core) owns
the process-level lifecycle.

---

## 8. Latency test

```mermaid
sequenceDiagram
    participant UI as [1] ServersScreen
    participant P as [2] ServersNotifier
    participant B as [3] BridgeApiClient
    participant T as [4] Transport
    participant G as [6] Core Facade
    participant M as [7] server.Manager
    participant L as [7] LatencyTester

    UI->>P: testLatency(serverId)  servers_screen.dart:292, providers.dart:101
    P->>B: testServerLatency(id)
    B->>T: request("testServerLatency")
    T->>G: dispatch -> Facade.TestServerLatency  core.go:318
    G->>M: TestLatency(ctx, id)  server/manager.go:137
    alt tester set
        M->>L: TestLatency -> TCP dial, 3s timeout  tunnel/latency.go:13
        L-->>M: ms
        M-->>G: {latencyMs}
    else no tester
        M-->>G: ErrLatencyUnavailable  server/manager.go:17
    end
    G-->>P: latencyTested event + response
    P-->>UI: snackbar "name: X ms", card updated  servers_screen.dart:298
```

**Why.** Latency testing is an injectable `LatencyTester` (`server/manager.go:24`); the
production implementation is a plain TCP dial with a 3 s timeout (`tunnel/latency.go:13`),
not a sing-box url-test. When no tester is installed the manager reports
`ErrLatencyUnavailable`, which `Facade.MapError` maps to the `engine_error` code
(`core/core.go:178`) instead of silently returning 0 — surfaced in the UI, never silently
degraded.

---

## 9. Server save — CRUD + encrypted secret storage

```mermaid
sequenceDiagram
    participant UI as [1] ServerEditScreen
    participant P as [2] ServersNotifier
    participant B as [3] BridgeApiClient
    participant T as [4] Transport
    participant G as [6] Core Facade
    participant M as [7] server.Manager
    participant R as [7] ServerRepository
    participant DB as [7] SQLite (WAL)
    participant K as [7] secret.Store + crypto

    UI->>P: add(server) / updateServer(server)  server_edit_screen.dart:384
    P->>B: addServer / updateServer
    B->>T: request("addServer" | "updateServer")
    T->>G: dispatch -> Facade.AddServer / UpdateServer  core.go:250,256
    G->>M: AddServer / UpdateServer  server/manager.go:60,70
    M->>R: Create / Update (full replace)  server_repository.go
    R->>DB: INSERT or UPDATE row, secret columns empty  server_repository.go
    R->>K: "Store secret refs: omniproxy.server.{id}.{ref} (server_repository.go:20)"
    K->>K: Seal(value, dataKey) AES-256-GCM  crypto.go:56
    K->>K: keyring / Keystore / libsecret store
    K-->>R: ok
    R-->>M: id
    M-->>G: id
    G-->>P: ok
    P-->>UI: pop editor, list refreshed
```

**Why.** Credentials never touch SQLite: the row stores empty strings for secret columns
and the real values live in OS-native secure storage keyed by
`omniproxy.server.<id>.<ref>` (`server_repository.go:20`), encrypted with the at-rest data
key (`crypto.go:56`). The Dart editor never round-trips secrets on edit — it sends the
profile and the core reconciles stored credentials by ref. On Android the store is
`KeystoreSecretStore` (AES-256 non-exportable key in Android Keystore, ciphertext in
private SharedPreferences) because a previous in-memory store silently corrupted stored
config on process restart (see `KeystoreSecretStore.kt`).

---

## 10. Import / export / share

```mermaid
sequenceDiagram
    participant UI as [1] ServersScreen
    participant P as [2] ServersNotifier
    participant B as [3] BridgeApiClient
    participant T as [4] Transport
    participant G as [6] Core Facade
    participant M as [7] server.Manager
    participant S as [7] link parser / serializer

    UI->>P: import(ImportSource)  servers_screen.dart:374
    P->>B: importServers(source)
    B->>T: request("importServers")
    T->>G: dispatch -> Facade.ImportServers  core.go:284
    G->>M: parse links (vless/vmess/ss/trojan) or .onnproxy blob
    M-->>G: ImportResponse {added, failed, errors}
    G-->>P: result -> snackbar / dialog  servers_screen.dart:378
    UI->>P: export(ids)  servers_screen.dart:321
    P->>B: exportServers(ids, format: 'onnproxy'|'links')
    B->>T: request("exportServers")
    T->>G: dispatch -> Facade.ExportServers  core.go:298
    G->>S: serialize selected servers
    S-->>G: blob (json or share links)
    G-->>P: blob -> copy-to-clipboard dialog  servers_screen.dart:737
```

**Why.** Export/import is fully delegated to core so every platform produces byte-identical
output (shared `.onnproxy` config format — PRD rule). Share-link generation mirrors
`core/server/linkgen.go`; the Dart mirror in `app/lib/core/share_links.dart:11` exists only
so the **mock** transport's `exportServers(format: 'links')` output matches the real core —
the bridge path always delegates to core. `shareLinkFor` returns `null` for `ssh`
(no share-link representation), which is surfaced rather than faked.

---

## 11. Error / reconnect path

```mermaid
sequenceDiagram
    participant E as [8] Engine
    participant R as [7] tunnel.Runner
    participant S as [7] vpn.Service
    participant G as [6] Core Facade
    participant P as [2] ConnectionNotifier
    participant U as [1] UI dashboard

    E-->>R: Start() fails (network, handshake, timeout)
    R-->>S: error
    S->>S: runLoop: shouldRetry(attempt)?  vpn/service.go:324
    alt attempt < max (default 5)  -- auto-retry from Reconnecting
        S->>S: transition(StateReconnecting, err)  service.go:297
        S->>S: wait(backoff 1s,2s,4s..30s)  service.go:347-366
        S->>R: Start(options) again (same goroutine)
        Note over S: success -> transition(StateConnected) service.go:282<br/>failure -> loop back to shouldRetry
    else attempts exhausted
        S->>S: transitionError -> StateError  service.go:387
        S->>G: emitState(error)  service.go:482
        G->>P: stateChanged {state: error, session.error}  providers.dart:152
        P-->>U: _ErrorBanner with message  dashboard_screen.dart:297
    end
    U->>P: disconnect() -> Dismiss  dashboard_screen.dart:376
    P->>S: disconnect -> forceDisconnect clears error  service.go:453
```

**Why.** Errors funnel through the same state machine as successful transitions so the UI
only ever reacts to `stateChanged` events (single source of truth). A failed `Start` with
retry budget left transitions straight to `StateReconnecting` and stays there across the
exponential backoff (`service.go:297`, default 1s..30s cap, 5 attempts — `DefaultRetryPolicy`
`service.go:46`) — it never emits `error` first; `StateError` is reached only when retries
are exhausted (`transitionError`, `service.go:387`). The retry is an in-loop loop, not a
re-entrant command, so a user-initiated `cmdDisconnect` during backoff cancels it cleanly
(`wait`, `service.go:347-366`). On Android, network changes are reported by the host's
`DefaultNetworkMonitor` (ConnectivityManager) into `FdTunPlatform`; sing-box re-evaluates
routes from there (`engine/platform_monitor.go`).
