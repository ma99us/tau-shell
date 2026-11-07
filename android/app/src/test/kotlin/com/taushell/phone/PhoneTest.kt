package com.taushell.phone

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.io.IOException
import java.net.InetAddress
import java.security.KeyStore
import java.security.cert.X509Certificate
import javax.net.ssl.ExtendedSSLSession
import javax.net.ssl.KeyManagerFactory
import javax.net.ssl.SNIHostName
import javax.net.ssl.SSLContext
import javax.net.ssl.SSLException
import javax.net.ssl.SSLServerSocket
import javax.net.ssl.SSLSocket

class PairingLinkTest {
    private val fp = "a".repeat(64)

    @Test
    fun parsesTheLinkTheDesktopMakes() {
        val link = PairingLink.parse("taushell://pair?code=ABCDE23456&fp=$fp&host=192.168.1.20&port=47821")
        assertNotNull(link)
        assertEquals("192.168.1.20", link!!.host)
        assertEquals(47821, link.port)
        assertEquals(fp, link.fingerprint)
        assertEquals("ABCDE23456", link.code)
    }

    @Test
    fun rejectsAnythingElse() {
        val bad = listOf(
            "", "https://pair?host=h&port=47821&fp=$fp&code=C",
            "taushell://other?host=h&port=47821&fp=$fp&code=C",
            "taushell://pair?host=h&port=80&fp=$fp&code=C",
            "taushell://pair?host=h&port=47821&fp=short&code=C",
            "taushell://pair?host=h&port=47821&fp=$fp",
            "taushell://pair?host=a b&port=47821&fp=$fp&code=C",
        )
        bad.forEach { assertNull(it, PairingLink.parse(it)) }
    }
}

class PromptHistoryTest {
    @Test
    fun putsNewestFirstAndMovesDuplicatesToTheTop() {
        val history = PromptHistory.add(listOf("second", "first"), "third")
        assertEquals(listOf("third", "second", "first"), history)
        assertEquals(listOf("second", "third", "first"), PromptHistory.add(history, "second"))
    }

    @Test
    fun keepsOnlyTheMostRecentTwentyPrompts() {
        val history = PromptHistory.add((1..20).map { "prompt $it" }, "new prompt")
        assertEquals(20, history.size)
        assertEquals("new prompt", history.first())
        assertEquals("prompt 19", history.last())
    }
}

class SessionMutePolicyTest {
    @Test
    fun followsGlobalAlertsUnlessTheSessionIsExplicitlyMuted() {
        assertTrue(isSessionMuted(globalAlertsEnabled = false, sessionExplicitlyMuted = false))
        assertFalse(isSessionMuted(globalAlertsEnabled = true, sessionExplicitlyMuted = false))
        assertTrue(isSessionMuted(globalAlertsEnabled = true, sessionExplicitlyMuted = true))
    }
}

class HttpTest {
    private fun read(raw: String) = Http.read(raw.toByteArray(Charsets.ISO_8859_1).inputStream())

    @Test
    fun readsAContentLengthBody() {
        val reply = read("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 11\r\n\r\n{\"ok\":true}\n")
        assertEquals(200, reply.status)
        assertEquals("{\"ok\":true}", reply.body.trim())
    }

    @Test
    fun readsAChunkedBody() {
        val reply = read("HTTP/1.1 409 Conflict\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n6\r\n world\r\n0\r\n\r\n")
        assertEquals(409, reply.status)
        assertEquals("hello world", reply.body)
    }

    @Test
    fun rejectsATruncatedBody() {
        try {
            read("HTTP/1.1 200 OK\r\nContent-Length: 50\r\n\r\nshort")
            fail("a short body must be an error")
        } catch (_: IOException) {
        }
    }
}

