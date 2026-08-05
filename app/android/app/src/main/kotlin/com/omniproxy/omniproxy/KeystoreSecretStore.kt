package com.omniproxy.omniproxy

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import com.omniproxy.bind.mobile.SecretStore
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * Keystore-backed `SecretStore` (`docs/platform-notes.md` §Android): the
 * core's at-rest data key and credential refs are encrypted with a
 * non-exportable AES-256 key held in the Android Keystore, and the ciphertext
 * lives in private SharedPreferences. Because the Keystore key never leaves
 * secure hardware and `setUserAuthenticationRequired(false)` keeps it usable
 * across screen locks and process restarts, stored credentials survive a
 * process kill/reopen (previously they lived in a throwaway in-memory store
 * and were regenerated on restart, corrupting the stored config).
 */
class KeystoreSecretStore(context: Context) : SecretStore {
    private val prefs =
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)

    private fun getOrCreateKey(): SecretKey {
        val keyStore = KeyStore.getInstance(ANDROID_KEYSTORE).apply { load(null) }
        (keyStore.getKey(KEY_ALIAS, null) as? SecretKey)?.let { return it }
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, ANDROID_KEYSTORE)
        generator.init(
            KeyGenParameterSpec.Builder(KEY_ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .setRandomizedEncryptionRequired(true)
                .setUserAuthenticationRequired(false)
                .build()
        )
        return generator.generateKey()
    }

    override fun get(key: String): String? {
        val raw = prefs.getString(PREFIX + key, null) ?: return null
        return try {
            decrypt(raw)
        } catch (_: Exception) {
            null
        }
    }

    override fun set(key: String, value: String) {
        prefs.edit().putString(PREFIX + key, encrypt(value)).apply()
    }

    override fun delete(key: String) {
        prefs.edit().remove(PREFIX + key).apply()
    }

    private fun encrypt(value: String): String {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.ENCRYPT_MODE, getOrCreateKey())
        val iv = cipher.iv
        val encrypted = cipher.doFinal(value.toByteArray(Charsets.UTF_8))
        return Base64.encodeToString(iv + encrypted, Base64.NO_WRAP)
    }

    private fun decrypt(raw: String): String {
        val bytes = Base64.decode(raw, Base64.NO_WRAP)
        val iv = bytes.copyOfRange(0, IV_LEN)
        val ciphertext = bytes.copyOfRange(IV_LEN, bytes.size)
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.DECRYPT_MODE, getOrCreateKey(), GCMParameterSpec(TAG_LEN_BITS, iv))
        return String(cipher.doFinal(ciphertext), Charsets.UTF_8)
    }

    private companion object {
        const val ANDROID_KEYSTORE = "AndroidKeyStore"
        const val KEY_ALIAS = "omniproxy_secret_key"
        const val PREFS = "omniproxy_keystore"
        const val PREFIX = "enc:"
        const val TRANSFORMATION = "AES/GCM/NoPadding"
        const val IV_LEN = 12
        const val TAG_LEN_BITS = 128
    }
}
