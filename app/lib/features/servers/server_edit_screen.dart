import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/models.dart';
import '../../state/providers.dart';

/// Add (server == null) or edit a server. Credential fields adapt to the
/// selected protocol; TLS applies to encrypted protocols.
class ServerEditScreen extends ConsumerStatefulWidget {
  const ServerEditScreen({super.key, this.server});

  final ServerProfile? server;

  @override
  ConsumerState<ServerEditScreen> createState() => _ServerEditScreenState();
}

class _ServerEditScreenState extends ConsumerState<ServerEditScreen> {
  final _formKey = GlobalKey<FormState>();
  late final TextEditingController _name;
  late final TextEditingController _address;
  late final TextEditingController _port;
  late final TextEditingController _username;
  late final TextEditingController _password;
  late final TextEditingController _cipher;
  late final TextEditingController _uuid;
  late final TextEditingController _sshUser;
  late final TextEditingController _sshKey;
  late final TextEditingController _serverName;

  late ServerProtocol _protocol;
  late bool _tlsEnabled;
  late bool _tlsAllowInsecure;
  late bool _favorite;
  bool _saving = false;

  bool get _isEdit => widget.server != null;

  @override
  void initState() {
    super.initState();
    final s = widget.server;
    _name = TextEditingController(text: s?.name ?? '');
    _address = TextEditingController(text: s?.address ?? '');
    _port = TextEditingController(text: s?.port.toString() ?? '');
    _username = TextEditingController(text: s?.username ?? '');
    _password = TextEditingController(text: s?.password ?? '');
    _cipher = TextEditingController(text: s?.cipher ?? 'aes-128-gcm');
    _uuid = TextEditingController(text: s?.uuid ?? '');
    _sshUser = TextEditingController(text: s?.ssh.user ?? '');
    _sshKey = TextEditingController(text: s?.ssh.privateKey ?? '');
    _serverName = TextEditingController(text: s?.tls.serverName ?? '');
    _protocol = s?.protocol ?? ServerProtocol.vless;
    _tlsEnabled = s?.tls.enabled ?? false;
    _tlsAllowInsecure = s?.tls.allowInsecure ?? false;
    _favorite = s?.favorite ?? false;
  }

