/// Dart models mirroring `docs/api-contract.md` (section 2).
///
/// Field names and wire strings must match the canonical JSON contract
/// exactly; the bridge transports and the mock both serialize through these.
library;

import 'dart:convert';

/// Connection states — `docs/api-contract.md` §2.
enum ConnectionState {
  disconnected('disconnected'),
  connecting('connecting'),
  connected('connected'),
  reconnecting('reconnecting'),
  error('error');

  const ConnectionState(this.wire);

  final String wire;

  static ConnectionState fromWire(String? value) {
    for (final state in ConnectionState.values) {
      if (state.wire == value) return state;
    }
    return ConnectionState.disconnected;
  }
}

/// Connection modes — `docs/api-contract.md` §2.
enum ConnectionMode {
  vpn('vpn'),
  proxy('proxy');

  const ConnectionMode(this.wire);

  final String wire;

  static ConnectionMode fromWire(String? value) {
    for (final mode in ConnectionMode.values) {
      if (mode.wire == value) return mode;
    }
    return ConnectionMode.vpn;
  }
}

/// Theme preference — `AppSettings.theme`.
enum ThemePreference {
  system('system'),
  light('light'),
  dark('dark');

  const ThemePreference(this.wire);

  final String wire;

  static ThemePreference fromWire(String? value) {
    for (final theme in ThemePreference.values) {
      if (theme.wire == value) return theme;
    }
    return ThemePreference.system;
  }
}

/// Log levels — `LogEntry.level` / `AppSettings.logLevel`.
enum LogLevel {
  trace('trace'),
  debug('debug'),
  info('info'),
  warn('warn'),
  error('error');

  const LogLevel(this.wire);

  final String wire;

  static LogLevel fromWire(String? value) {
    for (final level in LogLevel.values) {
      if (level.wire == value) return level;
    }
    return LogLevel.info;
  }
}

/// Server protocols — `docs/api-contract.md` §2.1.
enum ServerProtocol {
  vless('vless'),
  vmess('vmess'),
  shadowsocks('shadowsocks'),
  trojan('trojan'),
  socks5('socks5'),
  http('http'),
  ssh('ssh');

  const ServerProtocol(this.wire);

  final String wire;

  static ServerProtocol fromWire(String? value) {
    for (final protocol in ServerProtocol.values) {
      if (protocol.wire == value) return protocol;
    }
    return ServerProtocol.vless;
  }
}

/// Stream transports for vless/vmess/trojan outbounds — §2.1.
enum TransportType {
  tcp(''),
  ws('ws');

  const TransportType(this.wire);

  final String wire;

  static TransportType fromWire(String? value) {
    for (final type in TransportType.values) {
      if (type.wire == value) return type;
    }
    return TransportType.tcp;
  }
}

/// `ServerProfile.transport` — §2.1. Only WebSocket is wired in Phase 1.
class TransportSettings {
  const TransportSettings({
    this.type = TransportType.tcp,
    this.path,
    this.host,
    this.maxEarlyData = 0,
    this.earlyDataHeaderName,
  });

  final TransportType type;
  final String? path;
  final String? host;
  final int maxEarlyData;
  final String? earlyDataHeaderName;

  factory TransportSettings.fromJson(Map<String, dynamic> json) =>
      TransportSettings(
        type: TransportType.fromWire(json['type'] as String?),
        path: json['path'] as String?,
        host: json['host'] as String?,
        maxEarlyData: json['maxEarlyData'] as int? ?? 0,
        earlyDataHeaderName: json['earlyDataHeaderName'] as String?,
      );

  Map<String, dynamic> toJson() => {
        if (type != TransportType.tcp) 'type': type.wire,
        if (path != null) 'path': path,
        if (host != null) 'host': host,
        if (maxEarlyData != 0) 'maxEarlyData': maxEarlyData,
        if (earlyDataHeaderName != null)
          'earlyDataHeaderName': earlyDataHeaderName,
      };
}

