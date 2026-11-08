package com.taushell.phone

import android.Manifest
import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.view.WindowInsets
import android.widget.FrameLayout
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.Switch
import android.widget.TextView
import android.widget.Toast
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.codescanner.GmsBarcodeScannerOptions
import com.google.mlkit.vision.codescanner.GmsBarcodeScanning
import java.util.concurrent.Executors

private const val REQUEST_POST_NOTIFICATIONS = 1

private data class PendingAlertsEnable(
    val toggle: Switch?,
    val onGranted: () -> Unit,
    val onDenied: () -> Unit,
)

class MainActivity : Activity(), SessionActions {
    private lateinit var store: Store
    private lateinit var root: FrameLayout
    private val io = Executors.newCachedThreadPool()
    private val ui = Handler(Looper.getMainLooper())

    private enum class Screen { PAIR, HOME, SESSION, SETTINGS }

    private var screen = Screen.PAIR
    private var session: SessionScreen? = null
    private var openId: String? = null
    private var latest: List<SessionItem> = emptyList()
    private var pairing: Pairing? = null
    private val cards = LinkedHashMap<String, SessionCard>()
    private var list: LinearLayout? = null
    private var banner: TextView? = null
    private var empty: TextView? = null
    private var refreshing = false
    private var pairingInFlight = false
    private var pendingAlertsEnable: PendingAlertsEnable? = null
    private var pairStatus: TextView? = null
    private var linkField: android.widget.EditText? = null
    private var nameField: android.widget.EditText? = null
    private var pendingLink: String? = null
    private var pendingOpen: String? = null
    @Volatile private var pcPresets: List<String> = emptyList()

    private val poll = object : Runnable {
        override fun run() {
            refresh()
            ui.postDelayed(this, 3000)
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        store = Store(this)
        root = FrameLayout(this).apply {
            setBackgroundColor(Theme.bgApp)
            @Suppress("DEPRECATION")
            setOnApplyWindowInsetsListener { view, insets ->
                view.setPadding(insets.systemWindowInsetLeft, insets.systemWindowInsetTop, insets.systemWindowInsetRight, insets.systemWindowInsetBottom)
                insets
            }
        }
        setContentView(root)
        window.statusBarColor = Theme.bgApp
        window.navigationBarColor = Theme.bgApp
        pendingLink = intent?.dataString
        pendingOpen = intent?.getStringExtra(MonitorService.EXTRA_SESSION)
        pairing = store.pairing()
        // The service dies with an app update or a process kill, so reopening the app revives it.
        if (pairing != null && store.alerts) MonitorService.start(this)
        if (pairing != null && pendingLink == null) showHome() else showPairing()
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        intent.getStringExtra(MonitorService.EXTRA_SESSION)?.let {
            pendingOpen = it
            if (screen == Screen.HOME || screen == Screen.SESSION) refresh() else if (pairing != null && intent.dataString == null) showHome()
        }
        intent.dataString?.let {
            pendingLink = it
            if (screen == Screen.PAIR) linkField?.setText(it) else showPairing()
        }
    }

    override fun onResume() {
        super.onResume()
        if (screen == Screen.HOME || screen == Screen.SESSION) {
            ui.removeCallbacks(poll)
            ui.post(poll)
        }
    }

    override fun onPause() {
        ui.removeCallbacks(poll)
        super.onPause()
    }

    @Deprecated("Deprecated in Java")
    override fun onBackPressed() {
        if (screen == Screen.SETTINGS || screen == Screen.SESSION) showHome() else @Suppress("DEPRECATION") super.onBackPressed()
    }

    private fun scroll(content: View): ScrollView = ScrollView(this).apply {
        isFillViewport = true
        addView(content)
    }

    private fun page(): LinearLayout = linear(this).apply { setPadding(dp(18), dp(16), dp(18), dp(24)) }

    private fun brand(trailing: View? = null): View {
        val mark = text("τ", 22f, Theme.accent, true).apply {
            gravity = Gravity.CENTER
            background = shape(Theme.accentBg, 9, Theme.accentBorder)
        }
        val title = linear(this).apply {
            addView(eyebrow("TauShell"))
            addView(text("Phone", 17f, Theme.text1, true))
        }
        return LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            addView(mark, lp(dp(40), dp(40)))
            addView(title, lp(0, weight = 1f).apply { leftMargin = dp(12) })
            if (trailing != null) addView(trailing)
        }
    }

