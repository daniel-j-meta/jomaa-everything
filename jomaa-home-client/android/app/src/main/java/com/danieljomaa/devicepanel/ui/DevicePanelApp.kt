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
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items as rowItems
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Error
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Lightbulb
import androidx.compose.material.icons.filled.PowerSettingsNew
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.WifiOff
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilterChip
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LocalContentColor
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
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.mutableFloatStateOf
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
import com.danieljomaa.devicepanel.data.ColorPreset
import com.danieljomaa.devicepanel.data.Target
import com.danieljomaa.devicepanel.domain.MatterUiState
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
fun DevicePanelApp(viewModel: DevicePanelViewModel) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    MaterialTheme(
        colorScheme = darkColorScheme(
            primary = Accent,
            background = BackgroundTop,
            surface = BackgroundBottom,
        ),
    ) {
        // The gradient Box isn't a Material surface, so give text a light default.
        CompositionLocalProvider(LocalContentColor provides Color.White) {
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
                        signIn = viewModel::signIn,
                    )
                    AuthenticationPhase.SignedIn -> ControlScreen(
                        state = state,
                        select = viewModel::select,
                        setPower = viewModel::setPower,
                        setBrightness = viewModel::setBrightness,
                        setColor = viewModel::setColor,
                        setTemperature = viewModel::setTemperature,
                        refreshAction = { viewModel.refreshFromUi() },
                        retry = viewModel::retryLastCommand,
                        dismissError = viewModel::clearError,
                        signOut = viewModel::signOut,
                        loadMatterNodes = viewModel::loadMatterNodes,
                        pair = viewModel::pair,
                        clearPairingResult = viewModel::clearPairingResult,
                    )
                }
            }
        }
        }
    }
}

@Composable
private fun LoadingScreen() {
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        CircularProgressIndicator(Modifier.semantics { contentDescription = "Loading" })
    }
}

