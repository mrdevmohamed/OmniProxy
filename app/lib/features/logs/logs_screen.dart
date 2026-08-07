import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/models.dart';
import '../../state/providers.dart';

/// Log viewer for the core's redacted, structured log stream (`logAppended`
/// events + `getLogs`). Entries never contain credentials or raw traffic.
class LogsScreen extends ConsumerStatefulWidget {
  const LogsScreen({super.key});

  @override
  ConsumerState<LogsScreen> createState() => _LogsScreenState();
}

class _LogsScreenState extends ConsumerState<LogsScreen> {
  LogLevel? _minLevel;

  @override
  Widget build(BuildContext context) {
    final entries = ref.watch(logsProvider);
    final filtered = _minLevel == null
        ? entries
        : entries
            .where((e) => e.level.rank >= _minLevel!.rank)
            .toList(growable: false);

    return SafeArea(
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 760),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
                child: Row(
                  children: [
                    Text(
                      'Logs',
                      style: Theme.of(context)
                          .textTheme
                          .titleLarge
                          ?.copyWith(fontWeight: FontWeight.w700),
                    ),
                    const Spacer(),
                    DropdownButtonHideUnderline(
                      child: DropdownButton<LogLevel?>(
                        value: _minLevel,
                        hint: const Text('Level'),
                        items: [
                          const DropdownMenuItem<LogLevel?>(
                            value: null,
                            child: Text('All levels'),
                          ),
                          for (final level in LogLevel.values)
                            DropdownMenuItem<LogLevel?>(
                              value: level,
                              child: Text(
                                level.label,
                                style: TextStyle(color: level.color),
                              ),
                            ),
                        ],
                        onChanged: (v) => setState(() => _minLevel = v),
                      ),
                    ),
                    IconButton(
                      tooltip: 'Refresh',
                      onPressed: () =>
                          ref.read(logsProvider.notifier).refresh(),
                      icon: const Icon(Icons.refresh),
                    ),
                    IconButton(
                      tooltip: 'Clear',
                      onPressed: () => ref.read(logsProvider.notifier).clear(),
                      icon: const Icon(Icons.delete_sweep_outlined),
                    ),
                  ],
                ),
              ),
              Expanded(child: _buildBody(context, filtered)),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildBody(BuildContext context, List<LogEntry> entries) {
    if (entries.isEmpty) {
      return const _EmptyLogs();
    }
    // reverse:true keeps the view pinned to the newest entry at the bottom.
    return ListView.separated(
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
      reverse: true,
      itemCount: entries.length,
      separatorBuilder: (_, _) => const Divider(height: 1),
      itemBuilder: (context, index) {
        final entry = entries[entries.length - 1 - index];
        return _LogRow(entry: entry);
      },
    );
  }
}

class _LogRow extends StatelessWidget {
  const _LogRow({required this.entry});

  final LogEntry entry;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final time = entry.timestamp.toLocal();
    final hhmmss = '${time.hour.toString().padLeft(2, '0')}:'
        '${time.minute.toString().padLeft(2, '0')}:'
        '${time.second.toString().padLeft(2, '0')}';
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 56,
            child: Text(
              hhmmss,
              style: theme.textTheme.bodySmall
                  ?.copyWith(color: theme.colorScheme.outline),
            ),
          ),
          SizedBox(
            width: 52,
            child: Text(
              entry.level.label,
              style: theme.textTheme.bodySmall
                  ?.copyWith(color: entry.level.color, fontWeight: FontWeight.w700),
            ),
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  entry.component,
                  style: theme.textTheme.bodySmall
                      ?.copyWith(color: theme.colorScheme.primary),
                ),
                Text(
                  _stripAnsi(entry.message),
                  style: theme.textTheme.bodyMedium
                      ?.copyWith(fontFamily: 'monospace'),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _EmptyLogs extends StatelessWidget {
  const _EmptyLogs();

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.terminal_outlined, size: 48, color: scheme.outline),
          const SizedBox(height: 12),
          Text(
            'No log entries',
            style: Theme.of(context)
                .textTheme
                .titleMedium
                ?.copyWith(fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 4),
          Text(
            'Connect to a server or adjust the filter to see activity.',
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

extension on LogLevel {
  int get rank => switch (this) {
        LogLevel.trace => 0,
        LogLevel.debug => 1,
        LogLevel.info => 2,
        LogLevel.warn => 3,
        LogLevel.error => 4,
      };

  String get label => switch (this) {
        LogLevel.trace => 'TRACE',
        LogLevel.debug => 'DEBUG',
        LogLevel.info => 'INFO',
        LogLevel.warn => 'WARN',
        LogLevel.error => 'ERROR',
      };

  Color get color => switch (this) {
        LogLevel.trace || LogLevel.debug => const Color(0xFF8A93A6),
        LogLevel.info => const Color(0xFF4C9AFF),
        LogLevel.warn => const Color(0xFFE6A23C),
        LogLevel.error => const Color(0xFFF56C6C),
      };
}

/// Removes ANSI/VT escape sequences from a log message so renderers never
/// display control characters. The core strips these at the source; this is a
/// defensive fallback for older buffered entries or third-party sources.
String _stripAnsi(String s) {
  final buffer = StringBuffer();
  var i = 0;
  while (i < s.length) {
    final rune = s.codeUnitAt(i);
    if (rune != 0x1B) {
      buffer.writeCharCode(rune);
      i++;
      continue;
    }
    // ESC found: consume the escape sequence (CSI: ESC [ ... final byte).
    i++;
    if (i >= s.length) break;
    final next = s.codeUnitAt(i);
    if (next == 0x5B) {
      i++;
      while (i < s.length) {
        final c = s.codeUnitAt(i);
        i++;
        if (c >= 0x40 && c <= 0x7E) break;
      }
    } else {
      i++; // two-byte sequence
    }
  }
  return buffer.toString();
}
