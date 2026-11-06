# Device Panel for iOS

A native SwiftUI client for the repository's Cloudflare Worker. Requires iOS
17+ and Xcode. The generated `DevicePanel.xcodeproj` is included; XcodeGen is
only needed when changing `project.yml`.

## Run

Start the backend from the repository root:

```bash
python3 boot.py
```

Open `clients/ios/DevicePanel.xcodeproj`, select the **DevicePanel** scheme and
an iPhone Simulator, then run. Sign in with `device-panel-test` for the default
mock backend. The login screen lets you change the server URL. Its default is
`http://127.0.0.1:8787`; iOS Simulator's loopback reaches the host machine.

The app provides power, 10/50/75/100% brightness, red/orange color, and a
temperature slider. It fetches initial state after login. Each command carries
a client UUID and increasing revision; failed commands restore the previous
selection and show the server error. After a command, the app reconciles its
desired state on the same 3/6/10/15-second backoff as the web client. Selecting
an RGB color omits temperature from reconciliation so sync does not replace
the color with white light. Refresh reads the server's current state.

The password is kept only while signing in. An ephemeral URLSession manages
the Worker's HttpOnly cookie for the current app session; sign-out drops that
session. Only the server address is saved in UserDefaults. The app rejects
unexpected HTML responses and login redirects instead of treating them as
successful control commands. App Transport Security allows local networking
and loopback HTTP; use HTTPS for a remote server.

### Command line build and launch

```bash
cd clients/ios
xcodebuild -project DevicePanel.xcodeproj -scheme DevicePanel \
  -configuration Debug -sdk iphonesimulator \
  -destination 'generic/platform=iOS Simulator' \
  -derivedDataPath /tmp/device-panel-ios CODE_SIGNING_ALLOWED=NO build
xcrun simctl list devices available
# Use a booted simulator's UDID below, or boot one with simctl boot <udid>.
xcrun simctl install <udid> \
  /tmp/device-panel-ios/Build/Products/Debug-iphonesimulator/DevicePanel.app
xcrun simctl launch <udid> com.jomaa.DevicePanel
```

To select a different server for a simulator launch:

```bash
SIMCTL_CHILD_DEVICE_PANEL_BASE_URL=http://127.0.0.1:8797 \
  xcrun simctl launch <udid> com.jomaa.DevicePanel
```

## Tests

Seven URLSession contract tests cover form encoding, wrong passwords, expired
sessions, client revisions, RGB reconciliation, malformed state, and server URL
validation. Two simulator UI tests exercise real login and controls against an
isolated Worker/mock, including failed-command rollback and recovery.

Start the isolated test backend in another terminal:

```bash
PANEL_PASSWORD=device-panel-test python3 boot.py --port 8797 --mock-port 8798
```

Then run from `clients/ios` with a simulator UDID:

```bash
xcodebuild -project DevicePanel.xcodeproj -scheme DevicePanel \
  -destination 'platform=iOS Simulator,id=<udid>' \
  -derivedDataPath /tmp/device-panel-ios-tests \
  CODE_SIGNING_ALLOWED=NO test
```

UI tests reset and mutate the mock on ports 8797/8798; keep those ports dedicated
to testing. To run only the client contract tests, add
`-only-testing:DevicePanelTests` and omit the test backend.

Regenerate the project after editing `project.yml` with `xcodegen generate`.
