import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:omniproxy/app/app_root.dart';
import 'package:omniproxy/core/models.dart';
import 'package:omniproxy/state/providers.dart';

/// M7 Android bridge E2E: runs on a live device/emulator and drives the real
/// bridge end-to-end. Full chain exercised:
///
///   Flutter bridge client → MethodChannel → Kotlin Bridge → gomobile Go core →
///   in-process sing-box engine (proxy mode), and back: core event ring →
///   Kotlin poller → `invokeMethod("event")` → bridge stream → connection UI.
///
/// Proxy mode is used so the flow needs no system VPN-consent dialog. Run with:
///   flutter test integration_test/bridge_e2e_test.dart -d `device`
///
/// The VPN test case only runs when `--dart-define=OMNIPROXY_VPN_E2E=true`, and
/// needs the system consent dialog accepted by an external adb watcher.
void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  const runVpn = bool.fromEnvironment('OMNIPROXY_VPN_E2E');

  testWidgets('android bridge: version, CRUD, connect/disconnect',
      (tester) async {
    // Boot the real app so MainActivity wires the bridge channels + Go init.
    await tester.pumpWidget(const ProviderScope(child: OmniProxyApp()));
    await _pumpFor(tester, const Duration(seconds: 3));

    final client = ProviderScope.containerOf(
      tester.element(find.byType(OmniProxyApp)),
    ).read(apiClientProvider);

    // Version handshake through the gomobile core.
    final version = await client.getVersion();
    expect(version.version, isNotEmpty);
    expect(version.platform, 'android');
    expect(version.engineVersion, 'v1.13.15');

    // Server CRUD round-trip (127.0.0.1:9 is unreachable, but the proxy
    // inbound still comes up — that is what 'connected' reflects).
    final addedId = await client.addServer(ServerProfile(
      id: '',
      name: 'Mobile E2E',
      protocol: ServerProtocol.socks5,
      address: '127.0.0.1',
      port: 9,
      username: 'user',
      password: 'pass',
      createdAt: DateTime.now().toUtc(),
      updatedAt: DateTime.now().toUtc(),
    ));
    expect(addedId, isNotEmpty);
    final server = await client.getServer(addedId);
    expect(server.protocol, ServerProtocol.socks5);

    // Connect in proxy mode; wait for the connected stateChanged event to reach
    // the UI status chip via the event pipeline.
    await client.connect(serverId: addedId, mode: ConnectionMode.proxy);
    await _pumpUntilFound(tester, find.text('Connected'),
        timeout: const Duration(seconds: 30));
    expect(find.text('Connected'), findsWidgets);

    // Disconnect back to idle.
    await client.disconnect();
    await _pumpUntilFound(tester, find.text('Disconnected'),
        timeout: const Duration(seconds: 30));
    expect(find.text('Disconnected'), findsWidgets);
  });

  testWidgets('android bridge: vpn connect via consent dialog', (tester) async {
    if (!runVpn) return;

    await tester.pumpWidget(const ProviderScope(child: OmniProxyApp()));
    await _pumpFor(tester, const Duration(seconds: 3));

    final client = ProviderScope.containerOf(
      tester.element(find.byType(OmniProxyApp)),
    ).read(apiClientProvider);

    final addedId = await client.addServer(ServerProfile(
      id: '',
      name: 'VPN E2E',
      protocol: ServerProtocol.socks5,
      address: '127.0.0.1',
      port: 9,
      createdAt: DateTime.now().toUtc(),
      updatedAt: DateTime.now().toUtc(),
    ));

    // VPN connect triggers VpnService.prepare; an external adb watcher must
    // accept the consent dialog within this window. Then the VpnService
    // establishes the TUN, hands its fd to the core, and connects.
    await client
        .connect(serverId: addedId, mode: ConnectionMode.vpn)
        .timeout(const Duration(seconds: 90));
    await _pumpUntilFound(tester, find.text('Connected'),
        timeout: const Duration(seconds: 30));

    await client.disconnect();
    await _pumpUntilFound(tester, find.text('Disconnected'),
        timeout: const Duration(seconds: 30));
  });
}

Future<void> _pumpFor(WidgetTester tester, Duration duration) async {
  final end = DateTime.now().add(duration);
  while (DateTime.now().isBefore(end)) {
    await tester.pump(const Duration(milliseconds: 100));
  }
}

Future<void> _pumpUntilFound(
  WidgetTester tester,
  Finder finder, {
  Duration timeout = const Duration(seconds: 15),
}) async {
  final deadline = DateTime.now().add(timeout);
  while (DateTime.now().isBefore(deadline)) {
    await tester.pump(const Duration(milliseconds: 100));
    if (finder.evaluate().isNotEmpty) return;
  }
  fail('Timed out waiting for $finder');
}
