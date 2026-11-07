package com.taushell.phone

import android.content.Context
import android.graphics.Color
import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.os.SystemClock
import android.text.InputType
import android.util.TypedValue
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.widget.Button
import android.widget.EditText
import android.widget.ImageButton
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.TextView

/** Colors copied from the desktop app's theme tokens (frontend/src/theme/tokens.css). */
object Theme {
    val bgApp = Color.parseColor("#0c0e12")
    val bgPanel = Color.parseColor("#11141a")
    val bgCard = Color.parseColor("#14181f")
    val bgRaised = Color.parseColor("#171b22")
    val bgControl = Color.parseColor("#1b2028")
    val border = Color.parseColor("#292e37")
    val borderStrong = Color.parseColor("#343a45")
    val text1 = Color.parseColor("#f4f6fa")
    val text4 = Color.parseColor("#c0c6d0")
    val text6 = Color.parseColor("#9ba4b2")
    val text10 = Color.parseColor("#7e8896")
    val text11 = Color.parseColor("#717986")
    val accent = Color.parseColor("#78d49f")
    val accentBg = Color.parseColor("#1a2821")
    val accentBorder = Color.parseColor("#2f4a3d")
    val action = Color.parseColor("#3ddc84")
    val onAction = Color.parseColor("#07210f")
    val ok = Color.parseColor("#63d69a")
    val warn = Color.parseColor("#e0b24d")
    val input = Color.parseColor("#ff9f43")
    val failed = Color.parseColor("#e06c75")
    val dangerBg = Color.parseColor("#2a1b1b")
    val dangerText = Color.parseColor("#ffb4b4")

    val swatches = mapOf(
        "red" to "#e5534b", "orange" to "#d9822b", "yellow" to "#c9a227", "green" to "#4c9f58",
        "teal" to "#2fa4a4", "blue" to "#4f8cff", "purple" to "#9b73e6", "pink" to "#d65a9b",
    )
}

fun Context.dp(value: Int): Int = (value * resources.displayMetrics.density + 0.5f).toInt()

fun Context.shape(fill: Int, radiusDp: Int, stroke: Int? = null): GradientDrawable = GradientDrawable().apply {
    setColor(fill)
    cornerRadius = dp(radiusDp).toFloat()
    if (stroke != null) setStroke(dp(1), stroke)
}

fun Context.text(value: String, sizeSp: Float, color: Int, bold: Boolean = false): TextView = TextView(this).apply {
    text = value
    setTextSize(TypedValue.COMPLEX_UNIT_SP, sizeSp)
    setTextColor(color)
    if (bold) typeface = Typeface.DEFAULT_BOLD
}

/** Small uppercase label, like the desktop eyebrows. */
fun Context.eyebrow(value: String): TextView = text(value.uppercase(), 11f, Theme.accent, true).apply { letterSpacing = 0.12f }

fun Context.button(label: String, primary: Boolean, onClick: () -> Unit): Button = Button(this).apply {
    text = label
    isAllCaps = false
    stateListAnimator = null
    minHeight = dp(40)
    minimumHeight = dp(40)
    setPadding(dp(14), 0, dp(14), 0)
    setTextSize(TypedValue.COMPLEX_UNIT_SP, 14f)
    typeface = Typeface.DEFAULT_BOLD
    if (primary) {
        setTextColor(Theme.onAction)
        background = shape(Theme.action, 8)
    } else {
        setTextColor(Theme.text4)
        background = shape(Theme.bgControl, 8, Theme.border)
    }
    setOnClickListener { onClick() }
}

fun Context.iconButton(icon: Int, description: String, onClick: () -> Unit): ImageButton = ImageButton(this).apply {
    contentDescription = description
    tooltipText = description
    setImageResource(icon)
    setColorFilter(Theme.text4)
    scaleType = ImageView.ScaleType.CENTER
    setPadding(dp(12), dp(12), dp(12), dp(12))
    minimumWidth = dp(48)
    minimumHeight = dp(48)
    stateListAnimator = null
    background = shape(Theme.bgControl, 8, Theme.border)
    setOnClickListener { onClick() }
}

fun Context.setIconButtonActive(button: ImageButton, active: Boolean) {
    button.isSelected = active
    button.setColorFilter(if (active) Theme.accent else Theme.text4)
    button.background = shape(
        if (active) Theme.accentBg else Theme.bgControl,
        8,
        if (active) Theme.accentBorder else Theme.border,
    )
}

fun Context.field(hint: String, multiline: Boolean = false): EditText = EditText(this).apply {
    this.hint = hint
    setHintTextColor(Theme.text11)
    setTextColor(Theme.text1)
    setTextSize(TypedValue.COMPLEX_UNIT_SP, 14f)
    background = shape(Theme.bgApp, 8, Theme.border)
    setPadding(dp(12), dp(10), dp(12), dp(10))
    inputType = if (multiline) {
        InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_FLAG_MULTI_LINE or InputType.TYPE_TEXT_FLAG_CAP_SENTENCES
    } else {
        InputType.TYPE_CLASS_TEXT
    }
}

fun Context.dot(color: Int): View = View(this).apply {
    background = GradientDrawable().apply { shape = GradientDrawable.OVAL; setColor(color) }
    layoutParams = LinearLayout.LayoutParams(dp(8), dp(8)).apply { gravity = Gravity.CENTER_VERTICAL }
}