@Composable
private fun SignInScreen(
    state: DevicePanelUiState,
    signIn: (String, String) -> Unit,
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
        Text("Your home hub, one touch away.", color = Color.White.copy(alpha = 0.7f))
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
                label = { Text("Hub address") },
                placeholder = { Text("http://192.168.1.10:8080") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth().testTag("serverUrlField"),
            )
            OutlinedTextField(
                value = password,
                onValueChange = { password = it },
                label = { Text("Hub password") },
                visualTransformation = PasswordVisualTransformation(),
                singleLine = true,
                modifier = Modifier.fillMaxWidth().testTag("passwordField"),
            )
            state.error?.let {
                Text(it, color = Failure, modifier = Modifier.testTag("signInError"))
            }
            Button(
                onClick = {
                    signIn(serverUrl, password)
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
    select: (String) -> Unit,
    setPower: (Boolean) -> Unit,
    setBrightness: (Int) -> Unit,
    setColor: (ColorPreset) -> Unit,
    setTemperature: (Int) -> Unit,
    refreshAction: () -> Unit,
    retry: () -> Unit,
    dismissError: () -> Unit,
    signOut: () -> Unit,
    loadMatterNodes: () -> Unit,
    pair: (String, Boolean) -> Unit,
    clearPairingResult: () -> Unit,
) {
    var showSettings by remember { mutableStateOf(false) }
    var showMatter by remember { mutableStateOf(false) }
    val features = state.features
    val name = state.selected?.name ?: "Home"
    val room = state.room
    val haptics = LocalHapticFeedback.current
    var temperature by remember(room?.colorTemperaturePct) {
        mutableFloatStateOf(room?.colorTemperaturePct?.toFloat() ?: 50f)
    }

    Scaffold(
        modifier = Modifier.testTag("controlScreen"),
        containerColor = Color.Transparent,
        topBar = {
            TopAppBar(
                title = { Text(name, modifier = Modifier.testTag("targetTitle")) },
                actions = {
                    IconButton(
                        onClick = { showMatter = true; loadMatterNodes() },
                        modifier = Modifier.testTag("matterButton"),
                    ) { Icon(Icons.Default.Add, contentDescription = "Add Matter device") }
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
            if (state.targets.size > 1) {
                TargetPicker(state.targets, state.selected?.id, select)
            }
            if (room != null) {
                Button(
                    onClick = {
                        haptics.performHapticFeedback(HapticFeedbackType.LongPress)
                        setPower(!room.on)
                    },
                    enabled = !state.powerBusy,
                    colors = ButtonDefaults.buttonColors(containerColor = Surface, contentColor = Color.White),
                    contentPadding = PaddingValues(20.dp),
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(80.dp)
                        .testTag("powerControl")
                        .semantics { stateDescription = if (room.on) "On" else "Off" },
                ) {
                    Icon(Icons.Default.PowerSettingsNew, contentDescription = null)
                    Text(
                        if (room.on) "$name is on" else "$name is off",
                        modifier = Modifier.padding(start = 12.dp).weight(1f),
                    )
                    if (state.powerBusy) CircularProgressIndicator(Modifier.size(22.dp), strokeWidth = 2.dp)
                    else Icon(Icons.Default.CheckCircle, contentDescription = null, tint = if (room.on) Success else Color.Gray)
                }

                if (features.color) {
                    Text("Color", fontWeight = FontWeight.SemiBold)
                    Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                        ColorPreset.entries.forEach { preset ->
                            ColorButton(
                                preset.label,
                                Color(0xFF000000 or preset.rgb.toLong()),
                                room.color == preset,
                                Modifier.weight(1f),
                            ) { setColor(preset) }
                        }
                    }
                }

                if (features.brightness) {
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

                }

                if (features.colorTemperature) {
                Text("Color Temperature · ${signedTemperature(temperature)}", fontWeight = FontWeight.SemiBold)
                Slider(
                    value = temperature,
                    onValueChange = { temperature = it },
                    onValueChangeFinished = { setTemperature(temperature.toInt()) },
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
            text = { Text("Hub: ${state.serverUrl}\n\nThe session is encrypted with Android Keystore.") },
            confirmButton = { TextButton(onClick = { showSettings = false }) { Text("Done") } },
            dismissButton = { TextButton(onClick = signOut) { Text("Sign Out", color = Failure) } },
        )
    }

    if (showMatter) {
        MatterDialog(
            matter = state.matter,
            pair = pair,
            close = { showMatter = false; clearPairingResult() },
        )
    }
}

@Composable
private fun TargetPicker(targets: List<Target>, selectedId: String?, select: (String) -> Unit) {
    // Rooms first, then single devices.
    val ordered = targets.sortedBy { if (it.isRoom) 0 else 1 }
    LazyRow(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.testTag("targetPicker")) {
        rowItems(ordered, key = { it.id }) { target ->
            FilterChip(
                selected = target.id == selectedId,
                onClick = { select(target.id) },
                label = { Text(target.name) },
                modifier = Modifier.testTag("target-${target.id}"),
            )
        }
    }
}

@Composable
private fun MatterDialog(matter: MatterUiState, pair: (String, Boolean) -> Unit, close: () -> Unit) {
    var code by remember { mutableStateOf("") }
    var networkOnly by remember { mutableStateOf(false) }
    LaunchedEffect(matter.pairedNodeId) { if (matter.pairedNodeId != null) code = "" }
    AlertDialog(
        onDismissRequest = { if (!matter.pairing) close() },
        title = { Text("Matter devices") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp), modifier = Modifier.testTag("matterDialog")) {
                if (matter.loading) CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
                matter.nodes.forEach { node ->
                    Text(
                        "${node.displayName} · node ${node.nodeId} · ${if (node.available) "online" else "offline"}",
                        color = Color.White.copy(alpha = 0.8f),
                    )
                }
                if (!matter.loading && matter.nodes.isEmpty()) {
                    Text("No Matter devices paired yet.", color = Color.White.copy(alpha = 0.6f))
                }
                OutlinedTextField(
                    value = code,
                    onValueChange = { code = it },
                    label = { Text("Setup code") },
                    placeholder = { Text("MT:… or 1234-567-8901") },
                    singleLine = true,
                    enabled = !matter.pairing,
                    modifier = Modifier.fillMaxWidth().testTag("pairingCode"),
                )
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Checkbox(checked = networkOnly, onCheckedChange = { networkOnly = it }, enabled = !matter.pairing)
                    Text("Already on my network")
                }
                matter.pairedNodeId?.let {
                    Text(
                        "Paired as node $it. Add it to a room in the hub's config.json.",
                        color = Success,
                        modifier = Modifier.testTag("pairingResult"),
                    )
                }
                matter.error?.let { Text(it, color = Failure, modifier = Modifier.testTag("pairingError")) }
            }
        },
        confirmButton = {
            Button(
                onClick = { pair(code, networkOnly) },
                enabled = code.isNotBlank() && !matter.pairing,
                modifier = Modifier.testTag("pairButton"),
            ) {
                if (matter.pairing) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
                Text(if (matter.pairing) "Pairing…" else "Pair")
            }
        },
        dismissButton = { TextButton(onClick = close, enabled = !matter.pairing) { Text("Close") } },
    )
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
        colors = ButtonDefaults.buttonColors(containerColor = color, contentColor = BackgroundTop),
        modifier = modifier
            .height(86.dp)
            .testTag("color$label")
            .semantics { stateDescription = if (selected) "Selected" else "Not selected" },
    ) {
        if (selected) Icon(Icons.Default.CheckCircle, contentDescription = null)
        Text(label, modifier = Modifier.padding(start = if (selected) 8.dp else 0.dp))
    }
}

private fun signedTemperature(value: Float): String {
    val signed = (value.coerceIn(0f, 100f) * 2 - 100).toInt()
    return if (signed > 0) "+$signed%" else "$signed%"
}

private fun accessibleTemperature(value: Float): String = when {
    value < 50 -> "${((50 - value) * 2).toInt()} percent warm"
    value > 50 -> "${((value - 50) * 2).toInt()} percent cool"
    else -> "neutral"
}
