# Android app

Kotlin, Jetpack Compose, OkHttp, Android Keystore and Jetpack Glance. Android 8
(API 26) and later.

Sign in with the hub's address (for example `http://192.168.1.10:8080`) and
password. Plain http is only allowed for addresses on the home network; use
https anywhere else. The session token is encrypted with Android Keystore.

The home-screen widget controls the last room you opened in the app.

## Build and test

Needs JDK 17 and the Android SDK.

```bash
./gradlew :app:testDebugUnitTest :app:lintDebug :app:assembleDebug
./gradlew :app:connectedDebugAndroidTest      # with an emulator running
```

Kotlin warnings and lint warnings fail the build. The emulator tests drive the
real app against an in-process fake hub.

To try the app against the real hub, start `../scripts/test_hub.py` and run:

```bash
./gradlew :app:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=com.danieljomaa.devicepanel.RealHubSmokeTest \
  -Pandroid.testInstrumentationRunnerArguments.hubUrl=http://10.0.2.2:8797 \
  -Pandroid.testInstrumentationRunnerArguments.hubPassword=device-panel-test
```
