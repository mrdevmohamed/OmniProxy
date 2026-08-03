package com.omniproxy.omniproxy

import android.content.Context
import android.os.Handler
import android.os.HandlerThread
import com.omniproxy.bind.mobile.Mobile
import io.flutter.plugin.common.MethodChannel
import org.json.JSONArray
import org.json.JSONObject

/**
 * Pure-transport host for the gomobile bindings (`docs/api-contract.md` §5.1).
 *
 * - `request` runs on a background thread (the Go call blocks its caller);
 * - events are drained from the Go ring on a poller thread and forwarded to
 *   Dart over `MethodChannel("com.omniproxy/events")` as `invokeMethod("event")`;
 * - `setTunFd` hands the VpnService TUN fd into Go before a VPN-mode connect.
 *
 * No bridge business logic lives here.
 */
object Bridge {
    private const val TAG = "OmniProxyBridge"
    private const val EVENT_POLL_MS = 25L

    @Volatile private var initialized = false
    private var appContext: Context? = null

    private var pollThread: HandlerThread? = null
    private var pollHandler: Handler? = null

    @Volatile
    private var eventsChannel: MethodChannel? = null

    /** The MethodChannel.Result of the in-flight `connect` when VPN mode must
     * wait for the VpnService to establish the TUN before starting the core. */
    @Volatile
    var pendingConnect: MethodChannel.Result? = null

    /** Tracks which foreground component hosts the active connection, so a
     * disconnect can stop it. */
    @Volatile
    var vpnServiceActive = false

    @Volatile
    var proxyServiceActive = false

    @Synchronized
    fun ensureInit(context: Context) {
        if (initialized) return
        appContext = context.applicationContext
        val dataDir = context.filesDir.absolutePath
        val cfg = JSONObject()
            .put("dataDir", dataDir)
            .put("logLevel", "info")
        Mobile.init(cfg.toString())
        initialized = true
        startPolling()
    }

    fun setEventsChannel(channel: MethodChannel?) {
        eventsChannel = channel
    }

    fun setTunFd(fd: Int) {
        Mobile.setTunFd(fd)
    }

    /** Dispatches a bridge method on a background thread, completing [result]
     * with the canonical response JSON. */
    fun executeRequest(method: String, requestJson: String, result: MethodChannel.Result) {
        ensureInit(appContext ?: return)
        Thread {
            try {
                val response = Mobile.request(method, requestJson)
                postToMain { result.success(response) }
            } catch (e: Exception) {
                postToMain { result.error("mobile", e.message ?: e.toString(), null) }
            }
        }.start()
    }

    /** Same as [executeRequest] but completes a raw callback instead of a
     * channel result (used by the VpnService hand-off path). */
    fun executeRequest(method: String, requestJson: String, onDone: (String) -> Unit) {
        ensureInit(appContext ?: return)
        Thread {
            try {
                val response = Mobile.request(method, requestJson)
                postToMain { onDone(response) }
            } catch (e: Exception) {
                postToMain { onDone("""{"ok":false,"error":{"code":"mobile","message":"${e.message}"}}""") }
            }
        }.start()
    }

    fun shutdown() {
        try {
            Mobile.shutdown()
        } catch (_: Exception) {
        }
        pollHandler?.removeCallbacksAndMessages(null)
        pollThread?.quitSafely()
        pollThread = null
        pollHandler = null
        initialized = false
    }

    private fun startPolling() {
        val thread = HandlerThread("omniproxy-events").also { it.start() }
        pollThread = thread
        val handler = Handler(thread.looper)
        pollHandler = handler
        handler.post(object : Runnable {
            override fun run() {
                pollOnce()
                if (handler === pollHandler) {
                    handler.postDelayed(this, EVENT_POLL_MS)
                }
            }
        })
    }

    private fun pollOnce() {
        val channel = eventsChannel ?: return
        val batch: String
        try {
            batch = Mobile.pollEvents()
        } catch (_: Throwable) {
            return
        }
        val arr = try {
            JSONArray(batch)
        } catch (_: Exception) {
            return
        }
        for (i in 0 until arr.length()) {
            val eventJson = arr.optString(i)
            if (eventJson.isNotEmpty()) {
                postToMain { channel.invokeMethod("event", eventJson) }
            }
        }
    }

    private fun postToMain(runnable: () -> Unit) {
        val looper = appContext?.mainLooper ?: return
        Handler(looper).post(runnable)
    }
}
