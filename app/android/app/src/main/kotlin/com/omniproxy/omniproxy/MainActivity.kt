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
            "disconnect" -> {
                Bridge.executeRequest(method, requestJson, result)
                stopConnectionHosts()
            }
            else -> Bridge.executeRequest(method, requestJson, result)
        }
    }

    private fun handleConnect(requestJson: String, result: MethodChannel.Result) {
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
        Bridge.executeRequest("connect", requestJson, result)
    }

    /** After a disconnect response, stop whichever host held the connection. */
    private fun stopConnectionHosts() {
        if (Bridge.vpnServiceActive) {
            stopService(Intent(this, OmniProxyVpnService::class.java))
            Bridge.vpnServiceActive = false
        }
        if (Bridge.proxyServiceActive) {
            stopService(Intent(this, VpnProxyService::class.java))
            Bridge.proxyServiceActive = false
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
