import 'api_client.dart';
import 'bridge/bridge_transport.dart';
import 'models.dart';

/// Contract [ApiClient] backed by a [BridgeTransport] (the canonical pure
/// transport abstraction). Request/response shapes follow `docs/api-contract.md`
/// exactly; every method is one transport request, and [events] forwards the
/// transport's async event stream. No platform business logic lives here.
class BridgeApiClient implements ApiClient {
  BridgeApiClient(this._transport);

  final BridgeTransport _transport;

  Future<Map<String, dynamic>> _call(
      String method, Map<String, dynamic> request) async {
    final response = await _transport.request(method, request);
    if (!response.ok) {
      throw ApiError(
        code: response.errorCode ?? 'internal',
        message: response.errorMessage ?? 'Unknown error',
      );
    }
    return response.data ?? const <String, dynamic>{};
  }

  @override
  Future<AppVersion> getVersion() async {
    final data = await _call('getVersion', {});
    return AppVersion.fromJson(data);
  }

  @override
  Future<List<ServerProfile>> listServers({ServerListQuery? query}) async {
    final data = await _call('listServers', query?.toJson() ?? const {});
    final servers = data['servers'] as List<dynamic>? ?? const [];
    return servers
        .map((s) => ServerProfile.fromJson(s as Map<String, dynamic>))
        .toList();
  }

  @override
  Future<ServerProfile> getServer(String id) async {
    final data = await _call('getServer', {'id': id});
    return ServerProfile.fromJson(data);
  }

  @override
  Future<String> addServer(ServerProfile server) async {
    final data = await _call('addServer', {'server': server.toJson()});
    return data['id'] as String? ?? '';
  }

  @override
  Future<ServerProfile> updateServer(ServerProfile server) async {
    final data = await _call('updateServer', {'server': server.toJson()});
    return ServerProfile.fromJson(data);
  }

  @override
  Future<void> deleteServer(String id) async {
    await _call('deleteServer', {'id': id});
  }

  @override
  Future<String> duplicateServer(String id) async {
    final data = await _call('duplicateServer', {'id': id});
    return data['id'] as String? ?? '';
  }

  @override
  Future<ImportResult> importServers(ImportSource source) async {
    final data =
        await _call('importServers', {'source': source.toJson()});
    return ImportResult.fromJson(data);
  }

  @override
  Future<String> exportServers(
      {List<String>? ids, String format = 'onnproxy'}) async {
    final data = await _call('exportServers', {
      'ids': ids ?? const [],
      'format': format,
    });
    return data['blob'] as String? ?? '';
  }

  @override
  Future<int> testServerLatency(String id) async {
    final data = await _call('testServerLatency', {'id': id});
    return data['latencyMs'] as int? ?? 0;
  }

  @override
  Future<void> connect({required String serverId, ConnectionMode? mode}) async {
    await _call('connect', {
      'serverId': serverId,
      'mode': mode?.wire ?? '',
    });
  }

  @override
  Future<void> disconnect() async {
    await _call('disconnect', {});
  }

  @override
  Future<ConnectionSnapshot> getConnectionState() async {
    final data = await _call('getConnectionState', {});
    return ConnectionSnapshot.fromJson(data);
  }

  @override
  Future<List<LogEntry>> getLogs({int? afterSeq, int? limit}) async {
    final data = await _call('getLogs', {
      'afterSeq': afterSeq ?? 0,
      'limit': limit ?? 200,
    });
    final logs = data['logs'] as List<dynamic>? ?? const [];
    return logs
        .map((l) => LogEntry.fromJson(l as Map<String, dynamic>))
        .toList();
  }

  @override
  Future<AppSettings> getSettings() async {
    final data = await _call('getSettings', {});
    return AppSettings.fromJson(data['settings'] as Map<String, dynamic>);
  }

  @override
  Future<AppSettings> updateSettings(AppSettings settings) async {
    final data = await _call('updateSettings', {'settings': settings.toJson()});
    return AppSettings.fromJson(data['settings'] as Map<String, dynamic>);
  }

  @override
  Future<void> subscribe(List<String> events) async {
    await _call('subscribe', {'events': events});
  }

  @override
  Future<void> unsubscribe() async {
    await _call('unsubscribe', {});
  }

  @override
  Stream<AppEvent> get events =>
      _transport.events.map((e) => AppEvent.fromJson(e));

  @override
  String toString() => 'BridgeApiClient($_transport)';
}
