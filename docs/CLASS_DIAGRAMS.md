# Class Diagrams — OmniProxy Atlas

Static structure of the three codebases (Go core, Flutter/Dart app, Android host) and the
helper protocol, consolidated from source. This is the third of three *atlas* docs:

- `docs/SEQUENCE_DIAGRAMS.md` — dynamic flows between these types.
- `docs/FLOWCHARTS.md` — decisions and state machines that drive them.
- `docs/api-contract.md` — the wire contract the Dart `ApiClient` and Go `api.Handler`
  implement.

Reading conventions:

- `«interface»` marks an interface/abstract type; a dashed line to it (`<|--`) is an
  implementation.
- Solid diamond (`*--`) = composition (owner creates/destroys), hollow diamond (`o--`) =
  aggregation, plain arrow (`-->`) = usage dependency.
- Each class is anchored with `path:line`; only the primary location is listed.
- After each diagram, "Relationship notes" call out the non-obvious choices and the reason.

---

## 1. Core layer — Facade and collaborators

```mermaid
classDiagram
    direction LR
    class Facade {
        +Config cfg
        +api.Dispatcher dispatcher
        +config.Engine settings
        +server.Manager servers
        +tunnel.Manager tunnel
        +vpn.Service vpn
        +log.Logger logger
        +Resolve(serverID) ServerProfile
        +MapError(err) api.Error
        +GetVersion() GetVersionResponse
        +ListServers(req) ServerListResponse
        +AddServer(s) IDResponse
        +UpdateServer(s) ServerProfile
        +DeleteServer(id) error
        +DuplicateServer(id) IDResponse
        +ImportServers(req) ImportResponse
        +ExportServers(req) ExportResponse
        +TestServerLatency(id) LatencyResponse
        +Connect(req) StateResponse
        +Disconnect() StateResponse
        +ConnectionState() StateResponse
        +GetLogs(req) LogsResponse
        +GetSettings() SettingsResponse
        +UpdateSettings(req) SettingsResponse
        +Subscribe(req) error
        +Unsubscribe() error
        +Close() error
    }
    class Config {
        +DataDir
        +LogLevel
        +HelperPath
    }
    class api.Handler {
        <<interface>>
    }
    class api.Dispatcher {
        +Dispatch(method, requestJSON) Response
        -dispatch(method, requestJSON) Response
        +ok(data) Response
        +fail(err) Response
    }
    class api.EventBus {
        +Subscribe(sink) subscription
        +Unsubscribe(sub)
        +Publish(Event)
    }
    class api.Event {
        +Type
        +Data
    }
    class config.Engine {
        +Load() Settings
        +Save(Settings)
        +LoadLegacyServers()
    }
    class server.Manager {
        +ListServers() List
        +GetServer(id) ServerProfile
        +AddServer(p) string
        +UpdateServer(p) error
        +DeleteServer(id) error
        +Duplicate(id) string
        +TestLatency(ctx, id) int
    }
    class store.ServerRepository {
        <<interface>>
        +Create(p) string
        +Update(p) error
        +Get(id) ServerProfile
        +Delete(id) error
        +List(q) List
    }
    class server.LatencyTester {
        <<interface>>
        +TestLatency(ctx, p) int
    }
    class tunnel.Manager {
        +Start(profile, mode) error
        +Stop() error
        +Active() bool
        +Server() ServerProfile
        +Mode() Mode
    }
    class tunnel.Runner {
        <<interface>>
        +Start(options) error
        +Stop() error
        +Running() bool
    }
    class vpn.Service {
        +Connect(serverID, mode) error
        +Disconnect() error
        +Reconnect() error
        +State() ConnectionState
        +Session() VPNSession
    }
    class vpn.Tunneler {
        <<interface>>
        +Start(options) error
        +Stop() error
    }
    class vpn.RetryPolicy {
        <<interface>>
        +Backoff(attempt) Duration
        +MaxAttempts() int
    }
    class secret.Store {
        <<interface>>
        +Get(key) string
        +Set(key, value) error
        +Delete(key) error
    }
    class log.Logger {
        +Write(entry)
    }
    class models.ServerProfile

    Facade *-- Config
    Facade *-- api.Dispatcher
    Facade *-- config.Engine
    Facade *-- server.Manager
    Facade *-- tunnel.Manager
    Facade *-- vpn.Service
    Facade *-- log.Logger
    Facade ..> api.Handler
    api.Handler <|-- Facade
    api.Dispatcher --> api.Handler
    api.Dispatcher *-- api.EventBus
    api.EventBus o-- api.Event
    server.Manager --> store.ServerRepository
    server.Manager o-- server.LatencyTester
    tunnel.Manager --> tunnel.Runner
    vpn.Service o-- vpn.Tunneler
    vpn.Service o-- vpn.RetryPolicy
    server.Manager --> models.ServerProfile
    tunnel.Manager --> models.ServerProfile
    config.Engine --> secret.Store
```

