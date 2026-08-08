import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:omniproxy/app/app_root.dart';
import 'package:omniproxy/app/router.dart';
import 'package:omniproxy/core/mock_api_client.dart';
import 'package:omniproxy/core/models.dart';
import 'package:omniproxy/services/system_tray_service.dart';
import 'package:omniproxy/state/providers.dart';
import 'package:tray_manager/tray_manager.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  const trayChannel = MethodChannel('tray_manager');
  const windowChannel = MethodChannel('window_manager');

  final trayCalls = <String>[];
  final windowCalls = <String>[];
  Map<String, dynamic>? lastMenu;

  Future<void> settle([int ms = 5]) =>
      Future<void>.delayed(Duration(milliseconds: ms));

  setUp(() {
    trayCalls.clear();
    windowCalls.clear();
    lastMenu = null;
    final messenger =
        TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(trayChannel, (call) async {
      trayCalls.add(call.method);
      if (call.method == 'setContextMenu') {
        lastMenu = Map<String, dynamic>.from(
          (call.arguments as Map)['menu'] as Map,
        );
      }
      return null;
    });
    messenger.setMockMethodCallHandler(windowChannel, (call) async {
      windowCalls.add(call.method);
      // Return proper typed values so the plugin's Dart-side branching
      // (e.g. show() first checks isMinimized()) doesn't throw on null.
      switch (call.method) {
        case 'isMinimized':
        case 'isVisible':
        case 'isFullScreen':
        case 'isFocused':
        case 'isPreventClose':
        case 'isMaximized':
          return false;
        default:
          return null;
      }
    });
  });

  tearDown(() {
    final messenger =
        TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(trayChannel, null);
    messenger.setMockMethodCallHandler(windowChannel, null);
  });

  ProviderContainer buildContainer({MockApiClient? client}) {
    final container = ProviderContainer(
      overrides: [
        apiClientProvider.overrideWithValue(
          client ??
              MockApiClient(connectDelay: const Duration(milliseconds: 5)),
        ),
      ],
    );
    addTearDown(container.dispose);
    return container;
  }

  SystemTrayService buildService(
    ProviderContainer container, {
    Future<void> Function()? onExit,
  }) {
    final tray = SystemTrayService(
      container: container,
      onExit: onExit ?? () async {},
    );
    addTearDown(() async => tray.dispose());
    return tray;
  }

  String? currentToggleLabel() {
    final items = (lastMenu?['items'] as List?) ?? const [];
    for (final item in items) {
      if ((item as Map)['key'] == 'toggle') return item['label'] as String;
    }
    return null;
  }

  group('SystemTrayService', () {
    test('init installs icon/menu and is idempotent', () async {
      final container = buildContainer();
      await container.read(serversProvider.future);
      final tray = buildService(container);

      expect(await tray.init(), isTrue);
      expect(tray.isInitialized, isTrue);
      await settle();

      expect(trayCalls, contains('setIcon'));
      expect(trayCalls, contains('setContextMenu'));
      expect(windowCalls, contains('ensureInitialized'));
      expect(windowCalls, contains('setPreventClose'));
      expect(lastMenu?['items'], hasLength(5));
      expect(currentToggleLabel(), 'Connect');

      // Second init must not double-install anything.
      expect(await tray.init(), isTrue);
      expect(trayCalls.where((c) => c == 'setIcon').length, 1);
      expect(windowCalls.where((c) => c == 'setPreventClose').length, 1);
    });

    test('menu label tracks connection state', () async {
      final container = buildContainer();
      await container.read(serversProvider.future);
      final tray = buildService(container);
      await tray.init();
      await settle();

      expect(currentToggleLabel(), 'Connect');

      await container
          .read(connectionProvider.notifier)
          .connect('00000000-0000-4000-8000-000000000001');
      await settle(30);
      expect(
        container.read(connectionProvider).state,
        ConnectionState.connected,
      );
      expect(currentToggleLabel(), 'Disconnect');

      await container.read(connectionProvider.notifier).disconnect();
      await settle();
      expect(
        container.read(connectionProvider).state,
        ConnectionState.disconnected,
      );
      expect(currentToggleLabel(), 'Connect');
    });

    test('toggle connects to the selected server and disconnects', () async {
      final container = buildContainer();
      await container.read(serversProvider.future);
      final tray = buildService(container);
      await tray.init();

      tray.onTrayMenuItemClick(MenuItem(key: 'toggle', label: 'Connect'));
      await settle(40);
      expect(
        container.read(connectionProvider).state,
        ConnectionState.connected,
      );
      expect(
        container.read(connectionProvider).session?.serverId,
        '00000000-0000-4000-8000-000000000001',
      );

      tray.onTrayMenuItemClick(MenuItem(key: 'toggle', label: 'Disconnect'));
      await settle(10);
      expect(
        container.read(connectionProvider).state,
        ConnectionState.disconnected,
      );
    });

    test('open restores/focuses the window and settings navigates', () async {
      final container = buildContainer();
      await container.read(serversProvider.future);
      final tray = buildService(container);
      await tray.init();

      tray.onTrayMenuItemClick(MenuItem(key: 'open', label: 'Open'));
      await settle();
      expect(windowCalls, contains('restore'));
      expect(windowCalls, contains('show'));
      expect(windowCalls, contains('focus'));

      tray.onTrayMenuItemClick(MenuItem(key: 'settings', label: 'Settings'));
      await settle();
      expect(
        container.read(shellDestinationProvider),
        ShellDestination.settings,
      );
    });

    test('closing the window hides instead of quitting', () async {
      final container = buildContainer();
      await container.read(serversProvider.future);
      final tray = buildService(container);
      await tray.init();

      tray.onWindowClose();
      await settle();
      expect(windowCalls, contains('hide'));
      expect(windowCalls, isNot(contains('destroy')));
    });

    test('quit disconnects, tears down tray and exits', () async {
      final container = buildContainer();
      await container.read(serversProvider.future);
      var exited = false;
      final tray = SystemTrayService(
        container: container,
        onExit: () async => exited = true,
      );
      addTearDown(() async => tray.dispose());
      await tray.init();

      await container
          .read(connectionProvider.notifier)
          .connect('00000000-0000-4000-8000-000000000001');
      await settle(30);
      expect(
        container.read(connectionProvider).state,
        ConnectionState.connected,
      );

      tray.onTrayMenuItemClick(MenuItem(key: 'quit', label: 'Quit'));
      await settle(30);

      expect(exited, isTrue);
      expect(
        container.read(connectionProvider).state,
        ConnectionState.disconnected,
      );
      expect(trayCalls, contains('destroy'));
      expect(windowCalls, contains('setPreventClose'));
      expect(windowCalls, contains('destroy'));
      expect(trayManager.hasListeners, isFalse);
      expect(tray.isInitialized, isFalse);
    });

    test('quit is idempotent', () async {
      final container = buildContainer();
      await container.read(serversProvider.future);
      var exits = 0;
      final tray = SystemTrayService(
        container: container,
        onExit: () async => exits++,
      );
      addTearDown(() async => tray.dispose());
      await tray.init();

      tray.onTrayMenuItemClick(MenuItem(key: 'quit', label: 'Quit'));
      await settle();
      tray.onTrayMenuItemClick(MenuItem(key: 'quit', label: 'Quit'));
      await settle();
      expect(exits, 1);
    });

    test('dispose removes listeners and is safe to repeat', () async {
      final container = buildContainer();
      await container.read(serversProvider.future);
      final tray = buildService(container);
      await tray.init();
      expect(trayManager.hasListeners, isTrue);

      await tray.dispose();
      expect(tray.isInitialized, isFalse);
      expect(trayManager.hasListeners, isFalse);
      expect(trayCalls, contains('destroy'));

      await tray.dispose(); // no-op
      expect(trayCalls.where((c) => c == 'destroy').length, 1);
    });
  });

  testWidgets('shell follows shellDestinationProvider (tray Settings nav)', (
    tester,
  ) async {
    final client = MockApiClient(connectDelay: const Duration(milliseconds: 5));
    final container = ProviderContainer(
      overrides: [apiClientProvider.overrideWithValue(client)],
    );
    await tester.pumpWidget(
      UncontrolledProviderScope(
        container: container,
        child: const OmniProxyApp(),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('Dashboard'), findsWidgets);

    container
        .read(shellDestinationProvider.notifier)
        .select(ShellDestination.settings);
    await tester.pumpAndSettle();
    expect(find.text('Default mode'), findsOneWidget);
  });
}
