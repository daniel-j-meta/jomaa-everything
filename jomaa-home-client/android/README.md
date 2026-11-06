# Device Panel for Android

The native Android client targets Android 8 (API 26) and later. It uses Kotlin, Jetpack Compose, coroutines, OkHttp, Android Keystore, and Jetpack Glance.

## Included

- Secure sign-in against the shared `/api/v1` contract.
- Living Room power, red/orange color, 10/50/75/100 brightness presets, and commit-only color temperature control.
- Optimistic UI, rollback/retry, cached stale state, lifecycle refresh, and reconciliation.
- Keystore-encrypted session storage and shared widget state.
- Small and medium interactive Glance widgets for power and brightness.
- TalkBack semantics, scalable text, 48 dp targets, status text, and haptic feedback.

## Build and test

Use JDK 17, Android SDK 35, and Gradle 8.9:

```bash
gradle -p android :app:testDebugUnitTest :app:assembleDebug
gradle -p android :app:connectedDebugAndroidTest
```

The connected suite runs the real Compose UI, OkHttp client, repository, Keystore storage, and Glance content on an emulator. Its only fake is an in-process loopback HTTP server; the test build rejects non-loopback hosts. `.github/workflows/android.yml` provisions the emulator and runs both app and widget tests.
