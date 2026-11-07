package com.taushell.phone

object PromptHistory {
    const val maxItems = 20

    fun add(history: List<String>, text: String): List<String> =
        (listOf(text) + history.filterNot { it == text }).take(maxItems)
}
