import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/models.dart';
import '../../state/providers.dart';
import 'server_edit_screen.dart';

class ServersScreen extends ConsumerStatefulWidget {
  const ServersScreen({super.key});

  @override
  ConsumerState<ServersScreen> createState() => _ServersScreenState();
}

class _ServersScreenState extends ConsumerState<ServersScreen> {
  final _searchController = TextEditingController();
  ServerProtocol? _protocolFilter;
  String? _groupFilter;
  bool? _enabledFilter;
  ServerSort _sort = ServerSort.name;

  @override
  void dispose() {
    _searchController.dispose();
    super.dispose();
  }

  void _applyQuery() {
    ref.read(serversProvider.notifier).setQuery(ServerListQuery(
          search: _searchController.text,
          protocol: _protocolFilter,
          group: _groupFilter,
          enabled: _enabledFilter,
          sort: _sort,
        ));
  }

  @override
  Widget build(BuildContext context) {
    final servers = ref.watch(serversProvider);
    return SafeArea(
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 720),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(16, 16, 16, 0),
                child: Row(
                  children: [
                    Text(
                      'Servers',
                      style: Theme.of(context)
                          .textTheme
                          .titleLarge
                          ?.copyWith(fontWeight: FontWeight.w700),
                    ),
                    const Spacer(),
                    FilledButton.tonalIcon(
                      onPressed: () => _openEditor(),
                      icon: const Icon(Icons.add),
                      label: const Text('Add server'),
                    ),
                    IconButton(
                      tooltip: 'Import',
                      onPressed: () => _showImportDialog(),
                      icon: const Icon(Icons.file_download_outlined),
                    ),
                    IconButton(
                      tooltip: 'Export all',
                      onPressed: servers.value?.isNotEmpty ?? false
                          ? () => _export(ids: null)
                          : null,
                      icon: const Icon(Icons.upload_outlined),
                    ),
                  ],
                ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
                child: Row(
                  children: [
                    Expanded(
                      child: TextField(
                        controller: _searchController,
                        onChanged: (_) => _applyQuery(),
                        decoration: InputDecoration(
                          hintText: 'Search servers…',
                          prefixIcon: const Icon(Icons.search, size: 20),
                          isDense: true,
                          contentPadding: const EdgeInsets.symmetric(
                              horizontal: 12, vertical: 10),
                          border: OutlineInputBorder(
                              borderRadius: BorderRadius.circular(10)),
                        ),
                      ),
                    ),
                    const SizedBox(width: 8),
                    PopupMenuButton<ServerSort>(
                      tooltip: 'Sort by',
                      onSelected: (sort) {
                        setState(() => _sort = sort);
                        _applyQuery();
                      },
                      icon: const Icon(Icons.sort),
                      itemBuilder: (context) => [
                        for (final sort in ServerSort.values)
                          PopupMenuItem(
                            value: sort,
                            child: Text(_sortLabel(sort)),
                          ),
                      ],
                    ),
                    PopupMenuButton<String>(
                      tooltip: 'Filter',
                      onSelected: _onFilterSelected,
                      icon: const Icon(Icons.filter_list),
                      itemBuilder: (context) => [
                        const PopupMenuItem(
                          value: 'protocol',
                          child: Text('By protocol'),
                        ),
                        const PopupMenuItem(
                          value: 'group',
                          child: Text('By group'),
                        ),
                        const PopupMenuItem(
                          value: 'enabled',
                          child: Text('By enabled'),
                        ),
                        if (_protocolFilter != null ||
                            _groupFilter != null ||
                            _enabledFilter != null)
                          const PopupMenuDivider(),
                        if (_protocolFilter != null ||
                            _groupFilter != null ||
                            _enabledFilter != null)
                          const PopupMenuItem(
                            value: 'clear',
                            child: Text('Clear filters'),
                          ),
                      ],
                    ),
                  ],
                ),
              ),
              Expanded(child: _buildBody(context, servers)),
            ],
          ),
        ),
      ),
    );
  }

  String _sortLabel(ServerSort sort) => switch (sort) {
        ServerSort.name => 'Name',
        ServerSort.updatedAt => 'Last updated',
        ServerSort.latency => 'Latency',
      };

  void _onFilterSelected(String value) async {
    switch (value) {
      case 'protocol':
        final selected = await showDialog<ServerProtocol>(
          context: context,
          builder: (_) => _FilterDialog<ServerProtocol>(
            title: 'Protocol',
            options: ServerProtocol.values,
            selected: _protocolFilter,
            label: (p) => p.wire,
          ),
        );
        if (selected != null) {
          setState(() => _protocolFilter = selected);
          _applyQuery();
        }
      case 'group':
        final groups = ref
                .read(serversProvider).value
                ?.map((s) => s.group)
                .whereType<String>()
                .toSet()
                .toList()
              ??
              [];
        if (groups.isEmpty) {
          if (!mounted) return;
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(content: Text('No groups found')),
          );
          return;
        }
        groups.sort();
        final selected = await showDialog<String>(
          context: context,
          builder: (_) => _FilterDialog<String>(
            title: 'Group',
            options: groups,
            selected: _groupFilter,
            label: (g) => g,
          ),
        );
        if (selected != null) {
          setState(() => _groupFilter = selected);
          _applyQuery();
        }
      case 'enabled':
        final selected = await showDialog<bool>(
          context: context,
          builder: (_) => _FilterDialog<bool>(
            title: 'Enabled',
            options: const [true, false],
            selected: _enabledFilter,
            label: (b) => b ? 'Enabled' : 'Disabled',
          ),
        );
        if (selected != null) {
          setState(() => _enabledFilter = selected);
          _applyQuery();
        }
      case 'clear':
        setState(() {
          _protocolFilter = null;
          _groupFilter = null;
          _enabledFilter = null;
        });
        _applyQuery();
    }
  }

  Widget _buildBody(
    BuildContext context,
    AsyncValue<List<ServerProfile>> servers,
  ) {
    return servers.when(
      loading: () => const Center(child: CircularProgressIndicator()),
      error: (error, stack) => Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.error_outline, size: 40),
              const SizedBox(height: 12),
              Text('Could not load servers: $error'),
              const SizedBox(height: 12),
              FilledButton.tonal(
                onPressed: () =>
                    ref.read(serversProvider.notifier).refresh(),
                child: const Text('Retry'),
              ),
            ],
          ),
        ),
      ),
      data: (list) {
        if (list.isEmpty) return const _EmptyState();
        return ListView.separated(
          padding: const EdgeInsets.fromLTRB(24, 8, 24, 24),
          itemCount: list.length,
          separatorBuilder: (_, _) => const SizedBox(height: 10),
          itemBuilder: (context, index) {
            final server = list[index];
            return _ServerCard(
              server: server,
              onTap: () => _openEditor(server: server),
              onTestLatency: () => _testLatency(server),
              onToggleFavorite: () =>
                  ref.read(serversProvider.notifier).toggleFavorite(server.id),
              onToggleEnabled: () =>
                  ref.read(serversProvider.notifier).toggleEnabled(server.id),
              onDuplicate: () => _duplicate(server),
              onExport: () => _export(ids: [server.id]),
              onDelete: () => _confirmDelete(server),
            );
          },
        );
      },
    );
  }

  void _openEditor({ServerProfile? server}) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => ServerEditScreen(server: server),
      ),
    );
  }

  Future<void> _testLatency(ServerProfile server) async {
    try {
      final ms =
          await ref.read(serversProvider.notifier).testLatency(server.id);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('${server.name}: $ms ms')),
      );
    } on ApiError catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _duplicate(ServerProfile server) async {
    try {
      await ref.read(serversProvider.notifier).duplicate(server.id);
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Duplicated "${server.name}"')),
      );
    } on ApiError catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _export({List<String>? ids}) async {
    try {
      final blob =
          await ref.read(serversProvider.notifier).export(ids: ids);
      if (!mounted) return;
      showDialog<void>(
        context: context,
        builder: (_) => _ExportDialog(blob: blob),
      );
    } on ApiError catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _confirmDelete(ServerProfile server) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text('Delete "${server.name}"?'),
        content: const Text('This cannot be undone.'),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: const Text('Delete'),
          ),
        ],
      ),
    );
    if (confirmed != true) return;
    try {
      await ref.read(serversProvider.notifier).delete(server.id);
    } on ApiError catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  Future<void> _showImportDialog() async {
    final source = await showDialog<String>(
      context: context,
      builder: (context) => _ImportDialog(),
    );
    if (source == null || !mounted) return;
    final kind =
        source.trim().startsWith(RegExp(r'https?://')) ? 'link' : 'clipboard';
    try {
      final result = await ref
          .read(serversProvider.notifier)
          .import(ImportSource(kind: kind, data: source));
      if (!mounted) return;
      if (result.failed == 0) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Imported ${result.added} server(s)')),
        );
      } else {
        showDialog<void>(
          context: context,
          builder: (_) => _ImportResultDialog(result: result),
        );
      }
    } on ApiError catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(e.message)));
    }
  }
}

