package com.taushell.phone

import org.json.JSONObject
import java.io.IOException
import java.net.SocketTimeoutException
import javax.net.ssl.SSLException

data class Activity(
    val state: String,
    val confirmed: Boolean,
    val hasDraft: Boolean,
    /** A waiting harness shows a question or choice list, not a permission prompt. */
    val question: Boolean = false,
    /** Elapsed time since the desktop observed this state, in milliseconds. */
    val sinceMs: Long? = null,
)

data class SessionItem(
    val id: String,
    val name: String,
    val tool: String,
    val status: String,
    val running: Boolean,
    val workingDir: String,
    val color: String,
    val activity: Activity,
    val lastError: String,
    val totalUsd: Double? = null,
    val unpriced: Boolean = false,
    val notes: String = "",
    val tags: List<String> = emptyList(),
    val link: String = "",
    val linkLabel: String = "",
)

data class PairResult(val deviceId: String, val token: String, val fingerprint: String, val expires: String)

data class PromptPreset(
    val name: String,
    val text: String,
    val submit: Boolean,
    val remote: Boolean,
)

sealed class Result<out T> {
    data class Ok<T>(val value: T) : Result<T>()

    /** [code] is a server code (refused, not_found, ...) or unreachable, pin_mismatch, unauthorized, timeout. */
    data class Fail(val code: String, val message: String) : Result<Nothing>()
}

object Parse {
    fun sessions(json: String): Result<List<SessionItem>> {
        val root = JSONObject(json)
        if (!root.optBoolean("ok")) return failure(root)
        val data = root.optJSONArray("data") ?: return Result.Ok(emptyList())
        return Result.Ok((0 until data.length()).map { index ->
            val item = data.getJSONObject(index)
            val activity = item.optJSONObject("activity")
            SessionItem(
                id = item.getString("id"),
                name = item.optString("name"),
                tool = item.optString("tool"),
                status = item.optString("status"),
                running = item.optBoolean("runningHere"),
                workingDir = item.optString("workingDir"),
                color = item.optString("color"),
                activity = Activity(
                    state = activity?.optString("state") ?: "unknown",
                    confirmed = activity?.optBoolean("confirmed") ?: false,
                    hasDraft = activity?.optBoolean("hasDraft") ?: false,
                    question = activity?.optBoolean("question") ?: false,
                    sinceMs = if (activity?.has("sinceMs") == true) {
                        activity.optLong("sinceMs").takeIf { it >= 0 }
                    } else {
                        null
                    },
                ),
                lastError = item.optString("lastError"),
                totalUsd = if (item.has("totalUsd") && !item.isNull("totalUsd")) {
                    item.optDouble("totalUsd").takeIf { it.isFinite() }
                } else {
                    null
                },
                unpriced = item.optBoolean("unpriced"),
                notes = item.optString("notes"),
                tags = item.optJSONArray("tags")?.let { array -> (0 until array.length()).map { array.optString(it) }.filter { it.isNotBlank() } } ?: emptyList(),
                link = item.optString("link"),
                linkLabel = item.optString("linkLabel"),
            )
        })
    }

    fun screen(json: String): Result<List<String>> {
        val root = JSONObject(json)
        if (!root.optBoolean("ok")) return failure(root)
        val rows = root.optJSONObject("data")?.optJSONArray("rows") ?: return Result.Ok(emptyList())
        return Result.Ok((0 until rows.length()).map { rows.optString(it) })
    }

    fun presets(json: String): Result<List<String>> {
        val root = JSONObject(json)
        if (!root.optBoolean("ok")) return failure(root)
        val items = root.optJSONObject("data")?.optJSONArray("presets") ?: return Result.Ok(emptyList())
        return Result.Ok((0 until items.length()).map { items.optString(it) }.filter { it.isNotBlank() })
    }

    fun sessionPresets(json: String): Result<List<PromptPreset>> {
        val root = JSONObject(json)
        if (!root.optBoolean("ok")) return failure(root)
        val items = root.optJSONObject("data")?.optJSONArray("presets") ?: return Result.Ok(emptyList())
        return Result.Ok((0 until items.length()).mapNotNull { index ->
            val item = items.optJSONObject(index) ?: return@mapNotNull null
            val name = item.optString("name").trim()
            val text = item.optString("text")
            if (name.isBlank() || text.isBlank()) null else PromptPreset(
                name = name,
                text = text,
                submit = item.optBoolean("submit"),
                remote = item.optBoolean("remote"),
            )
        })
    }

    fun ack(json: String): Result<Unit> {
        val root = JSONObject(json)
        return if (root.optBoolean("ok")) Result.Ok(Unit) else failure(root)
    }

