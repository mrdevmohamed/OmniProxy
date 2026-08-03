import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:omniproxy/core/bridge/bridge_linux.dart';
import 'package:omniproxy/core/bridge_api_client.dart';
import 'package:omniproxy/core/models.dart';

/// M6 Linux bridge E2E: drives the real `libomniproxy.so` (built by
/// `tools/build_linux.sh` into `core/out`) through the canonical contract.
///
/// Full chain exercised: Dart contract client → FFI transport → Go core →
/// in-process sing-box engine (proxy mode) → SOCKS5 outbound → local SOCKS5
/// server → echo target. Requires the .so present; skips with instructions
/// otherwise so `flutter test` stays green on a fresh checkout.
void main() {
  final libPath = LinuxBridge.defaultLibraryPath();

  test('linux bridge: real connect/disconnect + traffic over the .so',
      () async {
    if (!File(libPath).existsSync()) {
      markTestSkipped(
          'libomniproxy.so not found at $libPath — run tools/build_linux.sh first');
      return;
    }

    // Local test infra: echo target + SOCKS5 CONNECT server forwarding to it.
    final echo = await startEchoServer();
    final socks = await startSocks5Server(echo.port);

    final dir = await Directory.systemTemp.createTemp('omniproxy_e2e');
    final dataDir = dir.path;
    final transport = LinuxBridge(
      libraryPath: libPath,
      initConfig: {'dataDir': dataDir},
    );
    final client = BridgeApiClient(transport);
    addTearDown(() async {
      await transport.stop();
      await echo.close();
      await socks.close();
      try {
        await dir.delete(recursive: true);
      } on FileSystemException {
        // best-effort cleanup
      }
    });

    // Version handshake.
    final version = await client.getVersion();
    expect(version.version, isNotEmpty);
    expect(version.platform, 'linux');
    expect(version.engineVersion, 'v1.13.15');

    // Server catalog round-trip.
    final addedId = await client.addServer(ServerProfile(
      id: '',
      name: 'E2E SOCKS',
      protocol: ServerProtocol.socks5,
      address: '127.0.0.1',
      port: socks.port,
      username: 'user',
      password: 'pass',
      createdAt: DateTime.now().toUtc(),
      updatedAt: DateTime.now().toUtc(),
    ));
    expect(addedId, isNotEmpty);
    final server = await client.getServer(addedId);
    expect(server.protocol, ServerProtocol.socks5);
    expect(server.port, socks.port);

    // Subscribe to events, then connect in proxy mode.
    await client.subscribe(const ['stateChanged', 'logAppended']);
    final connected = Completer<void>();
    final events = client.events.listen((e) {
      if (e.type == 'stateChanged') {
        final snap = ConnectionSnapshot.fromJson(e.data);
        if (snap.state == ConnectionState.connected && !connected.isCompleted) {
          connected.complete();
        }
      }
    });
    addTearDown(events.cancel);

    await client.connect(serverId: addedId, mode: ConnectionMode.proxy);
    await connected.future.timeout(const Duration(seconds: 15));

    final snapshot = await client.getConnectionState();
    expect(snapshot.state, ConnectionState.connected);
    expect(snapshot.session, isNotNull);

    // Traffic through the whole chain: dart SOCKS5 client -> engine inbound ->
    // engine SOCKS5 outbound -> local SOCKS5 server -> echo target.
    final (engineInbound, engineRecv) =
        await _connectSocks5('127.0.0.1', 1080, '127.0.0.1', echo.port);
    final payload = utf8.encode('omniproxy-ffi-roundtrip');
    engineInbound.add(payload);
    await engineInbound.flush();
    final reply = await _readN(engineRecv, payload.length);
    expect(utf8.decode(reply), utf8.decode(payload));
    await engineInbound.close();

    // Disconnect back to idle.
    await client.disconnect();
    final idle = await client.getConnectionState();
    expect(idle.state, ConnectionState.disconnected);
  });
}

// --- Local test servers ---

/// TCP server that echoes everything it receives.
Future<ServerSocket> startEchoServer() async {
  final server = await ServerSocket.bind('127.0.0.1', 0);
  server.listen((socket) {
    socket.listen(
      (data) => socket.add(data),
      onDone: socket.destroy,
      onError: (_) => socket.destroy(),
      cancelOnError: true,
    );
  });
  return server;
}

/// Minimal no-auth SOCKS5 CONNECT server tunneling to the local echo target.
Future<ServerSocket> startSocks5Server(int targetPort) async {
  final server = await ServerSocket.bind('127.0.0.1', 0);
  server.listen((socket) {
    _handleSocks5(socket, targetPort);
  });
  return server;
}

