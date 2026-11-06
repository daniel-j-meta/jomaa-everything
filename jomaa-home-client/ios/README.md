# iOS app

SwiftUI, iOS 17 and later. The Xcode project is generated from `project.yml`:

```bash
brew install xcodegen
xcodegen generate
open DevicePanel.xcodeproj
```

Pick a Development Team for the app and the widget before running on a
phone. The app group and Keychain group in `project.yml` must also exist in
your Apple Developer account.

Sign in with the hub's address (for example `http://192.168.1.10:8080`, or
your Cloudflare hostname) and password. Plain http is only allowed for
addresses on the home network; use https anywhere else. The session token is
kept in the Keychain, never the password.

## What's in it

- Every room and device the hub knows, with power, red and orange, brightness
  presets and white temperature. Controls a device can't use are hidden.
- Changes show at once and roll back if the hub says they failed, naming the
  devices that didn't respond. A few seconds later the app asks the hub to
  check each bulb and fix any that missed the command.
- Pairing Matter devices with a setup code.
- Home-screen widgets, Shortcuts and Siri, and an iOS 18 Control Center
  toggle. They act on the room the app last showed.

## Test

The UI tests run against the real hub and a fake Govee cloud. Start them in
another terminal first:

```bash
python3 ../scripts/test_hub.py
```

Then, with a simulator:

```bash
xcodebuild test -project DevicePanel.xcodeproj -scheme DevicePanel \
  -destination 'platform=iOS Simulator,name=iPhone 17' CODE_SIGNING_ALLOWED=NO
```

Add `-only-testing:DevicePanelCoreTests` to run just the core tests, which
don't need the hub.
