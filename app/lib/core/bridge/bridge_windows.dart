import 'bridge_transport.dart';

/// Windows FFI bridge: `dart:ffi` into `omniproxy.dll`
/// (see `docs/api-contract.md` §5.2, `docs/platform-notes.md` §Windows).
///
/// Pure transport. Code-complete in M8; documented untested on the Linux host.
class WindowsBridge implements BridgeTransport {
  @override
  Future<BridgeResponse> request(String method, Map<String, dynamic> requestJson) {
    throw UnsupportedError('WindowsBridge lands in M8');
  }

  @override
  Stream<Map<String, dynamic>> get events =>
      throw UnsupportedError('WindowsBridge lands in M8');

  @override
  Future<void> start() async {}

  @override
  Future<void> stop() async {}
}