/// Buffers a socket stream for sequential protocol reads. A [Socket] is a
/// single-subscription stream, so reads go through one listener.
class _Recv {
  _Recv(Stream<List<int>> stream) {
    stream.listen(
      (data) => _deliver(Uint8List.fromList(data)),
      onDone: () => _deliver(_empty),
      onError: (_) => _deliver(_empty),
    );
  }

  static final _empty = Uint8List(0);

  final _queue = <Uint8List>[];
  final _waiters = <Completer<Uint8List>>[];
  void Function(Uint8List)? _relay;

  /// Switches to relay mode: every subsequent chunk is handed to [onChunk];
  /// an empty chunk marks end-of-stream. Any bytes buffered since the last
  /// `next()` are drained first.
  void relay(void Function(Uint8List) onChunk) {
    _relay = onChunk;
    while (_queue.isNotEmpty) {
      onChunk(_queue.removeAt(0));
    }
  }

  void _deliver(Uint8List chunk) {
    final relay = _relay;
    if (relay != null) {
      relay(chunk);
      return;
    }
    if (_waiters.isNotEmpty) {
      _waiters.removeAt(0).complete(chunk);
    } else {
      _queue.add(chunk);
    }
  }

  Future<Uint8List> next({Duration timeout = const Duration(seconds: 5)}) {
    if (_queue.isNotEmpty) return Future.value(_queue.removeAt(0));
    final completer = Completer<Uint8List>();
    _waiters.add(completer);
    Timer(timeout, () {
      if (!completer.isCompleted) {
        completer.completeError(TimeoutException('read timed out'));
      }
    });
    return completer.future;
  }
}

Future<void> _handleSocks5(Socket client, int targetPort) async {
  try {
    final recv = _Recv(client);
    final greeting = await recv.next();
    if (greeting.isEmpty || greeting[0] != 0x05) {
      client.destroy();
      return;
    }
    client.add(Uint8List.fromList([0x05, 0x00])); // no-auth
    await client.flush();

    final request = await recv.next();
    if (request.length < 4 || request[0] != 0x05 || request[1] != 0x01) {
      client.destroy();
      return;
    }
    var off = 4;
    switch (request[3]) {
      case 0x01: // IPv4
        off += 4;
        break;
      case 0x03: // domain
        final len = request[4];
        off += 1 + len;
        break;
      default:
        client.destroy();
        return;
    }
    if (request.length < off + 2) {
      client.destroy();
      return;
    }

    final target = await Socket.connect('127.0.0.1', targetPort);
    client.add(Uint8List.fromList([0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0]));
    await client.flush();
    recv.relay((chunk) {
      if (chunk.isEmpty) {
        target.destroy();
      } else {
        target.add(chunk);
      }
    });
    target.listen(
      (data) => client.add(data),
      onDone: client.destroy,
      onError: (_) => client.destroy(),
      cancelOnError: true,
    );
  } catch (_) {
    client.destroy();
  }
}

/// Minimal no-auth SOCKS5 CONNECT client: returns the connected Socket with
/// its [ _Recv] already listening (a Socket stream is single-subscription).
Future<(Socket, _Recv)> _connectSocks5(
    String proxyHost, int proxyPort, String targetHost, int targetPort) async {
  final socket = await Socket.connect(proxyHost, proxyPort);
  final recv = _Recv(socket);
  socket.add(Uint8List.fromList([0x05, 0x01, 0x00]));
  await socket.flush();
  final methods = await recv.next();
  if (methods.length < 2 || methods[1] != 0x00) {
    socket.destroy();
    throw StateError('SOCKS5 auth negotiation failed: $methods');
  }
  final hostBytes = utf8.encode(targetHost);
  final request = <int>[
    0x05, 0x01, 0x00, 0x03, hostBytes.length, ...hostBytes,
    (targetPort >> 8) & 0xff, targetPort & 0xff,
  ];
  socket.add(Uint8List.fromList(request));
  await socket.flush();
  final reply = await recv.next();
  if (reply.length < 2 || reply[1] != 0x00) {
    socket.destroy();
    throw StateError('SOCKS5 CONNECT failed: $reply');
  }
  return (socket, recv);
}

/// Accumulates exactly [n] bytes from the socket via its [_Recv].
Future<Uint8List> _readN(_Recv recv, int n) async {
  final buffer = <int>[];
  while (buffer.length < n) {
    final chunk = await recv.next();
    if (chunk.isEmpty) {
      throw StateError('socket closed after ${buffer.length}/$n bytes');
    }
    buffer.addAll(chunk);
  }
  return Uint8List.fromList(buffer.sublist(0, n));
}
