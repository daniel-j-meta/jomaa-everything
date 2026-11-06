package com.danieljomaa.devicepanel.ui

import androidx.compose.animation.AnimatedContent
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Error
import androidx.compose.material.icons.filled.Lightbulb
import androidx.compose.material.icons.filled.PowerSettingsNew
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.WifiOff
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Slider
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableDoubleStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.danieljomaa.devicepanel.data.RoomColor
import com.danieljomaa.devicepanel.data.RoomColorMode
import com.danieljomaa.devicepanel.domain.AuthenticationPhase
import com.danieljomaa.devicepanel.domain.ConnectionPhase
import com.danieljomaa.devicepanel.domain.DevicePanelUiState
import com.danieljomaa.devicepanel.domain.DevicePanelViewModel

private val BackgroundTop = Color(0xFF1A1A2E)
private val BackgroundBottom = Color(0xFF16213E)
private val Accent = Color(0xFFFFB54D)
private val Success = Color(0xFF4CAF50)
private val Failure = Color(0xFFFF5555)
private val Surface = Color.White.copy(alpha = 0.08f)

@Composable
fun DevicePanelApp(viewModel: DevicePanelViewModel, deviceName: String) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    MaterialTheme(
        colorScheme = darkColorScheme(
            primary = Accent,
            background = BackgroundTop,
            surface = BackgroundBottom,
        ),
    ) {
        Box(
            modifier = Modifier
                .fillMaxSize()
                .background(Brush.linearGradient(listOf(BackgroundTop, BackgroundBottom))),
        ) {
            AnimatedContent(state.authentication, label = "authentication") { authentication ->
                when (authentication) {
                    AuthenticationPhase.Checking -> LoadingScreen()
                    AuthenticationPhase.SigningIn,
                    AuthenticationPhase.SignedOut,
                    -> SignInScreen(
                        state = state,
                        deviceName = deviceName,
                        signIn = viewModel::signIn,
                    )
                    is AuthenticationPhase.SignedIn -> ControlScreen(
                        state = state,
                        setPower = viewModel::setPower,
                        setBrightness = viewModel::setBrightness,
                        setColor = viewModel::setColor,
                        setTemperature = viewModel::setTemperature,
                        refreshAction = { viewModel.refreshFromUi() },
                        retry = viewModel::retryLastCommand,
                        dismissError = viewModel::clearError,
                        signOut = viewModel::signOut,
                    )
                }
            }
        }
    }
}

@Composable
private fun LoadingScreen() {
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        CircularProgressIndicator(Modifier.semantics { contentDescription = "Loading Device Panel" })
    }
}