/// `ServerProfile.tls` — `docs/api-contract.md` §2.1.
class TlsSettings {
  const TlsSettings({
    this.enabled = false,
    this.allowInsecure = false,
    this.serverName,
    this.alpn,
    this.fingerprint,
  });

  final bool enabled;
  final bool allowInsecure;
  final String? serverName;
  final List<String>? alpn;
  final String? fingerprint;

  factory TlsSettings.fromJson(Map<String, dynamic> json) => TlsSettings(
        enabled: json['enabled'] == true,
        allowInsecure: json['allowInsecure'] == true,
        serverName: json['serverName'] as String?,
        alpn: json['alpn'] == null
            ? null
            : (json['alpn'] as List<dynamic>).cast<String>(),
        fingerprint: json['fingerprint'] as String?,
      );

  Map<String, dynamic> toJson() => {
        'enabled': enabled,
        'allowInsecure': allowInsecure,
        'serverName': serverName,
        if (alpn != null) 'alpn': alpn,
        if (fingerprint != null) 'fingerprint': fingerprint,
      };
}

/// `ServerProfile.ssh` — `docs/api-contract.md` §2.1.
class SshSettings {
  const SshSettings({this.user, this.hostKey, this.privateKey});

  final String? user;
  final String? hostKey;
  final String? privateKey;

  factory SshSettings.fromJson(Map<String, dynamic> json) => SshSettings(
        user: json['user'] as String?,
        hostKey: json['hostKey'] as String?,
        privateKey: json['privateKey'] as String?,
      );

  Map<String, dynamic> toJson() => {
        'user': user,
        'hostKey': hostKey,
        'privateKey': privateKey,
      };
}

/// `ServerProfile` — `docs/api-contract.md` §2.1.
class ServerProfile {
  const ServerProfile({
    required this.id,
    required this.name,
    required this.protocol,
    required this.address,
    required this.port,
    this.username,
    this.password,
    this.cipher,
    this.uuid,
    this.flow,
    this.security,
    this.tls = const TlsSettings(),
    this.ssh = const SshSettings(),
    this.transport,
    this.globalPadding = false,
    this.packetEncoding,
    this.favorite = false,
    this.lastLatencyMs = 0,
    this.lastTestedAt,
    required this.createdAt,
    required this.updatedAt,
  });

  final String id;
  final String name;
  final ServerProtocol protocol;
  final String address;
  final int port;
  final String? username;
  final String? password;
  final String? cipher;
  final String? uuid;
  final String? flow;
  final String? security;
  final TlsSettings tls;
  final SshSettings ssh;
  final TransportSettings? transport;
  final bool globalPadding;
  final String? packetEncoding;
  final bool favorite;
  final int lastLatencyMs;
  final DateTime? lastTestedAt;
  final DateTime createdAt;
  final DateTime updatedAt;

  ServerProfile copyWith({
    String? id,
    String? name,
    ServerProtocol? protocol,
    String? address,
    int? port,
    String? username,
    String? password,
    String? cipher,
    String? uuid,
    String? flow,
    String? security,
    TlsSettings? tls,
    SshSettings? ssh,
    TransportSettings? transport,
    bool? globalPadding,
    String? packetEncoding,
    bool? favorite,
    int? lastLatencyMs,
    DateTime? lastTestedAt,
    DateTime? createdAt,
    DateTime? updatedAt,
  }) =>
      ServerProfile(
        id: id ?? this.id,
        name: name ?? this.name,
        protocol: protocol ?? this.protocol,
        address: address ?? this.address,
        port: port ?? this.port,
        username: username ?? this.username,
        password: password ?? this.password,
        cipher: cipher ?? this.cipher,
        uuid: uuid ?? this.uuid,
        flow: flow ?? this.flow,
        security: security ?? this.security,
        tls: tls ?? this.tls,
        ssh: ssh ?? this.ssh,
        transport: transport ?? this.transport,
        globalPadding: globalPadding ?? this.globalPadding,
        packetEncoding: packetEncoding ?? this.packetEncoding,
        favorite: favorite ?? this.favorite,
        lastLatencyMs: lastLatencyMs ?? this.lastLatencyMs,
        lastTestedAt: lastTestedAt ?? this.lastTestedAt,
        createdAt: createdAt ?? this.createdAt,
        updatedAt: updatedAt ?? this.updatedAt,
      );

