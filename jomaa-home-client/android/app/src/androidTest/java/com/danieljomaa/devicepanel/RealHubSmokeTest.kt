package com.danieljomaa.devicepanel

import android.content.Context
import android.content.Intent
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assume.assumeTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Drives the app against a real hub. Skipped unless you pass its address:
 *
 *   gradle -p android :app:connectedDebugAndroidTest \
 *     -Pandroid.testInstrumentationRunnerArguments.hubUrl=http://10.0.2.2:8080 \
 *     -Pandroid.testInstrumentationRunnerArguments.hubPassword=...
 *
 * (10.0.2.2 is the emulator's address for the host machine.) It turns the
 * first room on and sets it to 75%, so point it at a hub you don't mind changing.
 */
@RunWith(AndroidJUnit4::class)
class RealHubSmokeTest {
    @get:Rule
    val compose = createEmptyComposeRule()

    @Test
    fun signsInAndControlsTheFirstRoom() {
        val args = InstrumentationRegistry.getArguments()
        val hubUrl = args.getString("hubUrl")
        val password = args.getString("hubPassword")
        assumeTrue("pass hubUrl and hubPassword to run against a real hub", hubUrl != null && password != null)

        val context = ApplicationProvider.getApplicationContext<Context>()
        val intent = Intent(context, MainActivity::class.java)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            .putExtra(MainActivity.EXTRA_RESET, true)
        ActivityScenario.launch<MainActivity>(intent).use {
            compose.onNodeWithTag("serverUrlField").performTextInput(hubUrl!!)
            compose.onNodeWithTag("passwordField").performTextInput(password!!)
            compose.onNodeWithTag("signInButton").performClick()
            waitFor("powerControl", 15_000)

            if (stateOf("powerControl") == "Off") compose.onNodeWithTag("powerControl").performClick()
            waitForState("powerControl", "On")
            compose.onNodeWithTag("brightness75").performClick()
            waitForState("brightness75", "Selected")
            waitForState("connectionStatus", "Connected")
        }
    }

    private fun waitFor(tag: String, timeoutMs: Long = 10_000) = compose.waitUntil(timeoutMs) {
        compose.onAllNodes(hasTestTag(tag)).fetchSemanticsNodes().isNotEmpty()
    }

    private fun stateOf(tag: String): String? =
        compose.onNodeWithTag(tag).fetchSemanticsNode().config.getOrElseNullable(SemanticsProperties.StateDescription) { null }

    private fun waitForState(tag: String, value: String) = compose.waitUntil(10_000) {
        compose.onAllNodes(
            hasTestTag(tag) and SemanticsMatcher.expectValue(SemanticsProperties.StateDescription, value),
        ).fetchSemanticsNodes().isNotEmpty()
    }
}
