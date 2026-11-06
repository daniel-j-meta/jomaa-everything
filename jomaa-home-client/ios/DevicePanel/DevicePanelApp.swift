import SwiftUI

@main
struct DevicePanelApp: App {
    @StateObject private var panel = PanelModel()

    var body: some Scene {
        WindowGroup {
            ContentView(panel: panel)
                .preferredColorScheme(.dark)
        }
    }
}
