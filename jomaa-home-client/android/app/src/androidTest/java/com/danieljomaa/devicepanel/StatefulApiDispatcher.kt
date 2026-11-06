package com.danieljomaa.devicepanel

import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.RecordedRequest
import org.json.JSONObject

class StatefulApiDispatcher : Dispatcher() {
    data class Snapshot(
        val on: Boolean = false,
        val brightness: Int = 50,
        val colorTemperaturePct: Double = 50.0,
        val colorMode: String = "temperature",
    )

    private val lock = Any()
    private var room = Snapshot()
    private var failNextCommand = false
    private var expireNextRequest = false
    private val paths = mutableListOf<String>()

    fun snapshot(): Snapshot = synchronized(lock) { room }

    fun paths(): List<String> = synchronized(lock) { paths.toList() }

    fun failNextCommand() = synchronized(lock) { failNextCommand = true }

    fun expireNextRequest() = synchronized(lock) { expireNextRequest = true }

    fun updateRoom(update: (Snapshot) -> Snapshot) = synchronized(lock) {
        room = update(room)
    }

    override fun dispatch(request: RecordedRequest): MockResponse {
        synchronized(lock) { paths += request.path.orEmpty().substringBefore('?') }
        val path = request.path.orEmpty().substringBefore('?')

        if (request.method == "POST" && path == "/api/v1/sessions") {
            val password = JSONObject(request.body.readUtf8()).optString("password")
            return if (password == "test-pass") {
                json(
                    200,
                    """{"token":"android-test-token","expiresAt":"2099-01-01T00:00:00.000Z","serverLabel":"Emulator Home"}""",
                )
            } else {
                json(401, """{"code":"INVALID_CREDENTIALS","message":"Incorrect password"}""")
            }
        }

        if (request.getHeader("Authorization") != "Bearer android-test-token") {
            return json(401, """{"code":"AUTH_EXPIRED","message":"Sign in again"}""")
        }
        val expire = synchronized(lock) {
            expireNextRequest.also { expireNextRequest = false }
        }
        if (expire) return json(401, """{"code":"AUTH_EXPIRED","message":"Sign in again"}""")

        return when (request.method to path) {
            "GET" to "/api/v1/rooms/living-room" -> json(200, roomJson())
            "POST" to "/api/v1/rooms/living-room/commands" -> {
                val fail = synchronized(lock) {
                    failNextCommand.also { failNextCommand = false }
                }
                if (fail) {
                    json(502, """{"code":"UPSTREAM_ERROR","message":"Simulated command failure"}""")
                } else {
                    applyCommand(JSONObject(request.body.readUtf8()).getJSONObject("changes"))
                    json(200, """{"accepted":true,"state":null}""")
                }
            }
            "POST" to "/api/v1/rooms/living-room/reconcile" -> json(
                200,
                """{"state":${roomJson()},"synchronized":true,"corrected":[],"stale":false}""",
            )
            "DELETE" to "/api/v1/session" -> MockResponse().setResponseCode(204)
            else -> json(404, """{"code":"NOT_FOUND","message":"Not found"}""")
        }
    }

    private fun applyCommand(changes: JSONObject) = synchronized(lock) {
        room = room.copy(
            on = if (changes.has("on")) changes.getBoolean("on") else room.on,
            brightness = if (changes.has("brightness")) changes.getInt("brightness") else room.brightness,
            colorTemperaturePct = if (changes.has("colorTemperaturePct")) {
                changes.getDouble("colorTemperaturePct")
            } else room.colorTemperaturePct,
            colorMode = when {
                changes.has("color") -> changes.getString("color")
                changes.has("colorTemperaturePct") -> "temperature"
                else -> room.colorMode
            },
        )
    }

    private fun roomJson(): String {
        val value = snapshot()
        return """{"on":${value.on},"brightness":${value.brightness},"colorTemperaturePct":${value.colorTemperaturePct},"colorMode":"${value.colorMode}","observedAt":"2026-10-08T12:00:00.123Z","stateSource":"representative"}"""
    }

    private fun json(status: Int, body: String) = MockResponse()
        .setResponseCode(status)
        .setHeader("Content-Type", "application/json")
        .setBody(body)
}
