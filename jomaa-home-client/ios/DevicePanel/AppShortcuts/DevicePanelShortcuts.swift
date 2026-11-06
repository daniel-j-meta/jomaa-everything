import AppIntents
import DevicePanelCore

struct DevicePanelShortcuts: AppShortcutsProvider {
    static var appShortcuts: [AppShortcut] {
        AppShortcut(
            intent: SetPowerIntent(isOn: true),
            phrases: ["Turn on the lights with \(.applicationName)"],
            shortTitle: "Lights On",
            systemImageName: "lightbulb.fill"
        )
        AppShortcut(
            intent: SetPowerIntent(isOn: false),
            phrases: ["Turn off the lights with \(.applicationName)"],
            shortTitle: "Lights Off",
            systemImageName: "lightbulb.slash"
        )
        AppShortcut(
            intent: SetBrightnessIntent(),
            phrases: ["Set the lights' brightness with \(.applicationName)"],
            shortTitle: "Set Brightness",
            systemImageName: "sun.max.fill"
        )
    }
}
