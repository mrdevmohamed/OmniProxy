import 'dart:async';
import 'dart:convert';
import 'dart:math';

import 'api_client.dart';
import 'models.dart';

/// In-memory implementation of the canonical bridge contract
/// (`docs/api-contract.md`), used to wire the M5 Flutter shell before the
/// real FFI/MethodChannel transports land in M6–M8.
///
/// It mirrors the contract faithfully: same method names, response shapes,
/// error codes (`not_found`, `connected`, `busy`, `validation_failed`) and
/// async events (`stateChanged`, `logAppended`, `latencyTested`). Connection
/// transitions are simulated with small delays.
class MockApiClient implements ApiClient {
  MockApiClient({
    List<ServerProfile>? seed,
    this.connectDelay = const Duration(milliseconds: 600),
    this.latencyDelay = const Duration(milliseconds: 250),
    Random? random,
  })  : _servers = [...?seed, ..._defaultSeed()],
        _random = random ?? Random();

  final Duration connectDelay;
  final Duration latencyDelay;
  final Random _random;

  final List<ServerProfile> _servers;
  final _controller = StreamController<AppEvent>.broadcast();
  final List<LogEntry> _logs = [];

  AppSettings _settings = const AppSettings();
  ConnectionState _state = ConnectionState.disconnected;
  VpnSession? _session;
  bool _subscribed = false;
  int _seq = 0;
  int _gen = 0;

  static List<ServerProfile> _defaultSeed() {
    final now = DateTime.now().toUtc();
    return [
      ServerProfile(
        id: '00000000-0000-4000-8000-000000000001',
        name: 'Tokyo Relay',
        protocol: ServerProtocol.vless,
        address: 'jp.example.net',
        port: 443,
        uuid: '11111111-1111-4111-8111-111111111111',
        tls: const TlsSettings(enabled: true, serverName: 'jp.example.net'),
        favorite: true,
        createdAt: now,
        updatedAt: now,
      ),
      ServerProfile(
        id: '00000000-0000-4000-8000-000000000002',
        name: 'Frankfurt Shadowsocks',
        protocol: ServerProtocol.shadowsocks,
        address: 'de.example.net',
        port: 8388,
        password: 'change-me',
        cipher: 'aes-128-gcm',
        createdAt: now,
        updatedAt: now,
      ),
      ServerProfile(
        id: '00000000-0000-4000-8000-000000000003',
        name: 'Local HTTP',
        protocol: ServerProtocol.http,
        address: '127.0.0.1',
        port: 8080,
        username: 'proxy',
        password: 'secret',
        createdAt: now,
        updatedAt: now,
      ),
    ];
  }

  DateTime _now() => DateTime.now().toUtc();

  String _uuid() {
    final bytes = List<int>.generate(16, (_) => _random.nextInt(256));
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    final hex = bytes.map((b) => b.toRadixString(16).padLeft(2, '0')).join();
    return '${hex.substring(0, 8)}-${hex.substring(8, 12)}-${hex.substring(12, 16)}-'
        '${hex.substring(16, 20)}-${hex.substring(20)}';
  }

  void _addLog(
    String component,
    String message, {
    LogLevel level = LogLevel.info,
    Map<String, dynamic>? context,
  }) {
    _logs.add(LogEntry(
      seq: ++_seq,
      timestamp: _now(),
      level: level,
      component: component,
      message: message,
      context: context,
    ));
    if (_logs.length > 200) _logs.removeRange(0, _logs.length - 200);
    _emit(AppEvent(
      type: 'logAppended',
      data: _logs.last.toJson(),
    ));
  }

  void _emit(AppEvent event) {
    if (_subscribed) _controller.add(event);
  }

  ServerProfile _find(String id) {
    for (final server in _servers) {
      if (server.id == id) return server;
    }
    throw ApiError(code: 'not_found', message: 'Server "$id" not found');
  }

  void _validate(ServerProfile server, {required bool forCreate}) {
    if (server.name.trim().isEmpty) {
      throw const ApiError(code: 'validation_failed', message: 'Name is required');
    }
    if (server.address.trim().isEmpty) {
      throw const ApiError(
          code: 'validation_failed', message: 'Address is required');
    }
    if (server.port < 1 || server.port > 65535) {
      throw const ApiError(
          code: 'validation_failed', message: 'Port must be 1–65535');
    }
    if (server.protocol == ServerProtocol.ssh && (server.ssh.user ?? '').isEmpty) {
      throw const ApiError(
          code: 'validation_failed', message: 'SSH user is required');
    }
    if (forCreate && server.id.isEmpty) {
      throw const ApiError(code: 'invalid_argument', message: 'Missing server id');
    }
  }

