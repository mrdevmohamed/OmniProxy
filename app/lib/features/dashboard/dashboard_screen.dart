import 'dart:async';

import 'package:flutter/material.dart' hide ConnectionState;
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/models.dart';
import '../../state/providers.dart';

/// Default landing screen. MVP scope: status / current server / live
/// duration / connect–disconnect only (speed/ping/traffic are Phase 2).
class DashboardScreen extends ConsumerWidget {
  const DashboardScreen({super.key, this.onNavigateToServers});

  final VoidCallback? onNavigateToServers;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final connection = ref.watch(connectionProvider);
    final servers = ref.watch(serversProvider);
    final session = connection.session;
    final server = session == null
        ? null
        : servers.value?.where((s) => s.id == session.serverId).firstOrNull;

    return SafeArea(
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 560),
          child: ListView(
            padding: const EdgeInsets.all(24),
            children: [
              _StatusHero(state: connection.state),
              const SizedBox(height: 20),
              if (connection.state == ConnectionState.error)
                _ErrorBanner(connection: connection)
              else if (session != null &&
                  connection.state != ConnectionState.disconnected &&
                  server != null)
                _ServerCard(
                  server: server,
                  mode: session.mode,
                  startedAt: connection.state == ConnectionState.connected
                      ? session.startedAt
                      : null,
                )
              else
                _PlaceholderCard(
                  state: connection.state,
                  serverCount: servers.value?.length ?? 0,
                  onNavigateToServers: onNavigateToServers,
                ),
              const SizedBox(height: 20),
              _ConnectButton(
                state: connection.state,
                targetServerId: server?.id ??
                    _preferredServerId(servers.value),
                onConnect: () => _connect(ref),
                onDisconnect: () => ref.read(connectionProvider.notifier).disconnect(),
              ),
            ],
          ),
        ),
      ),
    );
  }

  String? _preferredServerId(List<ServerProfile>? servers) {
    if (servers == null || servers.isEmpty) return null;
    return servers.firstWhere(
      (s) => s.favorite,
      orElse: () => servers.first,
    ).id;
  }

  Future<void> _connect(WidgetRef ref) async {
    final servers = ref.read(serversProvider).value ?? const [];
    if (servers.isEmpty) return;
    final session = ref.read(connectionProvider).session;
    final target = session?.serverId ??
        servers.firstWhere(
          (s) => s.favorite,
          orElse: () => servers.first,
        ).id;
    await ref.read(connectionProvider.notifier).connect(target);
  }
}

class _StatusHero extends StatelessWidget {
  const _StatusHero({required this.state});

  final ConnectionState state;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final color = switch (state) {
      ConnectionState.connected => Colors.green,
      ConnectionState.connecting || ConnectionState.reconnecting => Colors.amber,
      ConnectionState.error => scheme.error,
      ConnectionState.disconnected => scheme.outline,
    };
    final icon = switch (state) {
      ConnectionState.connected => Icons.verified_user_outlined,
      ConnectionState.connecting || ConnectionState.reconnecting => Icons.sync,
      ConnectionState.error => Icons.error_outline,
      ConnectionState.disconnected => Icons.power_settings_new,
    };
    return Column(
      children: [
        Container(
          width: 96,
          height: 96,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            color: color.withValues(alpha: 0.14),
          ),
          child: Icon(icon, size: 44, color: color),
        ),
        const SizedBox(height: 16),
        Text(
          _label(state),
          style: Theme.of(context)
              .textTheme
              .headlineSmall
              ?.copyWith(fontWeight: FontWeight.w700),
        ),
        const SizedBox(height: 4),
        Text(
          _subtitle(state),
          style: Theme.of(context)
              .textTheme
              .bodyMedium
              ?.copyWith(color: scheme.onSurfaceVariant),
        ),
      ],
    );
  }

  String _label(ConnectionState state) => switch (state) {
        ConnectionState.disconnected => 'Disconnected',
        ConnectionState.connecting => 'Connecting',
        ConnectionState.connected => 'Connected',
        ConnectionState.reconnecting => 'Reconnecting',
        ConnectionState.error => 'Connection error',
      };

  String _subtitle(ConnectionState state) => switch (state) {
        ConnectionState.disconnected => 'Ready to connect',
        ConnectionState.connecting => 'Establishing secure tunnel',
        ConnectionState.connected => 'Your traffic is protected',
        ConnectionState.reconnecting => 'Restoring connection',
        ConnectionState.error => 'The connection could not be established',
      };
}

class _ServerCard extends StatelessWidget {
  const _ServerCard({required this.server, required this.mode, this.startedAt});

