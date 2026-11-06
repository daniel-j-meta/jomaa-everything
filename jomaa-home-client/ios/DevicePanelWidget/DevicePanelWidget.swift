import DevicePanelWidgetSupport
import SwiftUI
import WidgetKit

@main
struct DevicePanelWidgetBundle: WidgetBundle {
    var body: some Widget {
        LivingRoomWidget()
        if #available(iOSApplicationExtension 18.0, *) {
            LivingRoomControlWidget()
        }
    }
}