  @override
  Future<AppVersion> getVersion() async => const AppVersion(
        version: '0.1.0',
        engineVersion: 'v1.13.15',
        platform: 'linux',
      );

  @override
  Future<List<ServerProfile>> listServers() async => List.of(_servers);

  @override
  Future<ServerProfile> getServer(String id) async => _find(id);

  @override
  Future<String> addServer(ServerProfile server) async {
    _validate(server, forCreate: false);
    final now = _now();
    final created = server.copyWith(
      id: _uuid(),
      createdAt: now,
      updatedAt: now,
      lastLatencyMs: 0,
      lastTestedAt: null,
    );
    _servers.add(created);
    _addLog('server', 'Added server "${created.name}"');
    return created.id;
  }

  @override
  Future<ServerProfile> updateServer(ServerProfile server) async {
    _validate(server, forCreate: true);
    final index = _servers.indexWhere((s) => s.id == server.id);
    if (index < 0) {
      throw ApiError(code: 'not_found', message: 'Server "${server.id}" not found');
    }
    if (_state == ConnectionState.connected && _session?.serverId == server.id) {
      throw const ApiError(
          code: 'connected',
          message: 'Disconnect before editing the active server');
    }
    final updated = server.copyWith(
      createdAt: _servers[index].createdAt,
      updatedAt: _now(),
    );
    _servers[index] = updated;
    _addLog('server', 'Updated server "${updated.name}"');
    return updated;
  }

  @override
  Future<void> deleteServer(String id) async {
    if (_state == ConnectionState.connected && _session?.serverId == id) {
      throw const ApiError(
          code: 'connected',
          message: 'Disconnect before deleting the active server');
    }
    final index = _servers.indexWhere((s) => s.id == id);
    if (index < 0) {
      throw ApiError(code: 'not_found', message: 'Server "$id" not found');
    }
    _servers.removeAt(index);
    _addLog('server', 'Deleted server "$id"');
  }

  @override
  Future<ImportResult> importServers(ImportSource source) async {
    final added = <ServerProfile>[];
    final errors = <ImportError>[];
    Object? parsed;
    try {
      parsed = jsonDecode(source.data);
    } on FormatException catch (e) {
      errors.add(ImportError(index: 0, message: 'Invalid configuration: ${e.message}'));
    }

    if (parsed is Map<String, dynamic>) {
      final server = parsed['server'];
      if (server is Map<String, dynamic>) {
        _tryAddEnvelope(server, added, errors, 0);
      } else {
        _tryAddEnvelope(parsed, added, errors, 0);
      }
    } else if (parsed is List) {
      for (var i = 0; i < parsed.length; i++) {
        final item = parsed[i];
        if (item is Map<String, dynamic>) {
          _tryAddEnvelope(item['server'] is Map<String, dynamic>
              ? item['server'] as Map<String, dynamic>
              : item, added, errors, i);
        }
      }
    } else if (errors.isEmpty) {
      errors.add(const ImportError(index: 0, message: 'Expected a JSON server envelope'));
    }

    final now = _now();
    for (final server in added) {
      _servers.add(server.copyWith(
        id: _uuid(),
        createdAt: now,
        updatedAt: now,
        lastLatencyMs: 0,
        lastTestedAt: null,
      ));
    }
    if (added.isNotEmpty) _addLog('server', 'Imported ${added.length} server(s)');
    return ImportResult(added: added.length, failed: errors.length, errors: errors);
  }

  void _tryAddEnvelope(
    Map<String, dynamic> json,
    List<ServerProfile> added,
    List<ImportError> errors,
    int index,
  ) {
    try {
      final profile = ServerProfile.fromJson(json);
      _validate(profile, forCreate: false);
      added.add(profile);
    } catch (e) {
      errors.add(ImportError(index: index, message: e.toString()));
    }
  }