class _FilterDialog<T> extends StatefulWidget {
  const _FilterDialog({
    required this.title,
    required this.options,
    required this.selected,
    required this.label,
  });

  final String title;
  final List<T> options;
  final T? selected;
  final String Function(T) label;

  @override
  State<_FilterDialog<T>> createState() => _FilterDialogState<T>();
}

class _FilterDialogState<T> extends State<_FilterDialog<T>> {
  late T? _selected = widget.selected;

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: Text(widget.title),
      content: RadioGroup<T>(
        groupValue: _selected,
        onChanged: (v) => setState(() => _selected = v),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            for (final option in widget.options)
              RadioListTile<T>(
                title: Text(widget.label(option)),
                value: option,
              ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('Cancel'),
        ),
        FilledButton(
          onPressed: () => Navigator.of(context).pop(_selected),
          child: const Text('Apply'),
        ),
      ],
    );
  }
}

class _EmptyState extends StatelessWidget {
  const _EmptyState();

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.dns_outlined, size: 48, color: scheme.outline),
          const SizedBox(height: 12),
          Text(
            'No servers yet',
            style: Theme.of(context)
                .textTheme
                .titleMedium
                ?.copyWith(fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 4),
          Text(
            'Add a server manually or import a configuration.',
            style: Theme.of(context)
                .textTheme
                .bodySmall
                ?.copyWith(color: scheme.onSurfaceVariant),
          ),
        ],
      ),
    );
  }
}

