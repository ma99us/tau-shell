package com.taushell.phone

import java.net.URI
import java.net.URLDecoder

/** The pairing link from the PC: taushell://pair?host=&port=&fp=&code= */
data class PairingLink(val host: String, val port: Int, val fingerprint: String, val code: String) {
    companion object {
        private val hex64 = Regex("^[0-9a-fA-F]{64}$")
        private val hostPattern = Regex("^[0-9A-Za-z.:-]{1,253}$")

        /** Returns null unless the text is a well-formed pairing link. */
        fun parse(text: String): PairingLink? {
            val uri = try {
                URI(text.trim())
            } catch (_: Exception) {
                return null
            }
            if (uri.scheme != "taushell" || uri.host != "pair") return null
            val query = (uri.rawQuery ?: return null).split("&").mapNotNull { part ->
                val at = part.indexOf('=')
                if (at <= 0) null else URLDecoder.decode(part.substring(0, at), "UTF-8") to
                    URLDecoder.decode(part.substring(at + 1), "UTF-8")
            }.toMap()
            val host = query["host"] ?: return null
            val port = query["port"]?.toIntOrNull() ?: return null
            val fingerprint = query["fp"] ?: return null
            val code = query["code"] ?: return null
            if (!hostPattern.matches(host) || port !in 1024..65535 || !hex64.matches(fingerprint) || code.isBlank()) return null
            return PairingLink(host, port, fingerprint.lowercase(), code)
        }
    }
}
