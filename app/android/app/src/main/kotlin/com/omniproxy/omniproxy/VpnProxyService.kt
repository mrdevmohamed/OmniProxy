package com.omniproxy.omniproxy

import android.app.Service
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder

/**
 * Foreground host for proxy-mode connections (no VpnService/TUN involved): shows
 * the persistent connection notification while the core runs in-process.
 * `docs/platform-notes.md` §Android.
 */
class VpnProxyService : Service() {
    companion object {
        const val EXTRA_REQUEST = "request"
    }

    override fun onCreate() {
        super.onCreate()
        Bridge.ensureInit(applicationContext)
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
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
        Bridge.proxyServiceActive = true
        // Explicitly started and stopped host; never let the system recreate it.
        return START_NOT_STICKY
    }

    override fun onDestroy() {
        val wasActive = Bridge.proxyServiceActive
        Bridge.proxyServiceActive = false
        // A destroy that is not the tail of a clean disconnect (system kill,
        // forced stop) leaves the core running; stop it best-effort.
        if (wasActive) {
            Bridge.requestCoreDisconnect()
        }
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null
}
