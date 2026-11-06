package com.danieljomaa.devicepanel.data

import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import okhttp3.Call
import okhttp3.Callback
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import java.io.IOException
import java.net.SocketTimeoutException
import java.util.concurrent.TimeUnit
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

interface DevicePanelApi {
    suspend fun signIn(baseUrl: HttpUrl, request: SignInRequest): Session
    suspend fun signOut(baseUrl: HttpUrl, token: String)
    suspend fun fetchRoom(baseUrl: HttpUrl, token: String): RoomState
    suspend fun sendCommand(baseUrl: HttpUrl, token: String, command: RoomCommand): CommandResponse
    suspend fun reconcile(baseUrl: HttpUrl, token: String, request: ReconcileRequest): ReconcileResponse
}

class OkHttpDevicePanelApi(
    private val client: OkHttpClient = OkHttpClient.Builder()
        .connectTimeout(15, TimeUnit.SECONDS)
        .readTimeout(15, TimeUnit.SECONDS)
        .writeTimeout(15, TimeUnit.SECONDS)
        .callTimeout(30, TimeUnit.SECONDS)
        .build(),
    allowedHosts: Set<String>? = null,
) : DevicePanelApi {
    private val allowedHosts = allowedHosts?.map(String::lowercase)?.toSet()
    private val json = Json {
        ignoreUnknownKeys = false
        encodeDefaults = false
        explicitNulls = false
    }
    private val mediaType = "application/json; charset=utf-8".toMediaType()

    override suspend fun signIn(baseUrl: HttpUrl, request: SignInRequest): Session =
        execute(
            baseUrl,
            "api/v1/sessions",
            "POST",
            body = json.encodeToString(request),
            unauthorizedError = DevicePanelException.InvalidCredentials,
        )

    override suspend fun signOut(baseUrl: HttpUrl, token: String) {
        executeEmpty(baseUrl, "api/v1/session", "DELETE", token)
    }

    override suspend fun fetchRoom(baseUrl: HttpUrl, token: String): RoomState =
        execute(baseUrl, "api/v1/rooms/living-room", "GET", token)

    override suspend fun sendCommand(
        baseUrl: HttpUrl,
        token: String,
        command: RoomCommand,
    ): CommandResponse = execute(
        baseUrl,
        "api/v1/rooms/living-room/commands",
        "POST",
        token,
        json.encodeToString(command),
        mapOf("Idempotency-Key" to command.commandId),
    )

    override suspend fun reconcile(
        baseUrl: HttpUrl,
        token: String,
        request: ReconcileRequest,
    ): ReconcileResponse = execute(
        baseUrl,
        "api/v1/rooms/living-room/reconcile",
        "POST",
        token,
        json.encodeToString(request),
    )

    private suspend inline fun <reified T> execute(
        baseUrl: HttpUrl,
        path: String,
        method: String,
        token: String? = null,
        body: String? = null,
        headers: Map<String, String> = emptyMap(),
        unauthorizedError: DevicePanelException = DevicePanelException.AuthenticationRequired,
    ): T = withContext(Dispatchers.IO) {
        perform(baseUrl, path, method, token, body, headers).use {
            validate(it, unauthorizedError)
            val value = it.body?.string() ?: throw DevicePanelException.InvalidResponse
            try {
                json.decodeFromString<T>(value)
            } catch (error: Exception) {
                throw DevicePanelException.InvalidResponse
            }
        }
    }

    private suspend fun executeEmpty(
        baseUrl: HttpUrl,
        path: String,
        method: String,
        token: String,
    ) {
        withContext(Dispatchers.IO) {
            perform(baseUrl, path, method, token, null, emptyMap()).use {
                validate(it, DevicePanelException.AuthenticationRequired)
            }
        }
    }

    private suspend fun perform(
        baseUrl: HttpUrl,
        path: String,
        method: String,
        token: String?,
        body: String?,
        headers: Map<String, String>,
    ): Response {
        if (allowedHosts != null && baseUrl.host.lowercase() !in allowedHosts) {
            throw DevicePanelException.InvalidServerUrl
        }
        val url = baseUrl.newBuilder().addPathSegments(path).build()
        val requestBody = body?.toRequestBody(mediaType)
        val builder = Request.Builder().url(url).header("Accept", "application/json")
        token?.let { builder.header("Authorization", "Bearer $it") }
        headers.forEach(builder::header)
        when (method) {
            "GET" -> builder.get()
            "POST" -> builder.post(requestBody ?: ByteArray(0).toRequestBody(mediaType))
            "DELETE" -> builder.delete()
            else -> error("Unsupported HTTP method")
        }
        return try {
            client.newCall(builder.build()).await()
        } catch (error: SocketTimeoutException) {
            throw DevicePanelException.TimedOut
        } catch (error: IOException) {
            throw DevicePanelException.Offline
        }
    }

    private fun validate(response: Response, unauthorizedError: DevicePanelException) {
        if (response.isSuccessful) return
        val payload = response.body?.string()?.let {
            runCatching { json.decodeFromString<ApiErrorPayload>(it) }.getOrNull()
        }
        throw when (response.code) {
            401 -> unauthorizedError
            403 -> DevicePanelException.PermissionDenied
            429 -> DevicePanelException.RateLimited(response.header("Retry-After")?.toLongOrNull())
            else -> DevicePanelException.Server(response.code, payload?.code, payload?.message)
        }
    }
}

fun validateServerUrl(value: String, allowInsecureLoopback: Boolean): HttpUrl {
    val url = value.trim().toHttpUrlOrNull() ?: throw DevicePanelException.InvalidServerUrl
    val loopback = url.host in setOf("localhost", "127.0.0.1", "::1")
    if (!url.isHttps && !(allowInsecureLoopback && loopback)) {
        throw DevicePanelException.InvalidServerUrl
    }
    if (url.username.isNotEmpty() || url.password.isNotEmpty() || url.query != null || url.fragment != null) {
        throw DevicePanelException.InvalidServerUrl
    }
    return url
}

private suspend fun Call.await(): Response = suspendCancellableCoroutine { continuation ->
    continuation.invokeOnCancellation { cancel() }
    enqueue(object : Callback {
        override fun onFailure(call: Call, error: IOException) {
            if (continuation.isActive) continuation.resumeWithException(error)
        }

        override fun onResponse(call: Call, response: Response) {
            if (continuation.isActive) continuation.resume(response)
        }
    })
}