**Relationship notes.**

- `Facade` is a composition root: it owns every service (`*--`) and exposes them all
  through the `api.Handler` interface (`core/core.go:74`). `api.Dispatcher` talks only to
  the interface, so the two sides of the bridge are decoupled — that is what lets the same
  contract run over FFI, MethodChannel, and the mock.
- `server.LatencyTester`, `vpn.Tunneler`, `vpn.RetryPolicy`, and `store.ServerRepository`
  are interfaces so tests can substitute them (mock latency, in-memory repo, fixed retry
  policy). The core itself never depends on a concrete network prober.
- `config.Engine` → `secret.Store`: the settings blob is encrypted at rest with the
  OS-backed data key (`core/secret/crypto.go:56`); the repo itself never sees the plaintext
  settings.

---

## 2. Engine layer — sing-box wrapper

```mermaid
classDiagram
    direction LR
    class Engine {
        +box.Box box
        +LogSink logSink
        +Platform platform
        +New(logSink) Engine
        +SetPlatformInterface(pi) error
        +Start(options) error
        +Close() error
        +Running() bool
    }
    class Options {
        +Mode mode
        +LogLevel
        +CacheFile
        +Inbound
        +Outbound
        +DNS
    }
    class Outbound {
        +Protocol protocol
        +Address
        +Port uint16
        +Username
        +Password
        +UUID
        +Flow
        +Security
        +Transport
        +TLS
    }
    class Mode {
        <<enumeration>>
        VPN
        Proxy
    }
    class Protocol {
        <<enumeration>>
        vless
        vmess
        shadowsocks
        trojan
        socks
        http
        ssh
    }
    class TransportSettings {
        +Type
        +Path
        +Host
        +MaxEarlyData
        +EarlyDataHeaderName
    }
    class TLSSettings {
        +ServerName
        +Insecure
        +ALPN
        +Fingerprint
    }
    class Platform {
        <<interface>>
        +Initialize(manager) error
        +AutoDetectInterfaceControl(fd) error
        +CreateDefaultInterfaceMonitor() monitor
        +NetworkInterfaces() list
        +UpdateDefaultInterface(name, index)
    }
    class noopPlatform {
        -implemented = false
    }
    class FdTunPlatform {
        +fd int
        +SetFd(fd)
        +SetProtectFunc(f)
        +OpenInterface(options) Tun
    }
    class platformInterfaceMonitor {
        -networkManager
        +defaultInterfaceName
        +defaultInterfaceIndex
    }
    class LogSink {
        <<interface>>
        +WriteMessage(level, message)
    }
    class Level {
        <<enumeration>>
        trace
        debug
        info
        warn
        error
    }
    class registry {
        +newContext(ctx) ctx
    }

    Engine --> Options
    Engine o-- Platform
    Engine --> LogSink
    Engine --> registry
    Options o-- Mode
    Options o-- Outbound
    Options o-- Level
    Outbound o-- Protocol
    Outbound o-- TransportSettings
    Outbound o-- TLSSettings
    Platform <|-- noopPlatform
    Platform <|-- FdTunPlatform
    FdTunPlatform *-- platformInterfaceMonitor
    platformInterfaceMonitor o-- adapter.NetworkManager
```