class ParseTest {
    @Test
    fun readsSessionsAndActivity() {
        val json = """{"ok":true,"data":[{"id":"s1","name":"API","tool":"copilot-cli","status":"running","runningHere":true,
            "workingDir":"C:\\p\\api-server","color":"#4f8cff","totalUsd":1.25,"unpriced":true,"activity":{"state":"idle","confirmed":true,"hasDraft":false,"ageMs":5,"sinceMs":9}},
            {"id":"s2","name":"Old","tool":"gsd","status":"stopped","runningHere":false,"workingDir":"","activity":{"state":"stopped","confirmed":false,"ageMs":0,"sinceMs":0}}]}"""
        val result = Parse.sessions(json) as Result.Ok
        assertEquals(2, result.value.size)
        assertEquals("idle", result.value[0].activity.state)
        assertTrue(result.value[0].activity.confirmed)
        assertEquals(9L, result.value[0].activity.sinceMs)
        assertEquals("Idle", Labels.state(result.value[0]))
        assertEquals("Stopped", Labels.state(result.value[1]))
        assertEquals(1.25, result.value[0].totalUsd)
        assertEquals(true, result.value[0].unpriced)
        assertEquals(null, result.value[1].totalUsd)
        assertEquals("Est. $1.25+", Labels.totalSpent(result.value[0]))
        assertEquals("AS", Labels.initials(result.value[0]))
    }

    @Test
    fun marksAnUnconfirmedReading() {
        val item = (Parse.sessions("""{"ok":true,"data":[{"id":"s","name":"x","runningHere":true,"activity":{"state":"idle","confirmed":false}}]}""") as Result.Ok).value[0]
        assertEquals("Idle?", Labels.state(item))
    }

    @Test
    fun surfacesAServerRefusal() {
        val fail = Parse.ack("""{"ok":false,"error":{"code":"refused","message":"the harness is working"}}""") as Result.Fail
        assertEquals("refused", fail.code)
        assertEquals("the harness is working", fail.message)
    }

    @Test
    fun parsesScreenRows() {
        val rows = (Parse.screen("""{"ok":true,"data":{"rows":["a","  b",""]}}""") as Result.Ok).value
        assertEquals(listOf("a", "  b", ""), rows)
        assertEquals("disabled", (Parse.screen("""{"ok":false,"error":{"code":"disabled","message":"off"}}""") as Result.Fail).code)
    }

    @Test
    fun parsesPcPresets() {
        assertEquals(listOf("Run the tests"), (Parse.presets("""{"ok":true,"data":{"presets":["Run the tests"," "]}}""") as Result.Ok).value)
        assertEquals(emptyList<String>(), (Parse.presets("""{"ok":true,"data":{"presets":[]}}""") as Result.Ok).value)
    }

    @Test
    fun parsesSessionPresetsWithSubmitAndSourceMetadata() {
        val result = Parse.sessionPresets(
            """{"ok":true,"data":{"presets":[
                {"name":"Review","text":"Review the app","submit":true},
                {"name":"Draft","text":"Draft a reply","submit":false,"remote":true},
                {"name":" ","text":"ignored"}
            ]}}""",
        ) as Result.Ok
        assertEquals(
            listOf(
                PromptPreset("Review", "Review the app", true, false),
                PromptPreset("Draft", "Draft a reply", false, true),
            ),
            result.value,
        )
    }

    @Test
    fun stateClockReportsThinkingTimeAndWhenItFinished() {
        fun item(state: String, sinceMs: Long = 0) =
            SessionItem("s", "n", "claude-code", "running", true, "", "", Activity(state, true, false, sinceMs = sinceMs), "")
        StateClock.observe(item("idle"), 0)
        StateClock.observe(item("working", sinceMs = 15_000), 25_000)
        assertEquals("Thinking… 15s", StateClock.headline(item("working"), 25_000))
        StateClock.observe(item("idle", sinceMs = 1_000), 90_000)
        assertEquals("Idle · done 1m 6s ago", StateClock.headline(item("idle"), 155_000))
        assertEquals("Idle", StateClock.headline(SessionItem("x", "n", "t", "running", true, "", "", Activity("idle", true, false), ""), 1))
    }
    @Test
    fun alertsFireOnlyForConfirmedInputAndFinishedTurns() {
        fun item(state: String, confirmed: Boolean, running: Boolean = true) =
            SessionItem("s", "n", "claude-code", "running", running, "", "", Activity(state, confirmed, false), "")
        assertEquals("input", MonitorService.effectiveState(item("input", true), "working"))
        assertEquals("working", MonitorService.effectiveState(item("idle", false), "working"))
        assertEquals("other", MonitorService.effectiveState(item("input", true, running = false), "working"))
        assertEquals("Needs an answer", MonitorService.alertFor("working", "input"))
        assertEquals("Needs an answer", MonitorService.alertFor("idle", "input"))
        assertEquals("Finished", MonitorService.alertFor("working", "idle"))
        assertNull(MonitorService.alertFor("input", "input"))
        assertNull(MonitorService.alertFor(null, "input"))
        assertNull(MonitorService.alertFor("other", "idle"))
        assertNull(MonitorService.alertFor("input", "working"))
    }
}

