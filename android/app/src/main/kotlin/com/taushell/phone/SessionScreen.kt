package com.taushell.phone

import android.content.Context
import android.os.Handler
import android.os.Looper
import android.view.Gravity
import android.view.View
import android.view.inputmethod.InputMethodManager
import android.widget.EditText
import android.widget.HorizontalScrollView
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView

/**
 * Full-screen view of one session: fixed top and bottom bars, with only the terminal text between them scrolling.
 * It is updated in place so a half-typed prompt survives the 3 s refresh.
 */
class SessionScreen(
    private val context: Context,
    private val actions: SessionActions,
    private val onBack: () -> Unit,
) : LinearLayout(context) {
    lateinit var item: SessionItem
        private set

    private val handler = Handler(Looper.getMainLooper())
    private val icon = TextView(context)
    private val name = context.text("", 16f, Theme.text1, true)
    private val subtitle = context.text("", 12f, Theme.text10)
    private val mute = context.button("", false) {
        actions.setMuted(item.id, !actions.muted(item.id))
        renderMute()
    }
    private val stateBar = View(context)
    private val stateDot = context.dot(Theme.text10)
    private val stateText = context.text("", 13f, Theme.text10, true)
    private val notice = context.text("", 12f, Theme.text10)
    private val screenView = context.text("", 11f, Theme.text6).apply {
        typeface = android.graphics.Typeface.MONOSPACE
        setTextIsSelectable(true)
        setPadding(context.dp(12), context.dp(10), context.dp(12), context.dp(10))
    }
    private val vertical = ScrollView(context)
    private val chips = linear(context, vertical = false)
    private val chipScroll = HorizontalScrollView(context).apply {
        isHorizontalScrollBarEnabled = false
        addView(chips)
    }
    private val prompt: EditText = context.field("Prompt (hold Send to type only)", multiline = true).apply { maxLines = 5 }
    private val send = context.button("Send", true) { submit(false) }
    private val promptRow = LinearLayout(context)
    private val answerRow = LinearLayout(context)
    private val startRow = LinearLayout(context)
    private val result = context.text("", 12f, Theme.text6)
    private val clearResult = Runnable { result.visibility = View.GONE }

    private var shownPresets: List<String>? = null
    private var screenBusy = false
    private var pinNext = true
    private var busy = false

    init {
        orientation = VERTICAL

        val back = context.button("<", false) { onBack() }.apply { minWidth = context.dp(44); minimumWidth = context.dp(44); setPadding(0, 0, 0, 0); textSize = 20f }
        val titles = linear(context).apply { addView(name); addView(subtitle) }
        val top = LinearLayout(context).apply {
            gravity = Gravity.CENTER_VERTICAL
            setPadding(context.dp(10), context.dp(8), context.dp(10), context.dp(8))
            setBackgroundColor(Theme.bgPanel)
            addView(back, lp(ViewWrap, context.dp(40)))
            addView(icon, lp(context.dp(36), context.dp(36)).apply { leftMargin = context.dp(10) })
            addView(titles, lp(0, weight = 1f).apply { leftMargin = context.dp(10); rightMargin = context.dp(8) })
            addView(mute, lp(ViewWrap, context.dp(36)))
        }
        mute.apply { minHeight = context.dp(36); minimumHeight = context.dp(36); setPadding(context.dp(10), 0, context.dp(10), 0); textSize = 12f }
        addView(top, lp())
        addView(stateBar, lp(height = context.dp(3)))

        val status = linear(context).apply {
            setBackgroundColor(Theme.bgPanel)
            setPadding(context.dp(14), context.dp(8), context.dp(14), context.dp(8))
            addView(context.row(stateDot, stateText, gap = 8))
            addView(notice, lp().apply { topMargin = context.dp(2) })
        }
        addView(status, lp())

        vertical.addView(HorizontalScrollView(context).apply { addView(screenView) })
        vertical.setBackgroundColor(Theme.bgApp)
        addView(vertical, lp(height = 0, weight = 1f))

        answerRow.addView(context.button("Yes", true) { answer("yes") }, lp(0, weight = 1f))
        answerRow.addView(context.button("No", false) { answer("no") }, lp(0, weight = 1f).apply { leftMargin = context.dp(8) })
        startRow.addView(context.button("Resume", true) { startSession("restore") }, lp(0, weight = 1f))
        startRow.addView(context.button("Start fresh", false) { startSession("fresh") }, lp(0, weight = 1f).apply { leftMargin = context.dp(8) })
        send.setOnLongClickListener { submit(true); true }
        promptRow.gravity = Gravity.BOTTOM
        promptRow.addView(prompt, lp(0, weight = 1f))
        promptRow.addView(send, lp(ViewWrap, ViewWrap).apply { leftMargin = context.dp(8) })

        val bottom = linear(context).apply {
            setBackgroundColor(Theme.bgPanel)
            setPadding(context.dp(12), context.dp(8), context.dp(12), context.dp(10))
            addView(result, lp().apply { bottomMargin = context.dp(6) })
            addView(chipScroll, lp().apply { bottomMargin = context.dp(8) })
            addView(promptRow, lp())
            addView(answerRow, lp())
            addView(startRow, lp())
        }
        result.visibility = View.GONE
        addView(bottom, lp())
    }

    fun bind(next: SessionItem) {
        val first = !::item.isInitialized
        item = next
        name.text = next.name
        subtitle.text = listOf(Labels.tool(next.tool), Labels.folder(next.workingDir)).filter { it.isNotBlank() }.joinToString("  ·  ")
        styleIcon(context, icon, next)
        renderMute()
        render()
        if (first || next.running) fetchScreen()
    }

    private fun renderMute() { mute.text = if (actions.muted(item.id)) "Unmute" else "Mute" }

    private fun render() {
        val running = item.running
        val color = Labels.stateColor(item)
        val working = running && item.activity.confirmed && item.activity.state == "working"
        val waiting = running && item.activity.state == "input" && item.activity.confirmed
        val answerable = waiting && item.tool == "claude-code"

        stateText.text = StateClock.headline(item)
        stateText.setTextColor(color)
        stateDot.background = context.shape(color, 4)
        stateDot.pulse(working)
        stateBar.setBackgroundColor(color)
        notice.text = when {
            !running -> if (item.lastError.isNotBlank()) "Not running. Last error: ${item.lastError}" else "Not running."
            answerable -> "Waiting for an answer. Yes presses Enter and No presses Esc, only on a permission prompt."
            waiting -> "Waiting for an answer. Answer it on the PC; replies from here are blocked until then."
            working -> "Replies are blocked until this turn finishes."
            item.activity.state == "unknown" -> "State unknown. Only Claude Code and Copilot CLI report it, so replies are blocked."
            else -> ""
        }
        notice.visibility = if (notice.text.isEmpty()) View.GONE else View.VISIBLE
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
                    lp(ViewWrap, ViewWrap).apply { rightMargin = context.dp(8) },
                )
            }
        }
        send.alpha = if (running && !working && !waiting) 1f else 0.5f
        chipScroll.visibility = if (running && !answerable && shownNow.isNotEmpty()) View.VISIBLE else View.GONE
        promptRow.visibility = if (running && !answerable) View.VISIBLE else View.GONE
        answerRow.visibility = if (answerable) View.VISIBLE else View.GONE
        startRow.visibility = if (!running) View.VISIBLE else View.GONE
    }

    /** The text is read-only and never kept beyond this screen. */
    private fun fetchScreen() {
        if (screenBusy || !item.running) return
        screenBusy = true
        actions.screen(item) { outcome ->
            screenBusy = false
            val text = when (outcome) {
                is Result.Ok -> outcome.value.joinToString("\n").ifEmpty { "(the terminal is empty)" }
                is Result.Fail -> if (outcome.code == "disabled") "Screen sharing is off on the PC (mobile.screen: false)." else outcome.message
            }
            if (text == screenView.text.toString()) return@screen
            val pinned = pinNext || vertical.getChildAt(0).bottom - (vertical.scrollY + vertical.height) < context.dp(48)
            pinNext = false
            screenView.text = text
            if (pinned) vertical.post { vertical.scrollTo(0, vertical.getChildAt(0).height) }
        }
    }

    private fun submit(insertOnly: Boolean) = submitText(prompt.text.toString().trim(), insertOnly, clear = true)

    private fun submitText(text: String, insertOnly: Boolean, clear: Boolean = false) {
        if (text.isEmpty() || busy) return
        busy = true
        show("Sending…", Theme.text6, false)
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
        show("Asking the PC to start the session…", Theme.text6, false)
        actions.start(item, mode) { outcome ->
            busy = false
            when (outcome) {
                is Result.Ok -> show("Requested. The session appears as running when it is up.", Theme.ok)
                is Result.Fail -> show(outcome.message, Theme.failed)
            }
        }
    }

    private fun answer(choice: String) {
        if (busy) return
        busy = true
        show("Sending $choice…", Theme.text6, false)
        actions.answer(item, choice) { outcome ->
            busy = false
            when (outcome) {
                is Result.Ok -> show("Answered $choice.", Theme.ok)
                is Result.Fail -> show(outcome.message, Theme.failed)
            }
        }
    }

    /** Results are short-lived so the bar does not keep a stale message. */
    private fun show(message: String, color: Int, autoHide: Boolean = true) {
        handler.removeCallbacks(clearResult)
        result.text = message
        result.setTextColor(color)
        result.visibility = View.VISIBLE
        if (autoHide) handler.postDelayed(clearResult, 5000)
    }

    override fun onDetachedFromWindow() {
        handler.removeCallbacksAndMessages(null)
        super.onDetachedFromWindow()
    }

    private companion object {
        const val ViewWrap = LayoutParams.WRAP_CONTENT
    }
}