fun Context.row(vararg children: View, gap: Int = 8): LinearLayout = LinearLayout(this).apply {
    orientation = LinearLayout.HORIZONTAL
    gravity = Gravity.CENTER_VERTICAL
    children.forEachIndexed { index, child ->
        addView(child)
        if (index > 0) (child.layoutParams as? ViewGroup.MarginLayoutParams)?.leftMargin = dp(gap)
    }
}

fun linear(context: Context, vertical: Boolean = true): LinearLayout =
    LinearLayout(context).apply { orientation = if (vertical) LinearLayout.VERTICAL else LinearLayout.HORIZONTAL }

fun lp(width: Int = ViewGroup.LayoutParams.MATCH_PARENT, height: Int = ViewGroup.LayoutParams.WRAP_CONTENT, weight: Float = 0f) =
    LinearLayout.LayoutParams(width, height, weight)

/** Display strings shared by the screen and the notifications. */
object Labels {
    fun tool(id: String) = when (id) {
        "claude-code" -> "Claude Code"
        "copilot-cli" -> "Copilot CLI"
        "gsd" -> "GSD"
        "codex" -> "Codex CLI"
        "opencode" -> "OpenCode"
        "powershell", "" -> "PowerShell"
        else -> id
    }

    fun folder(workingDir: String): String = workingDir.split('\\', '/').lastOrNull { it.isNotBlank() } ?: ""

    fun initials(item: SessionItem): String {
        val letters = folder(item.workingDir).split(Regex("[-_. ]+"))
            .mapNotNull { word -> word.firstOrNull { it.isLetterOrDigit() } }.joinToString("").uppercase()
        return letters.ifEmpty { item.name.firstOrNull { it.isLetterOrDigit() }?.uppercase() ?: "?" }
    }

    fun totalSpent(item: SessionItem): String? =
        item.totalUsd?.takeIf { it.isFinite() }?.let {
            val amount = if (it <= 0 && item.unpriced) "?" else String.format(java.util.Locale.US, "$%.2f", it)
            "Est. $amount${if (item.unpriced && amount != "?") "+" else ""}"
        }

    /** Short state text; an unconfirmed reading is marked so it is never taken as fact. */
    fun state(item: SessionItem): String {
        if (!item.running) return "Stopped"
        val base = when (item.activity.state) {
            "working" -> "Working"
            "input" -> "Needs answer"
            "idle" -> "Idle"
            "stopped" -> "Stopped"
            else -> "Unknown"
        }
        return if (item.activity.state in listOf("working", "input", "idle") && !item.activity.confirmed) "$base?" else base
    }

    fun stateColor(item: SessionItem): Int {
        if (!item.running) return Theme.text11
        if (!item.activity.confirmed) return Theme.text10
        return when (item.activity.state) {
            "working" -> Theme.warn
            "input" -> Theme.input
            "idle" -> Theme.ok
            else -> Theme.text10
        }
    }
}

/** Remembers when each session entered its state, so the phone can say how long it has been thinking. */
object StateClock {
    private val seen = HashMap<String, Pair<String, Long>>()
    private val finished = HashMap<String, Long>()

    private fun key(item: SessionItem) = if (item.running) item.activity.state else "stopped"

    fun observe(item: SessionItem, now: Long = SystemClock.elapsedRealtime()) {
        val next = key(item)
        val previous = seen[item.id]
        val since = item.activity.sinceMs?.coerceAtLeast(0)?.let { now - it }
            ?: if (previous?.first == next) previous.second else now
        if (previous?.first == "working" && next == "idle") finished[item.id] = since
        seen[item.id] = next to since
    }

    fun elapsed(ms: Long): String {
        val seconds = ms / 1000
        return when {
            seconds < 60 -> "${seconds}s"
            seconds < 3600 -> "${seconds / 60}m ${seconds % 60}s"
            else -> "${seconds / 3600}h ${seconds % 3600 / 60}m"
        }
    }

    /** Text for the state pill and the session header. */
    fun headline(item: SessionItem, now: Long = SystemClock.elapsedRealtime()): String {
        val confirmed = item.running && item.activity.confirmed
        val seenState = seen[item.id]
        val since = seenState?.second?.takeIf { seenState.first == key(item) }
        return when {
            confirmed && item.activity.state == "working" -> "Thinking…" + (since?.let { " ${elapsed(now - it)}" } ?: "")
            confirmed && item.activity.state == "idle" -> finished[item.id]?.let { "Idle · done ${elapsed(now - it)} ago" } ?: "Idle"
            else -> Labels.state(item)
        }
    }
}

/** Fades a view in and out while [on], e.g. the state dot of a working session. */
fun View.pulse(on: Boolean) {
    val running = getTag(R.id.pulse) as? android.animation.ObjectAnimator
    if (on && running == null) {
        val animator = android.animation.ObjectAnimator.ofFloat(this, View.ALPHA, 1f, 0.25f).apply {
            duration = 800
            repeatMode = android.animation.ValueAnimator.REVERSE
            repeatCount = android.animation.ValueAnimator.INFINITE
            start()
        }
        setTag(R.id.pulse, animator)
    } else if (!on && running != null) {
        running.cancel()
        alpha = 1f
        setTag(R.id.pulse, null)
    }
}