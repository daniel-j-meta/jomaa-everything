# iOS app

SwiftUI, iOS 17 and later. Open `DevicePanel.xcodeproj` and run the
**DevicePanel** scheme. XcodeGen is only needed if you change `project.yml`.

Sign in with the hub's address (for example `http://192.168.1.10:8080`) and
password. Plain http is only allowed for addresses on the home network; use
https anywhere else. The session token is kept in the Keychain, never the
password.

## Test

The UI tests run against the real hub and a fake Govee cloud. Start them in
another terminal first:

```bash
python3 ../scripts/test_hub.py
```

Then, with a simulator:

```bash
xcodebuild test -project DevicePanel.xcodeproj -scheme DevicePanel \
  -destination 'platform=iOS Simulator,name=iPhone 16' CODE_SIGNING_ALLOWED=NO
```

Add `-only-testing:DevicePanelTests` to run just the request tests, which
don't need the hub.

To launch against a different hub: `SIMCTL_CHILD_DEVICE_PANEL_BASE_URL=http://127.0.0.1:8797 xcrun simctl launch <udid> com.jomaa.DevicePanel`.
