import 'package:flutter/material.dart';

/// Top-level destinations for the responsive shell (mobile bottom nav /
/// desktop nav rail). Dashboard is the default landing screen; the shell
/// builds the matching screen widget per destination.
enum ShellDestination {
  dashboard(0, 'Dashboard', Icons.dashboard_outlined, Icons.dashboard),
  servers(1, 'Servers', Icons.dns_outlined, Icons.dns),
  logs(2, 'Logs', Icons.terminal_outlined, Icons.terminal),
  settings(3, 'Settings', Icons.settings_outlined, Icons.settings);

  const ShellDestination(
    this.tab,
    this.label,
    this.icon,
    this.selectedIcon,
  );

  final int tab;
  final String label;
  final IconData icon;
  final IconData selectedIcon;
}