**Relationship notes.**

- `Engine` is deliberately thin: it holds one `box.Box`, adapts sing-box logs to
  `LogSink`, and wires the `Platform` (Linux: `engine/platform_fd.go:19`; Android overrides
  via the mobile bridge). `noopPlatform` (`engine/platform.go:22`) opts out of every
  platform hook so the same engine builds on hosts that create TUN themselves.
- `FdTunPlatform` composes a passive `platformInterfaceMonitor`
  (`engine/platform_monitor.go:31`): on Android the host reports the default interface
  through `Bridge.DefaultNetworkMonitor` instead of the engine watching netlink, because
  Android forbids an app from subscribing to physical-interface events. The monitor keeps
  `NetworkManager`'s interface list populated so `AutoDetectInterface` dialing doesn't fail
  with "no available network interface" (`platform_fd.go:145`).
- `registry.newContext` (`engine/registry.go:31`) is a package-level factory, shown as a
  static class, that registers exactly the MVP's protocol registries.

---

## 3. Dart app layer — contract, transport, state

```mermaid
classDiagram
    direction LR
    class ApiClient {
        <<interface>>
        +getVersion() AppVersion
        +listServers(query) List
        +getServer(id) ServerProfile
        +addServer(s) string
        +updateServer(s) ServerProfile
        +deleteServer(id)
        +duplicateServer(id) string
        +importServers(source) ImportResult
        +exportServers(ids, format) string
        +testServerLatency(id) int
        +connect(serverId, mode)
        +disconnect()
        +getConnectionState() ConnectionSnapshot
        +getLogs(afterSeq) List
        +getSettings() AppSettings
        +updateSettings(s) AppSettings
        +subscribe(types)
        +unsubscribe()
        +events Stream
    }
    class BridgeApiClient {
        -BridgeTransport transport
        -_call(method, body) Map
    }
    class MockApiClient {
        -servers List
        -state ConnectionState
        -connectDelay 600ms
        -latencyDelay 250ms
    }
    class BridgeTransport {
        <<interface>>
        +request(method, body) BridgeResponse
        +events Stream
        +start()
        +stop()
    }
    class BridgeResponse {
        +bool ok
        +Map data
        +String? errorCode
        +String? errorMessage
    }
    class LinuxBridge {
        +DynamicLibrary lib
        +Timer pollTimer
        -pollInterval 15ms
        +_initNative
        +_requestNative
        +_pollEventsNative
        +_shutdownNative
    }
    class AndroidBridge {
        +MethodChannel bridge
        +EventChannel events
        +_requestDart
    }
    class WindowsBridge {
        +UnsupportedError in start()
    }
    class ServerProfile {
        +String id
        +String name
        +ServerProtocol protocol
        +String address
        +int port
        +String? username
        +String? password
        +String? cipher
        +String? uuid
        +String? flow
        +TlsSettings tls
        +SshSettings ssh
        +TransportSettings? transport
        +bool favorite
        +bool enabled
        +int lastLatencyMs
    }
    class TlsSettings {
        +enabled
        +allowInsecure
        +serverName
        +fingerprint
    }
    class TransportSettings {
        +TransportType type
        +path
        +host
    }
    class SshSettings {
        +user
        +privateKey
    }
    class AppSettings {
        +ThemePreference theme
        +ConnectionMode connectionMode
        +IPv6Mode ipv6Mode
        +LogLevel logLevel
    }
    class LogEntry {
        +int seq
        +LogLevel level
        +component
        +message
        +timestamp
    }
    class AppEvent {
        +String type
        +Map data
    }
    class VpnSession {
        +serverId
        +mode
        +startedAt
        +error
    }
    class ConnectionSnapshot {
        +ConnectionState state
        +VpnSession session
    }
    class ApiError {
        +String code
        +String message
    }
    class ConnectionNotifier {
        +connect(serverId, mode)
        +disconnect()
        -_onEvent(AppEvent)
    }
    class ServersNotifier {
        +setQuery(query)
        +refresh()
        +add(server)
        +updateServer(server)
        +delete(id)
        +duplicate(id)
        +import(source)
        +export(ids, format)
        +testLatency(id)
        +toggleFavorite(id)
        +toggleEnabled(id)
    }
    class SettingsNotifier {
        +update(settings)
    }
    class LogsNotifier {
        -_cap 500
        +refresh()
        +clear()
    }

    ApiClient <|-- BridgeApiClient
    ApiClient <|-- MockApiClient
    BridgeApiClient *-- BridgeTransport
    BridgeTransport <|-- LinuxBridge
    BridgeTransport <|-- AndroidBridge
    BridgeTransport <|-- WindowsBridge
    LinuxBridge --> BridgeResponse
    ServerProfile *-- TlsSettings
    ServerProfile *-- TransportSettings
    ServerProfile *-- SshSettings
    AppEvent --> ConnectionSnapshot
    BridgeApiClient --> ApiError
    ConnectionNotifier --> ApiClient
    ServersNotifier --> ApiClient
    SettingsNotifier --> ApiClient
    LogsNotifier --> ApiClient
    ConnectionSnapshot o-- VpnSession
```

