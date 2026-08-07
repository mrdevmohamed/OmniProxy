import 'dart:async';
import 'dart:convert';
import 'dart:ffi';
import 'dart:io';

import 'package:ffi/ffi.dart';

import 'bridge_transport.dart';

/// Windows FFI bridge: `dart:ffi` into the c-shared `omniproxy.dll`
/// (see `docs/api-contract.md` §5.2, `docs/platform-notes.md` §Windows).
///
/// Pure transport — the same five-symbol C ABI as the Linux bridge, polled on
/// a short timer (a native callback into the Dart isolate would deadlock while
/// a synchronous request like `connect` is in flight). Differences from Linux:
/// the library is `omniproxy.dll`, the default data dir is `%APPDATA%\OmniProxy`,
/// and there is no `helperPath` — Windows runs the engine in-process.
class WindowsBridge implements BridgeTransport {
  WindowsBridge({String? libraryPath, Map<String, String>? initConfig})
      : _libraryPath = libraryPath, // ignore: prefer_initializing_formals
        _initConfig = initConfig; // ignore: prefer_initializing_formals

  final String? _libraryPath;
  final Map<String, String>? _initConfig;

  final _eventsController = StreamController<Map<String, dynamic>>.broadcast();

  DynamicLibrary? _lib;
  Timer? _pollTimer;
  bool _started = false;

  static const _pollInterval = Duration(milliseconds: 15);

  @override
  Stream<Map<String, dynamic>> get events => _eventsController.stream;

  @override
  Future<void> start() async {
    if (_started) return;
    final lib = DynamicLibrary.open(_libraryPath ?? defaultLibraryPath());
    _lib = lib;

    final config = <String, String>{
      ...?_initConfig,
      'dataDir': _initConfig?['dataDir'] ?? _defaultDataDir(),
      'logLevel': _initConfig?['logLevel'] ?? 'info',
    };
    final init = lib.lookupFunction<_InitNative, _InitDart>('omniproxy_init');
    final cfgPtr = jsonEncode(config).toNativeUtf8();
    final code = init(cfgPtr);
    malloc.free(cfgPtr);
    if (code != 0) {
      await stop();
      throw StateError('omniproxy_init failed (code $code)');
    }

    _pollTimer = Timer.periodic(_pollInterval, (_) => _poll());
    _started = true;
  }

  @override
  Future<void> stop() async {
    _pollTimer?.cancel();
    _pollTimer = null;
    if (_lib != null) {
      final shutdownFn =
          _lib!.lookupFunction<_ShutdownNative, _ShutdownDart>('omniproxy_shutdown');
      shutdownFn();
      _lib = null;
    }
    _started = false;
  }

  @override
  Future<BridgeResponse> request(
      String method, Map<String, dynamic> requestJson) async {
    await start();
    final lib = _lib!;
    final methodPtr = method.toNativeUtf8();
    final requestPtr = jsonEncode(requestJson).toNativeUtf8();
    final responsePtr =
        lib.lookupFunction<_RequestNative, _RequestDart>('omniproxy_request')(methodPtr, requestPtr);
    final raw = responsePtr.toDartString();
    _freeString(lib, responsePtr);
    malloc.free(methodPtr);
    malloc.free(requestPtr);

    final decoded = jsonDecode(raw);
    if (decoded is! Map<String, dynamic>) {
      throw StateError('malformed bridge response: $raw');
    }
    final ok = decoded['ok'] == true;
    final error = decoded['error'];
    if (ok) {
      final data = decoded['data'];
      return BridgeResponse(
        ok: true,
        data: data is Map<String, dynamic> ? data : const <String, dynamic>{},
      );
    }
    return BridgeResponse(
      ok: false,
      errorCode:
          error is Map ? (error['code'] as String? ?? 'internal') : 'internal',
      errorMessage: error is Map
          ? (error['message'] as String? ?? 'Unknown error')
          : 'Unknown error',
    );
  }

  void _poll() {
    final lib = _lib;
    if (lib == null) return;
    final pollFn =
        lib.lookupFunction<_PollEventsNative, _PollEventsDart>('omniproxy_poll_events');
    final eventsPtr = pollFn();
    if (eventsPtr == nullptr) return;
    final raw = eventsPtr.toDartString();
    _freeString(lib, eventsPtr);
    final decoded = jsonDecode(raw);
    if (decoded is! List<dynamic>) return;
    for (final item in decoded) {
      if (item is Map<String, dynamic>) {
        _eventsController.add(item);
      }
    }
  }

  void _freeString(DynamicLibrary lib, Pointer<Utf8> ptr) {
    final freeFn =
        lib.lookupFunction<_FreeStringNative, _FreeStringDart>('omniproxy_free_string');
    freeFn(ptr);
  }

  /// Default library path: env override, else the bundled location relative to
  /// the repo (core/out), else plain `omniproxy.dll` (next to the exe).
  static String defaultLibraryPath() {
    const env = String.fromEnvironment('OMNIPROXY_LIB');
    if (env.isNotEmpty) return env;
    if (Platform.environment['OMNIPROXY_LIB'] case final p?) return p;
    final cwd = Directory.current.path;
    final candidate = '$cwd\\..\\core\\out\\omniproxy.dll';
    if (File(candidate).existsSync()) return candidate;
    return 'omniproxy.dll';
  }

  static String _defaultDataDir() {
    final appData = Platform.environment['APPDATA'];
    if (appData != null && appData.isNotEmpty) return '$appData\\OmniProxy';
    final home = Platform.environment['USERPROFILE'] ?? '.';
    return '$home\\AppData\\Roaming\\OmniProxy';
  }
}

typedef _InitNative = Int32 Function(Pointer<Utf8> configJson);
typedef _InitDart = int Function(Pointer<Utf8> configJson);

typedef _RequestNative =
    Pointer<Utf8> Function(Pointer<Utf8> method, Pointer<Utf8> requestJson);
typedef _RequestDart =
    Pointer<Utf8> Function(Pointer<Utf8> method, Pointer<Utf8> requestJson);

typedef _PollEventsNative = Pointer<Utf8> Function();
typedef _PollEventsDart = Pointer<Utf8> Function();

typedef _ShutdownNative = Void Function();
typedef _ShutdownDart = void Function();

typedef _FreeStringNative = Void Function(Pointer<Utf8> ptr);
typedef _FreeStringDart = void Function(Pointer<Utf8> ptr);
