package com.danieljomaa.devicepanel

import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import com.danieljomaa.devicepanel.data.KeystoreSessionStore
import com.danieljomaa.devicepanel.data.OkHttpDevicePanelApi
import com.danieljomaa.devicepanel.data.SharedStateStore
import com.danieljomaa.devicepanel.domain.DevicePanelViewModel
import com.danieljomaa.devicepanel.ui.DevicePanelApp

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val loopbackOnly = intent.getBooleanExtra(EXTRA_LOOPBACK_ONLY, false)
        val reset = intent.getBooleanExtra(EXTRA_RESET, false)
        val viewModel = ViewModelProvider(this, object : ViewModelProvider.Factory {
            @Suppress("UNCHECKED_CAST")
            override fun <T : ViewModel> create(modelClass: Class<T>): T = DevicePanelViewModel(
                api = OkHttpDevicePanelApi(
                    allowedHosts = if (loopbackOnly) setOf("localhost", "127.0.0.1", "::1") else null,
                ),
                sessionStore = KeystoreSessionStore(applicationContext),
                stateStore = SharedStateStore(applicationContext),
                allowInsecureLoopback = BuildConfig.DEBUG,
                resetOnStart = reset,
                reconcileDelaysMs = if (loopbackOnly) listOf(50, 100) else listOf(3_000, 6_000, 10_000, 15_000),
            ) as T
        })[DevicePanelViewModel::class.java]

        setContent {
            DevicePanelApp(viewModel = viewModel, deviceName = "${Build.MANUFACTURER} ${Build.MODEL}")
        }
    }

    companion object {
        const val EXTRA_LOOPBACK_ONLY = "loopbackOnly"
        const val EXTRA_RESET = "resetState"
    }
}
