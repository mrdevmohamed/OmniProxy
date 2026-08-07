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
  ServerListQuery _query = const ServerListQuery();

  @override
  Future<List<ServerProfile>> build() {
    return ref.watch(apiClientProvider).listServers(query: _query);
  }

  void setQuery(ServerListQuery query) {
    _query = query;
    ref.invalidateSelf();
  }

  Future<void> _reload() async {
    state = const AsyncLoading();
    state = await AsyncValue.guard(
        () => ref.read(apiClientProvider).listServers(query: _query));
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

  Future<String> duplicate(String id) async {
    final newId = await ref.read(apiClientProvider).duplicateServer(id);
    await _reload();
    return newId;
  }

  Future<ImportResult> import(ImportSource source) async {
    final result = await ref.read(apiClientProvider).importServers(source);
    await _reload();
    return result;
  }

  Future<String> export({List<String>? ids, String format = 'onnproxy'}) {
    return ref.read(apiClientProvider).exportServers(ids: ids, format: format);
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

  Future<void> toggleEnabled(String id) async {
    final server = state.value?.firstWhere((s) => s.id == id);
    if (server == null) return;
    await updateServer(server.copyWith(enabled: !server.enabled));
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

  /// Re-sync from the sequence watermark, recovering any events missed by the
  /// live stream. The watermark is monotonic, so a refresh never resurrects
  /// entries the user has cleared and never duplicates entries already shown.
  Future<void> refresh() async {
    await _seed();
  }

  /// Clear the visible list. Advances the sequence watermark past everything
  /// currently shown, so cleared entries do not reappear on a refresh or via
  /// late `logAppended` deliveries; only genuinely new logs are shown after.
  void clear() {
    if (state.isNotEmpty) {
      _lastSeq = state.last.seq;
    }
    state = const [];
  }
}

/// The server the user has chosen as the connect target, shown on the
/// dashboard and used by quick connect. Defaults to the favorite server
/// (else the first). The selection re-anchors automatically when the catalog
/// changes: a still-present choice is kept, a deleted choice falls back to
/// the default, an empty catalog yields null.
final selectedServerProvider =
    NotifierProvider<SelectedServerNotifier, String?>(SelectedServerNotifier.new);

class SelectedServerNotifier extends Notifier<String?> {
  @override
  String? build() {
    // Re-anchor the selection whenever the catalog changes, without
    // rebuilding the provider (rebuild would discard the user's choice).
    ref.listen(serversProvider, (_, next) {
      final servers = next.value;
      final current = state;
      if (current != null && servers?.any((s) => s.id == current) == true) {
        return;
      }
      state = defaultSelection(servers);
    });
    return defaultSelection(ref.read(serversProvider).value);
  }

  static String? defaultSelection(List<ServerProfile>? servers) {
    if (servers == null || servers.isEmpty) return null;
    return servers.firstWhere((s) => s.favorite, orElse: () => servers.first).id;
  }

  void select(String? id) {
    if (id == null) {
      state = null;
      return;
    }
    final servers = ref.read(serversProvider).value ?? const [];
    if (servers.any((s) => s.id == id)) state = id;
  }
}