  @override
  void dispose() {
    _name.dispose();
    _address.dispose();
    _port.dispose();
    _username.dispose();
    _password.dispose();
    _cipher.dispose();
    _uuid.dispose();
    _sshUser.dispose();
    _sshKey.dispose();
    _serverName.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text(_isEdit ? 'Edit server' : 'Add server'),
      ),
      body: SafeArea(
        child: Form(
          key: _formKey,
          child: ListView(
            padding: const EdgeInsets.all(24),
            children: [
              TextFormField(
                controller: _name,
                textInputAction: TextInputAction.next,
                decoration: const InputDecoration(labelText: 'Name'),
                validator: (v) =>
                    (v == null || v.trim().isEmpty) ? 'Name is required' : null,
              ),
              const SizedBox(height: 16),
              DropdownButtonFormField<ServerProtocol>(
                initialValue: _protocol,
                decoration: const InputDecoration(labelText: 'Protocol'),
                items: [
                  for (final p in ServerProtocol.values)
                    DropdownMenuItem(value: p, child: Text(_protocolLabel(p))),
                ],
                onChanged: (p) => setState(() => _protocol = p ?? _protocol),
              ),
              const SizedBox(height: 16),
              Row(
                children: [
                  Expanded(
                    flex: 3,
                    child: TextFormField(
                      controller: _address,
                      textInputAction: TextInputAction.next,
                      decoration: const InputDecoration(labelText: 'Address'),
                      validator: (v) => (v == null || v.trim().isEmpty)
                          ? 'Address is required'
                          : null,
                    ),
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    flex: 1,
                    child: TextFormField(
                      controller: _port,
                      keyboardType: TextInputType.number,
                      decoration: const InputDecoration(labelText: 'Port'),
                      validator: (v) {
                        final port = int.tryParse(v ?? '');
                        if (port == null || port < 1 || port > 65535) {
                          return '1–65535';
                        }
                        return null;
                      },
                    ),
                  ),
                ],
              ),
              ..._credentialFields(),
              const SizedBox(height: 8),
              _TlsSection(
                enabled: _tlsEnabled,
                allowInsecure: _tlsAllowInsecure,
                serverName: _serverName,
                onEnabled: (v) => setState(() => _tlsEnabled = v),
                onAllowInsecure: (v) => setState(() => _tlsAllowInsecure = v),
              ),
              const SizedBox(height: 8),
              SwitchListTile(
                value: _favorite,
                onChanged: (v) => setState(() => _favorite = v),
                title: const Text('Favorite'),
                subtitle: const Text('Pin to the top of the server list'),
                contentPadding: EdgeInsets.zero,
              ),
              const SizedBox(height: 16),
              FilledButton(
                onPressed: _saving ? null : _save,
                child: _saving
                    ? const SizedBox(
                        width: 20,
                        height: 20,
                        child: CircularProgressIndicator(strokeWidth: 2.5),
                      )
                    : Text(_isEdit ? 'Save changes' : 'Add server'),
              ),
            ],
          ),
        ),
      ),
    );
  }

  List<Widget> _credentialFields() {
    final usesUsername = _protocol == ServerProtocol.socks5 ||
        _protocol == ServerProtocol.http ||
        _protocol == ServerProtocol.ssh;
    final usesPassword = _protocol == ServerProtocol.socks5 ||
        _protocol == ServerProtocol.http ||
        _protocol == ServerProtocol.shadowsocks;
    final usesCipher = _protocol == ServerProtocol.shadowsocks;
    final usesUuid = _protocol == ServerProtocol.vless ||
        _protocol == ServerProtocol.vmess;
    final usesSsh = _protocol == ServerProtocol.ssh;

    final fields = <Widget>[];
    if (usesUuid) {
      fields.add(const SizedBox(height: 16));
      fields.add(TextFormField(
        controller: _uuid,
        textInputAction: TextInputAction.next,
        decoration: const InputDecoration(labelText: 'UUID'),
        validator: (v) =>
            (v == null || v.trim().isEmpty) ? 'UUID is required' : null,
      ));
    }
    if (usesUsername) {
      fields.add(const SizedBox(height: 16));
      fields.add(TextFormField(
        controller: _username,
        textInputAction: TextInputAction.next,
        decoration: const InputDecoration(labelText: 'Username'),
      ));
    }
    if (usesPassword) {
      fields.add(const SizedBox(height: 16));
      fields.add(TextFormField(
        controller: _password,
        obscureText: true,
        textInputAction: TextInputAction.next,
        decoration: const InputDecoration(labelText: 'Password'),
      ));
    }
    if (usesCipher) {
      fields.add(const SizedBox(height: 16));
      fields.add(TextFormField(
        controller: _cipher,
        textInputAction: TextInputAction.next,
        decoration: const InputDecoration(
          labelText: 'Cipher',
          helperText: 'e.g. aes-128-gcm, chacha20-ietf-poly1305',
        ),
      ));
    }
    if (usesSsh) {
      fields.add(const SizedBox(height: 16));
      fields.add(TextFormField(
        controller: _sshUser,
        textInputAction: TextInputAction.next,
        decoration: const InputDecoration(labelText: 'SSH user'),
        validator: (v) =>
            (v == null || v.trim().isEmpty) ? 'SSH user is required' : null,
      ));
      fields.add(const SizedBox(height: 16));
      fields.add(TextFormField(
        controller: _sshKey,
        maxLines: 4,
        decoration: const InputDecoration(
          labelText: 'Private key (PEM)',
          alignLabelWithHint: true,
        ),
      ));
    }
    return fields;
  }

  Future<void> _save() async {
    if (!_formKey.currentState!.validate()) return;
    setState(() => _saving = true);
    final now = DateTime.now().toUtc();
    final server = ServerProfile(
      id: widget.server?.id ?? '',
      name: _name.text.trim(),
      protocol: _protocol,
      address: _address.text.trim(),
      port: int.tryParse(_port.text) ?? 0,
      username: _username.text.trim().isEmpty ? null : _username.text.trim(),
      password: _password.text.isEmpty ? null : _password.text,
      cipher: _cipher.text.trim().isEmpty ? null : _cipher.text.trim(),
      uuid: _uuid.text.trim().isEmpty ? null : _uuid.text.trim(),
      tls: TlsSettings(
        enabled: _tlsEnabled,
        allowInsecure: _tlsAllowInsecure,
        serverName: _serverName.text.trim().isEmpty
            ? null
            : _serverName.text.trim(),
      ),
      ssh: SshSettings(
        user: _sshUser.text.trim().isEmpty ? null : _sshUser.text.trim(),
        privateKey: _sshKey.text.isEmpty ? null : _sshKey.text,
      ),
      favorite: _favorite,
      lastLatencyMs: widget.server?.lastLatencyMs ?? 0,
      lastTestedAt: widget.server?.lastTestedAt,
      createdAt: widget.server?.createdAt ?? now,
      updatedAt: now,
    );
    try {
      final notifier = ref.read(serversProvider.notifier);
      if (_isEdit) {
        await notifier.updateServer(server);
      } else {
        await notifier.add(server);
      }
      if (!mounted) return;
      Navigator.of(context).pop();
    } on ApiError catch (e) {
      if (!mounted) return;
      setState(() => _saving = false);
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  String _protocolLabel(ServerProtocol protocol) => switch (protocol) {
        ServerProtocol.vless => 'VLESS',
        ServerProtocol.vmess => 'VMess',
        ServerProtocol.shadowsocks => 'Shadowsocks',
        ServerProtocol.socks5 => 'SOCKS5',
        ServerProtocol.http => 'HTTP Proxy',
        ServerProtocol.ssh => 'SSH Tunnel',
      };
}

class _TlsSection extends StatelessWidget {
  const _TlsSection({
    required this.enabled,
    required this.allowInsecure,
    required this.serverName,
    required this.onEnabled,
    required this.onAllowInsecure,
  });

  final bool enabled;
  final bool allowInsecure;
  final TextEditingController serverName;
  final ValueChanged<bool> onEnabled;
  final ValueChanged<bool> onAllowInsecure;

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        SwitchListTile(
          value: enabled,
          onChanged: onEnabled,
          title: const Text('TLS'),
          subtitle: const Text('Encrypt the connection'),
          contentPadding: EdgeInsets.zero,
        ),
        if (enabled) ...[
          TextFormField(
            controller: serverName,
            textInputAction: TextInputAction.done,
            decoration: const InputDecoration(
              labelText: 'Server name (SNI)',
              helperText: 'Leave empty to use the address',
            ),
          ),
          const SizedBox(height: 8),
          SwitchListTile(
            value: allowInsecure,
            onChanged: onAllowInsecure,
            title: const Text('Allow insecure certificates'),
            subtitle: const Text(
              'Disables certificate validation — use only for testing.',
            ),
            contentPadding: EdgeInsets.zero,
          ),
        ],
      ],
    );
  }
}
