import 'dart:async';
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tray_manager/tray_manager.dart';
import 'package:window_manager/window_manager.dart';

import '../app/router.dart';
import '../core/models.dart';
import '../state/providers.dart';

/// Desktop system tray integration built on `tray_manager` (icon + context
/// menu) and `window_manager` (hide-to-tray on close, restore/focus on open).
///
/// Owns the tray lifecycle and keeps the menu synchronized with the live
/// connection state from `connectionProvider`, reusing the app's existing
/// Riverpod state. It is deliberately *not* a widget — it is created once in
/// `main()` with the app's [ProviderContainer].
///
/// Only Windows and Linux are supported; on every other platform [isSupported]
/// is false and [init] is a strict no-op (so nothing tray-related ever runs on
/// Android).
class SystemTrayService with TrayListener, WindowListener {
  SystemTrayService({required this.container, Future<void> Function()? onExit})
    : _onExit = onExit ?? SystemTrayService._defaultExit;

  /// The app-wide Riverpod container; providers are read/watched through it.
  final ProviderContainer container;

  /// Exit hook — injectable so tests can observe a quit without terminating
  /// the test process. Production default is [exit].
  final Future<void> Function() _onExit;

  static const _tooltip = 'OmniProxy';
  static const _iconAssetWindows = 'assets/tray_icon.ico';
  static const _iconAssetLinux = 'assets/tray_icon.png';

  static const _keyOpen = 'open';
  static const _keyToggle = 'toggle';
  static const _keySettings = 'settings';
  static const _keyQuit = 'quit';

  bool _initialized = false;
  bool _quitting = false;
  ProviderSubscription<ConnectionUiState>? _connectionSub;

  static Future<void> _defaultExit() async => exit(0);

  /// Whether this platform has a system tray (desktop only — never Android).
  static bool get isSupported =>
      !kIsWeb && (Platform.isWindows || Platform.isLinux);

  /// Whether the tray has been set up successfully.
  bool get isInitialized => _initialized;

  /// Sets up the tray icon, menu, window close interception and connection
  /// state sync. Idempotent — repeated calls are no-ops.
  ///
  /// Returns `true` when the tray is active. If the tray cannot be created
  /// (e.g. headless Linux without an appindicator host) the app degrades to a
  /// plain windowed app: window close keeps quitting normally and the tray is
  /// left untouched.
  Future<bool> init() async {
    if (_initialized) return true;
    if (!isSupported) return false;

    try {
      await windowManager.ensureInitialized();

      trayManager.addListener(this);
      await trayManager.setIcon(
        Platform.isWindows ? _iconAssetWindows : _iconAssetLinux,
      );
      // `setToolTip` is only implemented on Windows (the Linux plugin returns
      // not-implemented for it).
      if (Platform.isWindows) {
        await trayManager.setToolTip(_tooltip);
      }
      _connectionSub = container.listen<ConnectionUiState>(
        connectionProvider,
        _onConnectionChanged,
      );

      // Mark initialized before syncing the menu so `_syncMenu` applies the
      // current connection state instead of bailing out.
      _initialized = true;
      await _syncMenu();

      // Only intercept window close once the tray is actually available —
      // otherwise hiding on close would strand the user.
      windowManager.addListener(this);
      await windowManager.setPreventClose(true);

      debugPrint('SystemTrayService: initialized');
    } catch (e) {
      trayManager.removeListener(this);
      windowManager.removeListener(this);
      _connectionSub?.close();
      _connectionSub = null;
      debugPrint('SystemTrayService: init failed, tray disabled: $e');
    }
    return _initialized;
  }

  /// Tears down listeners and the tray icon. Safe to call multiple times.
  Future<void> dispose() async {
    _connectionSub?.close();
    _connectionSub = null;
    if (trayManager.hasListeners) trayManager.removeListener(this);
    if (windowManager.hasListeners) windowManager.removeListener(this);
    if (_initialized) {
      await trayManager.destroy();
    }
    _initialized = false;
  }

