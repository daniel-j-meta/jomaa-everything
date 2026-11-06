import AppIntents
import DevicePanelCore
import SwiftUI
import WidgetKit

@available(iOSApplicationExtension 18.0, *)
struct LivingRoomControlValueProvider: ControlValueProvider {
    var previewValue: Bool { false }

    func currentValue() async throws -> Bool {
        if let cached = await SharedPreferences().cachedRoomState() {
            return cached.on
        }
        return try await IntentCommandService(
            api: HTTPDevicePanelAPI(),
            sessionStore: KeychainSessionStore(
                accessGroup: Bundle.main.object(
                    forInfoDictionaryKey: "DevicePanelKeychainAccessGroup"
                ) as? String
            ),
            preferences: SharedPreferences()
        ).currentState().on
    }
}

@available(iOSApplicationExtension 18.0, *)
struct LivingRoomControlWidget: ControlWidget {
    let kind = "LivingRoomControlWidget"

    var body: some ControlWidgetConfiguration {
        StaticControlConfiguration(
            kind: kind,
            provider: LivingRoomControlValueProvider()
        ) { isOn in
            ControlWidgetToggle(
                "Lights",
                isOn: isOn,
                action: SetPowerControlIntent()
            ) { isOn in
                Label(isOn ? "On" : "Off", systemImage: isOn ? "lightbulb.fill" : "lightbulb.slash")
            }
        }
        .displayName("Lights")
        .description("Turn the room you last used in Device Panel on or off.")
    }
}