  factory ServerProfile.fromJson(Map<String, dynamic> json) => ServerProfile(
        id: json['id'] as String? ?? '',
        name: json['name'] as String? ?? '',
        protocol: ServerProtocol.fromWire(json['protocol'] as String?),
        address: json['address'] as String? ?? '',
        port: json['port'] as int? ?? 0,
        username: json['username'] as String?,
        password: json['password'] as String?,
        cipher: json['cipher'] as String?,
        uuid: json['uuid'] as String?,
        flow: json['flow'] as String?,
        security: json['security'] as String?,
        tls: json['tls'] == null
            ? const TlsSettings()
            : TlsSettings.fromJson(json['tls'] as Map<String, dynamic>),
        ssh: json['ssh'] == null
            ? const SshSettings()
            : SshSettings.fromJson(json['ssh'] as Map<String, dynamic>),
        transport: json['transport'] == null
            ? null
            : TransportSettings.fromJson(
                json['transport'] as Map<String, dynamic>),
        globalPadding: json['globalPadding'] == true,
        packetEncoding: json['packetEncoding'] as String?,
        favorite: json['favorite'] == true,
        lastLatencyMs: json['lastLatencyMs'] as int? ?? 0,
        lastTestedAt: json['lastTestedAt'] == null
            ? null
            : DateTime.parse(json['lastTestedAt'] as String),
        createdAt: DateTime.parse(json['createdAt'] as String),
        updatedAt: DateTime.parse(json['updatedAt'] as String),
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'name': name,
        'protocol': protocol.wire,
        'address': address,
        'port': port,
        'username': username,
        'password': password,
        'cipher': cipher,
        'uuid': uuid,
        'flow': flow,
        'security': security,
        'tls': tls.toJson(),
        'ssh': ssh.toJson(),
        if (transport != null) 'transport': transport!.toJson(),
        if (globalPadding) 'globalPadding': globalPadding,
        if (packetEncoding != null) 'packetEncoding': packetEncoding,
        'favorite': favorite,
        'lastLatencyMs': lastLatencyMs,
        'lastTestedAt': lastTestedAt?.toUtc().toIso8601String(),
        'createdAt': createdAt.toUtc().toIso8601String(),
        'updatedAt': updatedAt.toUtc().toIso8601String(),
      };
}

/// `VPNSession` — `docs/api-contract.md` §2.2.
class VpnSession {
  const VpnSession({
    required this.id,
    required this.serverId,
    required this.mode,
    required this.startedAt,
    this.endedAt,
    this.bytesUp = 0,
    this.bytesDown = 0,
    this.statusHistory = const [],
    this.error,
  });

  final String id;
  final String serverId;
  final ConnectionMode mode;
  final DateTime startedAt;
  final DateTime? endedAt;
  final int bytesUp;
  final int bytesDown;
  final List<SessionStatus> statusHistory;
  final ApiError? error;