@Composable
private fun SignInScreen(
    state: DevicePanelUiState,
    deviceName: String,
    signIn: (String, String, String) -> Unit,
) {
    var serverUrl by remember(state.serverUrl) { mutableStateOf(state.serverUrl) }
    var password by remember { mutableStateOf("") }
    val signingIn = state.authentication == AuthenticationPhase.SigningIn

    Column(
        modifier = Modifier
            .fillMaxSize()
            .statusBarsPadding()
            .navigationBarsPadding()
            .verticalScroll(rememberScrollState())
            .padding(24.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
    ) {
        Icon(Icons.Default.Lightbulb, contentDescription = null, tint = Accent, modifier = Modifier.size(64.dp))
        Text("Device Panel", fontSize = 32.sp, fontWeight = FontWeight.Bold)
        Text("Control the Living Room from this device.", color = Color.White.copy(alpha = 0.7f))
        Spacer(Modifier.height(24.dp))
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(20.dp))
                .background(Surface)
                .padding(20.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            OutlinedTextField(
                value = serverUrl,
                onValueChange = { serverUrl = it },
                label = { Text("Server URL") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth().testTag("serverUrlField"),
            )
            OutlinedTextField(
                value = password,
                onValueChange = { password = it },
                label = { Text("Panel password") },
                visualTransformation = PasswordVisualTransformation(),
                singleLine = true,
                modifier = Modifier.fillMaxWidth().testTag("passwordField"),
            )
            state.error?.let {
                Text(it, color = Failure, modifier = Modifier.testTag("signInError"))
            }
            Button(
                onClick = {
                    signIn(serverUrl, password, deviceName)
                    password = ""
                },
                enabled = serverUrl.isNotBlank() && password.isNotEmpty() && !signingIn,
                modifier = Modifier.fillMaxWidth().height(52.dp).testTag("signInButton"),
            ) {
                if (signingIn) CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
                Text(if (signingIn) "Signing In…" else "Sign In")
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ControlScreen(
    state: DevicePanelUiState,
    setPower: (Boolean) -> Unit,
    setBrightness: (Int) -> Unit,
    setColor: (RoomColor) -> Unit,
    setTemperature: (Double) -> Unit,
    refreshAction: () -> Unit,
    retry: () -> Unit,
    dismissError: () -> Unit,
    signOut: () -> Unit,
) {
    var showSettings by remember { mutableStateOf(false) }
    val room = state.room
    val haptics = LocalHapticFeedback.current
    var temperature by remember(room?.colorTemperaturePct) {
        mutableDoubleStateOf(room?.colorTemperaturePct ?: 50.0)
    }

    Scaffold(
        modifier = Modifier.testTag("controlScreen"),
        containerColor = Color.Transparent,
        topBar = {
            TopAppBar(
                title = { Text("Living Room") },
                actions = {
                    IconButton(
                        onClick = refreshAction,
                        modifier = Modifier.testTag("refreshButton"),
                    ) { Icon(Icons.Default.Refresh, contentDescription = "Refresh") }
                    IconButton(
                        onClick = { showSettings = true },
                        modifier = Modifier.testTag("settingsButton"),
                    ) { Icon(Icons.Default.Settings, contentDescription = "Settings") }
                },
            )
        },
    ) { padding ->
        Column(
            modifier = Modifier
                .padding(padding)
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(18.dp),
            verticalArrangement = Arrangement.spacedBy(18.dp),
        ) {
            StatusCard(state.connection)
            if (room != null) {
                Button(
                    onClick = {
                        haptics.performHapticFeedback(HapticFeedbackType.LongPress)
                        setPower(!room.on)
                    },
                    enabled = !state.powerBusy,
                    colors = ButtonDefaults.buttonColors(containerColor = Surface),
                    contentPadding = PaddingValues(20.dp),
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(80.dp)
                        .testTag("powerControl")
                        .semantics { stateDescription = if (room.on) "On" else "Off" },
                ) {
                    Icon(Icons.Default.PowerSettingsNew, contentDescription = null)
                    Text(
                        if (room.on) "Living Room is on" else "Living Room is off",
                        modifier = Modifier.padding(start = 12.dp).weight(1f),
                    )
                    if (state.powerBusy) CircularProgressIndicator(Modifier.size(22.dp), strokeWidth = 2.dp)
                    else Icon(Icons.Default.CheckCircle, contentDescription = null, tint = if (room.on) Success else Color.Gray)
                }

                Text("Color", fontWeight = FontWeight.SemiBold)
                Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    ColorButton("Red", Color.Red, room.colorMode == RoomColorMode.RED, Modifier.weight(1f)) {
                        setColor(RoomColor.RED)
                    }
                    ColorButton("Orange", Color(0xFFFF8C00), room.colorMode == RoomColorMode.ORANGE, Modifier.weight(1f)) {
                        setColor(RoomColor.ORANGE)
                    }
                }

                Text("Brightness · Current ${room.brightness}%", fontWeight = FontWeight.SemiBold)
                LazyVerticalGrid(
                    columns = GridCells.Fixed(2),
                    modifier = Modifier.height(176.dp),
                    userScrollEnabled = false,
                    verticalArrangement = Arrangement.spacedBy(10.dp),
                    horizontalArrangement = Arrangement.spacedBy(10.dp),
                ) {
                    items(listOf(10, 50, 75, 100)) { preset ->
                        val selected = room.brightness == preset
                        OutlinedButton(
                            onClick = { setBrightness(preset) },
                            modifier = Modifier
                                .fillMaxWidth()
                                .height(80.dp)
                                .testTag("brightness$preset")
                                .semantics { stateDescription = if (selected) "Selected" else "Not selected" },
                        ) { Text("$preset%", fontWeight = FontWeight.Bold) }
                    }
                }

                Text("Color Temperature · ${signedTemperature(temperature)}", fontWeight = FontWeight.SemiBold)
                Slider(
                    value = temperature.toFloat(),
                    onValueChange = { temperature = it.toDouble() },
                    onValueChangeFinished = { setTemperature(temperature) },
                    valueRange = 0f..100f,
                    steps = 99,
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(56.dp)
                        .testTag("temperatureSlider")
                        .semantics { stateDescription = accessibleTemperature(temperature) },
                )
                Row(Modifier.fillMaxWidth()) {
                    Text("Warm −100%", color = Color.White.copy(alpha = 0.65f))
                    Spacer(Modifier.weight(1f))
                    Text("Cool +100%", color = Color.White.copy(alpha = 0.65f))
                }
            } else if (state.connection == ConnectionPhase.LOADING) {
                CircularProgressIndicator(Modifier.align(Alignment.CenterHorizontally))
            }

            state.error?.let { error ->
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(16.dp))
                        .background(Surface)
                        .padding(16.dp)
                        .testTag("errorBanner"),
                ) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Icon(Icons.Default.Error, contentDescription = null, tint = Failure)
                        Text(error, color = Failure, modifier = Modifier.padding(start = 8.dp).weight(1f))
                        IconButton(onClick = dismissError) {
                            Icon(Icons.Default.Close, contentDescription = "Dismiss")
                        }
                    }
                    Button(onClick = retry, modifier = Modifier.testTag("retryCommand")) {
                        Text(if (state.canRetryCommand) "Retry Command" else "Refresh")
                    }
                }
            }
        }
    }

    if (showSettings) {
        AlertDialog(
            onDismissRequest = { showSettings = false },
            title = { Text("Settings") },
            text = { Text("Server: ${state.serverUrl}\n\nThe session is encrypted with Android Keystore.") },
            confirmButton = { TextButton(onClick = { showSettings = false }) { Text("Done") } },
            dismissButton = { TextButton(onClick = signOut) { Text("Sign Out", color = Failure) } },
        )
    }
}

