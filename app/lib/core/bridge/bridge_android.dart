import 'dart:async';
import 'dart:convert';

import 'package:flutter/services.dart';

import 'bridge_transport.dart';

/// Android MethodChannel bridge: gomobile bind `.aar` exposed over
/// `MethodChannel("com.omniproxy/bridge")`, events over
/// `MethodChannel("com.omniproxy/events")` (see `docs/api-contract.md` §5.1,
/// `docs/platform-notes.md` §Android).
///
/// Pure transport — the Kotlin host calls the Go core on a background thread
/// and drains its event ring, forwarding each event as `invokeMethod("event")`;
/// Dart just re-emits them here. The VpnService consent + TUN fd hand-off is
/// handled entirely by the native layer during `connect`.
class AndroidBridge implements BridgeTransport {
  static const _bridge = MethodChannel('com.omniproxy/bridge');
  static const _events = MethodChannel('com.omniproxy/events');

  final _eventsController = StreamController<Map<String, dynamic>>.broadcast();
  bool _started = false;

  @override
  Stream<Map<String, dynamic>> get events => _eventsController.stream;

  @override
  Future<void> start() async {
    if (_started) return;
    _events.setMethodCallHandler(_handleEvent);
    _started = true;
  }

  @override
  Future<void> stop() async {
    if (!_started) return;
    _events.setMethodCallHandler(null);
    _started = false;
  }

  @override
  Future<BridgeResponse> request(
      String method, Map<String, dynamic> requestJson) async {
    await start();
    final response =
        await _bridge.invokeMethod<String>(method, jsonEncode(requestJson));
    if (response == null) {
      return const BridgeResponse(
        ok: false,
        errorCode: 'internal',
        errorMessage: 'Empty bridge response',
      );
    }
    final decoded = jsonDecode(response);
    if (decoded is! Map<String, dynamic>) {
      throw StateError('malformed bridge response: $response');
    }
    if (decoded['ok'] == true) {
      final data = decoded['data'];
      return BridgeResponse(
        ok: true,
        data: data is Map<String, dynamic> ? data : const <String, dynamic>{},
      );
    }
    final error = decoded['error'];
    return BridgeResponse(
      ok: false,
      errorCode:
          error is Map ? (error['code'] as String? ?? 'internal') : 'internal',
      errorMessage: error is Map
          ? (error['message'] as String? ?? 'Unknown error')
          : 'Unknown error',
    );
  }

  Future<void> _handleEvent(MethodCall call) async {
    if (call.method == 'event' && call.arguments is String) {
      final decoded = jsonDecode(call.arguments as String);
      if (decoded is Map<String, dynamic>) {
        _eventsController.add(decoded);
      }
    }
  }
}
