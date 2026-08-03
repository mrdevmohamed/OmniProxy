import 'package:flutter/material.dart' hide ConnectionState;
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../core/models.dart';
import '../features/dashboard/dashboard_screen.dart';
import '../features/servers/servers_screen.dart';
import '../features/settings/settings_screen.dart';
import '../state/providers.dart';
import 'router.dart';
import 'theme.dart';

/// Application root: resolves the theme preference and mounts the shell.
class OmniProxyApp extends ConsumerWidget {
  const OmniProxyApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final settings = ref.watch(settingsProvider);
    return MaterialApp(
      title: 'OmniProxy',
      debugShowCheckedModeBanner: false,
      theme: AppTheme.light(),
      darkTheme: AppTheme.dark(),
      themeMode: AppTheme.modeFor(settings.theme),
      home: const HomeShell(),
    );
  }
}

/// Responsive shell: NavigationRail on wide (desktop) layouts, NavigationBar
/// bottom navigation on narrow (mobile) layouts. Dashboard is the landing tab.
class HomeShell extends ConsumerStatefulWidget {
  const HomeShell({super.key});

  @override
  ConsumerState<HomeShell> createState() => _HomeShellState();
}

class _HomeShellState extends ConsumerState<HomeShell> {
  int _index = ShellDestination.dashboard.index;

  void _select(int index) => setState(() => _index = index);

  @override
  Widget build(BuildContext context) {
    final destination = ShellDestination.values[_index];
    final screens = <Widget>[
      DashboardScreen(onNavigateToServers: _selectServers),
      const ServersScreen(),
      const SettingsScreen(),
    ];
    final appBar = AppBar(
      title: Text(
        destination.label,
        style: const TextStyle(fontWeight: FontWeight.w600),
      ),
      centerTitle: false,
      actions: [
        Padding(
          padding: const EdgeInsets.only(right: 16),
          child: Center(child: _ConnectionStatusChip()),
        ),
      ],
    );

    return LayoutBuilder(builder: (context, constraints) {
      final wide = constraints.maxWidth >= 700;
      if (wide) {
        return Scaffold(
          appBar: appBar,
          body: Row(
            children: [
              NavigationRail(
                selectedIndex: _index,
                onDestinationSelected: _select,
                labelType: NavigationRailLabelType.all,
                leading: const Padding(
                  padding: EdgeInsets.symmetric(vertical: 8),
                  child: _Logo(),
                ),
                destinations: [
                  for (final d in ShellDestination.values)
                    NavigationRailDestination(
                      icon: Icon(d.icon),
                      selectedIcon: Icon(d.selectedIcon),
                      label: Text(d.label),
                    ),
                ],
              ),
              const VerticalDivider(width: 1, thickness: 1),
              Expanded(child: screens[_index]),
            ],
          ),
        );
      }
      return Scaffold(
        appBar: appBar,
        body: screens[_index],
        bottomNavigationBar: NavigationBar(
          selectedIndex: _index,
          onDestinationSelected: _select,
          destinations: [
            for (final d in ShellDestination.values)
              NavigationDestination(
                icon: Icon(d.icon),
                selectedIcon: Icon(d.selectedIcon),
                label: d.label,
              ),
          ],
        ),
      );
    });
  }

  void _selectServers() => _select(ShellDestination.servers.index);
}

class _ConnectionStatusChip extends ConsumerWidget {
  const _ConnectionStatusChip();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(connectionProvider).state;
    final color = switch (state) {
      ConnectionState.connected => Colors.green,
      ConnectionState.connecting || ConnectionState.reconnecting => Colors.amber,
      ConnectionState.error => Theme.of(context).colorScheme.error,
      ConnectionState.disconnected => Colors.grey,
    };
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.14),
        borderRadius: BorderRadius.circular(20),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.circle, size: 10, color: color),
          const SizedBox(width: 8),
          Text(
            _label(state),
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w600,
              color: color,
            ),
          ),
        ],
      ),
    );
  }

  String _label(ConnectionState state) => switch (state) {
        ConnectionState.disconnected => 'Disconnected',
        ConnectionState.connecting => 'Connecting',
        ConnectionState.connected => 'Connected',
        ConnectionState.reconnecting => 'Reconnecting',
        ConnectionState.error => 'Error',
      };
}

class _Logo extends StatelessWidget {
  const _Logo();

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Container(
      width: 40,
      height: 40,
      decoration: BoxDecoration(
        color: scheme.primary,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Icon(Icons.shield_outlined, color: scheme.onPrimary, size: 24),
    );
  }
}
