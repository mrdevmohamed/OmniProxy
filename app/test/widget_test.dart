import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:omniproxy/app/app_root.dart';
import 'package:omniproxy/core/models.dart';
import 'package:omniproxy/core/mock_api_client.dart';
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
}
