package com.danieljomaa.devicepanel.widget

import android.content.Context
import androidx.compose.runtime.Composable
import androidx.glance.GlanceId
import androidx.glance.GlanceModifier
import androidx.glance.LocalSize
import androidx.glance.Button
import androidx.glance.action.ActionParameters
import androidx.glance.action.actionParametersOf
import androidx.glance.appwidget.GlanceAppWidget
import androidx.glance.appwidget.GlanceAppWidgetReceiver
import androidx.glance.appwidget.action.ActionCallback
import androidx.glance.appwidget.action.actionRunCallback
import androidx.glance.appwidget.provideContent
import androidx.glance.background
import androidx.glance.layout.Alignment
import androidx.glance.layout.Box
import androidx.glance.layout.Column
import androidx.glance.layout.Row
import androidx.glance.layout.RowScope
import androidx.glance.layout.Spacer
import androidx.glance.layout.fillMaxSize
import androidx.glance.layout.fillMaxWidth
import androidx.glance.layout.height
import androidx.glance.layout.padding
import androidx.glance.layout.width
import androidx.glance.text.FontWeight
import androidx.glance.text.Text
import androidx.glance.text.TextStyle
import androidx.glance.unit.ColorProvider
import com.danieljomaa.devicepanel.data.KeystoreSessionStore
import com.danieljomaa.devicepanel.data.OkHttpDevicePanelApi
import com.danieljomaa.devicepanel.data.RoomChanges
import com.danieljomaa.devicepanel.data.RoomState
import com.danieljomaa.devicepanel.data.SharedStateStore
import com.danieljomaa.devicepanel.domain.IntentCommandService
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

class DevicePanelWidgetReceiver : GlanceAppWidgetReceiver() {
    override val glanceAppWidget: GlanceAppWidget = DevicePanelWidget()
}

class DevicePanelWidget : GlanceAppWidget() {
    override suspend fun provideGlance(context: Context, id: GlanceId) {
        val stateStore = SharedStateStore(context)
        val room = stateStore.roomState()
        val session = KeystoreSessionStore(context).load()
        val authenticated = session != null && !SharedStateStore.sessionExpired(session)
        provideContent {
            DevicePanelWidgetContent(room = room, authenticated = authenticated)
        }
    }
}

@Composable
fun DevicePanelWidgetContent(room: RoomState?, authenticated: Boolean) {
    Box(
        modifier = GlanceModifier
            .fillMaxSize()
            .background(ColorProvider(Color(0xFF16213E)))
            .padding(14.dp),
        contentAlignment = Alignment.Center,
    ) {
        when {
            !authenticated -> Column(
                modifier = GlanceModifier.fillMaxSize(),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text("Device Panel", style = headingStyle())
                Spacer(GlanceModifier.height(8.dp))
                Text("Sign in from the app", style = bodyStyle())
            }
            room == null -> Column(
                modifier = GlanceModifier.fillMaxSize(),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text("Living Room", style = headingStyle())
                Spacer(GlanceModifier.height(8.dp))
                Text("Open the app to load state", style = bodyStyle())
            }
            LocalSize.current.width >= 250.dp -> MediumWidget(room)
            else -> SmallWidget(room)
        }
    }
}

@Composable
private fun SmallWidget(room: RoomState) {
    Column(modifier = GlanceModifier.fillMaxSize()) {
        Text("Living Room", style = headingStyle())
        Spacer(GlanceModifier.defaultWeight())
        Text(if (room.on) "On · ${room.brightness}%" else "Off", style = bodyStyle())
        Spacer(GlanceModifier.height(8.dp))
        Button(
            text = if (room.on) "Turn Off" else "Turn On",
            onClick = actionRunCallback<SetPowerAction>(
                actionParametersOf(SetPowerAction.powerKey to !room.on),
            ),
            modifier = GlanceModifier.fillMaxWidth(),
        )
    }
}

@Composable
private fun MediumWidget(room: RoomState) {
    Row(modifier = GlanceModifier.fillMaxSize(), verticalAlignment = Alignment.CenterVertically) {
        Column(modifier = GlanceModifier.defaultWeight()) {
            Text("Living Room", style = headingStyle())
            Text(if (room.on) "On" else "Off", style = bodyStyle())
            Spacer(GlanceModifier.height(8.dp))
            Button(
                text = if (room.on) "Off" else "On",
                onClick = actionRunCallback<SetPowerAction>(
                    actionParametersOf(SetPowerAction.powerKey to !room.on),
                ),
            )
        }
        Spacer(GlanceModifier.width(12.dp))
        Column(modifier = GlanceModifier.defaultWeight()) {
            Text("Brightness", style = bodyStyle())
            Row {
                BrightnessButton(10)
                BrightnessButton(50)
            }
            Row {
                BrightnessButton(75)
                BrightnessButton(100)
            }
        }
    }
}

@Composable
private fun RowScope.BrightnessButton(value: Int) {
    Button(
        text = "$value%",
        onClick = actionRunCallback<SetBrightnessAction>(
            actionParametersOf(SetBrightnessAction.brightnessKey to value),
        ),
        modifier = GlanceModifier.defaultWeight().padding(2.dp),
    )
}

private fun headingStyle() = TextStyle(
    color = ColorProvider(Color.White),
    fontSize = 18.sp,
    fontWeight = FontWeight.Bold,
)

private fun bodyStyle() = TextStyle(
    color = ColorProvider(Color.White.copy(alpha = 0.75f)),
    fontSize = 14.sp,
)

class SetPowerAction : ActionCallback {
    override suspend fun onAction(context: Context, glanceId: GlanceId, parameters: ActionParameters) {
        val value = parameters[powerKey] ?: return
        commandService(context).apply(RoomChanges(on = value))
        DevicePanelWidget().update(context, glanceId)
    }

    companion object {
        val powerKey = ActionParameters.Key<Boolean>("power")
    }
}

class SetBrightnessAction : ActionCallback {
    override suspend fun onAction(context: Context, glanceId: GlanceId, parameters: ActionParameters) {
        val value = parameters[brightnessKey] ?: return
        commandService(context).apply(RoomChanges(brightness = value))
        DevicePanelWidget().update(context, glanceId)
    }

    companion object {
        val brightnessKey = ActionParameters.Key<Int>("brightness")
    }
}

private fun commandService(context: Context) = IntentCommandService(
    api = OkHttpDevicePanelApi(),
    sessionStore = KeystoreSessionStore(context),
    stateStore = SharedStateStore(context),
)