    fun pair(json: String): PairResult {
        val root = JSONObject(json)
        return PairResult(
            root.getString("deviceId"), root.getString("token"),
            root.getString("fingerprint"), root.optString("expires"),
        )
    }

    private fun failure(root: JSONObject): Result.Fail {
        val error = root.optJSONObject("error")
        return Result.Fail(error?.optString("code") ?: "error", error?.optString("message") ?: "The PC reported an error.")
    }
}

/** Talks to one PC. [sni] is "pair.taushell" while pairing and "d-<device id>.taushell" afterwards. */
class PcClient(
    private val host: String,
    private val port: Int,
    fingerprint: String,
    private val sni: String,
    private val token: String? = null,
) {
    private val factory = PinnedSocketFactory(fingerprint, sni)

    private fun call(method: String, path: String, body: String?, readTimeoutMs: Int): Result<String> {
        try {
            val reply = factory.connect(host, port, readTimeoutMs).use { socket ->
                Http.request(socket, sni, method, path, body, token)
            }
            val status = reply.status
            val text = reply.body
            return when {
                status == 401 -> Result.Fail("unauthorized", "The PC no longer accepts this phone. Pair it again.")
                status == 429 -> Result.Fail("rate_limited", "The PC is blocking requests after repeated failures. Try again in a minute.")
                status >= 400 && !text.trimStart().startsWith("{") ->
                    Result.Fail("http_$status", text.trim().ifEmpty { "The PC answered $status." })
                else -> Result.Ok(text)
            }
        } catch (error: SSLException) {
            val pinned = generateSequence<Throwable>(error) { it.cause }.any { it.message?.contains("pin mismatch") == true }
            return if (pinned) {
                Result.Fail("pin_mismatch", "The certificate does not match the pairing. This is not the PC you paired with, or it was re-paired.")
            } else if (token != null && generateSequence<Throwable>(error) { it.cause }.any { it.message?.contains("internal_error", ignoreCase = true) == true }) {
                // A revoked device has no certificate any more, so the PC aborts the handshake.
                Result.Fail("unauthorized", "The PC no longer accepts this phone. It was revoked or TauShell's data was reset. Pair it again.")
            } else {
                Result.Fail("unreachable", "Secure connection failed: ${error.message}")
            }
        } catch (_: SocketTimeoutException) {
            return Result.Fail("timeout", "The PC did not answer in time.")
        } catch (error: IOException) {
            return Result.Fail("unreachable", "Cannot reach the PC (${error.message ?: error.javaClass.simpleName}). Is it on the same network with the phone server on?")
        }
    }

    private fun <T> parsed(response: Result<String>, parse: (String) -> Result<T>): Result<T> = when (response) {
        is Result.Fail -> response
        is Result.Ok -> try {
            parse(response.value)
        } catch (_: Exception) {
            Result.Fail("bad_response", "The PC sent an answer this app does not understand. Update the app or TauShell.")
        }
    }

    /** Holds the request open until the PC user approves or denies. */
    fun pair(code: String, name: String): Result<PairResult> {
        val body = JSONObject().put("code", code).put("name", name).toString()
        return parsed(call("POST", "/v1/pair", body, 150_000)) { Result.Ok(Parse.pair(it)) }
    }

    fun sessions(): Result<List<SessionItem>> = parsed(call("GET", "/v1/sessions", null, 15_000), Parse::sessions)

    fun send(session: String, text: String, insertOnly: Boolean): Result<Unit> {
        val body = JSONObject().put("text", text).put("insertOnly", insertOnly).toString()
        return parsed(call("POST", "/v1/sessions/${enc(session)}/send", body, 15_000), Parse::ack)
    }

    /** Read-only tail of the session's visible terminal text. */
    fun screen(session: String, rows: Int): Result<List<String>> =
        parsed(call("GET", "/v1/sessions/${enc(session)}/screen?rows=$rows", null, 15_000), Parse::screen)

    fun presets(): Result<List<String>> = parsed(call("GET", "/v1/presets", null, 15_000), Parse::presets)

    fun sessionPresets(session: String): Result<List<PromptPreset>> =
        parsed(call("GET", "/v1/sessions/${enc(session)}/presets", null, 15_000), Parse::sessionPresets)

    fun start(session: String, mode: String): Result<Unit> {
        val body = JSONObject().put("mode", mode).toString()
        return parsed(call("POST", "/v1/sessions/${enc(session)}/start", body, 15_000), Parse::ack)
    }

    private fun enc(value: String) = java.net.URLEncoder.encode(value, "UTF-8").replace("+", "%20")
}
