import SwiftUI

@main
struct DevicePanelApp: App {
    @StateObject private var controller = AppEnvironment.makeRoomController()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            RootView(controller: controller)
                .preferredColorScheme(.dark)
                .task {
                    await controller.start()
                }
                .onChange(of: scenePhase) { _, phase in
                    guard phase == .active else { return }
                    guard case .signedIn = controller.authentication else { return }
                    Task { await controller.refresh() }
                }
        }
    }
}
