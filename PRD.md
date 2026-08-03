# Product Requirements Document (PRD)
## Cross-Platform VPN Client Application

**Document version:** 1.0
**Status:** Draft
**Owner:** Product / Engineering

---

## 1. Product Overview

This document defines the product requirements for a commercial, multi-platform VPN client application targeting **Android, Windows, and Linux**. The application provides a unified, premium user experience across all platforms through a shared Flutter-based UI layer backed by a high-performance Go core and a `sing-box`-based networking engine.

### 1.1 Technology Stack

| Layer | Technology | Responsibility |
|---|---|---|
| Frontend | **Flutter** | Single UI codebase, consistent design system, responsive layouts, native-quality experience across mobile and desktop |
| Backend Core | **Golang** | VPN controller, configuration management, server management, routing logic, tunnel management, logging, communication with the networking engine |
| Networking Engine | **sing-box** | Protocol implementations, tunneling, routing enforcement — the application does not implement networking protocols from scratch |

### 1.2 Rationale

- A single Flutter codebase reduces duplicated UI effort across three platforms and guarantees visual and behavioral consistency.
- A Go core isolates all VPN logic, configuration, and state management from the UI layer, allowing the backend to be tested, versioned, and reused independently of the frontend.
- Building on `sing-box` avoids the substantial risk, cost, and security exposure of re-implementing VPN/proxy protocols, and gives the product a maintained, actively updated protocol engine.

---

## 2. Product Vision

The product's vision is to deliver a VPN client that is simultaneously approachable for casual users and powerful for advanced users, without forcing either group into an experience designed for the other.

The application combines:

- A simple, one-click VPN connection experience for everyday users
- Advanced networking capabilities for power users
- Professional-grade routing management
- Support for multiple tunnel protocols and protocol chaining
- A consistent, native-quality cross-platform experience

### 2.1 Target Users

| Segment | Needs |
|---|---|
| **Normal VPN users** | One-click connect, clear status, minimal configuration |
| **Advanced networking users** | Protocol chaining, custom routing, fine-grained tunnel control |
| **Network administrators** | Configuration import/export, logs, debug visibility, predictable behavior at scale |

---

## 3. Platform Requirements

### 3.1 Android

- Implementation via Android's native `VpnService` API
- TUN interface creation and lifecycle management
- Background connection persistence
- Persistent (non-dismissible) notification while connected, per Android platform requirements
- Automatic reconnect on network changes or drops
- Battery optimization handling (guidance/exemption flow for users, doze-mode resilience)
- Mobile-optimized UI: touch-first targets, bottom navigation, compact dashboard

### 3.2 Windows

- Native desktop application (not a wrapped web view)
- TUN interface via **Wintun**
- Background Windows service for tunnel management independent of the UI process
- Optional "start with system" behavior
- Network change monitoring (adapter changes, sleep/wake, network switch) with automatic recovery

### 3.3 Linux

- Native desktop application
- TUN interface management
- Support for major distributions (Debian/Ubuntu-based, Fedora/RHEL-based, Arch-based at minimum)
- Explicit handling of permission models (e.g., capabilities vs. root, polkit where applicable)
- Integration with common Linux network management stacks (NetworkManager, systemd-resolved) to avoid DNS/routing conflicts

### 3.4 Cross-Platform Consistency Requirements

- Identical feature set across platforms wherever the OS allows it; any platform-specific limitation must be explicitly surfaced in the UI rather than silently degraded
- Shared configuration format across platforms so a server/profile export from one platform imports cleanly on another

---

## 4. Flutter UI Requirements

The application must present a single, unified design system across all three platforms.

### 4.1 Design System Requirements

- Shared branding, components, and interaction patterns across Android, Windows, and Linux
- Responsive layout system that adapts between mobile (single-column, touch-first) and desktop (multi-pane, pointer/keyboard-first) form factors
- Material 3 design system as the foundation
- Full dark/light theme support, including a system-theme-follow option
- Smooth, purposeful animations (state transitions, connect/disconnect feedback) — not decorative-only motion
- Clean, predictable navigation structure (persistent primary navigation on desktop; bottom or drawer navigation on mobile)
- A professional dashboard as the default landing screen

---

## 5. Main Screens

### 5.1 Dashboard

