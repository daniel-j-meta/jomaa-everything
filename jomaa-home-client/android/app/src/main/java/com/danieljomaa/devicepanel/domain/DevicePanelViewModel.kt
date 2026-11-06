package com.danieljomaa.devicepanel.domain

import android.util.Log
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.danieljomaa.devicepanel.data.DevicePanelApi
import com.danieljomaa.devicepanel.data.DevicePanelException
import com.danieljomaa.devicepanel.data.ReconcileRequest
import com.danieljomaa.devicepanel.data.RoomChanges
import com.danieljomaa.devicepanel.data.RoomColor
import com.danieljomaa.devicepanel.data.RoomColorMode
import com.danieljomaa.devicepanel.data.RoomCommand
import com.danieljomaa.devicepanel.data.RoomState
import com.danieljomaa.devicepanel.data.Session
import com.danieljomaa.devicepanel.data.SessionStore
import com.danieljomaa.devicepanel.data.SharedStateStore
import com.danieljomaa.devicepanel.data.SignInRequest
import com.danieljomaa.devicepanel.data.applying
import com.danieljomaa.devicepanel.data.validateServerUrl
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import okhttp3.HttpUrl

sealed interface AuthenticationPhase {
    data object Checking : AuthenticationPhase
    data object SigningIn : AuthenticationPhase
    data object SignedOut : AuthenticationPhase
    data class SignedIn(val serverLabel: String) : AuthenticationPhase
}

enum class ConnectionPhase(val label: String) {
    IDLE("Not connected"),
    LOADING("Loading"),
    CONNECTED("Connected"),
    APPLYING("Applying"),
    STALE("Last known state"),
    FAILED("Command failed"),
}

data class DevicePanelUiState(
    val authentication: AuthenticationPhase = AuthenticationPhase.Checking,
    val connection: ConnectionPhase = ConnectionPhase.IDLE,
    val room: RoomState? = null,
    val serverUrl: String = "",
    val powerBusy: Boolean = false,
    val error: String? = null,
    val canRetryCommand: Boolean = false,
)

