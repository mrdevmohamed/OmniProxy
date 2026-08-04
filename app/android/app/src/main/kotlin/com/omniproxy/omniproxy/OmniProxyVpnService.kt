package com.omniproxy.omniproxy

import android.app.Service
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.VpnService
import android.os.Build
import android.os.IBinder

/**
 * VpnService for VPN mode (PRD §3.1). Establishes the TUN, hands its file
 * descriptor to the Go core, and hosts the core + persistent notification until
 * disconnected. `docs/platform-notes.md` §Android.
 */
class OmniProxyVpnService : VpnService() {
    companion object {
        const val EXTRA_REQUEST = "request"
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val requestJson = intent?.getStringExtra(EXTRA_REQUEST)
        if (requestJson == null) {
            stopSelf()
            return START_NOT_STICKY
        }
        // Started via startForegroundService() (MainActivity), so startForeground()
        // must run promptly or the system throws a
        // ForegroundServiceDidNotStartInTimeException. The system additionally
        // renders the persistent, non-dismissible VPN banner once establish() succeeds.
        val notification = Notifications.build(this, "Connecting…")
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            startForeground(
                Notifications.NOTIFICATION_ID,
                notification,
                ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC,
            )
        } else {
            startForeground(Notifications.NOTIFICATION_ID, notification)
        }
        Bridge.ensureInit(applicationContext)
        Bridge.vpnServiceActive = true

        val result = Bridge.pendingConnect
        Bridge.pendingConnect = null

        Thread {
            try {
                // Capture the physical default network + interface list before
                // establish() turns the VPN into the app's default network; the
                // tunnel's own sockets dial through this interface
                // (engine/platform_fd.go).
                Bridge.syncDefaultInterface()
                val builder = Builder()
                    .setSession("omniproxy")
                    .setMtu(1500)
                    .addAddress("10.0.0.1", 24)
                    .addRoute("0.0.0.0", 0)
                    .addAddress("fd00::1", 64)
                    .addRoute("::", 0)
                val pfd = builder.establish() ?: throw IllegalStateException("VpnService.establish() returned null")
                // The tunnel's own sockets must not re-enter the TUN (DNS
                // bootstrap + the proxy server connection would loop). protect()
                // sends their traffic over the physical network instead.
                Bridge.setSocketProtector(this)
                Bridge.setTunFd(pfd.fd)
                Bridge.executeRequest("connect", requestJson) { response -> result?.success(response) }
            } catch (e: Exception) {
                postError(result, e)
                stopSelf()
            }
        }.start()
        return START_STICKY
    }

    private fun postError(result: io.flutter.plugin.common.MethodChannel.Result?, e: Exception) {
        if (result != null) {
            val message = e.message ?: e.toString()
            result.error("vpn", message, null)
        }
        stopSelf()
    }

    override fun onDestroy() {
        Bridge.vpnServiceActive = false
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? {
        // VpnService must be started via startService; binding unused.
        return super.onBind(intent)
    }

    override fun onRevoke() {
        // User revoked the VPN grant from the system UI; the core is no longer
        // usable. A disconnect round-trip keeps the app consistent.
        try {
            Bridge.executeRequest("disconnect", "{}") {}
        } catch (_: Exception) {
        }
        stopSelf()
        super.onRevoke()
    }}