  final ServerProfile server;
  final ConnectionMode mode;
  final DateTime? startedAt;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.dns_outlined,
                    color: theme.colorScheme.primary),
                const SizedBox(width: 10),
                Expanded(
                  child: Text(
                    server.name,
                    style: theme.textTheme.titleMedium
                        ?.copyWith(fontWeight: FontWeight.w600),
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
                if (server.favorite)
                  const Icon(Icons.star, size: 18, color: Colors.amber),
              ],
            ),
            const SizedBox(height: 8),
            Text(
              '${server.protocol.wire.toUpperCase()} · ${server.address}:${server.port}',
              style: theme.textTheme.bodySmall
                  ?.copyWith(color: theme.colorScheme.onSurfaceVariant),
            ),
            const SizedBox(height: 12),
            Row(
              children: [
                _InfoChip(
                  icon: Icons.route,
                  label: mode == ConnectionMode.vpn ? 'VPN' : 'Proxy',
                ),
                const SizedBox(width: 8),
                if (server.lastLatencyMs > 0)
                  _InfoChip(
                    icon: Icons.timelapse,
                    label: '${server.lastLatencyMs} ms',
                  ),
                const Spacer(),
                if (startedAt != null) ConnectionDuration(startedAt: startedAt!),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

class _InfoChip extends StatelessWidget {
  const _InfoChip({required this.icon, required this.label});

  final IconData icon;
  final String label;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
        color: scheme.secondaryContainer.withValues(alpha: 0.5),
        borderRadius: BorderRadius.circular(20),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(icon, size: 14, color: scheme.onSecondaryContainer),
          const SizedBox(width: 6),
          Text(
            label,
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w600,
              color: scheme.onSecondaryContainer,
            ),
          ),
        ],
      ),
    );
  }
}

class _PlaceholderCard extends StatelessWidget {
  const _PlaceholderCard({
    required this.state,
    required this.serverCount,
    this.onNavigateToServers,
  });

  final ConnectionState state;
  final int serverCount;
  final VoidCallback? onNavigateToServers;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final scheme = theme.colorScheme;
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Row(
          children: [
            Icon(
              state == ConnectionState.disconnected
                  ? Icons.cloud_off_outlined
                  : Icons.info_outline,
              color: scheme.onSurfaceVariant,
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                serverCount == 0
                    ? 'No servers yet. Add a server to get started.'
                    : 'Choose a server to connect.',
                style: theme.textTheme.bodyMedium,
              ),
            ),
            if (serverCount == 0)
              TextButton(onPressed: onNavigateToServers, child: const Text('Add server')),
          ],
        ),
      ),
    );
  }
}

class _ErrorBanner extends StatelessWidget {
  const _ErrorBanner({required this.connection});

  final ConnectionUiState connection;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final message = connection.session?.error?.message ?? 'Unknown error';
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: scheme.errorContainer.withValues(alpha: 0.6),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Row(
        children: [
          Icon(Icons.error_outline, color: scheme.onErrorContainer),
          const SizedBox(width: 12),
          Expanded(
            child: Text(
              message,
              style: TextStyle(color: scheme.onErrorContainer),
            ),
          ),
        ],
      ),
    );
  }
}

class _ConnectButton extends StatelessWidget {
  const _ConnectButton({
    required this.state,
    required this.targetServerId,
    required this.onConnect,
    required this.onDisconnect,
  });

  final ConnectionState state;
  final String? targetServerId;
  final VoidCallback onConnect;
  final VoidCallback onDisconnect;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    switch (state) {
      case ConnectionState.connected:
        return FilledButton(
          style: FilledButton.styleFrom(backgroundColor: scheme.errorContainer),
          onPressed: onDisconnect,
          child: const Row(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Icon(Icons.power_settings_new),
              SizedBox(width: 8),
              Text('Disconnect'),
            ],
          ),
        );
      case ConnectionState.connecting:
      case ConnectionState.reconnecting:
        return const FilledButton(
          onPressed: null,
          child: Row(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              SizedBox(
                width: 20,
                height: 20,
                child: CircularProgressIndicator(strokeWidth: 2.5),
              ),
              SizedBox(width: 12),
              Text('Connecting…'),
            ],
          ),
        );
      case ConnectionState.error:
        return FilledButton.tonal(
          onPressed: onDisconnect,
          child: const Text('Dismiss'),
        );
      case ConnectionState.disconnected:
        return FilledButton(
          onPressed: targetServerId == null ? null : onConnect,
          child: const Row(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Icon(Icons.power_settings_new),
              SizedBox(width: 8),
              Text('Connect'),
            ],
          ),
        );
    }
  }
}

/// Live HH:MM:SS timer rendered while connected (session.startedAt based).
class ConnectionDuration extends StatefulWidget {
  const ConnectionDuration({super.key, required this.startedAt});

  final DateTime startedAt;

  @override
  State<ConnectionDuration> createState() => _ConnectionDurationState();
}

class _ConnectionDurationState extends State<ConnectionDuration> {
  Timer? _timer;

  @override
  void initState() {
    super.initState();
    _timer = Timer.periodic(const Duration(seconds: 1), (_) {
      if (mounted) setState(() {});
    });
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final elapsed = DateTime.now().toUtc().difference(widget.startedAt);
    final text = _format(elapsed);
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(Icons.timer_outlined,
            size: 16, color: Theme.of(context).colorScheme.onSurfaceVariant),
        const SizedBox(width: 6),
        Text(
          text,
          style: TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w600,
            color: Theme.of(context).colorScheme.onSurfaceVariant,
          ),
        ),
      ],
    );
  }

  String _format(Duration d) {
    String two(int v) => v.toString().padLeft(2, '0');
    final h = d.inHours;
    final m = d.inMinutes % 60;
    final s = d.inSeconds % 60;
    return h > 0 ? '${two(h)}:${two(m)}:${two(s)}' : '${two(m)}:${two(s)}';
  }
}
