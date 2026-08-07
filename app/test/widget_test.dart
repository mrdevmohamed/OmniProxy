import 'package:flutter/material.dart' hide ConnectionState;
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:omniproxy/app/app_root.dart';
import 'package:omniproxy/core/models.dart';
import 'package:omniproxy/core/mock_api_client.dart';
import 'package:omniproxy/features/dashboard/dashboard_screen.dart';
import 'package:omniproxy/features/logs/logs_screen.dart';
import 'package:omniproxy/features/servers/servers_screen.dart';
import 'package:omniproxy/features/settings/settings_screen.dart';
import 'package:omniproxy/state/providers.dart';

void main() {
  ProviderScope buildApp({MockApiClient? client}) {
    return ProviderScope(
      overrides: [
        apiClientProvider.overrideWithValue(client ?? MockApiClient()),
      ],
      child: const OmniProxyApp(),
    );
  }

  testWidgets('dashboard shows disconnected status and seeded servers',
      (tester) async {
    await tester.pumpWidget(buildApp());
    await tester.pumpAndSettle();

    expect(find.text('Dashboard'), findsWidgets);
    expect(find.text('Disconnected'), findsWidgets);
    expect(find.text('Ready to connect'), findsOneWidget);
    expect(find.text('Connect'), findsOneWidget);
  });

  testWidgets('connect flow transitions to connected and back',
      (tester) async {
    final client = MockApiClient(connectDelay: const Duration(milliseconds: 500));
    await tester.pumpWidget(buildApp(client: client));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Connect'));
    await tester.pump();
    expect(find.text('Connecting'), findsWidgets);

    await tester.pump(const Duration(milliseconds: 600));
    expect(find.text('Connected'), findsWidgets);
    expect(find.text('Disconnect'), findsOneWidget);
    expect(find.text('Tokyo Relay'), findsOneWidget);

    await tester.tap(find.text('Disconnect'));
    await tester.pump();
    expect(find.text('Disconnected'), findsWidgets);
  });

  testWidgets('servers tab lists seeded servers', (tester) async {
    await tester.pumpWidget(buildApp());
    await tester.pumpAndSettle();

    await tester.tap(find.text('Servers'));
    await tester.pumpAndSettle();

    expect(find.text('Tokyo Relay'), findsOneWidget);
    expect(find.text('Frankfurt Shadowsocks'), findsOneWidget);
    expect(find.text('Local HTTP'), findsOneWidget);
    expect(find.text('Add server'), findsOneWidget);
  });

  testWidgets('servers tab opens add form and saves a new server',
      (tester) async {
    await tester.pumpWidget(buildApp());
    await tester.pumpAndSettle();

    await tester.tap(find.text('Servers'));
    await tester.pumpAndSettle();

    await tester.tap(find.widgetWithText(FilledButton, 'Add server'));
    await tester.pumpAndSettle();

    await tester.enterText(
        find.widgetWithText(TextFormField, 'Name'), 'New Server');
    await tester.enterText(
        find.widgetWithText(TextFormField, 'Address'), 'new.example.com');
    await tester.enterText(find.widgetWithText(TextFormField, 'Port'), '443');
    await tester.enterText(find.widgetWithText(TextFormField, 'UUID'),
        'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa');

    final submit = find.widgetWithText(FilledButton, 'Add server');
    await tester.scrollUntilVisible(
      submit,
      200,
      scrollable: find
          .descendant(
            of: find.byType(ListView),
            matching: find.byType(Scrollable),
          )
          .first,
    );
    await tester.pumpAndSettle();
    await tester.tap(submit);
    await tester.pumpAndSettle();

    expect(find.text('New Server'), findsOneWidget);
  });

  testWidgets('settings tab switches connection mode', (tester) async {
    await tester.pumpWidget(buildApp());
    await tester.pumpAndSettle();

    await tester.tap(find.text('Settings'));
    await tester.pumpAndSettle();

    expect(find.text('Default mode'), findsOneWidget);
    await tester.tap(find.text('Proxy'));
    await tester.pumpAndSettle();

    final container =
        ProviderScope.containerOf(tester.element(find.byType(SettingsScreen)));
    expect(container.read(settingsProvider).connectionMode,
        ConnectionMode.proxy);
  });

  testWidgets('settings tab changes IPv6 mode', (tester) async {
    await tester.pumpWidget(buildApp());
    await tester.pumpAndSettle();

    await tester.tap(find.text('Settings'));
    await tester.pumpAndSettle();

    expect(find.text('IPv6'), findsOneWidget);
    expect(find.text('Prefer IPv4 (default)'), findsOneWidget);

    await tester.scrollUntilVisible(
      find.text('Prefer IPv4 (default)'),
      200,
      scrollable: find
          .descendant(
            of: find.byType(ListView),
            matching: find.byType(Scrollable),
          )
          .first,
    );
    await tester.pumpAndSettle();
    await tester.tap(find.text('Prefer IPv4 (default)'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Disable IPv6'));
    await tester.pumpAndSettle();

    final container =
        ProviderScope.containerOf(tester.element(find.byType(SettingsScreen)));
    expect(container.read(settingsProvider).ipv6Mode, IPv6Mode.disableIpv6);
  });

  testWidgets('logs tab streams core log entries', (tester) async {
    final client = MockApiClient(connectDelay: const Duration(milliseconds: 100));
    await tester.pumpWidget(buildApp(client: client));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Connect'));
    await tester.pumpAndSettle();
    expect(find.text('Connected'), findsWidgets);

    await tester.tap(find.text('Logs'));
    await tester.pumpAndSettle();

    expect(find.textContaining('Connected to'), findsOneWidget);
    expect(find.text('INFO'), findsWidgets);
  });

  testWidgets('settings tab toggles Advanced Mode', (tester) async {
    await tester.pumpWidget(buildApp());
    await tester.pumpAndSettle();

    await tester.tap(find.text('Settings'));
    await tester.pumpAndSettle();

    await tester.scrollUntilVisible(
      find.text('Enable Advanced Mode'),
      200,
      scrollable: find
          .descendant(
            of: find.byType(ListView),
            matching: find.byType(Scrollable),
          )
          .first,
    );
    await tester.pumpAndSettle();
    await tester.tap(find.text('Enable Advanced Mode'));
    await tester.pumpAndSettle();

    final container =
        ProviderScope.containerOf(tester.element(find.byType(SettingsScreen)));
    expect(container.read(settingsProvider).advancedModeEnabled, isTrue);
  });

  testWidgets('server form gates insecure certs behind Advanced Mode',
      (tester) async {
    await tester.pumpWidget(buildApp());
    await tester.pumpAndSettle();

    await tester.tap(find.text('Servers'));
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(FilledButton, 'Add server'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('TLS'));
    await tester.pumpAndSettle();

    // Default flow (Advanced Mode off): toggle present but locked.
    final locked = tester.widget<SwitchListTile>(find.widgetWithText(
        SwitchListTile, 'Allow insecure certificates'));
    expect(locked.onChanged, isNull);
    expect(find.textContaining('Requires Advanced Mode'), findsOneWidget);

    // Back out, enable Advanced Mode, and re-open the form.
    await tester.pageBack();
    await tester.pumpAndSettle();
    await tester.tap(find.text('Settings'));
    await tester.pumpAndSettle();
    await tester.scrollUntilVisible(
      find.text('Enable Advanced Mode'),
      200,
      scrollable: find
          .descendant(
            of: find.byType(ListView),
            matching: find.byType(Scrollable),
          )
          .first,
    );
    await tester.pumpAndSettle();
    await tester.tap(find.text('Enable Advanced Mode'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Servers'));
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(FilledButton, 'Add server'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('TLS'));
    await tester.pumpAndSettle();

    // Advanced Mode on: interactive, and enabling shows an explicit warning.
    final editable = tester.widget<SwitchListTile>(find.widgetWithText(
        SwitchListTile, 'Allow insecure certificates'));
    expect(editable.onChanged, isNotNull);

    await tester.ensureVisible(find.text('Allow insecure certificates'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Allow insecure certificates'));
    await tester.pumpAndSettle();
    expect(find.text('Disable certificate validation?'), findsOneWidget);

    await tester.tap(find.text('Cancel'));
    await tester.pumpAndSettle();
    expect(
      tester
          .widget<SwitchListTile>(find.widgetWithText(
              SwitchListTile, 'Allow insecure certificates'))
          .value,
      isFalse,
    );

    await tester.ensureVisible(find.text('Allow insecure certificates'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Allow insecure certificates'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('I understand'));
    await tester.pumpAndSettle();
    expect(
      tester
          .widget<SwitchListTile>(find.widgetWithText(
              SwitchListTile, 'Allow insecure certificates'))
          .value,
      isTrue,
    );
  });

  testWidgets('dashboard defaults target to the favorite and honors the selector',
      (tester) async {
    final client = MockApiClient(connectDelay: const Duration(milliseconds: 50));
    await tester.pumpWidget(buildApp(client: client));
    await tester.pumpAndSettle();

    final container =
        ProviderScope.containerOf(tester.element(find.byType(DashboardScreen)));
    const tokyo = '00000000-0000-4000-8000-000000000001';
    const frankfurt = '00000000-0000-4000-8000-000000000002';

    // The favorite server is the default connect target.
    expect(container.read(selectedServerProvider), tokyo);

    // Switching the dropdown re-targets the connection.
    await tester.tap(find.byType(DropdownButton<String>));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Frankfurt Shadowsocks').last);
    await tester.pumpAndSettle();
    expect(container.read(selectedServerProvider), frankfurt);

    // Connecting uses the newly selected server, not the favorite.
    await tester.tap(find.text('Connect'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 80));
    await tester.pumpAndSettle();
    expect(container.read(connectionProvider).session?.serverId, frankfurt);
  });

  testWidgets('after a disconnect, changing server re-targets the next connect',
      (tester) async {
    final client = MockApiClient(connectDelay: const Duration(milliseconds: 50));
    await tester.pumpWidget(buildApp(client: client));
    await tester.pumpAndSettle();

    const tokyo = '00000000-0000-4000-8000-000000000001';
    const frankfurt = '00000000-0000-4000-8000-000000000002';
    final container =
        ProviderScope.containerOf(tester.element(find.byType(DashboardScreen)));

    // Connect to the default target (the favorite, Tokyo = A), then disconnect.
    await tester.tap(find.text('Connect'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 80));
    await tester.pumpAndSettle();
    expect(container.read(connectionProvider).session?.serverId, tokyo);

    await tester.tap(find.text('Disconnect'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 80));
    await tester.pumpAndSettle();
    expect(container.read(connectionProvider).state,
        ConnectionState.disconnected);

    // The stale session still points at Tokyo; pick Frankfurt (B) instead.
    await tester.tap(find.byType(DropdownButton<String>));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Frankfurt Shadowsocks').last);
    await tester.pumpAndSettle();
    expect(container.read(selectedServerProvider), frankfurt);

    // Reconnecting must use the newly selected server, not the old session's.
    await tester.tap(find.text('Connect'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 80));
    await tester.pumpAndSettle();
    expect(container.read(connectionProvider).session?.serverId, frankfurt);
    expect(find.text('Frankfurt Shadowsocks'), findsOneWidget);
  });

  testWidgets('servers tab highlights selected server and Select re-targets',
      (tester) async {
    await tester.pumpWidget(buildApp());
    await tester.pumpAndSettle();

    await tester.tap(find.text('Servers'));
    await tester.pumpAndSettle();

    // The favorite (Tokyo Relay) is selected by default.
    final tokyoCard =
        find.ancestor(of: find.text('Tokyo Relay'), matching: find.byType(Card));
    expect(
      find.descendant(of: tokyoCard, matching: find.byIcon(Icons.check_circle)),
      findsOneWidget,
    );

    // Select Frankfurt Shadowsocks via its card menu.
    final frankfurtCard = find.ancestor(
        of: find.text('Frankfurt Shadowsocks'), matching: find.byType(Card));
    await tester.tap(find.descendant(
        of: frankfurtCard, matching: find.byType(PopupMenuButton<String>)));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Select server'));
    await tester.pumpAndSettle();

    final container =
        ProviderScope.containerOf(tester.element(find.byType(ServersScreen)));
    expect(container.read(selectedServerProvider),
        '00000000-0000-4000-8000-000000000002');
    expect(
      find.descendant(
          of: frankfurtCard, matching: find.byIcon(Icons.check_circle)),
      findsOneWidget,
    );
  });

  testWidgets('logs tab live-updates while viewing without manual refresh',
      (tester) async {
    final client = MockApiClient(connectDelay: const Duration(milliseconds: 50));
    await tester.pumpWidget(buildApp(client: client));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Logs'));
    await tester.pumpAndSettle();

    final container =
        ProviderScope.containerOf(tester.element(find.byType(LogsScreen)));
    final before = container.read(logsProvider).length;

    // Drive a connect externally while the Logs tab is on screen.
    final connectFuture = container
        .read(connectionProvider.notifier)
        .connect('00000000-0000-4000-8000-000000000002');
    await tester.pump();
    await tester.pump();
    expect(find.textContaining('Connecting to'), findsOneWidget);

    await tester.pump(const Duration(milliseconds: 80));
    await connectFuture;
    await tester.pumpAndSettle();

    expect(find.textContaining('Connected to'), findsOneWidget);
    expect(container.read(logsProvider).length, greaterThan(before));
  });

  testWidgets('logs clear is durable: refresh does not resurrect cleared logs',
      (tester) async {
    final client = MockApiClient(connectDelay: const Duration(milliseconds: 50));
    await tester.pumpWidget(buildApp(client: client));
    await tester.pumpAndSettle();

    // Connect to seed log entries through the mock.
    await tester.tap(find.text('Connect'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 80));
    await tester.pumpAndSettle();
    expect(find.text('Connected'), findsWidgets);

    await tester.tap(find.text('Logs'));
    await tester.pumpAndSettle();

    final container =
        ProviderScope.containerOf(tester.element(find.byType(LogsScreen)));
    expect(container.read(logsProvider), isNotEmpty);
    final clearedCount = container.read(logsProvider).length;

    await tester.tap(find.byIcon(Icons.delete_sweep_outlined));
    await tester.pump();
    expect(container.read(logsProvider), isEmpty);

    // Refresh must not resurrect cleared entries.
    await tester.tap(find.byIcon(Icons.refresh));
    await tester.pumpAndSettle();
    expect(container.read(logsProvider), isEmpty,
        reason: 'cleared logs must not reappear on refresh');

    // New activity after clearing still streams in live.
    final disconnectFuture =
        container.read(connectionProvider.notifier).disconnect();
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 80));
    await disconnectFuture;
    await tester.pumpAndSettle();
    expect(container.read(logsProvider).length, lessThan(clearedCount));
    expect(find.text('Disconnected'), findsWidgets);
  });

  testWidgets('logs strip ANSI escape codes from messages', (tester) async {
    final entry = LogEntry(
      seq: 1,
      timestamp: DateTime.now().toUtc(),
      level: LogLevel.info,
      component: 'vpn',
      message: '\x1b[32mConnected to Tokyo Relay\x1b[0m',
    );
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          logsProvider.overrideWith(() => _StaticLogsNotifier([entry])),
        ],
        child: const MaterialApp(
          home: Scaffold(body: LogsScreen()),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Connected to Tokyo Relay'), findsOneWidget);
    expect(find.textContaining('\x1b['), findsNothing);
  });
}

/// Fixed-view log notifier for tests that want to seed exact entries.
class _StaticLogsNotifier extends LogsNotifier {
  _StaticLogsNotifier(this.entries);

  final List<LogEntry> entries;

  @override
  List<LogEntry> build() => entries;
}
