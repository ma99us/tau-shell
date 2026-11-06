package com.taushell.phone

import android.content.Context
import android.os.Handler
import android.os.Looper
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.view.inputmethod.InputMethodManager
import android.widget.ArrayAdapter
import android.widget.AutoCompleteTextView
import android.widget.HorizontalScrollView
import android.widget.LinearLayout
import android.widget.PopupMenu
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
    private val totalSpent = context.text("", 11f, Theme.accent, true).apply {
        background = context.shape(Theme.accentBg, 16, Theme.accentBorder)
        setPadding(context.dp(9), context.dp(6), context.dp(9), context.dp(6))
        gravity = Gravity.CENTER
    }
    private val screenView = context.text("", 11f, Theme.text6).apply {
        typeface = android.graphics.Typeface.MONOSPACE
        setTextIsSelectable(true)
        setPadding(context.dp(12), context.dp(10), context.dp(12), context.dp(10))
    }
    private val vertical = ScrollView(context)
    private val horizontal = HorizontalScrollView(context).apply {
        isHorizontalScrollBarEnabled = true
        isFillViewport = true
    }
    private val wrap = context.button("Wrap: Off", false) { setWrap(!wrapEnabled) }
    private val promptsButton = context.button("Prompts", false) { showPresets() }
    private val prompt: AutoCompleteTextView = AutoCompleteTextView(context).apply {
        hint = "Prompt (hold Send to type only)"
        setHintTextColor(Theme.text11)
        setTextColor(Theme.text1)
        setTextSize(android.util.TypedValue.COMPLEX_UNIT_SP, 14f)
        background = context.shape(Theme.bgApp, 8, Theme.border)
        setPadding(context.dp(12), context.dp(10), context.dp(12), context.dp(10))
        inputType = android.text.InputType.TYPE_CLASS_TEXT or android.text.InputType.TYPE_TEXT_FLAG_MULTI_LINE or
            android.text.InputType.TYPE_TEXT_FLAG_CAP_SENTENCES
        maxLines = 5
        threshold = 0
    }
    private val send = context.button("Send", true) { submit(false) }
    private val promptRow = LinearLayout(context)
    private val answerRow = LinearLayout(context)
    private val startRow = LinearLayout(context)
    private val result = context.text("", 12f, Theme.text6)
    private val clearResult = Runnable { result.visibility = View.GONE }

    private var shownPromptHistory: List<String>? = null
    private var screenBusy = false
    private var stickToLatest = true
    private var applyingScreenUpdate = false
    private var wrapEnabled = false
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
            addView(totalSpent, lp(ViewWrap, ViewWrap).apply { rightMargin = context.dp(8) })
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

        horizontal.addView(screenView, ViewGroup.LayoutParams(ViewWrap, ViewWrap))
        vertical.isFillViewport = true
        vertical.addView(horizontal, ViewGroup.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT))
        vertical.setBackgroundColor(Theme.bgApp)
        vertical.setOnScrollChangeListener { _, _, _, _, _ ->
            if (!applyingScreenUpdate) stickToLatest = isAtBottom()
        }
        addView(vertical, lp(height = 0, weight = 1f))
        answerRow.addView(context.button("Yes", true) { answer("yes") }, lp(0, weight = 1f))
        answerRow.addView(context.button("No", false) { answer("no") }, lp(0, weight = 1f).apply { leftMargin = context.dp(8) })
        startRow.addView(context.button("Resume", true) { startSession("restore") }, lp(0, weight = 1f))
        startRow.addView(context.button("Start fresh", false) { startSession("fresh") }, lp(0, weight = 1f).apply { leftMargin = context.dp(8) })
        send.setOnLongClickListener { submit(true); true }
        promptRow.gravity = Gravity.BOTTOM
        promptRow.addView(prompt, lp(0, weight = 1f))
        promptRow.addView(send, lp(ViewWrap, ViewWrap).apply { leftMargin = context.dp(8) })
        prompt.setAdapter(ArrayAdapter(context, android.R.layout.simple_dropdown_item_1line, emptyList<String>()))
        prompt.setOnItemClickListener { _, _, position, _ ->
            val selected = prompt.adapter.getItem(position)?.toString().orEmpty()
            prompt.dismissDropDown()
            submitText(selected, false, clear = true)
        }
        prompt.setOnClickListener { if (prompt.adapter.count > 0) prompt.showDropDown() }

        val bottom = linear(context).apply {
            setBackgroundColor(Theme.bgPanel)
            setPadding(context.dp(12), context.dp(8), context.dp(12), context.dp(10))
            addView(result, lp().apply { bottomMargin = context.dp(6) })
            addView(LinearLayout(context).apply {
                gravity = Gravity.CENTER_VERTICAL
                addView(wrap, lp(0, weight = 1f))
                addView(promptsButton, lp(0, weight = 1f).apply { leftMargin = context.dp(6) })
            }, lp().apply { bottomMargin = context.dp(8) })
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
        totalSpent.text = Labels.totalSpent(next).orEmpty()
        totalSpent.visibility = if (next.totalUsd?.isFinite() == true) View.VISIBLE else View.GONE
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
        val answerable = waiting && (item.tool == "claude-code" || item.tool == "copilot-cli")
        // A question or choice list takes typed text; only a permission prompt gets Yes/No.
        val question = answerable && item.activity.question
        val yesNo = answerable && !question

        stateText.text = StateClock.headline(item)
        stateText.setTextColor(color)
        stateDot.background = context.shape(color, 4)
        stateDot.pulse(working)
        stateBar.setBackgroundColor(color)
        notice.text = when {
            !running -> if (item.lastError.isNotBlank()) "Not running. Last error: ${item.lastError}" else "Not running."
            question -> "Waiting for an answer to a question. Type the reply (the option number for a list) and send it."
            answerable -> "Waiting for an answer. Yes presses Enter and No presses Esc, only on a permission prompt."
            waiting -> "Waiting for an answer. Answer it on the PC; replies from here are blocked until then."
            working -> "Turn active. Send submits your prompt now."
            item.activity.state == "unknown" -> "State unknown. Only Claude Code and Copilot CLI report it, so replies are blocked."
            else -> ""
        }
        notice.visibility = if (notice.text.isEmpty()) View.GONE else View.VISIBLE
        notice.setTextColor(if (waiting) Theme.input else Theme.text10)

        val history = actions.promptHistory(item.id)
        if (history != shownPromptHistory) {
            shownPromptHistory = history
            prompt.setAdapter(ArrayAdapter(context, android.R.layout.simple_dropdown_item_1line, history))
        }
        send.text = "Send"
        send.alpha = if (running && (!waiting || question || working)) 1f else 0.5f
        promptsButton.visibility = if (running && !answerable) View.VISIBLE else View.GONE
        promptRow.visibility = if (running && !yesNo) View.VISIBLE else View.GONE
        answerRow.visibility = if (yesNo) View.VISIBLE else View.GONE
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
            val pinned = stickToLatest
            val previousY = vertical.scrollY
            applyingScreenUpdate = true
            screenView.text = text
            vertical.post {
                vertical.scrollTo(0, if (pinned) vertical.getChildAt(0).height else previousY)
                applyingScreenUpdate = false
            }
        }
    }

    private fun submit(insertOnly: Boolean) {
        val text = prompt.text.toString().trim()
        if (text.isNotEmpty()) prompt.dismissDropDown()
        submitText(text, insertOnly, clear = true)
    }

    private fun showPresets() {
        actions.sessionPresets(item) { outcome ->
            when (outcome) {
                is Result.Ok -> showPresetMenu(outcome.value)
                is Result.Fail -> show(outcome.message, Theme.failed)
            }
        }
    }

    private fun showPresetMenu(sessionPresets: List<PromptPreset>) {
        val popup = PopupMenu(context, promptsButton)
        val skills = sessionPresets.filter { it.remote }
        val prompts = sessionPresets.filterNot { it.remote }
        if (skills.isNotEmpty()) {
            val submenu = popup.menu.addSubMenu("Skills")
            skills.forEach { addPreset(submenu, it) }
        }
        if (prompts.isNotEmpty()) {
            val submenu = popup.menu.addSubMenu("Session prompts")
            prompts.forEach { addPreset(submenu, it) }
        }
        val replies = actions.quickReplies()
        if (replies.isNotEmpty()) {
            val submenu = popup.menu.addSubMenu("Quick replies")
            replies.forEach { reply ->
                submenu.add(reply).setOnMenuItemClickListener {
                    submitText(reply, insertOnly = false)
                    true
                }
            }
        }
        if (sessionPresets.isEmpty() && replies.isEmpty()) {
            popup.menu.add("No prompts available").isEnabled = false
        }
        popup.show()
    }

    private fun addPreset(menu: android.view.Menu, preset: PromptPreset) {
        menu.add(preset.name).setOnMenuItemClickListener {
            submitText(preset.text, insertOnly = !preset.submit)
            true
        }
    }

    private fun submitText(text: String, insertOnly: Boolean, clear: Boolean = false) {
        if (text.isEmpty() || busy) return
        busy = true
        show("Sending…", Theme.text6, false)
        fun finish(outcome: Result<Unit>) {
            busy = false
            when (outcome) {
                is Result.Ok -> {
                    actions.rememberPrompt(item.id, text)
                    shownPromptHistory = actions.promptHistory(item.id)
                    prompt.setAdapter(ArrayAdapter(context, android.R.layout.simple_dropdown_item_1line, shownPromptHistory.orEmpty()))
                    if (clear) prompt.setText("")
                    show(if (insertOnly) "Typed into the session. Press Enter in the session to submit." else "Sent.", Theme.ok)
                    (context.getSystemService(Context.INPUT_METHOD_SERVICE) as InputMethodManager).hideSoftInputFromWindow(prompt.windowToken, 0)
                }
                is Result.Fail -> show(outcome.message, if (outcome.code == "refused") Theme.input else Theme.failed)
            }
        }
        actions.send(item, text, insertOnly) { outcome -> finish(outcome) }
    }

    private fun setWrap(enabled: Boolean) {
        if (wrapEnabled == enabled) return
        val previousY = vertical.scrollY
        applyingScreenUpdate = true
        wrapEnabled = enabled
        wrap.text = if (enabled) "Wrap: On" else "Wrap: Off"
        vertical.removeView(horizontal)
        if (enabled) {
            horizontal.removeView(screenView)
            screenView.setHorizontallyScrolling(false)
            vertical.addView(screenView, ViewGroup.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT))
        } else {
            vertical.removeView(screenView)
            screenView.setHorizontallyScrolling(true)
            horizontal.addView(screenView, ViewGroup.LayoutParams(ViewWrap, ViewWrap))
            vertical.addView(horizontal, ViewGroup.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT))
        }
        vertical.post {
            vertical.scrollTo(0, previousY)
            applyingScreenUpdate = false
        }
    }

    private fun isAtBottom(): Boolean {
        val child = vertical.getChildAt(0) ?: return true
        return child.bottom - (vertical.scrollY + vertical.height) <= context.dp(48)
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
