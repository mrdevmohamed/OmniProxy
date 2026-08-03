import 'dart:io';

import 'api_client.dart';
import 'bridge/bridge_android.dart';
import 'bridge/bridge_linux.dart';
import 'bridge/bridge_transport.dart';
import 'bridge_api_client.dart';
import 'mock_api_client.dart';

/// Selects the [ApiClient] for the current platform.
///
/// The bridge transports are pure transport (`docs/api-contract.md` §5); each
/// platform milestone swaps in the real one:
///   - Linux:  `dart:ffi` into `libomniproxy.so` (M6)
///   - Android: MethodChannel over gomobile bind (M7)
///   - Windows: `dart:ffi` into `omniproxy.dll` (M8)
///
/// Until the M8 transport lands, Windows falls back to the mock so the shell
/// keeps running.
ApiClient buildApiClient() {
  if (Platform.isAndroid) {
    return BridgeApiClient(AndroidBridge());
  }
  if (Platform.isLinux) {
    return BridgeApiClient(createLinuxTransport());
  }
  return MockApiClient();
}

/// Creates the Linux transport with the init config (data dir, helper path).
/// Overridable for tests.
BridgeTransport createLinuxTransport({
  String? libraryPath,
  String? dataDir,
  String? helperPath,
}) {
  return LinuxBridge(
    libraryPath: libraryPath,
    initConfig: {
      'dataDir': ?dataDir,
      'helperPath': ?helperPath,
    },
  );
}
