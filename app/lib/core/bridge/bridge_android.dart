import 'bridge_transport.dart';

/// Android MethodChannel bridge: gomobile bind `.aar` exposed over
/// `MethodChannel("com.omniproxy/bridge")`, events over
/// `MethodChannel("com.omniproxy/events")` (see `docs/api-contract.md` §5.1,
/// `docs/platform-notes.md` §Android).
///
/// Pure transport. Not implemented until M7 (Android bridge E2E).
class AndroidBridge implements BridgeTransport {
  @override
  Future<BridgeResponse> request(String method, Map<String, dynamic> requestJson) {
    throw UnsupportedError('AndroidBridge lands in M7');
  }

  @override
  Stream<Map<String, dynamic>> get events =>
      throw UnsupportedError('AndroidBridge lands in M7');

  @override
  Future<void> start() async {}

  @override
  Future<void> stop() async {}
}
