package com.taushell.phone

import java.io.ByteArrayOutputStream
import java.io.InputStream
import java.io.OutputStream

class HttpReply(val status: Int, val body: String)

/**
 * A minimal HTTP/1.1 client over a socket the caller already secured. The
 * platform HTTP stacks replace the TLS server name with the address, but the PC
 * picks its certificate by that name, so the request is written here.
 */
object Http {
    fun request(
        socket: java.net.Socket,
        host: String,
        method: String,
        path: String,
        body: String?,
        token: String?,
    ): HttpReply {
        val payload = body?.toByteArray(Charsets.UTF_8)
        val head = StringBuilder()
            .append("$method $path HTTP/1.1\r\n")
            .append("Host: $host\r\n")
            .append("Accept: application/json\r\n")
            .append("Connection: close\r\n")
        if (token != null) head.append("Authorization: Bearer $token\r\n")
        if (payload != null) head.append("Content-Type: application/json\r\n").append("Content-Length: ${payload.size}\r\n")
        head.append("\r\n")
        socket.getOutputStream().apply {
            write(head.toString().toByteArray(Charsets.ISO_8859_1))
            if (payload != null) write(payload)
            flush()
        }
        return read(socket.getInputStream())
    }

    fun read(input: InputStream): HttpReply {
        val status = readLine(input).split(" ").getOrNull(1)?.toIntOrNull() ?: throw java.io.IOException("not an HTTP answer")
        var length = -1
        var chunked = false
        while (true) {
            val line = readLine(input)
            if (line.isEmpty()) break
            val at = line.indexOf(':')
            if (at <= 0) continue
            val value = line.substring(at + 1).trim()
            when (line.substring(0, at).lowercase()) {
                "content-length" -> length = value.toIntOrNull() ?: -1
                "transfer-encoding" -> chunked = value.lowercase().contains("chunked")
            }
        }
        val bytes = ByteArrayOutputStream()
        when {
            chunked -> readChunked(input, bytes)
            length >= 0 -> copy(input, bytes, length)
            else -> input.copyTo(bytes)
        }
        return HttpReply(status, bytes.toString(Charsets.UTF_8.name()))
    }

    private fun readChunked(input: InputStream, out: OutputStream) {
        while (true) {
            val size = readLine(input).substringBefore(';').trim().toIntOrNull(16) ?: throw java.io.IOException("bad chunk")
            if (size == 0) break
            copy(input, out, size)
            readLine(input)
        }
    }

    private fun copy(input: InputStream, out: OutputStream, count: Int) {
        val buffer = ByteArray(4096)
        var left = count
        while (left > 0) {
            val n = input.read(buffer, 0, minOf(buffer.size, left))
            if (n < 0) throw java.io.IOException("connection closed early")
            out.write(buffer, 0, n)
            left -= n
        }
    }

    private fun readLine(input: InputStream): String {
        val line = StringBuilder()
        while (true) {
            val b = input.read()
            if (b < 0) throw java.io.IOException("connection closed early")
            if (b == '\n'.code) return line.toString().trimEnd('\r')
            line.append(b.toChar())
            if (line.length > 8192) throw java.io.IOException("header too long")
        }
    }
}
