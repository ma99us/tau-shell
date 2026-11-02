package com.taushell.phone

import java.net.InetAddress
import java.net.Socket
import java.security.MessageDigest
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import javax.net.ssl.SNIHostName
import javax.net.ssl.SSLContext
import javax.net.ssl.SSLSocket
import javax.net.ssl.SSLSocketFactory
import javax.net.ssl.X509TrustManager

fun sha256Hex(bytes: ByteArray): String =
    MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }

/**
 * Accepts exactly one certificate: the one whose SHA-256 matches the pin. The
 * system trust store is never consulted. The pin is the identity, so the host
 * name is not checked.
 */
class PinTrustManager(fingerprint: String) : X509TrustManager {
    private val pin = fingerprint.lowercase().toByteArray()

    override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {
        val leaf = chain?.firstOrNull() ?: throw CertificateException("pin mismatch: no certificate")
        val actual = sha256Hex(leaf.encoded).toByteArray()
        if (!MessageDigest.isEqual(actual, pin)) throw CertificateException("pin mismatch")
        leaf.checkValidity()
    }

    override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) {
        throw CertificateException("client certificates are not used")
    }

    override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()
}

/**
 * Builds TLS 1.3-only sockets that trust only the pin and send [sni] as the
 * server name (the PC picks the certificate by it), whatever the address is.
 */
class PinnedSocketFactory(fingerprint: String, private val sni: String) : SSLSocketFactory() {
    private val delegate: SSLSocketFactory = SSLContext.getInstance("TLSv1.3").apply {
        init(null, arrayOf(PinTrustManager(fingerprint)), null)
    }.socketFactory

    private fun tls13(socket: Socket): Socket {
        val ssl = socket as SSLSocket
        ssl.sslParameters = ssl.sslParameters.apply {
            protocols = arrayOf("TLSv1.3")
            serverNames = listOf(SNIHostName(sni))
        }
        return ssl
    }

    private fun plain(host: String, port: Int): Socket = Socket().also { it.connect(java.net.InetSocketAddress(host, port), 8000) }

    /** Connects to [host]:[port] and completes the pinned TLS 1.3 handshake. */
    fun connect(host: String, port: Int, readTimeoutMs: Int): SSLSocket {
        val raw = plain(host, port)
        try {
            val socket = createSocket(raw, host, port, true) as SSLSocket
            socket.soTimeout = readTimeoutMs
            socket.startHandshake()
            return socket
        } catch (error: Exception) {
            raw.close()
            throw error
        }
    }

    override fun getDefaultCipherSuites(): Array<String> = delegate.defaultCipherSuites
    override fun getSupportedCipherSuites(): Array<String> = delegate.supportedCipherSuites
    override fun createSocket(socket: Socket, host: String?, port: Int, autoClose: Boolean): Socket =
        tls13(delegate.createSocket(socket, sni, port, autoClose))

    override fun createSocket(host: String, port: Int): Socket = createSocket(plain(host, port), host, port, true)
    override fun createSocket(host: String, port: Int, local: InetAddress?, localPort: Int): Socket = createSocket(host, port)
    override fun createSocket(host: InetAddress, port: Int): Socket = createSocket(host.hostAddress ?: "", port)
    override fun createSocket(host: InetAddress, port: Int, local: InetAddress?, localPort: Int): Socket =
        createSocket(host, port)
}
