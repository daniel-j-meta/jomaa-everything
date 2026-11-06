package com.danieljomaa.devicepanel.domain

import com.danieljomaa.devicepanel.data.DevicePanelApi
import com.danieljomaa.devicepanel.data.DevicePanelException
import com.danieljomaa.devicepanel.data.RoomChanges
import com.danieljomaa.devicepanel.data.RoomCommand
import com.danieljomaa.devicepanel.data.RoomState
import com.danieljomaa.devicepanel.data.SessionStore
import com.danieljomaa.devicepanel.data.SharedStateStore
import com.danieljomaa.devicepanel.data.applying
import com.danieljomaa.devicepanel.data.validateServerUrl

class IntentCommandService(
    private val api: DevicePanelApi,
    private val sessionStore: SessionStore,
    private val stateStore: SharedStateStore,
) {
    suspend fun apply(changes: RoomChanges): RoomState {
        val session = sessionStore.load()?.takeUnless(SharedStateStore::sessionExpired)
            ?: throw DevicePanelException.AuthenticationRequired
        val url = stateStore.serverUrl()?.let { validateServerUrl(it, allowInsecureLoopback = false) }
            ?: throw DevicePanelException.InvalidServerUrl
        val current = stateStore.roomState() ?: api.fetchRoom(url, session.token)
        val command = RoomCommand(
            clientId = stateStore.clientId(),
            revision = stateStore.nextRevision(),
            changes = changes,
        )
        val response = api.sendCommand(url, session.token, command)
        if (!response.accepted) throw DevicePanelException.InvalidResponse
        return (response.state ?: current.applying(changes)).also(stateStore::saveRoomState)
    }
}
