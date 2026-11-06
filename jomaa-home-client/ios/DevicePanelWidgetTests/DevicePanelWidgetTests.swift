import DevicePanelCore
import SwiftUI
import UIKit
import WidgetKit
import XCTest
@testable import DevicePanelWidgetSupport

@MainActor
final class DevicePanelWidgetTests: XCTestCase {
    func testWidgetLoadsSharedStateAndRendersSmallAndMediumFamilies() async throws {
        let preferences = SharedPreferences(suiteName: "WidgetTests.\(UUID().uuidString)")
        let state = try RoomState(
            on: true,
            brightness: 75,
            colorTemperaturePct: 50,
            colorMode: .temperature,
            observedAt: Date()
        )
        await preferences.saveSelectedTarget(
            CachedTarget(id: "lamp-a", name: "Lamp A", features: Features(brightness: true), state: state)
        )
        let sessionStore = InMemorySessionStore(session: Session(token: "widget-test-token"))
        let loader = LiveWidgetStateLoader(
            preferences: preferences,
            sessionStore: sessionStore
        )

        let entry = await loader.load()

        XCTAssertTrue(entry.isAuthenticated)
        XCTAssertEqual(entry.name, "Lamp A")
        XCTAssertTrue(entry.features.brightness)
        XCTAssertEqual(entry.state, state)
        XCTAssertNotNil(render(entry: entry, family: .systemSmall, size: CGSize(width: 170, height: 170)))
        XCTAssertNotNil(render(entry: entry, family: .systemMedium, size: CGSize(width: 364, height: 170)))
    }

    func testWidgetShowsSignedOutWithoutSession() async {
        let loader = LiveWidgetStateLoader(
            preferences: SharedPreferences(suiteName: "WidgetTests.\(UUID().uuidString)"),
            sessionStore: InMemorySessionStore()
        )

        let entry = await loader.load()

        XCTAssertFalse(entry.isAuthenticated)
        XCTAssertNil(entry.state)
        XCTAssertNotNil(render(entry: entry, family: .systemSmall, size: CGSize(width: 170, height: 170)))
    }

    private func render(
        entry: LivingRoomEntry,
        family: WidgetFamily,
        size: CGSize
    ) -> UIImage? {
        let content = LivingRoomWidgetContent(entry: entry, familyOverride: family)
            .frame(width: size.width, height: size.height)
        let renderer = ImageRenderer(content: content)
        renderer.scale = 2
        return renderer.uiImage
    }
}
