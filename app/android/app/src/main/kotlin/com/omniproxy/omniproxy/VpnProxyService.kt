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
        return START_STICKY
    }

    override fun onDestroy() {
        Bridge.proxyServiceActive = false
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null
}