The dashboard is the primary screen and default entry point.

**Displays:**
- VPN connection status (Disconnected / Connecting / Connected / Reconnecting / Error)
- Current server (name, country, protocol)
- Connect/Disconnect control (primary action)
- Connection duration (live timer)
- Real-time upload speed
- Real-time download speed
- Ping / latency to current server
- Session and cumulative traffic statistics (data used)

### 5.2 Server Manager

**Features:**
- Add servers (manual entry or via configuration link/QR where applicable)
- Edit existing servers
- Delete servers
- Import configurations (single or bulk)
- Export configurations (single or bulk)
- Test latency (single server and "test all")
- Mark/unmark servers as favorites

**Supported protocols:**
- VLESS
- VMess
- SOCKS
- SSH Tunnel
- HTTP Injector–style custom payload tunnels
- Other protocols supported by `sing-box` (extensible without UI rework)

**Server card information:**
- Name
- Country (with flag/indicator)
- Protocol
- Latency
- Status (online/offline/untested)

### 5.3 Tunnel Chain Builder

A visual, node-based editor for composing multi-hop tunnel chains.

**Example chain:**

```
VLESS → SOCKS → SSH Tunnel → Final VPN Interface
```

**Features:**
- Drag-and-drop node placement
- Add tunnel layers (nodes) to a chain
- Remove nodes from a chain
- Validate the chain (detect incompatible orderings, missing required parameters, cycles)
- Visual, inline error and warning indicators on invalid nodes/connections

### 5.4 Routing Manager

A visual rule-based routing system. **Users are never required to hand-edit JSON.**

**Rule types supported:**
- Domain rules
- IP rules
- Geo rules (GeoIP/Geosite-based)
- Direct rules
- Proxy rules
- Block rules

**Rule construction model (example):**

```
IF   Domain = example.com
THEN Use VPN Server A
```

Rules are composed through a structured form/condition builder in the UI; the underlying `sing-box` routing configuration is generated automatically from the visual rule set.

### 5.5 Advanced Mode

An opt-in mode for power users, hidden from the default experience to keep the primary flow simple.

**Exposes:**
- Raw protocol settings
- Advanced routing configuration
- Live preview of the generated `sing-box` configuration
- Application and connection logs
- Debug information (for support and troubleshooting)

---

## 6. VPN Connection Manager

**Responsibilities:**
- Start connection (establish tunnel via the configured protocol/chain)
- Stop connection (tear down tunnel cleanly, restore prior network state)
- Automatic reconnection on failure or network change, with backoff
- Continuous connection state monitoring
- Structured error handling with user-facing, actionable error messages
- Display of connection details (protocol, server, chain, session start time, negotiated parameters where relevant)

---

## 7. Flutter Architecture Requirements

### 7.1 Layered Architecture

**Flutter (UI) layer:**
- UI layer (screens, widgets)
- State management layer
- Navigation layer
- Theme system

**Go core:**
- VPN Service (orchestrates connection lifecycle)
- Configuration Engine (validates and persists server/routing/app configuration)
- Server Manager (CRUD and health-check logic for servers)
- Tunnel Manager (chain construction and lifecycle, delegates to `sing-box`)
- Logging (structured, leveled logging shared across all platforms)

### 7.2 Flutter ↔ Go Communication

The Go core runs as a native component on every platform; Flutter communicates with it through platform-appropriate bridges rather than a single one-size-fits-all mechanism:

- **Android:** Go core compiled via `gomobile`/cgo into a native library, invoked through Flutter **platform channels** (or FFI to the native library), with the Android `VpnService` implemented in the native layer and coordinated through the same bridge
- **Windows:** Go core compiled as a native library (DLL) or long-running background service; Flutter desktop communicates via **FFI** (direct calls into the DLL) and/or a local IPC channel if the core runs as a separate service process
- **Linux:** Go core compiled as a native shared library or background daemon; Flutter communicates via **FFI** or local IPC (e.g., a Unix domain socket) depending on whether elevated-privilege operations require process separation from the UI

A consistent internal API contract (method names, request/response schemas) should be defined once and implemented identically across all three bridge mechanisms, so the Flutter layer's calls into the core do not need to differ meaningfully by platform.

---

## 8. Data Models