**Relationship notes.**

- `ApiClient` is the app's only view of the core (`app/lib/core/api_client.dart:9`).
  `BridgeApiClient` (`bridge_api_client.dart:9`) is pure transport plumbing — it calls
  `BridgeTransport.request`, unwraps `BridgeResponse`, and throws `ApiError` with the
  contract error code; `MockApiClient` implements the same interface in memory for dev/test
  and any platform without a bridge (`mock_api_client.dart:17`, seeds three servers — "Tokyo
  Relay" vless, "Frankfurt Shadowsocks", "Local HTTP", `mock_api_client.dart:41-83`).
- `BridgeTransport` is the seam (`bridge_transport.dart:24`): Linux (FFI), Android
  (MethodChannel), Windows (FFI). Each transport owns the poller/timer that drains the
  event ring, so the poll cadence is a per-platform concern (15 ms Linux, 15 ms Windows, 25 ms Android).
- `ServerProfile` composes its TLS/transport/SSH settings; credential fields are plain
  strings on the wire and are stored as *refs* by the core (see FLOWCHARTS § 9/10 in the
  sequence doc) — never as plaintext.
- Notifiers (`ConnectionNotifier`, `ServersNotifier`, …) depend on `ApiClient` and react
  to `AppEvent`s; the UI never calls the transport directly.

---

## 4. Android host (Kotlin)

```mermaid
classDiagram
    direction LR
    class MainActivity {
        +configureFlutterEngine(engine)
        -handleCall(method, requestJson, result)
        -handleConnect(requestJson, result)
        -startVpnHost(requestJson, result)
        -startProxyHost(requestJson, result)
        -stopConnectionHosts()
    }
    class Bridge {
        +ensureInit(context)
        +setEventsChannel(channel)
        +setTunFd(fd)
        +executeRequest(method, requestJson, result)
        -startPolling()
        -pollOnce()
        -refreshDefaultInterface(force)
    }
    class DefaultNetworkMonitor {
        +onNetworkChange()
    }
    class KeystoreSecretStore {
        -getOrCreateKey()
        -encrypt(value)
        -decrypt(raw)
    }
    class SecretStore {
        <<interface>>
        +get(key)
        +set(key, value)
        +delete(key)
    }
    class OmniProxyVpnService {
        +onStartCommand()
        -establishTun()
        -startForeground()
    }
    class VpnProxyService {
        +onStartCommand()
    }
    class Notifications {
        +notifyConnected()
        +cancel()
    }
    class MethodChannel {
        <<external>>
        com.omniproxy/bridge
    }

    MainActivity --> Bridge
    MainActivity --> OmniProxyVpnService
    MainActivity --> VpnProxyService
    Bridge --> MethodChannel
    Bridge o-- DefaultNetworkMonitor
    Bridge --> KeystoreSecretStore
    KeystoreSecretStore ..> SecretStore
    SecretStore <|-- KeystoreSecretStore
    OmniProxyVpnService --> Bridge
    VpnProxyService --> Bridge
    OmniProxyVpnService --> Notifications
    VpnProxyService --> Notifications
    Bridge --> mobile.GoCore
```

**Relationship notes.**

- `Bridge` is a singleton (`object`, `Bridge.kt:29`) holding the app context, the events
  channel, and the poll thread — one instance per process. `MainActivity` only wires it in
  `configureFlutterEngine` (`MainActivity.kt:23`) and routes the `connect` method specially
  (`handleConnect`) because of the `VpnService` consent flow; every other method goes
  straight to `executeRequest`.
- `OmniProxyVpnService` (VPN mode) and `VpnProxyService` (proxy mode) both depend on
  `Bridge` to hand the core the TUN fd and to run the `connect` command; `MainActivity`
  starts whichever matches the requested mode and calls `stopConnectionHosts` on
  disconnect (`MainActivity.kt:92`).
- `KeystoreSecretStore` implements `SecretStore` (`KeystoreSecretStore.kt:28`) with a
  non-exportable Keystore AES-256 key and ciphertext in private SharedPreferences. It is
  the OS-native secure store that the core's `secret.Store` interface abstracts over.

---

## 5. Helper protocol (Linux privileged path)

```mermaid
classDiagram
    direction LR
    class HelperRunner {
        +Start(options) error
        +Stop() error
        +Running() bool
        +Close() error
        -ensureHelper()
    }
    class HelperSpawner {
        <<interface>>
        +Spawn(socketPath) cmd
    }
    class HelperSpawnFunc {
        +Spawn(socketPath) cmd
    }
    class helperClient {
        -conn
        -seq uint64
        +connect(options) error
        +disconnect() error
        +close(sendQuit) error
        +alive() bool
        +send(msg)
        +await(seq, timeout)
    }
    class ClientMessage {
        +Type (connect|disconnect|ping|quit)
        +Seq
        +Options
    }
    class ServerMessage {
        +Type (response|event)
        +Seq
        +OK
        +State
        +Error
        +Event HelperLogEvent
    }
    class HelperLogEvent {
        +Level
        +Message
    }
    class Host {
        +Run(socketPath) error
        -Connect(msg)
        -Disconnect(seq)
        -Ping(seq)
        -StopEngine()
    }
    class Engine {
        +Start(options) error
        +Close() error
    }
    class tunnel.Runner {
        <<interface>>
    }

    tunnel.Runner <|-- HelperRunner
    HelperRunner *-- helperClient
    HelperRunner --> HelperSpawner
    HelperSpawner <|-- HelperSpawnFunc
    helperClient --> ClientMessage
    helperClient --> ServerMessage
    ServerMessage o-- HelperLogEvent
    ClientMessage o-- engine.Options
    Host *-- Engine
    Host --> ClientMessage
    Host --> ServerMessage
```

**Relationship notes.**

- `HelperRunner` (the `tunnel.Runner` implementation used for VPN mode on Linux,
  `helper_client.go:83`) composes one `helperClient` per session and a `HelperSpawner`.
  `NewHelperSpawner` (`helper_client.go:41`) is a `HelperSpawnFunc` that runs
  `pkexec omniproxy-helper`; the `OMNIPROXY_HELPER` env override swaps in a direct spawn
  for tests — a seam identical in shape to the core's other injected interfaces.
- Messages are `seq`-matched: `ClientMessage` carries the caller's sequence and
  `ServerMessage` echoes it (`helperproto.go`), so `await(seq, timeout)` can't be confused
  by a stale reply.
- `Host` (`helperhost.go`) owns the `Engine` lifecycle and serves exactly one control
  connection: closing the connection or a `quit` message triggers `StopEngine()`, which is
  how the design guarantees no orphaned root TUN on crash.