  factory VpnSession.fromJson(Map<String, dynamic> json) => VpnSession(
        id: json['id'] as String? ?? '',
        serverId: json['serverId'] as String? ?? '',
        mode: ConnectionMode.fromWire(json['mode'] as String?),
        startedAt: DateTime.parse(json['startedAt'] as String),
        endedAt: json['endedAt'] == null
            ? null
            : DateTime.parse(json['endedAt'] as String),
        bytesUp: json['bytesUp'] as int? ?? 0,
        bytesDown: json['bytesDown'] as int? ?? 0,
        statusHistory: (json['statusHistory'] as List<dynamic>? ?? const [])
            .map((e) => SessionStatus.fromJson(e as Map<String, dynamic>))
            .toList(),
        error: json['error'] == null
            ? null
            : ApiError.fromJson(json['error'] as Map<String, dynamic>),
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'serverId': serverId,
        'mode': mode.wire,
        'startedAt': startedAt.toUtc().toIso8601String(),
        'endedAt': endedAt?.toUtc().toIso8601String(),
        'bytesUp': bytesUp,
        'bytesDown': bytesDown,
        'statusHistory':
            statusHistory.map((e) => e.toJson()).toList(growable: false),
        'error': error?.toJson(),
      };
}

/// One entry in `VPNSession.statusHistory`.
class SessionStatus {
  const SessionStatus({required this.state, required this.at});

  final ConnectionState state;
  final DateTime at;

  factory SessionStatus.fromJson(Map<String, dynamic> json) => SessionStatus(
        state: ConnectionState.fromWire(json['state'] as String?),
        at: DateTime.parse(json['at'] as String),
      );

  Map<String, dynamic> toJson() => {'state': state.wire, 'at': at.toUtc().toIso8601String()};
}

/// `AppSettings` — `docs/api-contract.md` §2.3.
class AppSettings {
  const AppSettings({
    this.theme = ThemePreference.system,
    this.connectionMode = ConnectionMode.vpn,
    this.autoConnect = false,
    this.startWithSystem = false,
    this.advancedModeEnabled = false,
    this.notificationsEnabled = true,
    this.logLevel = LogLevel.info,
  });

  final ThemePreference theme;
  final ConnectionMode connectionMode;
  final bool autoConnect;
  final bool startWithSystem;
  final bool advancedModeEnabled;
  final bool notificationsEnabled;
  final LogLevel logLevel;

  AppSettings copyWith({
    ThemePreference? theme,
    ConnectionMode? connectionMode,
    bool? autoConnect,
    bool? startWithSystem,
    bool? advancedModeEnabled,
    bool? notificationsEnabled,
    LogLevel? logLevel,
  }) =>
      AppSettings(
        theme: theme ?? this.theme,
        connectionMode: connectionMode ?? this.connectionMode,
        autoConnect: autoConnect ?? this.autoConnect,
        startWithSystem: startWithSystem ?? this.startWithSystem,
        advancedModeEnabled: advancedModeEnabled ?? this.advancedModeEnabled,
        notificationsEnabled: notificationsEnabled ?? this.notificationsEnabled,
        logLevel: logLevel ?? this.logLevel,
      );

  factory AppSettings.fromJson(Map<String, dynamic> json) => AppSettings(
        theme: ThemePreference.fromWire(json['theme'] as String?),
        connectionMode: ConnectionMode.fromWire(json['connectionMode'] as String?),
        autoConnect: json['autoConnect'] == true,
        startWithSystem: json['startWithSystem'] == true,
        advancedModeEnabled: json['advancedModeEnabled'] == true,
        notificationsEnabled: json['notificationsEnabled'] != false,
        logLevel: LogLevel.fromWire(json['logLevel'] as String?),
      );

  Map<String, dynamic> toJson() => {
        'theme': theme.wire,
        'connectionMode': connectionMode.wire,
        'autoConnect': autoConnect,
        'startWithSystem': startWithSystem,
        'advancedModeEnabled': advancedModeEnabled,
        'notificationsEnabled': notificationsEnabled,
        'logLevel': logLevel.wire,
      };
}

/// `LogEntry` — `docs/api-contract.md` §2.4.
class LogEntry {
  const LogEntry({
    required this.seq,
    required this.timestamp,
    required this.level,
    required this.component,
    required this.message,
    this.context,
  });

