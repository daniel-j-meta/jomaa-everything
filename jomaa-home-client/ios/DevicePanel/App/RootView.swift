import DevicePanelCore
import SwiftUI

struct RootView: View {
    @ObservedObject var controller: RoomController

    var body: some View {
        Group {
            switch controller.authentication {
            case .checking:
                LaunchView()
            case .signingIn, .signedOut:
                SignInView(controller: controller)
            case .signedIn:
                RoomControlView(controller: controller)
            }
        }
        .animation(.easeInOut(duration: 0.2), value: controller.authentication)
        .sensoryFeedback(.error, trigger: controller.lastError) { oldValue, newValue in
            newValue != nil && oldValue != newValue
        }
    }
}

private struct LaunchView: View {
    var body: some View {
        ZStack {
            AppBackground()
            ProgressView("Loading Device Panel")
                .tint(.white)
                .foregroundStyle(.white)
        }
        .accessibilityElement(children: .combine)
    }
}
