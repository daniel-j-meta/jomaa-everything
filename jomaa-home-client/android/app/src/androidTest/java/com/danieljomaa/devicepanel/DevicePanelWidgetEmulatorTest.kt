package com.danieljomaa.devicepanel

import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import androidx.glance.appwidget.testing.unit.runGlanceAppWidgetUnitTest
import androidx.glance.testing.unit.hasClickAction
import androidx.glance.testing.unit.hasText
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.danieljomaa.devicepanel.data.RoomColorMode
import com.danieljomaa.devicepanel.data.RoomState
import com.danieljomaa.devicepanel.widget.DevicePanelWidgetContent
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class DevicePanelWidgetEmulatorTest {
    @Test
    fun smallWidgetRendersStateAndInteractivePower() = runGlanceAppWidgetUnitTest {
        setAppWidgetSize(DpSize(170.dp, 170.dp))
        provideComposable {
            DevicePanelWidgetContent(room = roomState(), authenticated = true)
        }

        onNode(hasText("Living Room")).assertExists()
        onNode(hasText("On · 75%")).assertExists()
        onNode(hasText("Turn Off").and(hasClickAction())).assertExists()
    }

    @Test
    fun mediumWidgetRendersAllBrightnessActions() = runGlanceAppWidgetUnitTest {
        setAppWidgetSize(DpSize(364.dp, 170.dp))
        provideComposable {
            DevicePanelWidgetContent(room = roomState(), authenticated = true)
        }

        listOf("10%", "50%", "75%", "100%").forEach { label ->
            onNode(hasText(label).and(hasClickAction())).assertExists()
        }
    }

    @Test
    fun signedOutWidgetDoesNotExposeControls() = runGlanceAppWidgetUnitTest {
        provideComposable {
            DevicePanelWidgetContent(room = null, authenticated = false)
        }

        onNode(hasText("Sign in from the app")).assertExists()
    }

    private fun roomState() = RoomState(
        on = true,
        brightness = 75,
        colorTemperaturePct = 50.0,
        colorMode = RoomColorMode.TEMPERATURE,
        observedAt = "2026-10-08T12:00:00.123Z",
        stateSource = "representative",
    )
}
