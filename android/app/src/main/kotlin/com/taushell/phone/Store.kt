package com.taushell.phone

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/** The bearer token never touches disk unencrypted: AES-GCM with a key that cannot leave the Android Keystore. */
private object SecretBox {
    private const val ALIAS = "taushell-token"

    private fun key(): SecretKey {
        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (store.getKey(ALIAS, null) as? SecretKey)?.let { return it }
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
        generator.init(
            KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .build(),
        )
        return generator.generateKey()
    }

    fun seal(plain: String): String {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding").apply { init(Cipher.ENCRYPT_MODE, key()) }
        return Base64.encodeToString(cipher.iv + cipher.doFinal(plain.toByteArray()), Base64.NO_WRAP)
    }

    fun open(sealed: String): String? = try {
        val raw = Base64.decode(sealed, Base64.NO_WRAP)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            .apply { init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, raw, 0, 12)) }
        String(cipher.doFinal(raw, 12, raw.size - 12))
    } catch (_: Exception) {
        null
    }
}

/** One paired PC. */
data class Pairing(
    val host: String,
    val port: Int,
    val deviceId: String,
    val fingerprint: String,
    val token: String,
    val expires: String,
) {
    fun client() = PcClient(host, port, fingerprint, "d-$deviceId.taushell", token)
}

class Store(context: Context) {
    private val prefs = context.getSharedPreferences("taushell", Context.MODE_PRIVATE)

    fun pairing(): Pairing? {
        val token = prefs.getString("token", null)?.let { SecretBox.open(it) } ?: return null
        return Pairing(
            host = prefs.getString("host", null) ?: return null,
            port = prefs.getInt("port", 0),
            deviceId = prefs.getString("deviceId", null) ?: return null,
            fingerprint = prefs.getString("fingerprint", null) ?: return null,
            token = token,
            expires = prefs.getString("expires", "") ?: "",
        )
    }

    fun save(pairing: Pairing) {
        prefs.edit()
            .putString("host", pairing.host).putInt("port", pairing.port)
            .putString("deviceId", pairing.deviceId).putString("fingerprint", pairing.fingerprint)
            .putString("token", SecretBox.seal(pairing.token)).putString("expires", pairing.expires)
            .apply()
    }

    fun forget() {
        prefs.edit().remove("host").remove("port").remove("deviceId").remove("fingerprint").remove("token").remove("expires").apply()
    }

    var muted: Set<String>
        get() = prefs.getStringSet("muted", emptySet()) ?: emptySet()
        set(value) = prefs.edit().putStringSet("muted", value).apply()

    var alerts: Boolean
        get() = prefs.getBoolean("alerts", false)
        set(value) = prefs.edit().putBoolean("alerts", value).apply()

    var presets: List<String>
        get() = prefs.getString("presets", null)?.split("\n")?.filter { it.isNotBlank() } ?: DEFAULT_PRESETS
        set(value) = prefs.edit().putString("presets", value.filter { it.isNotBlank() }.joinToString("\n")).apply()

    companion object {
        val DEFAULT_PRESETS = listOf("Continue", "Go ahead", "Summarize what you did", "Run the tests")
    }
}