  /// Restores, shows and focuses the main window ("Open").
  Future<void> openWindow() async {
    await windowManager.restore();
    await windowManager.show();
    await windowManager.focus();
  }

  /// Focuses the window and navigates the shell to the Settings tab.
  Future<void> openSettings() async {
    container
        .read(shellDestinationProvider.notifier)
        .select(ShellDestination.settings);
    await openWindow();
  }

  /// Connects to the current target or disconnects, mirroring the dashboard's
  /// targeting rules (active session wins; otherwise the selected server).
  Future<void> toggleConnection() async {
    final state = container.read(connectionProvider).state;
    switch (state) {
      case ConnectionState.connected:
        await container.read(connectionProvider.notifier).disconnect();
      case ConnectionState.connecting || ConnectionState.reconnecting:
        return; // Busy — the menu item is disabled, but stay safe anyway.
      case ConnectionState.disconnected:
      case ConnectionState.error:
        await _connectToTarget();
    }
  }

  /// Cleanly stops the tunnel, disposes tray resources/listeners and exits.
  /// Idempotent.
  Future<void> quit() async {
    if (_quitting) return;
    _quitting = true;
    debugPrint('SystemTrayService: quitting');

    final notifier = container.read(connectionProvider.notifier);
    try {
      await notifier.disconnect();
    } catch (e) {
      debugPrint('SystemTrayService: disconnect during quit failed: $e');
    }

    await dispose();
    await windowManager.setPreventClose(false);
    await windowManager.destroy();
    await _onExit();
  }

  Future<void> _connectToTarget() async {
    try {
      final connection = container.read(connectionProvider);
      final servers =
          container.read(serversProvider).value ?? const <ServerProfile>[];
      if (servers.isEmpty) {
        // Nothing to connect to — surface the window so the user can add one.
        await openWindow();
        return;
      }
      final active =
          connection.session != null &&
          connection.state != ConnectionState.disconnected &&
          connection.state != ConnectionState.error;
      final target = active
          ? connection.session?.serverId
          : container.read(selectedServerProvider);
      if (target == null) return;
      await container.read(connectionProvider.notifier).connect(target);
    } catch (e) {
      debugPrint('SystemTrayService: connect failed: $e');
    }
  }

  void _onConnectionChanged(
    ConnectionUiState? previous,
    ConnectionUiState next,
  ) {
    unawaited(_syncMenu());
  }

  Future<void> _syncMenu() async {
    if (!_initialized) return;
    final state = container.read(connectionProvider).state;
    final connected = state == ConnectionState.connected;
    final busy =
        state == ConnectionState.connecting ||
        state == ConnectionState.reconnecting;
    await trayManager.setContextMenu(
      Menu(
        items: [
          MenuItem(key: _keyOpen, label: 'Open'),
          MenuItem(
            key: _keyToggle,
            label: connected ? 'Disconnect' : 'Connect',
            disabled: busy,
          ),
          MenuItem(key: _keySettings, label: 'Settings'),
          MenuItem.separator(),
          MenuItem(key: _keyQuit, label: 'Quit'),
        ],
      ),
    );
  }

  // --- TrayListener ---

  @override
  void onTrayMenuItemClick(MenuItem menuItem) {
    switch (menuItem.key) {
      case _keyOpen:
        unawaited(openWindow());
      case _keyToggle:
        unawaited(toggleConnection());
      case _keySettings:
        unawaited(openSettings());
      case _keyQuit:
        unawaited(quit());
    }
  }

  @override
  void onTrayIconMouseDown() {
    // Windows: left-click opens the window. Linux shows the menu via the
    // desktop shell (appindicator) and never delivers mouse events.
    unawaited(openWindow());
  }

  @override
  void onTrayIconRightMouseDown() {
    // Windows: right-click pops the context menu (the shell does not).
    unawaited(trayManager.popUpContextMenu());
  }

  // --- WindowListener ---

  @override
  void onWindowClose() {
    // Closing the window hides to tray instead of quitting.
    unawaited(windowManager.hide());
  }
}
