import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'app/app_root.dart';
import 'services/system_tray_service.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();

  // Own the provider container so the tray service can read/watch the app's
  // Riverpod state (connection status, server selection, navigation).
  final container = ProviderContainer();
  final systemTray = SystemTrayService(container: container);
  // No-op on Android and other unsupported platforms; degrades gracefully on
  // desktops without a tray host.
  await systemTray.init();

  runApp(
    UncontrolledProviderScope(
      container: container,
      child: const OmniProxyApp(),
    ),
  );
}
