import 'models.dart';

/// Single internal API contract (see `docs/api-contract.md`) implemented
/// identically by every bridge transport and by the mock used in M5.
///
/// Bridges are **pure transport** — no platform business logic lives here.
/// Errors are surfaced as [ApiError] carrying the contract error code
/// (`not_found`, `validation_failed`, `connected`, `busy`, ...).
abstract class ApiClient {
  Future<AppVersion> getVersion();

  Future<List<ServerProfile>> listServers();

  Future<ServerProfile> getServer(String id);

  /// Returns the core-assigned id.
  Future<String> addServer(ServerProfile server);

  Future<ServerProfile> updateServer(ServerProfile server);

  Future<void> deleteServer(String id);

  Future<ImportResult> importServers(ImportSource source);

  /// Returns the `.onnproxy` interchange blob.
  Future<String> exportServers({List<String>? ids});

  /// Returns latency in ms; throws [ApiError] on timeout/failure.
  Future<int> testServerLatency(String id);

  Future<void> connect({required String serverId, ConnectionMode? mode});

  Future<void> disconnect();

  Future<ConnectionSnapshot> getConnectionState();

  Future<List<LogEntry>> getLogs({int? afterSeq, int? limit});

  Future<AppSettings> getSettings();

  Future<AppSettings> updateSettings(AppSettings settings);

  Future<void> subscribe(List<String> events);

  Future<void> unsubscribe();

  /// Async events: `stateChanged` · `logAppended` · `latencyTested`.
  Stream<AppEvent> get events;
}