The following core data models must be defined and shared across the application:

| Model | Purpose |
|---|---|
| **Server Profile** | A single configured server/protocol endpoint (name, protocol, address, credentials/keys, TLS settings, favorite flag, last-tested latency) |
| **Tunnel Node** | A single hop within a tunnel chain (protocol type, parameters, position) |
| **Tunnel Chain** | An ordered sequence of Tunnel Nodes forming a complete route to the VPN interface |
| **Routing Rule** | A single condition → action routing rule (match type, match value, action/target) |
| **VPN Session** | A single connection session (start time, duration, server/chain used, bytes up/down, status history) |
| **Application Settings** | User-level and app-level preferences (theme, auto-connect, startup behavior, notification preferences, advanced-mode toggle) |
| **Log Entry** | A single structured log line (timestamp, level, source component, message, optional context payload) |

---

## 9. Security Requirements

- Secure credential storage using OS-native secure storage (Android Keystore, Windows Credential Manager/DPAPI, Linux Secret Service/libsecret) rather than plaintext files
- Encryption of any locally persisted sensitive configuration data at rest
- Sound key management practices for any keys/secrets generated or imported by the app
- Safe logging: logs must never contain credentials, private keys, or full raw traffic content
- Configuration files protected against tampering and unauthorized read access
- Certificate validation enforced for all TLS-based protocol connections, with explicit, user-visible warnings if validation is ever bypassed (e.g., for self-signed certs in advanced mode)

---

## 10. Performance Requirements

### 10.1 Desktop (Windows/Linux)

- Fast application startup
- Low idle CPU usage
- Low idle memory consumption

### 10.2 Android

- Battery-efficient background operation
- Stable, long-running background connection without unexpected service termination
- Fast reconnect after network changes or app resume

---

## 11. Roadmap

### Phase 1 — MVP
- Flutter application shell (Android, Windows, Linux)
- Server management (add/edit/delete/import/export)
- Basic VPN connection (single server, no chaining)
- `sing-box` integration
- Dashboard

### Phase 2
- Advanced routing (visual routing manager)
- Tunnel chain builder
- Connection monitoring and statistics

### Phase 3
- User accounts
- Cloud synchronization of configuration
- Subscription system
- Remote server management

---

## 12. Risks

| Risk | Description | Mitigation Direction |
|---|---|---|
| **Flutter desktop limitations** | Flutter's desktop support (Windows/Linux) is less mature than mobile; native integrations (system tray, background services, OS-level networking hooks) may require additional platform-specific plugin work | Budget extra engineering time for desktop-specific native glue; validate early with spikes before committing to Phase 1 scope |
| **Android VPN restrictions** | Android's `VpnService` model, background execution limits, and battery optimization behavior vary by OEM and OS version | Test across multiple OEM skins/versions early; follow Android's official background-service guidance closely |
| **TUN permissions** | TUN interface creation requires elevated privileges on Windows and Linux, and a system-level VPN permission grant on Android | Design clear, platform-appropriate permission request/education flows; fail gracefully with actionable messaging when permissions are denied |
| **sing-box updates** | The app depends on an external, actively developed engine; upstream breaking changes or protocol deprecations could impact stability | Pin known-good `sing-box` versions per release; maintain a regression test suite against core protocols before upgrading |
| **Cross-platform integration** | Keeping Flutter ↔ Go bridge behavior identical across three different mechanisms (platform channels, FFI, IPC) increases integration risk | Define a single internal API contract up front and treat platform bridges as pure transport, not places for platform-specific business logic |
| **Security challenges** | VPN clients are high-value attack targets; credential handling, TLS validation, and privilege escalation paths must be airtight | Independent security review before general availability; conservative defaults (certificate validation on by default, no plaintext credential storage) |

---

## 13. Success Metrics

- **Connection success rate** — percentage of connection attempts that succeed without error, tracked per protocol and per platform
- **Application stability** — crash-free session rate across all three platforms
- **Startup time** — cold-start time to interactive dashboard, per platform
- **Resource usage** — steady-state CPU and memory footprint, idle and connected, per platform
- **User experience metrics** — task success rate for core flows (connect, add server, build a tunnel chain, create a routing rule), and qualitative usability feedback from advanced vs. normal users
