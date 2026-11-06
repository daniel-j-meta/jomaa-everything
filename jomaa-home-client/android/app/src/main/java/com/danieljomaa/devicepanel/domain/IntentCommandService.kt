package com.danieljomaa.devicepanel.domain

import com.danieljomaa.devicepanel.data.CachedTarget
import com.danieljomaa.devicepanel.data.CommandRequest
import com.danieljomaa.devicepanel.data.DevicePanelApi
import com.danieljomaa.devicepanel.data.DevicePanelException
import com.danieljomaa.devicepanel.data.RoomChanges
import com.danieljomaa.devicepanel.data.RoomState
import com.danieljomaa.devicepanel.data.SessionStore
import com.danieljomaa.devicepanel.data.SharedStateStore
import com.danieljomaa.devicepanel.data.validateServerUrl

/** Sends a command to the last room the app showed, for the home-screen widget. */
class IntentCommandService(
    private val api: DevicePanelApi,
    private val sessionStore: SessionStore,
    private val stateStore: SharedStateStore,
) {
    suspend fun apply(changes: RoomChanges): CachedTarget {
        val session = sessionStore.load() ?: throw DevicePanelException.AuthenticationRequired
        val url = stateStore.serverUrl()?.let(::validateServerUrl) ?: throw DevicePanelException.InvalidServerUrl
        val target = stateStore.selectedTarget() ?: throw DevicePanelException.InvalidResponse
        val current = target.state?.let(RoomState::from)
            ?: api.state(url, session.token, target.id).state?.let(RoomState::from)
            ?: throw DevicePanelException.InvalidResponse
        api.command(
            url,
            session.token,
            target.id,
            CommandRequest.from(changes, stateStore.clientId(), stateStore.nextRevision()),
        )
        return target.copy(state = current.applying(changes).toLightState()).also(stateStore::saveSelectedTarget)
    }
}
