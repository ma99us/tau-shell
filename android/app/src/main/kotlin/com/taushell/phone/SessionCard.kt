package com.taushell.phone

import android.content.Context
import android.graphics.Color
import android.view.Gravity
import android.view.View
import android.widget.LinearLayout
import android.widget.TextView

/** Actions the session screens need from the activity. */
interface SessionActions {
    fun send(item: SessionItem, text: String, insertOnly: Boolean, done: (Result<Unit>) -> Unit)
    fun start(item: SessionItem, mode: String, done: (Result<Unit>) -> Unit)
    fun screen(item: SessionItem, done: (Result<List<String>>) -> Unit)
    fun answer(item: SessionItem, answer: String, done: (Result<Unit>) -> Unit)
    fun presets(): List<String>
    fun muted(id: String): Boolean
    fun setMuted(id: String, value: Boolean)
}

/** Colors and initials for a session icon, shared by the tile and the session header. */
fun styleIcon(context: Context, icon: TextView, item: SessionItem) {
    val tint = runCatching { Color.parseColor(item.color) }.getOrNull() ?: Theme.bgControl
    icon.gravity = Gravity.CENTER
    icon.typeface = android.graphics.Typeface.DEFAULT_BOLD
    icon.background = context.shape(tint, 8)
    icon.text = Labels.initials(item)
    icon.setTextColor(
        when {
            item.color.isBlank() -> Theme.text1
            luminance(tint) > 0.3 -> Color.parseColor("#101318")
            else -> Color.WHITE
        },
    )
    icon.textSize = if (icon.text.length > 2) 11f else 15f
}

private fun luminance(color: Int): Double {
    fun channel(value: Int): Double {
        val c = value / 255.0
        return if (c <= 0.03928) c / 12.92 else Math.pow((c + 0.055) / 1.055, 2.4)
    }
    return 0.2126 * channel(Color.red(color)) + 0.7152 * channel(Color.green(color)) + 0.0722 * channel(Color.blue(color))
}

/** A compact tile on Home; tapping it opens the session screen. */
class SessionCard(
    private val context: Context,
    private val onOpen: (SessionItem) -> Unit,
) : LinearLayout(context) {
    lateinit var item: SessionItem
        private set

    private val icon = TextView(context)
    private val name = context.text("", 15f, Theme.text1, true)
    private val subtitle = context.text("", 12f, Theme.text10)
    private val stateDot = context.dot(Theme.text10)
    private val stateText = context.text("", 12f, Theme.text10, true)
    private val bar = View(context)

    init {
        orientation = HORIZONTAL
        background = context.shape(Theme.bgRaised, 10, Theme.border)
        addView(bar, LayoutParams(context.dp(4), LayoutParams.MATCH_PARENT))

        val titles = linear(context).apply {
            addView(name)
            addView(subtitle)
        }
        val state = context.row(stateDot, stateText, gap = 6).apply {
            background = context.shape(Theme.bgControl, 20, Theme.border)
            setPadding(context.dp(10), context.dp(5), context.dp(10), context.dp(5))
        }
        val content = LinearLayout(context).apply {
            orientation = HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            setPadding(context.dp(12), context.dp(12), context.dp(12), context.dp(12))
            addView(icon, lp(context.dp(40), context.dp(40)))
            addView(titles, lp(0, weight = 1f).apply { leftMargin = context.dp(12); rightMargin = context.dp(8) })
            addView(state, lp(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT))
        }
        addView(content, lp(0, weight = 1f))
        setOnClickListener { if (::item.isInitialized) onOpen(item) }
    }

    fun bind(next: SessionItem) {
        item = next
        name.text = next.name
        subtitle.text = listOf(Labels.tool(next.tool), Labels.folder(next.workingDir)).filter { it.isNotBlank() }.joinToString("  ·  ")
        val color = Labels.stateColor(next)
        stateText.text = StateClock.headline(next)
        stateText.setTextColor(color)
        stateDot.background = context.shape(color, 4)
        stateDot.pulse(next.running && next.activity.confirmed && next.activity.state == "working")
        bar.setBackgroundColor(color)
        styleIcon(context, icon, next)
    }
}
