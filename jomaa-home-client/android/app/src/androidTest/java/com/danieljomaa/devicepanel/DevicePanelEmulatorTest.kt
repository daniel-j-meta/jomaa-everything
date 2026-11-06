package com.danieljomaa.devicepanel

import android.content.Context
import android.content.Intent
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.junit4.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.percentOffset
import androidx.compose.ui.test.printToLog
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.click
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class DevicePanelEmulatorTest {
    @get:Rule
    val compose = createEmptyComposeRule()

    private lateinit var server: MockWebServer
    private lateinit var dispatcher: StatefulApiDispatcher
    private lateinit var scenario: ActivityScenario<MainActivity>

    @Before
    fun setUp() {
        dispatcher = StatefulApiDispatcher()
        server = MockWebServer().apply {
            this.dispatcher = this@DevicePanelEmulatorTest.dispatcher
            start()
        }
        val context = ApplicationProvider.getApplicationContext<Context>()
        val intent = Intent(context, MainActivity::class.java).apply {
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            putExtra(MainActivity.EXTRA_LOOPBACK_ONLY, true)
            putExtra(MainActivity.EXTRA_RESET, true)
        }
        scenario = ActivityScenario.launch(intent)
    }

    @After
    fun tearDown() {
        scenario.close()
        server.shutdown()
    }

    @Test
    fun completeControlFlowUsesOnlyLoopbackAndRollsBackFailures() {
        signIn()
        assertState("powerControl", "Off")

        compose.onNodeWithTag("powerControl").performClick()
        waitForServer { it.on }
        assertState("powerControl", "On")

        compose.onNodeWithTag("brightness75").performClick()
        waitForServer { it.brightness == 75 }
        assertState("brightness75", "Selected")

        compose.onNodeWithTag("colorRed").performClick()
        waitForServer { it.colorMode == "red" }
        assertState("colorRed", "Selected")

        compose.onNodeWithTag("temperatureSlider").performTouchInput {
            click(percentOffset(0.25f, 0.5f))
        }
        waitForServer { kotlin.math.abs(it.colorTemperaturePct - 25) <= 3 && it.colorMode == "temperature" }

        dispatcher.failNextCommand()
        compose.onNodeWithTag("powerControl").performClick()
        compose.waitUntil(5_000) {
            compose.onAllNodes(
                SemanticsMatcher.expectValue(SemanticsProperties.TestTag, "errorBanner"),
            ).fetchSemanticsNodes().isNotEmpty()
        }
        compose.onNodeWithTag("errorBanner").assertExists()
        assertState("powerControl", "On")

        compose.onNodeWithTag("retryCommand").performScrollTo().performClick()
        waitForServer { !it.on }
        assertState("powerControl", "Off")

        dispatcher.updateRoom { it.copy(on = true, brightness = 10, colorTemperaturePct = 80.0) }
        compose.onNodeWithTag("refreshButton").performClick()
        assertState("powerControl", "On")
        assertState("brightness10", "Selected")

        check(dispatcher.paths().isNotEmpty())
        check(dispatcher.paths().all { it.startsWith("/api/v1/") })
    }

    @Test
    fun expiredSessionReturnsToSignIn() {
        signIn()
        dispatcher.expireNextRequest()

        compose.onNodeWithTag("refreshButton").performClick()

        compose.waitUntil(5_000) {
            compose.onAllNodes(SemanticsMatcher.expectValue(SemanticsProperties.TestTag, "serverUrlField"))
                .fetchSemanticsNodes().isNotEmpty()
        }
        compose.onNodeWithText("Your session has expired. Sign in again.").assertExists()
    }

    @Test
    fun incorrectPasswordStaysSignedOut() {
        val loopbackUrl = server.url("/").newBuilder().host("127.0.0.1").build()
        compose.onNodeWithTag("serverUrlField").performTextInput(loopbackUrl.toString().removeSuffix("/"))
        compose.onNodeWithTag("passwordField").performTextInput("wrong-password")
        compose.onNodeWithTag("signInButton").performClick()

        compose.waitUntil(5_000) {
            compose.onAllNodes(
                SemanticsMatcher.expectValue(SemanticsProperties.TestTag, "signInError"),
            ).fetchSemanticsNodes().isNotEmpty()
        }
        compose.onNodeWithText("Incorrect password.").assertExists()
        check(dispatcher.paths() == listOf("/api/v1/sessions"))
    }

    private fun signIn() {
        val loopbackUrl = server.url("/").newBuilder().host("127.0.0.1").build()
        compose.onNodeWithTag("serverUrlField").performTextInput(loopbackUrl.toString().removeSuffix("/"))
        compose.onNodeWithTag("passwordField").performTextInput("test-pass")
        compose.onNodeWithTag("signInButton").performClick()
        try {
            compose.waitUntil(8_000) {
                compose.onAllNodes(
                    SemanticsMatcher.expectValue(SemanticsProperties.TestTag, "controlScreen"),
                ).fetchSemanticsNodes().isNotEmpty()
            }
        } catch (error: Throwable) {
            compose.onRoot().printToLog("DevicePanelSignInFailure")
            throw AssertionError(
                "Timed out signing in; server=${dispatcher.snapshot()}, paths=${dispatcher.paths()}",
                error,
            )
        }
        compose.onNodeWithTag("connectionStatus").assertExists()
        assertState("connectionStatus", "Connected")
    }

    private fun assertState(tag: String, value: String) {
        try {
            compose.waitUntil(5_000) {
                compose.onAllNodes(
                    SemanticsMatcher.expectValue(SemanticsProperties.TestTag, tag) and
                        SemanticsMatcher.expectValue(SemanticsProperties.StateDescription, value),
                ).fetchSemanticsNodes().isNotEmpty()
            }
        } catch (error: Throwable) {
            compose.onRoot().printToLog("DevicePanelStateFailure")
            throw AssertionError(
                "Timed out waiting for $tag to report '$value'; server=${dispatcher.snapshot()}, paths=${dispatcher.paths()}",
                error,
            )
        }
        compose.onNodeWithTag(tag).assert(
            SemanticsMatcher.expectValue(SemanticsProperties.StateDescription, value),
        )
    }

    private fun waitForServer(condition: (StatefulApiDispatcher.Snapshot) -> Boolean) {
        try {
            compose.waitUntil(5_000) { condition(dispatcher.snapshot()) }
        } catch (error: Throwable) {
            throw AssertionError(
                "Timed out waiting for loopback server; server=${dispatcher.snapshot()}, paths=${dispatcher.paths()}",
                error,
            )
        }
    }
}