class DevicePanelViewModel(
    private val api: DevicePanelApi,
    private val sessionStore: SessionStore,
    private val stateStore: SharedStateStore,
    private val allowInsecureLoopback: Boolean = false,
    private val resetOnStart: Boolean = false,
    private val reconcileDelaysMs: List<Long> = listOf(3_000, 6_000, 10_000, 15_000),
) : ViewModel() {
    private val mutableState = MutableStateFlow(DevicePanelUiState())
    val state: StateFlow<DevicePanelUiState> = mutableState.asStateFlow()

    private val commandMutex = Mutex()
    private var baseUrl: HttpUrl? = null
    private var session: Session? = null
    private var confirmedRoom: RoomState? = null
    private var latestRevision = 0L
    private var lastFailedChanges: RoomChanges? = null
    private var reconcileJob: Job? = null

    init {
        viewModelScope.launch { start() }
    }

    suspend fun start() {
        if (resetOnStart) {
            sessionStore.clear()
            stateStore.reset()
        }
        stateStore.roomState()?.let {
            confirmedRoom = it
            mutableState.value = mutableState.value.copy(room = it, connection = ConnectionPhase.STALE)
        }
        val savedUrl = stateStore.serverUrl()
        mutableState.value = mutableState.value.copy(serverUrl = savedUrl.orEmpty())
        val storedSession = sessionStore.load()
        if (savedUrl == null || storedSession == null || SharedStateStore.sessionExpired(storedSession)) {
            mutableState.value = mutableState.value.copy(authentication = AuthenticationPhase.SignedOut)
            return
        }
        try {
            baseUrl = validateServerUrl(savedUrl, allowInsecureLoopback)
            session = storedSession
            mutableState.value = mutableState.value.copy(
                authentication = AuthenticationPhase.SignedIn(storedSession.serverLabel),
            )
            refresh()
        } catch (error: Exception) {
            Log.e("DevicePanel", "Sign-in failed", error)
            mutableState.value = mutableState.value.copy(
                authentication = AuthenticationPhase.SignedOut,
                error = error.userMessage(),
                connection = ConnectionPhase.FAILED,
            )
        }
    }

    fun signIn(serverUrl: String, password: String, deviceName: String) = viewModelScope.launch {
        if (mutableState.value.authentication == AuthenticationPhase.SigningIn) return@launch
        mutableState.value = mutableState.value.copy(
            authentication = AuthenticationPhase.SigningIn,
            error = null,
        )
        try {
            val validated = validateServerUrl(serverUrl, allowInsecureLoopback)
            val newSession = api.signIn(
                validated,
                SignInRequest(password, stateStore.clientId(), deviceName),
            )
            if (SharedStateStore.sessionExpired(newSession)) {
                throw DevicePanelException.AuthenticationRequired
            }
            sessionStore.save(newSession)
            stateStore.saveServerUrl(validated.toString().removeSuffix("/"))
            baseUrl = validated
            session = newSession
            mutableState.value = mutableState.value.copy(
                authentication = AuthenticationPhase.SignedIn(newSession.serverLabel),
                serverUrl = validated.toString().removeSuffix("/"),
            )
            refresh()
        } catch (error: Exception) {
            mutableState.value = mutableState.value.copy(
                authentication = AuthenticationPhase.SignedOut,
                connection = ConnectionPhase.FAILED,
                error = error.userMessage(),
            )
        }
    }

    suspend fun refresh() = commandMutex.withLock {
        val url = baseUrl ?: return@withLock
        val token = session?.token ?: return@withLock
        mutableState.value = mutableState.value.copy(
            connection = if (mutableState.value.room == null) ConnectionPhase.LOADING else ConnectionPhase.APPLYING,
            error = null,
        )
        try {
            val room = api.fetchRoom(url, token)
            confirmedRoom = room
            stateStore.saveRoomState(room)
            mutableState.value = mutableState.value.copy(
                room = room,
                connection = ConnectionPhase.CONNECTED,
            )
        } catch (error: Exception) {
            handle(error)
        }
    }

    fun refreshFromUi() = viewModelScope.launch { refresh() }

    fun setPower(on: Boolean) = viewModelScope.launch {
        mutableState.value = mutableState.value.copy(powerBusy = true)
        apply(RoomChanges(on = on))
        mutableState.value = mutableState.value.copy(powerBusy = false)
    }

    fun setBrightness(value: Int) = viewModelScope.launch { apply(RoomChanges(brightness = value)) }

    fun setColor(color: RoomColor) = viewModelScope.launch { apply(RoomChanges(color = color)) }

    fun setTemperature(value: Double) = viewModelScope.launch {
        apply(RoomChanges(colorTemperaturePct = value))
    }

    fun retryLastCommand() = viewModelScope.launch {
        lastFailedChanges?.let { apply(it) }
    }

    fun clearError() {
        mutableState.value = mutableState.value.copy(
            error = null,
            connection = if (mutableState.value.room == null) ConnectionPhase.IDLE else ConnectionPhase.STALE,
        )
    }

    fun signOut() = viewModelScope.launch {
        reconcileJob?.cancel()
        val url = baseUrl
        val token = session?.token
        if (url != null && token != null) runCatching { api.signOut(url, token) }
        sessionStore.clear()
        session = null
        baseUrl = null
        confirmedRoom = null
        lastFailedChanges = null
        mutableState.value = DevicePanelUiState(
            authentication = AuthenticationPhase.SignedOut,
            serverUrl = stateStore.serverUrl().orEmpty(),
        )
    }

    private suspend fun apply(changes: RoomChanges) = commandMutex.withLock {
        val url = baseUrl ?: return@withLock handle(DevicePanelException.AuthenticationRequired)
        val token = session?.token ?: return@withLock handle(DevicePanelException.AuthenticationRequired)
        val before = mutableState.value.room ?: return@withLock
        val optimistic = before.applying(changes)
        val revision = stateStore.nextRevision()
        latestRevision = revision
        mutableState.value = mutableState.value.copy(
            room = optimistic,
            connection = ConnectionPhase.APPLYING,
            error = null,
            canRetryCommand = false,
        )
        lastFailedChanges = null

        try {
            val response = api.sendCommand(
                url,
                token,
                RoomCommand(clientId = stateStore.clientId(), revision = revision, changes = changes),
            )
            if (!response.accepted) throw DevicePanelException.InvalidResponse
            val confirmed = response.state ?: optimistic
            confirmedRoom = confirmed
            stateStore.saveRoomState(confirmed)
            mutableState.value = mutableState.value.copy(
                room = optimistic,
                connection = ConnectionPhase.CONNECTED,
            )
            startReconciliation(revision)
        } catch (error: Exception) {
            mutableState.value = mutableState.value.copy(room = confirmedRoom ?: before)
            lastFailedChanges = changes
            mutableState.value = mutableState.value.copy(canRetryCommand = true)
            handle(error)
        }
    }

    private fun startReconciliation(revision: Long) {
        reconcileJob?.cancel()
        if (reconcileDelaysMs.isEmpty()) return
        reconcileJob = viewModelScope.launch {
            for (wait in reconcileDelaysMs) {
                delay(wait)
                if (revision != latestRevision) return@launch
                val room = mutableState.value.room ?: return@launch
                val url = baseUrl ?: return@launch
                val token = session?.token ?: return@launch
                try {
                    val response = api.reconcile(
                        url,
                        token,
                        ReconcileRequest(
                            clientId = stateStore.clientId(),
                            revision = revision,
                            on = room.on,
                            brightness = room.brightness.takeIf { room.on },
                            colorTemperaturePct = room.colorTemperaturePct.takeIf {
                                room.on && room.colorMode == RoomColorMode.TEMPERATURE
                            },
                        ),
                    )
                    if (response.stale || revision != latestRevision) return@launch
                    if (response.synchronized) {
                        mutableState.value = mutableState.value.copy(connection = ConnectionPhase.CONNECTED)
                        return@launch
                    }
                } catch (error: Exception) {
                    if (error is DevicePanelException.AuthenticationRequired) {
                        handle(error)
                        return@launch
                    }
                    mutableState.value = mutableState.value.copy(
                        connection = ConnectionPhase.STALE,
                        error = error.userMessage(),
                    )
                }
            }
            if (revision == latestRevision) {
                mutableState.value = mutableState.value.copy(connection = ConnectionPhase.STALE)
            }
        }
    }

    private suspend fun handle(error: Exception) {
        Log.e("DevicePanel", "Device Panel operation failed", error)
        if (error is DevicePanelException.AuthenticationRequired) {
            sessionStore.clear()
            session = null
            mutableState.value = mutableState.value.copy(authentication = AuthenticationPhase.SignedOut)
        }
        mutableState.value = mutableState.value.copy(
            connection = if (error is DevicePanelException.Offline) ConnectionPhase.STALE else ConnectionPhase.FAILED,
            error = error.userMessage(),
        )
    }
}

private fun Exception.userMessage(): String =
    (this as? DevicePanelException)?.message ?: "Device Panel could not complete the request."
