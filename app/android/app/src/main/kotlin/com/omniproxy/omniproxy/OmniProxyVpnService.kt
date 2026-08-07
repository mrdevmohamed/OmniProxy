package com.omniproxy.omniproxy

import android.app.Service
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.VpnService
import android.os.Build
import android.os.IBinder
import android.os.ParcelFileDescriptor

/**
 * VpnService for VPN mode (PRD §3.1). Establishes the TUN, hands its file
 * descriptor to the Go core, and hosts the core + persistent notification until
 * disconnected. `docs/platform-notes.md` §Android.
 *
 * Teardown contract: disconnect is serialized through [Bridge.submitDisconnect],
 * so the core has fully stopped before this service is stopped, and the TUN fd
 * is owned here (never by the connect thread) and closed exactly once, in
 * [onDestroy].
 */
class OmniProxyVpnService : VpnService() {
    companion object {
        const val EXTRA_REQUEST = "request"

        @Volatile
        private var instance: OmniProxyVpnService? = null

        /** Closes the live service's TUN fd, if any, and marks the service as
         * torn down. The disconnect path calls this before stopService: the
         * system binds to a VpnService (act=android.net.VpnService) and keeps it
         * alive after a stop until the fd is closed, so stopping the service
         * first would leave the TUN interface and the foreground VPN running
         * forever (the fd close in onDestroy never runs while bound). Closing
         * the fd triggers the system's VPN teardown, which releases the binding
         * and lets the pending stop complete. */
        fun releaseTun() {
            instance?.let { svc ->
                synchronized(svc.lock) {
                    svc.destroyed = true
                }
                svc.closeTun()
            }
        }
    }

    override fun onCreate() {
        super.onCreate()
        instance = this
    }

    private val lock = Any()

    /** The established TUN. Owned by the service so a disconnect can always
     * release it, even when the connect hand-off is still in flight. */
    private var tunPfd: ParcelFileDescriptor? = null

    /** Set once the service is being torn down; a connect that reaches
     * [claimTun] afterwards must release its fresh TUN and never start. */
    private var destroyed = false

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
                if (!claimTun(pfd)) {
                    // The service was torn down (a disconnect landed) while
                    // establish() was in flight: release the fresh TUN and never
                    // start the core.
                    runCatching { pfd.close() }
                    return@Thread
                }
                // The tunnel's own sockets must not re-enter the TUN (DNS
                // bootstrap + the proxy server connection would loop). protect()
                // sends their traffic over the physical network instead.
                Bridge.setSocketProtector(this)
                Bridge.setTunFd(pfd.fd)
                Bridge.submitConnect(
                    requestJson,
                    result,
                    onAbort = {
                        closeTun()
                        stopSelf()
                    },
                    onError = {
                        closeTun()
                        stopSelf()
                    },
                )
            } catch (e: Exception) {
                postError(result, e)
                // A failure after establish() may leave a claimed TUN fd open;
                // a dangling fd keeps the kernel tun device and the system's
                // VpnService binding alive, so release it before stopping.
                closeTun()
                stopSelf()
            }
        }.start()
        // Explicitly started and stopped host: the system must never recreate it
        // (START_STICKY would restart it with a null intent and a stale TUN).
        return START_NOT_STICKY
    }

    /** Stores the established TUN fd, or rejects it (and returns false) when the
     * service is already being torn down. */
    private fun claimTun(pfd: ParcelFileDescriptor): Boolean {
        synchronized(lock) {
            if (destroyed) return false
            tunPfd = pfd
            return true
        }
    }

    /** Releases the TUN fd. Idempotent. */
    private fun closeTun() {
        synchronized(lock) {
            runCatching { tunPfd?.close() }
            tunPfd = null
        }
    }

    private fun postError(result: io.flutter.plugin.common.MethodChannel.Result?, e: Exception) {
        if (result != null) {
            val message = e.message ?: e.toString()
            result.error("vpn", message, null)
        }
    }

    override fun onDestroy() {
        if (instance === this) instance = null
        val wasActive = Bridge.vpnServiceActive
        synchronized(lock) {
            destroyed = true
        }
        closeTun()
        Bridge.vpnServiceActive = false
        // A destroy that is not the tail of a clean disconnect (system kill,
        // forced stop) leaves the core running; stop it best-effort.
        if (wasActive) {
            Bridge.requestCoreDisconnect()
        }
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? {
        // VpnService must be started via startService; binding unused.
        return super.onBind(intent)
    }

    override fun onRevoke() {
        // User revoked the VPN grant from the system UI: the TUN is torn down
        // under us and the core is no longer usable. Close the fd, then run a
        // serialized disconnect round-trip to keep the app consistent before
        // stopping.
        closeTun()
        Bridge.submitDisconnect("{}") {
            stopSelf()
        }
        super.onRevoke()
    }
}
