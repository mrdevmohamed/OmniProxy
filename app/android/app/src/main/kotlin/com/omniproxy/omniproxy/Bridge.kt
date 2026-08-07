package com.omniproxy.omniproxy

import android.content.Context
import android.net.ConnectivityManager
import android.net.LinkProperties
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import android.net.VpnService
import android.os.Handler
import android.os.HandlerThread
import com.omniproxy.bind.mobile.Mobile
import com.omniproxy.bind.mobile.SocketProtector
import io.flutter.plugin.common.MethodChannel
import java.net.NetworkInterface
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicInteger
import org.json.JSONArray
import org.json.JSONObject

/**
 * Pure-transport host for the gomobile bindings (`docs/api-contract.md` §5.1).
 *
 * - `request` runs on a background thread (the Go call blocks its caller);
 * - `connect`/`disconnect` run on a single serialized lifecycle executor so a
 *   disconnect can never race an in-flight connect;
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

    @Volatile private var defaultName: String? = null
    @Volatile private var defaultIndex: Int = -1

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

    /** Single worker for connect/disconnect lifecycle requests. The Go calls
     * block, so executing them on one thread guarantees they reach the core in
     * submission order — a disconnect queued behind a connect runs only after
     * that connect has completed, never while it is starting. */
    private val lifecycleExecutor = Executors.newSingleThreadExecutor { r ->
        Thread(r, "omniproxy-lifecycle").apply { isDaemon = true }
    }

    /** Generation counter bumped on every requested disconnect. A queued connect
     * compares its intent-time snapshot against it and aborts when it has
     * changed: the executor FIFO alone cannot express "a connect that landed
     * before this disconnect must not start the core after it". */
    private val disconnectCount = AtomicInteger(0)

    /** Disconnect generation captured when a connect intent starts (tap time).
     * VPN-mode connects reach the core only after consent + TUN establishment,
     * so the snapshot must be taken when the user asked to connect, not when the
     * queued task runs. */
    @Volatile
    private var connectGeneration = 0

    @Synchronized
    fun ensureInit(context: Context) {
        if (initialized) return
        appContext = context.applicationContext
        val dataDir = context.filesDir.absolutePath
        val cfg = JSONObject()
            .put("dataDir", dataDir)
            .put("logLevel", "info")
        // Must be registered before init: Init rebuilds the core facade with
        // the registered SecretStore, and the at-rest data key + credential
        // refs must survive a process restart (KeystoreSecretStore).
        Mobile.setSecretStore(KeystoreSecretStore(appContext!!))
        Mobile.init(cfg.toString())
        initialized = true
        startDefaultNetworkMonitor()
        startPolling()
    }

    /** Tracks the physical default network from ConnectivityManager and pushes
     * it into the core (mobile.SetDefaultInterface). The VpnService routes
     * 0.0.0.0/0 into the TUN, so sing-box cannot discover the real default
     * network itself; it dials the tunnel's own sockets through whatever
     * interface we report (engine/platform_monitor.go). */
    private fun startDefaultNetworkMonitor() {
        val ctx = appContext ?: return
        val manager = ctx.getSystemService(Context.CONNECTIVITY_SERVICE) as? ConnectivityManager ?: return
        val request = NetworkRequest.Builder()
            .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
            .addTransportType(NetworkCapabilities.TRANSPORT_WIFI)
            .addTransportType(NetworkCapabilities.TRANSPORT_CELLULAR)
            .build()
        val callback = object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) = refreshNetworkState()
            override fun onCapabilitiesChanged(network: Network, capabilities: NetworkCapabilities) =
                refreshNetworkState()
            override fun onLinkPropertiesChanged(network: Network, linkProperties: LinkProperties) =
                refreshNetworkState()
            override fun onLost(network: Network) = refreshNetworkState()
            override fun onUnavailable() = refreshNetworkState()
        }
        try {
            manager.registerNetworkCallback(request, callback)
        } catch (_: Exception) {
        }
    }

    /** Pushes the current physical default network + interface list into the
     * core unconditionally. Called from the VpnService before establish() so the
     * values are cached before the engine starts. */
    fun syncDefaultInterface() {
        refreshDefaultInterface(force = true)
        pushNetworkInterfaces()
    }

    /** Re-reads the default network and interface list after a network change. */
    private fun refreshNetworkState() {
        refreshDefaultInterface()
        pushNetworkInterfaces()
    }

    /** Enumerates the physical interfaces through java.net.NetworkInterface
     * (never netlink, which the app sandbox forbids) as the JSON array the core
     * expects: [{name, index, mtu, up, addresses:[ip/prefix]}]. The VpnService
     * TUN is excluded — it must never be a dial candidate. */
    private fun enumerateNetworkInterfaces(): String {
        val arr = JSONArray()
        try {
            for (iface in NetworkInterface.getNetworkInterfaces()) {
                val name = iface.name ?: continue
                if (name.startsWith("tun") || name.startsWith("omni")) continue
                val obj = JSONObject()
                    .put("name", name)
                    .put("index", iface.index)
                    .put("mtu", iface.mtu)
                    .put("up", iface.isUp)
                val addrs = JSONArray()
                for (a in iface.interfaceAddresses) {
                    val address = a.address ?: continue
                    val prefix = a.networkPrefixLength
                    if (prefix >= 0) {
                        addrs.put("${address.hostAddress}/$prefix")
                    }
                }
                obj.put("addresses", addrs)
                arr.put(obj)
            }
        } catch (_: Exception) {
        }
        return arr.toString()
    }

    private fun pushNetworkInterfaces() {
        try {
            Mobile.setNetworkInterfaces(enumerateNetworkInterfaces())
        } catch (_: Exception) {
        }
    }

    private fun refreshDefaultInterface(force: Boolean = false) {
        val ctx = appContext ?: return
        val manager = ctx.getSystemService(Context.CONNECTIVITY_SERVICE) as? ConnectivityManager ?: return
        // Pick the best physical network: the VpnService TUN is the app's
        // default network once connected, so it must be excluded. VALIDATED is
        // not required — some devices never flag the network while the VPN is
        // establishing.
        var best: Network? = null
        for (network in manager.allNetworks) {
            val capabilities = manager.getNetworkCapabilities(network) ?: continue
            if (capabilities.hasTransport(NetworkCapabilities.TRANSPORT_VPN)) continue
            if (!capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)) continue
            if (capabilities.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)) {
                best = network
                break
            }
            if (capabilities.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR)) {
                if (best == null) best = network
            }
        }
        val network = best ?: return
        val linkProperties = manager.getLinkProperties(network) ?: return
        val name = linkProperties.interfaceName ?: return
        val index = try {
            NetworkInterface.getByName(name)?.index ?: -1
        } catch (_: Exception) {
            -1
        }
        if (index < 0) return
        if (!force && name == defaultName && index == defaultIndex) return
        defaultName = name
        defaultIndex = index
        try {
            Mobile.setDefaultInterface(name, index)
        } catch (_: Exception) {
        }
    }

    fun setEventsChannel(channel: MethodChannel?) {
        eventsChannel = channel
    }

    fun setTunFd(fd: Int) {
        Mobile.setTunFd(fd)
    }

    /** Registers [vpnService] as the socket protector: its `protect(fd)` marks
     * a socket so the system sends its traffic over the physical network
     * instead of into the VpnService TUN (see engine/platform_fd.go). */
    fun setSocketProtector(vpnService: VpnService) {
        Mobile.setSocketProtector(object : SocketProtector {
            override fun protect(fd: Int): Boolean = vpnService.protect(fd)
        })
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

    /**
     * Marks the start of a connect intent. Called synchronously when the user
     * requests a connect; the generation snapshot is used by [submitConnect] so
     * a connect aborts if a disconnect was requested while the intent was still
     * en route to the core (VpnService consent / TUN establishment).
     */
    fun recordConnectIntent() {
        connectGeneration = disconnectCount.get()
    }

    /**
     * Queues a `connect` on the serialized lifecycle executor. If a disconnect
     * was requested after this connect's intent started (see
     * [recordConnectIntent]), it aborts instead of starting the core (the
     * executor would otherwise run it after the disconnect and leave the engine
     * running). The response is posted to [result] on the main thread; [onAbort]
     * runs when the connect was superseded (release any claimed TUN fd);
     * [onError] runs after an error result is posted (stop the hosting service).
     */
    fun submitConnect(
        requestJson: String,
        result: MethodChannel.Result?,
        onAbort: () -> Unit = {},
        onError: () -> Unit = {},
    ) {
        val generation = connectGeneration
        lifecycleExecutor.execute {
            val context = appContext
            if (context == null) {
                postToMain { result?.error("mobile", "core not initialized", null) }
                return@execute
            }
            ensureInit(context)
            if (generation != disconnectCount.get()) {
                postToMain {
                    onAbort()
                    result?.error("vpn", "disconnect requested before connect completed", null)
                }
                return@execute
            }
            try {
                val response = Mobile.request("connect", requestJson)
                postToMain { result?.success(response) }
            } catch (e: Exception) {
                postToMain {
                    result?.error("mobile", e.message ?: e.toString(), null)
                    onError()
                }
            }
        }
    }

    /**
     * Queues a `disconnect` on the serialized lifecycle executor and bumps the
     * disconnect generation so any pending connect aborts. [onDone] runs on the
     * main thread after the core has confirmed the disconnect — the point at
     * which it is safe to tear down the foreground host (VpnService/TUN last).
     */
    fun submitDisconnect(
        requestJson: String,
        result: MethodChannel.Result? = null,
        onDone: () -> Unit = {},
    ) {
        disconnectCount.incrementAndGet()
        lifecycleExecutor.execute {
            val context = appContext
            if (context == null) {
                postToMain {
                    result?.error("mobile", "core not initialized", null)
                    onDone()
                }
                return@execute
            }
            ensureInit(context)
            try {
                val response = Mobile.request("disconnect", requestJson)
                postToMain {
                    result?.success(response)
                    onDone()
                }
            } catch (e: Exception) {
                postToMain {
                    result?.error("mobile", e.message ?: e.toString(), null)
                    onDone()
                }
            }
        }
    }

    /** Best-effort serialized core disconnect for a hosting service torn down
     * without a prior clean disconnect (system-initiated destroy, force stop).
     * No-op against an idle core. */
    fun requestCoreDisconnect() {
        disconnectCount.incrementAndGet()
        lifecycleExecutor.execute {
            val context = appContext ?: return@execute
            ensureInit(context)
            try {
                Mobile.request("disconnect", "{}")
            } catch (_: Exception) {
            }
        }
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
        lifecycleExecutor.shutdownNow()
        disconnectCount.set(0)
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
