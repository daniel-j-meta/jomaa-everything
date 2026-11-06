import DevicePanelCore
import Foundation

@MainActor
enum AppEnvironment {
    static func makeRoomController() -> RoomController {
        let accessGroup = Bundle.main.object(
            forInfoDictionaryKey: "DevicePanelKeychainAccessGroup"
        ) as? String

        // UI tests run against a hub on this machine; see DevicePanelUITests.
        if ProcessInfo.processInfo.arguments.contains("-ui-testing-reset") {
            return RoomController(
                api: HTTPDevicePanelAPI(allowedHosts: ["127.0.0.1", "localhost", "::1"]),
                sessionStore: KeychainSessionStore(accessGroup: accessGroup),
                preferences: SharedPreferences(),
                reconcileDelays: [.milliseconds(50), .milliseconds(100)],
                resetPersistentStateOnStart: true
            )
        }

        return RoomController(
            api: HTTPDevicePanelAPI(),
            sessionStore: KeychainSessionStore(accessGroup: accessGroup),
            preferences: SharedPreferences()
        )
    }
}
