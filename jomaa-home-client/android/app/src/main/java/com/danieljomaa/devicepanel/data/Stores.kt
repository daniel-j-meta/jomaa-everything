package com.danieljomaa.devicepanel.data

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import java.security.KeyStore
import java.time.Instant
import java.util.UUID
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

interface SessionStore {
    suspend fun load(): Session?
    suspend fun save(session: Session)
    suspend fun clear()
}

class KeystoreSessionStore(context: Context) : SessionStore {
    private val preferences = context.getSharedPreferences("device_panel_secure", Context.MODE_PRIVATE)
    private val json = Json

    override suspend fun load(): Session? {
        val encoded = preferences.getString(KEY_VALUE, null) ?: return null
        return try {
            val combined = Base64.decode(encoded, Base64.NO_WRAP)
            val iv = combined.copyOfRange(0, IV_BYTES)
            val encrypted = combined.copyOfRange(IV_BYTES, combined.size)
            val cipher = Cipher.getInstance(TRANSFORMATION)
            cipher.init(Cipher.DECRYPT_MODE, secretKey(), GCMParameterSpec(128, iv))
            json.decodeFromString<Session>(cipher.doFinal(encrypted).decodeToString())
        } catch (error: Exception) {
            clear()
            null
        }
    }

    override suspend fun save(session: Session) {
        try {
            val cipher = Cipher.getInstance(TRANSFORMATION)
            cipher.init(Cipher.ENCRYPT_MODE, secretKey())
            val encrypted = cipher.doFinal(json.encodeToString(session).encodeToByteArray())
            val combined = cipher.iv + encrypted
            preferences.edit().putString(KEY_VALUE, Base64.encodeToString(combined, Base64.NO_WRAP)).commit()
        } catch (error: Exception) {
            throw DevicePanelException.SecureStorage
        }
    }

    override suspend fun clear() {
        preferences.edit().remove(KEY_VALUE).commit()
    }

    private fun secretKey(): SecretKey {
        val keyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (keyStore.getKey(KEY_ALIAS, null) as? SecretKey)?.let { return it }
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
        generator.init(
            KeyGenParameterSpec.Builder(
                KEY_ALIAS,
                KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
            )
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setUserAuthenticationRequired(false)
                .build(),
        )
        return generator.generateKey()
    }

    private companion object {
        const val KEY_ALIAS = "device_panel_session_key"
        const val KEY_VALUE = "session"
        const val TRANSFORMATION = "AES/GCM/NoPadding"
        const val IV_BYTES = 12
    }
}

class SharedStateStore(context: Context) {
    private val preferences = context.getSharedPreferences("device_panel_shared", Context.MODE_PRIVATE)
    private val json = Json { encodeDefaults = false; explicitNulls = false }
    private val lock = Any()

    fun serverUrl(): String? = preferences.getString(KEY_SERVER_URL, null)

    fun saveServerUrl(value: String) {
        preferences.edit().putString(KEY_SERVER_URL, value).commit()
    }

    fun clientId(): String = synchronized(lock) {
        preferences.getString(KEY_CLIENT_ID, null)?.let { return@synchronized it }
        UUID.randomUUID().toString().also {
            preferences.edit().putString(KEY_CLIENT_ID, it).commit()
        }
    }

    fun nextRevision(): Long = synchronized(lock) {
        val current = preferences.getLong(KEY_REVISION, 0)
        val timestamp = System.currentTimeMillis() * 1_000
        maxOf(current + 1, timestamp).also {
            preferences.edit().putLong(KEY_REVISION, it).commit()
        }
    }

    fun roomState(): RoomState? = preferences.getString(KEY_ROOM, null)?.let {
        runCatching { json.decodeFromString<RoomState>(it) }.getOrNull()
    }

    fun saveRoomState(state: RoomState) {
        preferences.edit().putString(KEY_ROOM, json.encodeToString(state)).commit()
    }

    fun reset() {
        preferences.edit().clear().commit()
    }

    companion object {
        private const val KEY_SERVER_URL = "server_url"
        private const val KEY_CLIENT_ID = "client_id"
        private const val KEY_REVISION = "revision"
        private const val KEY_ROOM = "room_state"

        fun sessionExpired(session: Session): Boolean = runCatching {
            Instant.parse(session.expiresAt).isBefore(Instant.now())
        }.getOrDefault(true)
    }
}