  final int seq;
  final DateTime timestamp;
  final LogLevel level;
  final String component;
  final String message;
  final Map<String, dynamic>? context;

  factory LogEntry.fromJson(Map<String, dynamic> json) => LogEntry(
        seq: json['seq'] as int? ?? 0,
        timestamp: DateTime.parse(json['timestamp'] as String),
        level: LogLevel.fromWire(json['level'] as String?),
        component: json['component'] as String? ?? '',
        message: json['message'] as String? ?? '',
        context: json['context'] as Map<String, dynamic>?,
      );

  Map<String, dynamic> toJson() => {
        'seq': seq,
        'timestamp': timestamp.toUtc().toIso8601String(),
        'level': level.wire,
        'component': component,
        'message': message,
        if (context != null) 'context': context,
      };
}

/// Error envelope — `docs/api-contract.md` §1.
class ApiError implements Exception {
  const ApiError({required this.code, required this.message});

  final String code;
  final String message;

  factory ApiError.fromJson(Map<String, dynamic> json) => ApiError(
        code: json['code'] as String? ?? 'internal',
        message: json['message'] as String? ?? 'Unknown error',
      );

  Map<String, dynamic> toJson() => {'code': code, 'message': message};

  @override
  String toString() => 'ApiError($code): $message';
}

/// `getVersion` response data.
class AppVersion {
  const AppVersion({
    required this.version,
    required this.engineVersion,
    required this.platform,
  });

  final String version;
  final String engineVersion;
  final String platform;

  factory AppVersion.fromJson(Map<String, dynamic> json) => AppVersion(
        version: json['version'] as String? ?? '',
        engineVersion: json['engineVersion'] as String? ?? '',
        platform: json['platform'] as String? ?? '',
      );
}

/// `getConnectionState` response data.
class ConnectionSnapshot {
  const ConnectionSnapshot({required this.state, this.session});

  final ConnectionState state;
  final VpnSession? session;

  factory ConnectionSnapshot.fromJson(Map<String, dynamic> json) =>
      ConnectionSnapshot(
        state: ConnectionState.fromWire(json['state'] as String?),
        session: json['session'] == null
            ? null
            : VpnSession.fromJson(json['session'] as Map<String, dynamic>),
      );
}

/// `importServers` request source.
class ImportSource {
  const ImportSource({required this.kind, required this.data});

  final String kind; // file | link | clipboard
  final String data;

  Map<String, dynamic> toJson() => {'kind': kind, 'data': data};
}

/// `importServers` response data.
class ImportResult {
  const ImportResult({required this.added, required this.failed, this.errors = const []});

  final int added;
  final int failed;
  final List<ImportError> errors;

  factory ImportResult.fromJson(Map<String, dynamic> json) => ImportResult(
        added: json['added'] as int? ?? 0,
        failed: json['failed'] as int? ?? 0,
        errors: (json['errors'] as List<dynamic>? ?? const [])
            .map((e) => ImportError.fromJson(e as Map<String, dynamic>))
            .toList(),
      );
}

class ImportError {
  const ImportError({required this.index, required this.message});

  final int index;
  final String message;

  factory ImportError.fromJson(Map<String, dynamic> json) => ImportError(
        index: json['index'] as int? ?? 0,
        message: json['message'] as String? ?? '',
      );
}

/// Async event delivered to subscribers — `docs/api-contract.md` §4.
class AppEvent {
  const AppEvent({required this.type, required this.data});

  final String type; // stateChanged | logAppended | latencyTested
  final Map<String, dynamic> data;

  factory AppEvent.fromJson(Map<String, dynamic> json) => AppEvent(
        type: json['type'] as String? ?? '',
        data: json['data'] == null
            ? const <String, dynamic>{}
            : Map<String, dynamic>.from(json['data'] as Map),
      );

  String encode() => jsonEncode({'type': type, 'data': data});
}