class _ServerCard extends StatelessWidget {
  const _ServerCard({
    required this.server,
    required this.onTap,
    required this.onTestLatency,
    required this.onToggleFavorite,
    required this.onToggleEnabled,
    required this.onDuplicate,
    required this.onExport,
    required this.onDelete,
  });

  final ServerProfile server;
  final VoidCallback onTap;
  final VoidCallback onTestLatency;
  final VoidCallback onToggleFavorite;
  final VoidCallback onToggleEnabled;
  final VoidCallback onDuplicate;
  final VoidCallback onExport;
  final VoidCallback onDelete;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final scheme = theme.colorScheme;
    return Opacity(
      opacity: server.enabled ? 1.0 : 0.55,
      child: Card(
        child: InkWell(
          borderRadius: BorderRadius.circular(16),
          onTap: onTap,
          child: Padding(
            padding: const EdgeInsets.fromLTRB(16, 12, 8, 12),
            child: Row(
              children: [
                Container(
                  width: 40,
                  height: 40,
                  decoration: BoxDecoration(
                    color: scheme.primaryContainer,
                    borderRadius: BorderRadius.circular(10),
                  ),
                  alignment: Alignment.center,
                  child: Text(
                    _abbreviation(server.protocol),
                    style: TextStyle(
                      fontSize: 10,
                      fontWeight: FontWeight.w700,
                      color: scheme.onPrimaryContainer,
                    ),
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(
                        children: [
                          Flexible(
                            child: Text(
                              server.name,
                              style: theme.textTheme.titleMedium
                                  ?.copyWith(fontWeight: FontWeight.w600),
                              overflow: TextOverflow.ellipsis,
                            ),
                          ),
                          if (server.favorite) ...[
                            const SizedBox(width: 6),
                            const Icon(Icons.star,
                                size: 14, color: Colors.amber),
                          ],
                        ],
                      ),
                      const SizedBox(height: 2),
                      Text(
                        '${server.protocol.wire} · ${server.address}:${server.port}'
                        '${_transportLabel(server)}',
                        style: theme.textTheme.bodySmall
                            ?.copyWith(color: scheme.onSurfaceVariant),
                        overflow: TextOverflow.ellipsis,
                      ),
                    ],
                  ),
                ),
                const SizedBox(width: 8),
                Container(
                  padding:
                      const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
                  decoration: BoxDecoration(
                    color: server.lastLatencyMs > 0
                        ? scheme.secondaryContainer.withValues(alpha: 0.6)
                        : scheme.surfaceContainerHighest,
                    borderRadius: BorderRadius.circular(20),
                  ),
                  child: Text(
                    server.lastLatencyMs > 0
                        ? '${server.lastLatencyMs} ms'
                        : '—',
                    style: TextStyle(
                      fontSize: 12,
                      fontWeight: FontWeight.w600,
                      color: scheme.onSurfaceVariant,
                    ),
                  ),
                ),
                PopupMenuButton<String>(
                  tooltip: 'Server actions',
                  onSelected: (action) => switch (action) {
                    'test' => onTestLatency(),
                    'favorite' => onToggleFavorite(),
                    'enable' => onToggleEnabled(),
                    'duplicate' => onDuplicate(),
                    'export' => onExport(),
                    'delete' => onDelete(),
                    _ => null,
                  },
                  itemBuilder: (context) => [
                    const PopupMenuItem(
                      value: 'test',
                      child: ListTile(
                        leading: Icon(Icons.timelapse),
                        title: Text('Test latency'),
                        contentPadding: EdgeInsets.zero,
                      ),
                    ),
                    PopupMenuItem(
                      value: 'favorite',
                      child: ListTile(
                        leading: Icon(server.favorite
                            ? Icons.star_outline
                            : Icons.star),
                        title: Text(server.favorite
                            ? 'Remove favorite'
                            : 'Mark favorite'),
                        contentPadding: EdgeInsets.zero,
                      ),
                    ),
                    PopupMenuItem(
                      value: 'enable',
                      child: ListTile(
                        leading: Icon(server.enabled
                            ? Icons.toggle_off
                            : Icons.toggle_on),
                        title: Text(
                            server.enabled ? 'Disable server' : 'Enable server'),
                        contentPadding: EdgeInsets.zero,
                      ),
                    ),
                    const PopupMenuItem(
                      value: 'duplicate',
                      child: ListTile(
                        leading: Icon(Icons.copy),
                        title: Text('Duplicate'),
                        contentPadding: EdgeInsets.zero,
                      ),
                    ),
                    const PopupMenuItem(
                      value: 'export',
                      child: ListTile(
                        leading: Icon(Icons.upload_outlined),
                        title: Text('Export'),
                        contentPadding: EdgeInsets.zero,
                      ),
                    ),
                    const PopupMenuItem(
                      value: 'delete',
                      child: ListTile(
                        leading: Icon(Icons.delete_outline),
                        title: Text('Delete'),
                        contentPadding: EdgeInsets.zero,
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  String _abbreviation(ServerProtocol protocol) => switch (protocol) {
        ServerProtocol.vless => 'VL',
        ServerProtocol.vmess => 'VM',
        ServerProtocol.shadowsocks => 'SS',
        ServerProtocol.trojan => 'TR',
        ServerProtocol.socks5 => 'S5',
        ServerProtocol.http => 'HTTP',
        ServerProtocol.ssh => 'SSH',
      };

  String _transportLabel(ServerProfile server) {
    final t = server.transport;
    if (t == null) return '';
    return switch (t.type) {
      TransportType.ws => ' · ws',
      TransportType.tcp => '',
    };
  }
}

class _ImportDialog extends StatefulWidget {
  @override
  State<_ImportDialog> createState() => _ImportDialogState();
}

class _ImportDialogState extends State<_ImportDialog> {
  final _controller = TextEditingController();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('Import servers'),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'Paste an exported configuration or one or more share links '
            '(vmess://, vless://, ss://, trojan://).',
            style: TextStyle(fontSize: 13),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _controller,
            maxLines: 6,
            minLines: 3,
            decoration: const InputDecoration(
              hintText: 'Paste links or .onnproxy configuration…',
              alignLabelWithHint: true,
            ),
          ),
        ],
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('Cancel'),
        ),
        FilledButton(
          onPressed: () => Navigator.of(context).pop(_controller.text),
          child: const Text('Import'),
        ),
      ],
    );
  }
}

class _ExportDialog extends StatelessWidget {
  const _ExportDialog({required this.blob});

  final String blob;

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('Exported configuration'),
      content: SizedBox(
        width: 420,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Text(
              'Share this configuration on another device. Credentials are '
              'included — share it securely.',
              style: TextStyle(fontSize: 13),
            ),
            const SizedBox(height: 12),
            Container(
              width: double.infinity,
              padding: const EdgeInsets.all(10),
              decoration: BoxDecoration(
                color: Theme.of(context).colorScheme.surfaceContainerHighest,
                borderRadius: BorderRadius.circular(8),
              ),
              child: SelectableText(
                blob,
                maxLines: 8,
                style: const TextStyle(fontSize: 12, fontFamily: 'monospace'),
              ),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('Close'),
        ),
        FilledButton.icon(
          onPressed: () {
            Clipboard.setData(ClipboardData(text: blob));
            ScaffoldMessenger.of(context).showSnackBar(
              const SnackBar(content: Text('Copied to clipboard')),
            );
            Navigator.of(context).pop();
          },
          icon: const Icon(Icons.copy),
          label: const Text('Copy'),
        ),
      ],
    );
  }
}

class _ImportResultDialog extends StatelessWidget {
  const _ImportResultDialog({required this.result});

  final ImportResult result;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return AlertDialog(
      title: Text(
        result.added > 0
            ? 'Imported ${result.added} server(s)'
            : 'Nothing imported',
      ),
      content: SizedBox(
        width: 420,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              result.failed == 1
                  ? '1 item failed to import'
                  : '${result.failed} items failed to import',
              style: TextStyle(
                color: scheme.error,
                fontWeight: FontWeight.w600,
              ),
            ),
            const SizedBox(height: 12),
            Flexible(
              child: SingleChildScrollView(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    for (final error in result.errors)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 6),
                        child: Text(
                          error.message,
                          style: const TextStyle(fontSize: 12),
                        ),
                      ),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
      actions: [
        FilledButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('Close'),
        ),
      ],
    );
  }
}
