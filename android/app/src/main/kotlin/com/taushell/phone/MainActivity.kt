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
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.codescanner.GmsBarcodeScannerOptions
import com.google.mlkit.vision.codescanner.GmsBarcodeScanning
import java.util.concurrent.Executors

class MainActivity : Activity(), SessionCard.Actions {
    private lateinit var store: Store
    private lateinit var root: FrameLayout
    private val io = Executors.newCachedThreadPool()
    private val ui = Handler(Looper.getMainLooper())

    private enum class Screen { PAIR, HOME, SETTINGS }

    private var screen = Screen.PAIR
    private var pairing: Pairing? = null
    private val cards = LinkedHashMap<String, SessionCard>()
    private var list: LinearLayout? = null
    private var banner: TextView? = null
    private var empty: TextView? = null
    private var refreshing = false
    private var pairingInFlight = false
    private var pairStatus: TextView? = null
    private var linkField: android.widget.EditText? = null
    private var nameField: android.widget.EditText? = null
    private var pendingLink: String? = null

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
        pairing = store.pairing()
        if (pairing != null && pendingLink == null) showHome() else showPairing()
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        intent.dataString?.let {
            pendingLink = it
            if (screen == Screen.PAIR) linkField?.setText(it) else showPairing()
        }
    }

    override fun onResume() {
        super.onResume()
        if (screen == Screen.HOME) {
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
        if (screen == Screen.SETTINGS) showHome() else @Suppress("DEPRECATION") super.onBackPressed()
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
        val content = page()
        val gear = button("Settings", false) { showSettings() }
        content.addView(brand(gear))
        content.addView(text("${current.host}:${current.port}", 12f, Theme.text10), lp().apply { topMargin = dp(6) })
        val bannerView = text("", 13f, Theme.dangerText).apply {
            background = shape(Theme.dangerBg, 8, Theme.failed)
            setPadding(dp(12), dp(10), dp(12), dp(10))
            visibility = View.GONE
        }.also { banner = it }
        content.addView(bannerView, lp().apply { topMargin = dp(14) })
        content.addView(eyebrow("Sessions"), lp().apply { topMargin = dp(20) })
        val container = linear(this).also { list = it }
        content.addView(container, lp().apply { topMargin = dp(10) })
        val emptyView = text("Loading sessions…", 14f, Theme.text10).also { empty = it }
        content.addView(emptyView, lp().apply { topMargin = dp(4) })
        swap(scroll(content), Screen.HOME)
        ui.removeCallbacks(poll)
        ui.post(poll)
    }

    private fun refresh() {
        val current = pairing ?: return
        if (refreshing) return
        refreshing = true
        io.execute {
            val outcome = current.client().sessions()
            ui.post {
                refreshing = false
                if (screen == Screen.HOME) apply(outcome)
            }
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
                val container = list ?: return
                (cards.keys - items.map { it.id }.toSet()).forEach { id -> cards.remove(id)?.let { container.removeView(it) } }
                items.forEachIndexed { index, item ->
                    val card = cards.getOrPut(item.id) {
                        SessionCard(this, this).also { container.addView(it, lp().apply { bottomMargin = dp(10) }) }
                    }
                    card.bind(item)
                    if (container.getChildAt(index) !== card) {
                        container.removeView(card)
                        container.addView(card, index, lp().apply { bottomMargin = dp(10) })
                    }
                }
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
            val outcome = current.client().screen(item.id, 40)
            ui.post { done(outcome) }
        }
    }

    override fun presets(): List<String> = store.presets

    // Settings

    private fun showSettings() {
        val current = pairing ?: return
        ui.removeCallbacks(poll)
        cards.clear()
        val content = page()
        content.addView(brand(button("Done", false) { showHome() }))
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
        swap(scroll(content), Screen.SETTINGS)
    }

    private fun setAlerts(on: Boolean, toggle: Switch) {
        if (on && Build.VERSION.SDK_INT >= 33 && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 1)
        }
        store.alerts = on
        if (on) MonitorService.start(this) else MonitorService.stop(this)
        toggle.isChecked = on
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