    private fun swap(view: View, next: Screen) {
        screen = next
        root.removeAllViews()
        root.addView(view, FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT))
    }

    // Pairing

    private fun showPairing() {
        ui.removeCallbacks(poll)
        cards.clear()
        val content = page()
        content.addView(brand())
        content.addView(text("Connect to your PC", 22f, Theme.text1, true), lp().apply { topMargin = dp(28) })
        content.addView(
            text(
                "On the PC open TauShell, go to Home > Phone, turn the phone server on and press Add phone. Scan the QR code here, or paste the link.",
                14f, Theme.text6,
            ),
            lp().apply { topMargin = dp(8) },
        )
        val scan = button("Scan QR code", false) { scanQr() }
        content.addView(scan, lp().apply { topMargin = dp(20) })
        content.addView(eyebrow("Or paste the pairing link"), lp().apply { topMargin = dp(24) })
        val link = field("taushell://pair?...").also { linkField = it }
        content.addView(link, lp().apply { topMargin = dp(8) })
        content.addView(eyebrow("Name shown on the PC"), lp().apply { topMargin = dp(16) })
        val name = field("Phone name").also { nameField = it }
        name.setText(Build.MODEL ?: "Android phone")
        content.addView(name, lp().apply { topMargin = dp(8) })
        content.addView(button("Pair", true) { pair(link.text.toString()) }, lp().apply { topMargin = dp(16) })
        val status = text("", 13f, Theme.text6).also { pairStatus = it }
        content.addView(status, lp().apply { topMargin = dp(14) })
        pendingLink?.let {
            link.setText(it)
            status.text = "Link received. Check the name and tap Pair."
            pendingLink = null
        }
        swap(scroll(content), Screen.PAIR)
    }

    private fun setPairStatus(message: String, color: Int) {
        pairStatus?.text = message
        pairStatus?.setTextColor(color)
    }

    private fun scanQr() {
        val options = GmsBarcodeScannerOptions.Builder().setBarcodeFormats(Barcode.FORMAT_QR_CODE).build()
        GmsBarcodeScanning.getClient(this, options).startScan()
            .addOnSuccessListener { barcode ->
                val raw = barcode.rawValue ?: ""
                linkField?.setText(raw)
                pair(raw)
            }
            .addOnFailureListener { error ->
                setPairStatus("The QR scanner is not available (${error.message ?: "cancelled"}). Paste the link instead.", Theme.input)
            }
    }

    private fun pair(raw: String) {
        if (pairingInFlight) return
        val link = PairingLink.parse(raw)
        if (link == null) {
            setPairStatus("That is not a TauShell pairing link.", Theme.failed)
            return
        }
        val deviceName = nameField?.text?.toString()?.trim().orEmpty().ifEmpty { "Android phone" }
        pairingInFlight = true
        setPairStatus("Waiting for you to approve “$deviceName” on the PC…", Theme.warn)
        io.execute {
            val outcome = PcClient(link.host, link.port, link.fingerprint, "pair.taushell").pair(link.code, deviceName)
            ui.post {
                pairingInFlight = false
                when (outcome) {
                    is Result.Ok -> {
                        val paired = Pairing(link.host, link.port, outcome.value.deviceId, outcome.value.fingerprint, outcome.value.token, outcome.value.expires)
                        store.save(paired)
                        pairing = paired
                        showHome()
                    }
                    is Result.Fail -> setPairStatus(outcome.message, Theme.failed)
                }
            }
        }
    }

    // Home

    private fun showHome() {
        val current = pairing ?: return showPairing()
        cards.clear()
        session = null
        openId = null
        val gear = iconButton(R.drawable.ic_gear, "Settings") { showSettings() }
        val header = linear(this).apply {
            setPadding(dp(18), dp(16), dp(18), dp(8))
            addView(brand(gear))
            addView(text("${current.host}:${current.port}", 12f, Theme.text10), lp().apply { topMargin = dp(6) })
            val bannerView = text("", 13f, Theme.dangerText).apply {
                background = shape(Theme.dangerBg, 8, Theme.failed)
                setPadding(dp(12), dp(10), dp(12), dp(10))
                visibility = View.GONE
            }.also { banner = it }
            addView(bannerView, lp().apply { topMargin = dp(14) })
            addView(eyebrow("Sessions"), lp().apply { topMargin = dp(14) })
        }
        val content = linear(this).apply { setPadding(dp(18), dp(4), dp(18), dp(24)) }
        val container = linear(this).also { list = it }
        content.addView(container, lp())
        val emptyView = text("Loading sessions…", 14f, Theme.text10).also { empty = it }
        content.addView(emptyView, lp().apply { topMargin = dp(4) })
        val page = linear(this).apply {
            addView(header, lp())
            addView(ScrollView(this@MainActivity).apply { addView(content) }, lp(height = 0, weight = 1f))
        }
        swap(page, Screen.HOME)
        ui.removeCallbacks(poll)
        ui.post(poll)
    }

    private fun showSession(item: SessionItem) {
        cards.clear()
        openId = item.id
        pendingOpen = null
        val view = SessionScreen(this, this) { showHome() }
        session = view
        view.bind(item)
        swap(view, Screen.SESSION)
        ui.removeCallbacks(poll)
        ui.post(poll)
    }

    private fun refresh() {
        val current = pairing ?: return
        if (refreshing) return
        refreshing = true
        io.execute {
            val outcome = current.client().sessions()
            // An older PC without the route fails here; the phone's own replies still work.
            if (outcome is Result.Ok) (current.client().presets() as? Result.Ok)?.let { pcPresets = it.value }
            ui.post {
                refreshing = false
                if (screen == Screen.HOME) apply(outcome) else if (screen == Screen.SESSION) applyToSession(outcome)
            }
        }
    }

    private fun applyToSession(outcome: Result<List<SessionItem>>) {
        val open = session ?: return
        if (outcome is Result.Ok) {
            outcome.value.forEach { StateClock.observe(it) }
            latest = outcome.value
            val target = pendingOpen?.let { id -> outcome.value.firstOrNull { it.id == id } }
            if (target != null && target.id != openId) return showSession(target)
            pendingOpen = null
            outcome.value.firstOrNull { it.id == openId }?.let { open.bind(it) } ?: showHome()
        } else if (outcome is Result.Fail) {
            Toast.makeText(this, outcome.message, Toast.LENGTH_SHORT).show()
        }
    }

    private fun apply(outcome: Result<List<SessionItem>>) {
        when (outcome) {
            is Result.Fail -> {
                banner?.text = outcome.message + if (outcome.code == "unauthorized" || outcome.code == "pin_mismatch") "\nOpen Settings to forget this PC and pair again." else ""
                banner?.visibility = View.VISIBLE
                if (cards.isEmpty()) empty?.text = "No connection."
            }
            is Result.Ok -> {
                banner?.visibility = View.GONE
                val items = outcome.value
                items.forEach { StateClock.observe(it) }
                latest = items
                val container = list ?: return
                (cards.keys - items.map { it.id }.toSet()).forEach { id -> cards.remove(id)?.let { container.removeView(it) } }
                items.forEachIndexed { index, item ->
                    val card = cards.getOrPut(item.id) {
                        SessionCard(this) { showSession(it) }.also { container.addView(it, lp().apply { bottomMargin = dp(10) }) }
                    }
                    card.bind(item)
                    if (container.getChildAt(index) !== card) {
                        container.removeView(card)
                        container.addView(card, index, lp().apply { bottomMargin = dp(10) })
                    }
                }
                pendingOpen?.let { id -> items.firstOrNull { it.id == id }?.let { showSession(it); return } }
                empty?.text = if (items.isEmpty()) "No sessions on this PC yet." else ""
                empty?.visibility = if (items.isEmpty()) View.VISIBLE else View.GONE
            }
        }
    }

    // SessionCard.Actions

    override fun send(item: SessionItem, text: String, insertOnly: Boolean, done: (Result<Unit>) -> Unit) {
        val current = pairing ?: return
        io.execute {
            val outcome = current.client().send(item.id, text, insertOnly)
            ui.post {
                done(outcome)
                refresh()
            }
        }
    }

    override fun start(item: SessionItem, mode: String, done: (Result<Unit>) -> Unit) {
        val current = pairing ?: return
        io.execute {
            val outcome = current.client().start(item.id, mode)
            ui.post {
                done(outcome)
                refresh()
            }
        }
    }

    override fun screen(item: SessionItem, done: (Result<List<String>>) -> Unit) {
        val current = pairing ?: return
        io.execute {
            val outcome = current.client().screen(item.id, 120)
            ui.post { done(outcome) }
        }
    }

    override fun quickReplies(): List<String> = (store.presets + pcPresets).distinct()

    override fun sessionPresets(item: SessionItem, done: (Result<List<PromptPreset>>) -> Unit) {
        val current = pairing ?: return
        io.execute {
            val outcome = current.client().sessionPresets(item.id)
            ui.post { done(outcome) }
        }
    }

    override fun promptHistory(id: String): List<String> = store.promptHistory[id].orEmpty()

    override fun rememberPrompt(id: String, text: String) {
        store.promptHistory = store.promptHistory + (id to PromptHistory.add(promptHistory(id), text))
    }

    override fun muted(id: String) = isSessionMuted(store.alerts, id in store.muted)

    override fun setMuted(id: String, value: Boolean, done: (Result<Unit>) -> Unit) {
        if (value) {
            store.muted = store.muted + id
            done(Result.Ok(Unit))
            return
        }
        enableAlerts(
            onGranted = {
                store.muted = store.muted - id
                done(Result.Ok(Unit))
            },
            onDenied = {
                done(Result.Fail("permission_denied", "Allow Android notification permission to unmute this session."))
            },
        )
    }

    // Settings

    private fun showSettings() {
        val current = pairing ?: return
        ui.removeCallbacks(poll)
        cards.clear()
        val content = page()
        content.addView(brand(iconButton(R.drawable.ic_close, "Close settings") { showHome() }))
        content.addView(eyebrow("This PC"), lp().apply { topMargin = dp(24) })
        content.addView(text("${current.host}:${current.port}", 15f, Theme.text1), lp().apply { topMargin = dp(6) })
        content.addView(text("Pairing expires ${current.expires.ifEmpty { "unknown" }}. Pair again to renew.", 12f, Theme.text10), lp().apply { topMargin = dp(2) })

        content.addView(eyebrow("Alerts"), lp().apply { topMargin = dp(24) })
        val alerts = Switch(this).apply {
            text = "Notify when a session needs an answer or finishes"
            setTextColor(Theme.text4)
            isChecked = store.alerts
            setOnCheckedChangeListener { _, checked -> setAlerts(checked, this) }
        }
        content.addView(alerts, lp().apply { topMargin = dp(8) })
        content.addView(
            text("Keeps a quiet notification running and checks every few seconds. Turn on battery “Unrestricted” for TauShell Phone if alerts stop.", 12f, Theme.text10),
            lp().apply { topMargin = dp(4) },
        )

        content.addView(eyebrow("Quick replies"), lp().apply { topMargin = dp(24) })
        val presetField = field("One reply per line", multiline = true).apply { setText(store.presets.joinToString("\n")); minLines = 4 }
        content.addView(presetField, lp().apply { topMargin = dp(8) })
        content.addView(button("Save replies", false) { store.presets = presetField.text.toString().lines().map { it.trim() }; presetField.setText(store.presets.joinToString("\n")) }, lp().apply { topMargin = dp(8) })

        content.addView(eyebrow("Pairing"), lp().apply { topMargin = dp(24) })
        val forget = button("Forget this PC", false) { forgetPc() }.apply {
            setTextColor(Theme.dangerText)
            background = shape(Theme.dangerBg, 8, Theme.failed)
        }
        content.addView(forget, lp().apply { topMargin = dp(8) })
        content.addView(text("Also revoke this phone in TauShell on the PC (Home > Phone).", 12f, Theme.text10), lp().apply { topMargin = dp(6) })
        val version = text("v${installedVersionName()}", 11f, Theme.text10).apply { gravity = Gravity.END }
        val settings = linear(this).apply {
            addView(scroll(content), lp(height = 0, weight = 1f))
            addView(version, lp().apply { leftMargin = dp(18); rightMargin = dp(18); topMargin = dp(4); bottomMargin = dp(10) })
        }
        swap(settings, Screen.SETTINGS)
    }

    @Suppress("DEPRECATION")
    private fun installedVersionName(): String = packageManager.getPackageInfo(packageName, 0).versionName ?: "unknown"

    private fun setAlerts(on: Boolean, toggle: Switch) {
        if (!on) {
            store.alerts = false
            MonitorService.stop(this)
            setAlertsToggle(toggle, false)
            return
        }
        enableAlerts(
            toggle = toggle,
            onGranted = { setAlertsToggle(toggle, true) },
            onDenied = {
                setAlertsToggle(toggle, false)
                Toast.makeText(this, "Allow Android notification permission to enable alerts.", Toast.LENGTH_SHORT).show()
            },
        )
    }

    private fun enableAlerts(
        toggle: Switch? = null,
        onGranted: () -> Unit,
        onDenied: () -> Unit,
    ) {
        if (Build.VERSION.SDK_INT >= 33 &&
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            pendingAlertsEnable = PendingAlertsEnable(toggle, onGranted, onDenied)
            toggle?.isEnabled = false
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), REQUEST_POST_NOTIFICATIONS)
            return
        }
        store.alerts = true
        MonitorService.start(this)
        setAlertsToggle(toggle, true)
        onGranted()
    }

    private fun setAlertsToggle(toggle: Switch?, enabled: Boolean) {
        if (toggle == null) return
        toggle.setOnCheckedChangeListener(null)
        toggle.isEnabled = true
        toggle.isChecked = enabled
        toggle.setOnCheckedChangeListener { _, checked -> setAlerts(checked, toggle) }
    }

    @Suppress("DEPRECATION")
    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode != REQUEST_POST_NOTIFICATIONS) return
        val pending = pendingAlertsEnable ?: return
        pendingAlertsEnable = null
        val granted = Build.VERSION.SDK_INT < 33 ||
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED
        if (granted) {
            store.alerts = true
            MonitorService.start(this)
            setAlertsToggle(pending.toggle, true)
            pending.onGranted()
        } else {
            store.alerts = false
            MonitorService.stop(this)
            setAlertsToggle(pending.toggle, false)
            pending.onDenied()
        }
    }

    private fun forgetPc() {
        MonitorService.stop(this)
        store.alerts = false
        store.forget()
        pairing = null
        showPairing()
    }

    override fun onDestroy() {
        ui.removeCallbacksAndMessages(null)
        super.onDestroy()
    }
}
