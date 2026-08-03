import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/models.dart';
import '../../state/providers.dart';

class SettingsScreen extends ConsumerWidget {
  const SettingsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final settings = ref.watch(settingsProvider);
    final version = ref.watch(versionProvider);
    final notifier = ref.read(settingsProvider.notifier);

    return SafeArea(
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 720),
          child: ListView(
            padding: const EdgeInsets.all(24),
            children: [
              _Section(
                title: 'Appearance',
                child: Column(
                  children: [
                    _SettingHeader(
                      icon: Icons.palette_outlined,
                      title: 'Theme',
                      subtitle: 'Follow the system or force light/dark.',
                    ),
                    const SizedBox(height: 12),
                    SegmentedButton<ThemePreference>(
                      segments: const [
                        ButtonSegment(
                          value: ThemePreference.system,
                          label: Text('System'),
                          icon: Icon(Icons.brightness_auto),
                        ),
                        ButtonSegment(
                          value: ThemePreference.light,
                          label: Text('Light'),
                          icon: Icon(Icons.light_mode_outlined),
                        ),
                        ButtonSegment(
                          value: ThemePreference.dark,
                          label: Text('Dark'),
                          icon: Icon(Icons.dark_mode_outlined),
                        ),
                      ],
                      selected: {settings.theme},
                      onSelectionChanged: (selection) =>
                          notifier.update(settings.copyWith(theme: selection.first)),
                    ),
                  ],
                ),
              ),
              const SizedBox(height: 16),
              _Section(
                title: 'Connection',
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    _SettingHeader(
                      icon: Icons.route_outlined,
                      title: 'Default mode',
                      subtitle:
                          'VPN routes all traffic through a virtual interface. '
                          'Proxy serves a local SOCKS5/HTTP proxy for specific apps.',
                    ),
                    const SizedBox(height: 12),
                    SegmentedButton<ConnectionMode>(
                      segments: const [
                        ButtonSegment(
                          value: ConnectionMode.vpn,
                          label: Text('VPN'),
                          icon: Icon(Icons.vpn_lock_outlined),
                        ),
                        ButtonSegment(
                          value: ConnectionMode.proxy,
                          label: Text('Proxy'),
                          icon: Icon(Icons.hub_outlined),
                        ),
                      ],
                      selected: {settings.connectionMode},
                      onSelectionChanged: (selection) => notifier.update(
                          settings.copyWith(connectionMode: selection.first)),
                    ),
                  ],
                ),
              ),
              const SizedBox(height: 16),
              _Section(
                title: 'About',
                child: version.when(
                  loading: () => const Padding(
                    padding: EdgeInsets.all(8),
                    child: LinearProgressIndicator(),
                  ),
                  error: (_, _) => const Padding(
                    padding: EdgeInsets.all(8),
                    child: Text('Version unavailable'),
                  ),
                  data: (v) => Column(
                    children: [
                      ListTile(
                        leading: const Icon(Icons.info_outline),
                        title: const Text('App version'),
                        trailing: Text(v.version),
                        contentPadding: EdgeInsets.zero,
                      ),
                      ListTile(
                        leading: const Icon(Icons.memory),
                        title: const Text('Engine'),
                        trailing: Text(v.engineVersion),
                        contentPadding: EdgeInsets.zero,
                      ),
                      ListTile(
                        leading: const Icon(Icons.desktop_windows_outlined),
                        title: const Text('Platform'),
                        trailing: Text(v.platform),
                        contentPadding: EdgeInsets.zero,
                      ),
                    ],
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _Section extends StatelessWidget {
  const _Section({required this.title, required this.child});

  final String title;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              title,
              style: Theme.of(context)
                  .textTheme
                  .titleSmall
                  ?.copyWith(fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 12),
            child,
          ],
        ),
      ),
    );
  }
}

class _SettingHeader extends StatelessWidget {
  const _SettingHeader({
    required this.icon,
    required this.title,
    required this.subtitle,
  });

  final IconData icon;
  final String title;
  final String subtitle;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Icon(icon, color: theme.colorScheme.primary),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(title,
                  style: theme.textTheme.titleMedium
                      ?.copyWith(fontWeight: FontWeight.w600)),
              const SizedBox(height: 2),
              Text(
                subtitle,
                style: theme.textTheme.bodySmall
                    ?.copyWith(color: theme.colorScheme.onSurfaceVariant),
              ),
            ],
          ),
        ),
      ],
    );
  }
}
