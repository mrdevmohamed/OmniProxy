import 'bridge_transport.dart';

/// Linux FFI bridge: `dart:ffi` into the c-shared `libomniproxy.so`
/// (see `docs/api-contract.md` §5.2, `docs/platform-notes.md` §Linux).
///
/// Pure transport — frame the request, ship it to the native core, forward
/// events. Not implemented until M6 (Linux bridge E2E).
class LinuxBridge implements BridgeTransport {
  @override
  Future<BridgeResponse> request(String method, Map<String, dynamic> requestJson) {
    throw UnsupportedError('LinuxBridge lands in M6');
  }

  @override
  Stream<Map<String, dynamic>> get events =>
      throw UnsupportedError('LinuxBridge lands in M6');

  @override
  Future<void> start() async {}

  @override
  Future<void> stop() async {}
}
