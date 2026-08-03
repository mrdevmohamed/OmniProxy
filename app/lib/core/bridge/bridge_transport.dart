/// Pure-transport abstraction for the Flutter ↔ Go bridges.
///
/// Every bridge transport implements the single canonical contract
/// (`docs/api-contract.md`): [request] encodes one method call, [events]
/// streams async events back. No transport contains platform business logic.
///
/// Concrete transports land in later milestones:
///   - `bridge_linux.dart`   — `dart:ffi` into `libomniproxy.so` (M6)
///   - `bridge_android.dart` — MethodChannel over gomobile bind (M7)
///   - `bridge_windows.dart` — `dart:ffi` into `omniproxy.dll` (M8)
library;

/// Result envelope: `{ "ok": true, "data": ... }` or
/// `{ "ok": false, "error": {code, message} }`.
class BridgeResponse {
  const BridgeResponse({required this.ok, this.data, this.errorCode, this.errorMessage});

  final bool ok;
  final Map<String, dynamic>? data;
  final String? errorCode;
  final String? errorMessage;
}

abstract class BridgeTransport {
  /// Sends one method request (JSON request object) and returns the decoded
  /// response. Throws [UnsupportedError] on unmapped methods.
  Future<BridgeResponse> request(String method, Map<String, dynamic> requestJson);

  /// Async events (one JSON object per event, per contract §4).
  Stream<Map<String, dynamic>> get events;

  /// Starts the transport and the underlying core (call once at app startup).
  Future<void> start();

  /// Stops the transport and shuts the core down.
  Future<void> stop();
}
