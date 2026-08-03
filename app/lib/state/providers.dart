import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../core/api_client.dart';
import '../core/client_factory.dart';
import '../core/models.dart';

/// Backs the whole app. M5 wired the mocked contract; M6+ uses the real
/// transport-backed client (selected per platform in `client_factory.dart`).
/// Widget tests override this with a mock.
final apiClientProvider = Provider<ApiClient>((ref) {
  return buildApiClient();
});

final versionProvider = FutureProvider<AppVersion>((ref) {
  return ref.watch(apiClientProvider).getVersion();
});

/// App settings (theme, connection mode). Loads persisted settings once at
/// startup; writes go through the contract's `updateSettings`.
final settingsProvider =
    NotifierProvider<SettingsNotifier, AppSettings>(SettingsNotifier.new);

class SettingsNotifier extends Notifier<AppSettings> {
  @override
  AppSettings build() {
    unawaited(_load());
    return const AppSettings();
  }

  Future<void> _load() async {
    final settings = await ref.read(apiClientProvider).getSettings();
    if (ref.mounted) state = settings;
  }

  Future<void> update(AppSettings settings) async {
    state = await ref.read(apiClientProvider).updateSettings(settings);
  }
}

/// Server catalog. All mutations re-fetch through the contract so the list
/// always reflects core state.
final serversProvider =
    AsyncNotifierProvider<ServersNotifier, List<ServerProfile>>(
        ServersNotifier.new);

class ServersNotifier extends AsyncNotifier<List<ServerProfile>> {
  @override
  Future<List<ServerProfile>> build() {
    return ref.watch(apiClientProvider).listServers();
  }

  Future<void> _reload() async {
    state = const AsyncLoading();
    state =
        await AsyncValue.guard(() => ref.read(apiClientProvider).listServers());
  }

  Future<void> refresh() => _reload();

  Future<void> add(ServerProfile server) async {
    await ref.read(apiClientProvider).addServer(server);
    await _reload();
  }

  Future<void> updateServer(ServerProfile server) async {
    await ref.read(apiClientProvider).updateServer(server);
    await _reload();
  }

  Future<void> delete(String id) async {
    await ref.read(apiClientProvider).deleteServer(id);
    await _reload();
  }

  Future<ImportResult> import(ImportSource source) async {
    final result = await ref.read(apiClientProvider).importServers(source);
    await _reload();
    return result;
  }

  Future<String> export({List<String>? ids}) {
    return ref.read(apiClientProvider).exportServers(ids: ids);
  }

  Future<int> testLatency(String id) async {
    final ms = await ref.read(apiClientProvider).testServerLatency(id);
    await _reload();
    return ms;
  }

  Future<void> toggleFavorite(String id) async {
    final server = state.value?.firstWhere((s) => s.id == id);
    if (server == null) return;
    await updateServer(server.copyWith(favorite: !server.favorite));
  }
}

/// Live connection state driven by the contract's `stateChanged` events.
class ConnectionUiState {
  const ConnectionUiState({required this.state, this.session});

  final ConnectionState state;
  final VpnSession? session;
}

final connectionProvider =
    NotifierProvider<ConnectionNotifier, ConnectionUiState>(
        ConnectionNotifier.new);

class ConnectionNotifier extends Notifier<ConnectionUiState> {
  StreamSubscription<AppEvent>? _subscription;

  @override
  ConnectionUiState build() {
    final client = ref.watch(apiClientProvider);
    unawaited(client.subscribe(const ['stateChanged', 'logAppended']));
    _subscription?.cancel();
    _subscription = client.events.listen(_onEvent);
    ref.onDispose(() => _subscription?.cancel());
    unawaited(client.getConnectionState().then((snapshot) {
      if (ref.mounted && snapshot.session != null) {
        state = ConnectionUiState(
          state: snapshot.state,
          session: snapshot.session,
        );
      }
    }));
    return const ConnectionUiState(state: ConnectionState.disconnected);
  }

  void _onEvent(AppEvent event) {
    if (event.type != 'stateChanged') return;
    final snapshot = ConnectionSnapshot.fromJson(event.data);
    state = ConnectionUiState(
      state: snapshot.state,
      session: snapshot.session,
    );
  }

  Future<void> connect(String serverId, {ConnectionMode? mode}) async {
    await ref
        .read(apiClientProvider)
        .connect(serverId: serverId, mode: mode ?? ref.read(settingsProvider).connectionMode);
  }

  Future<void> disconnect() async {
    await ref.read(apiClientProvider).disconnect();
  }
}

/// Live log viewer buffer driven by the contract's `logAppended` events,
/// seeded from `getLogs`. Entries are deduplicated by sequence number.
final logsProvider =
    NotifierProvider<LogsNotifier, List<LogEntry>>(LogsNotifier.new);

class LogsNotifier extends Notifier<List<LogEntry>> {
  static const _cap = 500;
  StreamSubscription<AppEvent>? _subscription;
  int _lastSeq = 0;

  @override
  List<LogEntry> build() {
    final client = ref.watch(apiClientProvider);
    _subscription?.cancel();
    _subscription = client.events.listen(_onEvent);
    ref.onDispose(() => _subscription?.cancel());
    unawaited(_seed());
    return const [];
  }

  Future<void> _seed() async {
    try {
      final logs =
          await ref.read(apiClientProvider).getLogs(afterSeq: _lastSeq);
      if (!ref.mounted) return;
      final fresh = logs.where((l) => l.seq > _lastSeq).toList();
      if (fresh.isEmpty) return;
      _lastSeq = fresh.last.seq;
      state = [...state, ...fresh];
      _trim();
    } on ApiError {
      // The log viewer degrades gracefully if getLogs fails.
    }
  }

  void _onEvent(AppEvent event) {
    if (event.type != 'logAppended') return;
    final entry = LogEntry.fromJson(event.data);
    if (entry.seq <= _lastSeq) return;
    _lastSeq = entry.seq;
    state = [...state, entry];
    _trim();
  }

  void _trim() {
    if (state.length > _cap) {
      state = state.sublist(state.length - _cap);
    }
  }

  /// Re-fetch recent entries from core (e.g. after clearing).
  Future<void> refresh() async {
    state = const [];
    _lastSeq = 0;
    await _seed();
  }

  void clear() => state = const [];
}
