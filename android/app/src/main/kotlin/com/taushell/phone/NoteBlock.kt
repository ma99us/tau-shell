package com.taushell.phone

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.text.method.LinkMovementMethod
import android.text.util.Linkify
import android.view.View
import android.widget.LinearLayout
import android.widget.TextView
import java.util.regex.Pattern

/** Opens an http(s) address from a session's link, notes or tags; anything else is ignored. */
fun Context.openLink(url: String) {
    val uri = runCatching { Uri.parse(url) }.getOrNull() ?: return
    if (uri.scheme != "http" && uri.scheme != "https") return
    try {
        startActivity(Intent(Intent.ACTION_VIEW, uri).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
    } catch (_: ActivityNotFoundException) {
    }
}

private val linkPattern: Pattern = Pattern.compile("https?://[^\\s<>\"'`]*[^\\s<>\"'`.,;:!?)\\]}]")

/** The session's link, notes and tags as set on the desktop; hidden when there are none. */
class NoteBlock(private val context: Context, private val tappableNotes: Boolean, maxNoteLines: Int) : LinearLayout(context) {
    private val link = context.text("", 12f, Theme.accent, true)
    private val notes = context.text("", 12f, Theme.text10)
    private val tags = context.text("", 11f, Theme.text10)

    init {
        orientation = VERTICAL
        link.setOnClickListener { (it.tag as? String)?.let(context::openLink) }
        notes.maxLines = maxNoteLines
        notes.ellipsize = android.text.TextUtils.TruncateAt.END
        if (tappableNotes) notes.movementMethod = LinkMovementMethod.getInstance()
        addView(link, lp())
        addView(notes, lp())
        addView(tags, lp())
        visibility = GONE
    }

    fun bind(item: SessionItem) {
        val hasLink = item.link.isNotBlank()
        link.visibility = if (hasLink) VISIBLE else GONE
        link.text = item.linkLabel.ifBlank { item.link }
        link.tag = item.link.takeIf { hasLink }
        notes.visibility = if (item.notes.isBlank()) GONE else VISIBLE
        notes.text = item.notes
        if (tappableNotes) Linkify.addLinks(notes, linkPattern, null)
        tags.visibility = if (item.tags.isEmpty()) GONE else VISIBLE
        tags.text = item.tags.joinToString("  ·  ")
        visibility = if (hasLink || item.notes.isNotBlank() || item.tags.isNotEmpty()) View.VISIBLE else GONE
    }
}
