package com.omniproxy.omniproxy

import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel
import org.json.JSONObject

/** Flutter activity. Hosts the bridge channels and the VpnService consent flow.
 * `docs/platform-notes.md` §Android. */
class MainActivity : FlutterActivity() {
    companion object {
        const val CHANNEL_BRIDGE = "com.omniproxy/bridge"
        const val CHANNEL_EVENTS = "com.omniproxy/events"
    }

    private var vpnRequestCode = 1
    private var pendingConnectRequest: String? = null

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        Bridge.ensureInit(applicationContext)

        val bridge = MethodChannel(flutterEngine.dartExecutor.binaryMessenger, CHANNEL_BRIDGE)
        val events = MethodChannel(flutterEngine.dartExecutor.binaryMessenger, CHANNEL_EVENTS)
        Bridge.setEventsChannel(events)

        bridge.setMethodCallHandler { call, result ->
            val requestJson = call.arguments as? String ?: "{}"
            handleCall(call.method, requestJson, result)
        }
    }

    private fun handleCall(method: String, requestJson: String, result: MethodChannel.Result) {
        when (method) {
            "connect" -> handleConnect(requestJson, result)
            "disconnect" -> handleDisconnect(requestJson, result)
            else -> Bridge.executeRequest(method, requestJson, result)
        }
    }

    private fun handleConnect(requestJson: String, result: MethodChannel.Result) {
        // Snapshot the disconnect generation now: the core may only reach the
        // task queue after VpnService consent + TUN establishment, and a
        // disconnect requested meanwhile must supersede this connect.
        Bridge.recordConnectIntent()
        val mode = try {
            JSONObject(requestJson).optString("mode", "")
        } catch (_: Exception) {
            ""
        }
        if (mode != "vpn") {
            startProxyHost(requestJson, result)
            return
        }

        val consentIntent = VpnService.prepare(this)
        if (consentIntent != null) {
            pendingConnectRequest = requestJson
            Bridge.pendingConnect = result
            startActivityForResult(consentIntent, vpnRequestCode)
            return
        }
        startVpnHost(requestJson, result)
    }

    private fun startVpnHost(requestJson: String, result: MethodChannel.Result) {
        Bridge.pendingConnect = result
        val intent = Intent(this, OmniProxyVpnService::class.java)
            .putExtra(OmniProxyVpnService.EXTRA_REQUEST, requestJson)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            startForegroundService(intent)
        } else {
            startService(intent)
        }
    }

    private fun startProxyHost(requestJson: String, result: MethodChannel.Result) {
        val intent = Intent(this, VpnProxyService::class.java)
            .putExtra(VpnProxyService.EXTRA_REQUEST, requestJson)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            startForegroundService(intent)
        } else {
            startService(intent)
        }
        Bridge.submitConnect(
            requestJson,
            result,
            onError = { stopService(intent) },
        )
    }

    /** Disconnect is serialized with any in-flight connect; the foreground host
     * is torn down only after the core confirms it has stopped, so the TUN fd is
     * released last (see OmniProxyVpnService.onDestroy). */
    private fun handleDisconnect(requestJson: String, result: MethodChannel.Result) {
        Bridge.submitDisconnect(requestJson, result) {
            stopConnectionHosts()
        }
    }

    /** After the core has fully stopped, stop whichever host held the
     * connection. The active flags are cleared first so the services' onDestroy
     * sees a clean teardown and does not re-trigger a core disconnect.
     *
     * The VpnService's TUN fd must be closed before stopService: the system
     * binds to a VpnService and a bound service survives stopService until the
     * fd closes (which is what releases the system binding and triggers the
     * VPN teardown). Without this, onDestroy never runs and the foreground
     * VPN keeps running after the core has disconnected. */
    private fun stopConnectionHosts() {
        if (Bridge.vpnServiceActive) {
            Bridge.vpnServiceActive = false
            OmniProxyVpnService.releaseTun()
            applicationContext.stopService(Intent(this, OmniProxyVpnService::class.java))
        }
        if (Bridge.proxyServiceActive) {
            Bridge.proxyServiceActive = false
            applicationContext.stopService(Intent(this, VpnProxyService::class.java))
        }
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != vpnRequestCode) return
        val requestJson = pendingConnectRequest
        val result = Bridge.pendingConnect
        pendingConnectRequest = null
        Bridge.pendingConnect = null
        if (resultCode == RESULT_OK && requestJson != null) {
            startVpnHost(requestJson, result!!)
        } else {
            result?.error("vpn_consent", "VPN permission denied", null)
        }
    }
}
