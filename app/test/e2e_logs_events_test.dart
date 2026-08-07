import 'dart:io';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:omniproxy/core/bridge/bridge_linux.dart';
import 'package:omniproxy/core/bridge_api_client.dart';
import 'package:omniproxy/core/models.dart';
import 'package:omniproxy/state/providers.dart';

/// Regression coverage for live `logAppended` delivery over the real Linux
/// bridge (requires `libomniproxy.so` built by `tools/build_linux.sh`).
///
/// Guards against the seq=0 regression: live log events must carry the
/// ring-assigned sequence number, otherwise the app's incremental log viewer
/// silently drops every live entry and logs only appear on an explicit
/// refresh.
void main() {
  final libPath = LinuxBridge.defaultLibraryPath();

  test('linux bridge: logAppended events carry a valid seq over the .so',
      () async {
    if (!File(libPath).existsSync()) {
      markTestSkipped(
          'libomniproxy.so not found at $libPath — run tools/build_linux.sh first');
      return;
    }

    final dir = await Directory.systemTemp.createTemp('omniproxy_logs');
    final transport = LinuxBridge(
      libraryPath: libPath,
      initConfig: {'dataDir': dir.path},
    );
    final client = BridgeApiClient(transport);
    addTearDown(() async {
      await transport.stop();
      try {
        await dir.delete(recursive: true);
      } on FileSystemException {
        // best-effort cleanup
      }
    });

    await client.subscribe(const ['stateChanged', 'logAppended']);

    final received = <LogEntry>[];
    final sub = client.events.listen((e) {
      if (e.type == 'logAppended') {
        received.add(LogEntry.fromJson(e.data));
      }
    });
    addTearDown(sub.cancel);

    await client.addServer(_profile('Seq SOCKS'));

    await Future<void>.delayed(const Duration(milliseconds: 300));

    expect(received, isNotEmpty,
        reason: 'expected a live logAppended event after addServer');
    expect(received.map((l) => l.message), contains(contains('added server')));
    expect(received.map((l) => l.seq).toSet(), isNot(contains(0)),
        reason: 'live events must carry the ring-assigned sequence number');
  });

  test('app providers: live logs reach the viewer without manual refresh',
      () async {
    if (!File(libPath).existsSync()) {
      markTestSkipped(
          'libomniproxy.so not found at $libPath — run tools/build_linux.sh first');
      return;
    }

    final dir = await Directory.systemTemp.createTemp('omniproxy_prov');
    final transport = LinuxBridge(
      libraryPath: libPath,
      initConfig: {'dataDir': dir.path},
    );
    final client = BridgeApiClient(transport);
    addTearDown(() async {
      await transport.stop();
      try {
        await dir.delete(recursive: true);
      } on FileSystemException {
        // best-effort cleanup
      }
    });

    final container = ProviderContainer(overrides: [
      apiClientProvider.overrideWithValue(client),
    ]);
    addTearDown(container.dispose);

    // Watch connectionProvider: triggers ConnectionNotifier.build() which
    // issues subscribe(['stateChanged', 'logAppended']) — exactly like startup.
    container.listen(connectionProvider, (_, _) {});
    await Future<void>.delayed(const Duration(milliseconds: 200));

    final seen = <List<LogEntry>>[];
    container.listen(logsProvider, (_, next) => seen.add(next),
        fireImmediately: true);

    await container.read(serversProvider.notifier).add(_profile('Prov SOCKS'));

    // No manual refresh: the live logAppended event must append on its own.
    await Future<void>.delayed(const Duration(milliseconds: 500));

    final messages = seen.expand((l) => l).map((l) => l.message).toList();
    expect(messages.any((m) => m.contains('added server')), isTrue,
        reason: 'expected the log to appear live via logAppended');
  });
}

ServerProfile _profile(String name) => ServerProfile(
      id: '',
      name: name,
      protocol: ServerProtocol.socks5,
      address: '127.0.0.1',
      port: 1,
      username: 'user',
      password: 'pass',
      createdAt: DateTime.now().toUtc(),
      updatedAt: DateTime.now().toUtc(),
    );