/** Real TLS 1.3 handshakes against a local server built from the fixture keystores. */
class PinnedTlsTest {
    private fun load(name: String): Pair<SSLContext, String> {
        val store = KeyStore.getInstance("PKCS12")
        PinnedTlsTest::class.java.getResourceAsStream("/$name.p12")!!.use { store.load(it, "changeit".toCharArray()) }
        val keys = KeyManagerFactory.getInstance(KeyManagerFactory.getDefaultAlgorithm()).apply { init(store, "changeit".toCharArray()) }
        val context = SSLContext.getInstance("TLSv1.3").apply { init(keys.keyManagers, null, null) }
        val cert = store.getCertificate("t") as X509Certificate
        return context to sha256Hex(cert.encoded)
    }

    /** Accepts one connection, returns the SNI name the client sent. */
    private fun serve(context: SSLContext, protocols: Array<String> = arrayOf("TLSv1.3")): Pair<Int, java.util.concurrent.Future<String?>> {
        val server = context.serverSocketFactory.createServerSocket(0, 1, InetAddress.getLoopbackAddress()) as SSLServerSocket
        server.enabledProtocols = protocols
        val port = server.localPort
        val result = java.util.concurrent.Executors.newSingleThreadExecutor().submit<String?> {
            server.use {
                val socket = it.accept() as SSLSocket
                socket.use { s ->
                    s.startHandshake()
                    val names = (s.session as ExtendedSSLSession).requestedServerNames
                    s.outputStream.write(1)
                    (names.firstOrNull() as? SNIHostName)?.asciiName
                }
            }
        }
        return port to result
    }

    private fun connect(port: Int, pin: String, sni: String) {
        val factory = PinnedSocketFactory(pin, sni)
        val plain = java.net.Socket(InetAddress.getLoopbackAddress(), port)
        val tls = factory.createSocket(plain, "127.0.0.1", port, true) as SSLSocket
        tls.use {
            it.startHandshake()
            it.inputStream.read()
        }
    }

    @Test
    fun connectsWhenThePinMatchesAndSendsTheServerName() {
        val (context, pin) = load("a")
        val (port, name) = serve(context)
        connect(port, pin, "d-abc123")
        assertEquals("d-abc123", name.get())
    }

    @Test
    fun refusesAnotherCertificate() {
        val (context, _) = load("a")
        val (_, otherPin) = load("b")
        val (port, name) = serve(context)
        try {
            connect(port, otherPin, "pair")
            fail("a different certificate must not be accepted")
        } catch (error: SSLException) {
            assertTrue(generateSequence<Throwable>(error) { it.cause }.any { it.message?.contains("pin mismatch") == true })
        } catch (_: IOException) {
            // The server may also see the aborted handshake first.
        }
        runCatching { name.get() }
    }

    @Test
    fun refusesAServerThatOnlySpeaksTls12() {
        val (context, pin) = load("a")
        val tls12 = SSLContext.getInstance("TLSv1.2").apply {
            val keys = KeyManagerFactory.getInstance(KeyManagerFactory.getDefaultAlgorithm())
            keys.init(KeyStore.getInstance("PKCS12").apply {
                PinnedTlsTest::class.java.getResourceAsStream("/a.p12")!!.use { load(it, "changeit".toCharArray()) }
            }, "changeit".toCharArray())
            init(keys.keyManagers, null, null)
        }
        check(context.protocol.isNotEmpty())
        val (port, _) = serve(tls12, arrayOf("TLSv1.2"))
        try {
            connect(port, pin, "pair")
            fail("TLS 1.2 must be refused")
        } catch (_: IOException) {
        }
    }
}
