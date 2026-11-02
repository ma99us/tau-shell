package com.taushell.phone

import android.content.Context
import android.graphics.Color
import android.view.Gravity
import android.view.View
import android.view.inputmethod.InputMethodManager
import android.widget.EditText
import android.widget.HorizontalScrollView
import android.widget.LinearLayout
import android.widget.TextView

/** One session tile. It is updated in place so a half-typed prompt survives a refresh. */
class SessionCard(
    private val context: Context,
    private val actions: Actions,
) : LinearLayout(context) {
    interface Actions {
        fun send(item: SessionItem, text: String, insertOnly: Boolean, done: (Result<Unit>) -> Unit)
        fun start(item: SessionItem, mode: String, done: (Result<Unit>) -> Unit)
        fun screen(item: SessionItem, done: (Result<List<String>>) -> Unit)
        fun presets(): List<String>
    }

    lateinit var item: SessionItem
        private set

    private val icon = TextView(context)
    private val name = context.text("", 15f, Theme.text1, true)
    private val subtitle = context.text("", 12f, Theme.text10)
    private val stateDot = context.dot(Theme.text10)
    private val stateText = context.text("", 12f, Theme.text10, true)
    private val body = linear(context)
    private val notice = context.text("", 12f, Theme.input)
    private val chips = linear(context, vertical = false)
    private val prompt: EditText = context.field("Type a prompt for this session", multiline = true)
    private val result = context.text("", 12f, Theme.text6)
    private val screenView = context.text("", 11f, Theme.text6).apply {
        typeface = android.graphics.Typeface.MONOSPACE
        setTextIsSelectable(true)
        setPadding(context.dp(10), context.dp(8), context.dp(10), context.dp(8))
        background = context.shape(Theme.bgControl, 8, Theme.border)
    }
    private val screenScroll = HorizontalScrollView(context).apply { addView(screenView) }
    private var screenBusy = false
    private var shownPresets: List<String>? = null
    private var replyViews: List<View> = emptyList()
    private var expanded = false
    private var busy = false

    init {
        orientation = VERTICAL
        background = context.shape(Theme.bgRaised, 10, Theme.border)
        setPadding(context.dp(14), context.dp(14), context.dp(14), context.dp(14))

        icon.gravity = Gravity.CENTER
        icon.typeface = android.graphics.Typeface.DEFAULT_BOLD

        val titles = linear(context).apply {
            addView(name)
            addView(subtitle)
        }
        val state = context.row(stateDot, stateText, gap = 6).apply {
            background = context.shape(Theme.bgControl, 20, Theme.border)
            setPadding(context.dp(10), context.dp(5), context.dp(10), context.dp(5))
        }
        val header = LinearLayout(context).apply {
            orientation = HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            addView(icon, lp(context.dp(40), context.dp(40)))
            addView(titles, lp(0, weight = 1f).apply { leftMargin = context.dp(12); rightMargin = context.dp(8) })
            addView(state, lp(ViewGroupWrap, ViewGroupWrap))
        }
        addView(header)
        header.setOnClickListener { setExpanded(!expanded) }

        val scroller = HorizontalScrollView(context).apply {
            isHorizontalScrollBarEnabled = false
            addView(chips)
        }
        val send = context.button("Send", true) { submit(false) }
        send.setOnLongClickListener { submit(true); true }
        val promptRow = LinearLayout(context).apply {
            orientation = HORIZONTAL
            gravity = Gravity.BOTTOM
            addView(prompt, lp(0, weight = 1f))
            addView(send, lp(ViewGroupWrap, ViewGroupWrap).apply { leftMargin = context.dp(8) })
        }
        prompt.maxLines = 5
        val hint = context.text("Hold Send to type the text without pressing Enter.", 11f, Theme.text11)
        replyViews = listOf(scroller, promptRow, hint)
        body.addView(notice, lp().apply { topMargin = context.dp(12) })
        body.addView(screenScroll, lp().apply { topMargin = context.dp(12) })
        body.addView(scroller, lp().apply { topMargin = context.dp(12) })
        body.addView(promptRow, lp().apply { topMargin = context.dp(10) })
        body.addView(hint, lp().apply { topMargin = context.dp(6) })
        body.addView(result, lp().apply { topMargin = context.dp(6) })
        addView(body)
    }

    fun bind(next: SessionItem) {
        item = next
        name.text = next.name
        subtitle.text = listOf(Labels.tool(next.tool), Labels.folder(next.workingDir)).filter { it.isNotBlank() }.joinToString("  ·  ")
        stateText.text = Labels.state(next)
        stateText.setTextColor(Labels.stateColor(next))
        stateDot.background = context.shape(Labels.stateColor(next), 4)

        val tint = runCatching { Color.parseColor(next.color) }.getOrNull() ?: Theme.bgControl
        icon.background = context.shape(tint, 8)
        icon.text = Labels.initials(next)
        icon.setTextColor(
            when {
                next.color.isBlank() -> Theme.text1
                luminance(tint) > 0.3 -> Color.parseColor("#101318")
                else -> Color.WHITE
            },
        )
        icon.textSize = if (icon.text.length > 2) 11f else 15f

        renderBody()
    }

    private fun renderBody() {
        val running = item.running
        body.visibility = if (expanded || (running && item.activity.state == "input")) VISIBLE else GONE
        val waiting = running && item.activity.state == "input" && item.activity.confirmed
        notice.visibility = if (waiting || !running || item.activity.state == "unknown") VISIBLE else GONE
        notice.text = when {
            !running -> if (item.lastError.isNotBlank()) "Not running. Last error: ${item.lastError}" else "Not running."
            waiting -> "Waiting for an answer. Answer it on the PC; replies from here are blocked until then."
            item.activity.state == "unknown" -> "State unknown. Only Claude Code and Copilot CLI report it, so replies are blocked."
            else -> ""
        }
        notice.setTextColor(if (waiting) Theme.input else Theme.text10)

        val shownNow = actions.presets()
        if (shownNow != shownPresets) {
            shownPresets = shownNow
            chips.removeAllViews()
            shownNow.forEach { preset ->
                chips.addView(
                    context.button(preset, false) { submitText(preset, false) }.apply {
                        minHeight = context.dp(34); minimumHeight = context.dp(34)
                        setPadding(context.dp(12), 0, context.dp(12), 0)
                    },
                    lp(ViewGroupWrap, ViewGroupWrap).apply { rightMargin = context.dp(8) },
                )
            }
        }
        val canType = running
        replyViews.forEach { it.visibility = if (canType) VISIBLE else GONE }
        if (!running) renderStart() else startRow?.visibility = GONE
        screenScroll.visibility = if (running && expanded && screenView.text.isNotEmpty()) VISIBLE else GONE
        if (running && expanded) fetchScreen()
    }

    /** Shown only while the card is open; the text is read-only and never kept. */
    private fun fetchScreen() {
        if (screenBusy) return
        screenBusy = true
        actions.screen(item) { outcome ->
            screenBusy = false
            when (outcome) {
                is Result.Ok -> screenView.text = outcome.value.joinToString("\n").ifEmpty { "(the terminal is empty)" }
                is Result.Fail -> screenView.text = if (outcome.code == "disabled") "Screen sharing is off on the PC (mobile.screen: false)." else outcome.message
            }
            screenScroll.visibility = if (expanded && item.running) VISIBLE else GONE
        }
    }

    private var startRow: LinearLayout? = null

    private fun renderStart() {
        if (startRow == null) {
            val resume = context.button("Resume", true) { startSession("restore") }
            val fresh = context.button("Start fresh", false) { startSession("fresh") }
            startRow = LinearLayout(context).apply {
                orientation = HORIZONTAL
                addView(resume, lp(0, weight = 1f))
                addView(fresh, lp(0, weight = 1f).apply { leftMargin = context.dp(8) })
            }
            body.addView(startRow, 1, lp().apply { topMargin = context.dp(12) })
        }
        startRow?.visibility = VISIBLE
    }

    private fun setExpanded(value: Boolean) {
        expanded = value
        if (::item.isInitialized) renderBody()
    }

    private fun submit(insertOnly: Boolean) = submitText(prompt.text.toString().trim(), insertOnly, clear = true)

    private fun submitText(text: String, insertOnly: Boolean, clear: Boolean = false) {
        if (text.isEmpty() || busy) return
        busy = true
        show("Sending…", Theme.text6)
        actions.send(item, text, insertOnly) { outcome ->
            busy = false
            when (outcome) {
                is Result.Ok -> {
                    if (clear) prompt.setText("")
                    show(if (insertOnly) "Typed into the session. Press Enter on the PC to submit." else "Sent.", Theme.ok)
                    (context.getSystemService(Context.INPUT_METHOD_SERVICE) as InputMethodManager).hideSoftInputFromWindow(prompt.windowToken, 0)
                }
                is Result.Fail -> show(outcome.message, if (outcome.code == "refused") Theme.input else Theme.failed)
            }
        }
    }

    private fun startSession(mode: String) {
        if (busy) return
        busy = true
        show("Asking the PC to start the session…", Theme.text6)
        actions.start(item, mode) { outcome ->
            busy = false
            when (outcome) {
                is Result.Ok -> show("Requested. The session appears as running when it is up.", Theme.ok)
                is Result.Fail -> show(outcome.message, Theme.failed)
            }
        }
    }

    private fun show(message: String, color: Int) {
        result.text = message
        result.setTextColor(color)
        result.visibility = VISIBLE
        if (!expanded) setExpanded(true)
    }

    private fun luminance(color: Int): Double {
        fun channel(value: Int): Double {
            val c = value / 255.0
            return if (c <= 0.03928) c / 12.92 else Math.pow((c + 0.055) / 1.055, 2.4)
        }
        return 0.2126 * channel(Color.red(color)) + 0.7152 * channel(Color.green(color)) + 0.0722 * channel(Color.blue(color))
    }

    private companion object {
        const val ViewGroupWrap = LayoutParams.WRAP_CONTENT
    }
}
