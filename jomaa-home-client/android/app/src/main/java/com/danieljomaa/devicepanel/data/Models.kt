package com.danieljomaa.devicepanel.data

import kotlinx.serialization.KSerializer
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException
import kotlinx.serialization.descriptors.PrimitiveKind
import kotlinx.serialization.descriptors.PrimitiveSerialDescriptor
import kotlinx.serialization.descriptors.SerialDescriptor
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder
import java.util.UUID

@Serializable(with = RoomColorModeSerializer::class)
enum class RoomColorMode(val wireValue: String) {
    TEMPERATURE("temperature"),
    RED("red"),
    ORANGE("orange"),
    UNKNOWN("unknown"),
}

object RoomColorModeSerializer : KSerializer<RoomColorMode> {
    override val descriptor: SerialDescriptor =
        PrimitiveSerialDescriptor("RoomColorMode", PrimitiveKind.STRING)

    override fun deserialize(decoder: Decoder): RoomColorMode {
        val value = decoder.decodeString()
        return RoomColorMode.entries.firstOrNull { it.wireValue == value } ?: RoomColorMode.UNKNOWN
    }

    override fun serialize(encoder: Encoder, value: RoomColorMode) {
        encoder.encodeString(value.wireValue)
    }
}

@Serializable
data class RoomState(
    val on: Boolean,
    val brightness: Int,
    val colorTemperaturePct: Double,
    val colorMode: RoomColorMode,
    val observedAt: String,
    val stateSource: String,
) {
    init {
        require(brightness in 1..100) { "brightness must be from 1 to 100" }
        require(colorTemperaturePct in 0.0..100.0) { "colorTemperaturePct must be from 0 to 100" }
    }
}

@Serializable
data class Session(
    val token: String,
    val expiresAt: String,
    val serverLabel: String,
)

@Serializable
data class SignInRequest(
    val password: String,
    val clientId: String,
    val deviceName: String,
)

@Serializable
enum class RoomColor(val wireValue: String) {
    @kotlinx.serialization.SerialName("red")
    RED("red"),

    @kotlinx.serialization.SerialName("orange")
    ORANGE("orange"),
}

@Serializable
data class RoomChanges(
    val on: Boolean? = null,
    val brightness: Int? = null,
    val colorTemperaturePct: Double? = null,
    val color: RoomColor? = null,
) {
    init {
        require(on != null || brightness != null || colorTemperaturePct != null || color != null) {
            "A command must contain a change"
        }
        brightness?.let { require(it in 1..100) }
        colorTemperaturePct?.let { require(it in 0.0..100.0) }
        require(color == null || colorTemperaturePct == null) {
            "Color and temperature cannot change together"
        }
    }
}

@Serializable
data class RoomCommand(
    val commandId: String = UUID.randomUUID().toString(),
    val clientId: String,
    val revision: Long,
    val changes: RoomChanges,
)

@Serializable
data class CommandResponse(
    val accepted: Boolean,
    val state: RoomState? = null,
)

@Serializable
data class ReconcileRequest(
    val clientId: String,
    val revision: Long,
    val on: Boolean,
    val brightness: Int? = null,
    val colorTemperaturePct: Double? = null,
)

@Serializable
data class ReconcileResponse(
    val state: RoomState? = null,
    val synchronized: Boolean,
    val corrected: List<String>,
    val stale: Boolean,
)

@Serializable
data class ApiErrorPayload(val code: String? = null, val message: String? = null)

sealed class DevicePanelException(message: String) : Exception(message) {
    data object InvalidServerUrl : DevicePanelException("Enter a valid HTTPS Device Panel address.")
    data object InvalidCredentials : DevicePanelException("Incorrect password.")
    data object AuthenticationRequired : DevicePanelException("Your session has expired. Sign in again.")
    data object PermissionDenied : DevicePanelException("This device cannot control the room.")
    data object Offline : DevicePanelException("Device Panel is offline. Check your connection and try again.")
    data object TimedOut : DevicePanelException("Device Panel took too long to respond.")
    data class RateLimited(val retryAfterSeconds: Long?) :
        DevicePanelException("Too many requests. Wait a moment and try again.")
    data class Server(val status: Int, val code: String?, override val message: String?) :
        DevicePanelException(message ?: "Device Panel could not complete the request.")
    data object InvalidResponse : DevicePanelException("Device Panel returned an unexpected response.")
    data object SecureStorage : DevicePanelException("Secure storage is unavailable.")
}

fun RoomState.applying(changes: RoomChanges): RoomState = copy(
    on = changes.on ?: on,
    brightness = changes.brightness ?: brightness,
    colorTemperaturePct = changes.colorTemperaturePct ?: colorTemperaturePct,
    colorMode = when {
        changes.color == RoomColor.RED -> RoomColorMode.RED
        changes.color == RoomColor.ORANGE -> RoomColorMode.ORANGE
        changes.colorTemperaturePct != null -> RoomColorMode.TEMPERATURE
        else -> colorMode
    },
)

fun validateUuid(value: String): String = try {
    UUID.fromString(value).toString()
} catch (error: IllegalArgumentException) {
    throw SerializationException("Invalid UUID", error)
}
