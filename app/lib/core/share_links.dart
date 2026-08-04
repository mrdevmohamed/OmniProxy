import 'dart:convert';

import 'models.dart';

/// Native share-link serialization mirroring `core/server/linkgen.go` so the
/// mock's `exportServers(format: 'links')` output matches the real core.
///
/// Only used by the mock transport; the bridge path delegates link export to
/// core. Returns `null` for protocols without a share-link representation
/// (ssh).
String? shareLinkFor(ServerProfile server) {
  switch (server.protocol) {
    case ServerProtocol.vless:
      return _linkVLESS(server);
    case ServerProtocol.vmess:
      return _linkVMess(server);
    case ServerProtocol.shadowsocks:
      return _linkShadowsocks(server);
    case ServerProtocol.trojan:
      return _linkTrojan(server);
    case ServerProtocol.socks5:
    case ServerProtocol.http:
      return _linkUserPass(server);
    case ServerProtocol.ssh:
      return null;
  }
}

String _encodeQuery(Map<String, String> params) {
  final keys = params.keys.toList()..sort();
  return keys
      .map((k) => '$k=${Uri.encodeQueryComponent(params[k]!)}')
      .join('&');
}

String _hostPort(ServerProfile p) => '${p.address}:${p.port}';

String _linkVLESS(ServerProfile p) {
  final params = <String, String>{
    ..._streamParams(p),
    'encryption': 'none',
    if (p.flow != null && p.flow!.isNotEmpty) 'flow': p.flow!,
  };
  return 'vless://${p.uuid}@${_hostPort(p)}?${_encodeQuery(params)}'
      '#${Uri.encodeComponent(p.name)}';
}

String _linkVMess(ServerProfile p) {
  final params = <String, String>{
    ..._streamParams(p),
    'encryption': p.security?.isEmpty ?? true ? 'auto' : p.security!,
    if (p.globalPadding) 'global_padding': '1',
    if (p.authenticatedLength) 'authenticated_length': '1',
  };
  return 'vmess://${p.uuid}@${_hostPort(p)}?${_encodeQuery(params)}'
      '#${Uri.encodeComponent(p.name)}';
}

String _linkShadowsocks(ServerProfile p) {
  final userInfo =
      base64Url.encode(utf8.encode('${p.cipher}:${p.password}')).replaceAll('=', '');
  return 'ss://$userInfo@${_hostPort(p)}#${Uri.encodeComponent(p.name)}';
}

String _linkTrojan(ServerProfile p) {
  return 'trojan://${p.password}@${_hostPort(p)}?${_encodeQuery(_streamParams(p))}'
      '#${Uri.encodeComponent(p.name)}';
}

String _linkUserPass(ServerProfile p) {
  final scheme = p.protocol == ServerProtocol.http ? 'http' : 'socks5';
  final creds = p.username != null && p.username!.isNotEmpty
      ? '${p.username}${p.password != null ? ':${p.password}' : ''}@'
      : '';
  return '$scheme://$creds${_hostPort(p)}#${Uri.encodeComponent(p.name)}';
}

Map<String, String> _streamParams(ServerProfile p) {
  final params = <String, String>{
    'security': p.tls.enabled ? 'tls' : 'none',
    if (p.tls.serverName != null && p.tls.serverName!.isNotEmpty)
      'sni': p.tls.serverName!,
    if (p.tls.alpn != null && p.tls.alpn!.isNotEmpty) 'alpn': p.tls.alpn!.join(','),
    if (p.tls.fingerprint != null && p.tls.fingerprint!.isNotEmpty)
      'fp': p.tls.fingerprint!,
    if (p.tls.allowInsecure) 'allowInsecure': '1',
    if (p.transport?.type == TransportType.ws) ...{
      'type': 'ws',
      if (p.transport!.path != null && p.transport!.path!.isNotEmpty)
        'path': p.transport!.path!,
      if (p.transport!.host != null && p.transport!.host!.isNotEmpty)
        'host': p.transport!.host!,
    },
    if (p.packetEncoding != null && p.packetEncoding!.isNotEmpty)
      'packetEncoding': p.packetEncoding!,
  };
  return params;
}