  @override
  Future<String> exportServers({List<String>? ids}) async {
    final selected = ids == null || ids.isEmpty
        ? _servers
        : _servers.where((s) => ids.contains(s.id)).toList();
    final envelopes = selected
        .map((s) => {'format': 'onnproxy', 'version': 1, 'server': s.toJson()})
        .toList();
    return jsonEncode(envelopes.length == 1 ? envelopes.first : envelopes);
  }

  @override
  Future<int> testServerLatency(String id) async {
    final server = _find(id);
    await Future.delayed(latencyDelay);
    final ms = 40 + _random.nextInt(160);
    final now = _now();
    final index = _servers.indexWhere((s) => s.id == id);
    _servers[index] = server.copyWith(lastLatencyMs: ms, lastTestedAt: now);
    _emit(AppEvent(
      type: 'latencyTested',
      data: {'id': id, 'latencyMs': ms},
    ));
    _addLog('server', 'Latency for "${server.name}": ${ms}ms');
    return ms;
  }

  @override
  Future<void> connect({required String serverId, ConnectionMode? mode}) async {
    final server = _find(serverId);
    if (_state == ConnectionState.connecting ||
        _state == ConnectionState.connected ||
        _state == ConnectionState.reconnecting) {
      throw const ApiError(
          code: 'busy', message: 'A connection is already in progress');
    }
    final gen = ++_gen;
    final now = _now();
    _state = ConnectionState.connecting;
    _session = VpnSession(
      id: _uuid(),
      serverId: serverId,
      mode: mode ?? _settings.connectionMode,
      startedAt: now,
      statusHistory: [SessionStatus(state: ConnectionState.connecting, at: now)],
    );
    _addLog('vpn', 'Connecting to ${server.name}');
    _emit(AppEvent(
      type: 'stateChanged',
      data: {
        'state': ConnectionState.connecting.wire,
        'session': _session!.toJson(),
      },
    ));

    await Future.delayed(connectDelay);
    if (gen != _gen) return;

    final connectedAt = _now();
    final session = _session!;
    _state = ConnectionState.connected;
    _session = VpnSession(
      id: session.id,
      serverId: session.serverId,
      mode: session.mode,
      startedAt: connectedAt,
      statusHistory: [
        ...session.statusHistory,
        SessionStatus(state: ConnectionState.connected, at: connectedAt),
      ],
    );
    _addLog('vpn', 'Connected to ${server.name}');
    _emit(AppEvent(
      type: 'stateChanged',
      data: {
        'state': ConnectionState.connected.wire,
        'session': _session!.toJson(),
      },
    ));
  }

  @override
  Future<void> disconnect() async {
    ++_gen;
    final session = _session;
    if (session == null || _state == ConnectionState.disconnected) return;
    final now = _now();
    _state = ConnectionState.disconnected;
    _session = VpnSession(
      id: session.id,
      serverId: session.serverId,
      mode: session.mode,
      startedAt: session.startedAt,
      endedAt: now,
      statusHistory: [
        ...session.statusHistory,
        SessionStatus(state: ConnectionState.disconnected, at: now),
      ],
    );
    _addLog('vpn', 'Disconnected');
    _emit(AppEvent(
      type: 'stateChanged',
      data: {
        'state': ConnectionState.disconnected.wire,
        'session': _session!.toJson(),
      },
    ));
  }

  @override
  Future<ConnectionSnapshot> getConnectionState() async =>
      ConnectionSnapshot(state: _state, session: _session);

  @override
  Future<List<LogEntry>> getLogs({int? afterSeq, int? limit}) async {
    var logs = _logs.where((l) => l.seq > (afterSeq ?? 0)).toList();
    if (limit != null && logs.length > limit) {
      logs = logs.sublist(logs.length - limit);
    }
    return logs;
  }

  @override
  Future<AppSettings> getSettings() async => _settings;

  @override
  Future<AppSettings> updateSettings(AppSettings settings) async {
    _settings = settings;
    _addLog('config', 'Settings updated');
    return _settings;
  }

  @override
  Future<void> subscribe(List<String> events) async {
    _subscribed = true;
    _addLog('config', 'Subscribed to events: ${events.join(', ')}');
  }

  @override
  Future<void> unsubscribe() async {
    _subscribed = false;
  }

  @override
  Stream<AppEvent> get events => _controller.stream;

  void dispose() {
    _controller.close();
  }
}