@Composable
private fun StatusCard(phase: ConnectionPhase) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(16.dp))
            .background(Surface)
            .padding(14.dp)
            .testTag("connectionStatus")
            .semantics { stateDescription = phase.label },
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            imageVector = if (phase == ConnectionPhase.STALE) Icons.Default.WifiOff else Icons.Default.CheckCircle,
            contentDescription = null,
            tint = when (phase) {
                ConnectionPhase.CONNECTED -> Success
                ConnectionPhase.FAILED -> Failure
                else -> Accent
            },
        )
        Text(phase.label, modifier = Modifier.padding(start = 10.dp), fontWeight = FontWeight.SemiBold)
    }
}

@Composable
private fun ColorButton(
    label: String,
    color: Color,
    selected: Boolean,
    modifier: Modifier,
    onClick: () -> Unit,
) {
    Button(
        onClick = onClick,
        colors = ButtonDefaults.buttonColors(containerColor = color),
        modifier = modifier
            .height(86.dp)
            .testTag("color$label")
            .semantics { stateDescription = if (selected) "Selected" else "Not selected" },
    ) {
        if (selected) Icon(Icons.Default.CheckCircle, contentDescription = null)
        Text(label, modifier = Modifier.padding(start = if (selected) 8.dp else 0.dp))
    }
}

private fun signedTemperature(value: Double): String {
    val signed = (value.coerceIn(0.0, 100.0) * 2 - 100).toInt()
    return if (signed > 0) "+$signed%" else "$signed%"
}

private fun accessibleTemperature(value: Double): String = when {
    value < 50 -> "${((50 - value) * 2).toInt()} percent warm"
    value > 50 -> "${((value - 50) * 2).toInt()} percent cool"
    else -> "neutral"
}
