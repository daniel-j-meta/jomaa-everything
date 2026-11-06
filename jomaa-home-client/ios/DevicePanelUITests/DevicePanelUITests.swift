import XCTest

/// Runs against the real hub and a fake Govee cloud. Start them first with:
///     python3 jomaa-home-client/scripts/test_hub.py
@MainActor
final class DevicePanelUITests: XCTestCase {
    private let server = "http://127.0.0.1:8797"
    private let mock = "http://127.0.0.1:8798"

    func testNativeControlsReachEveryDevice() async throws {
        _ = try await mockRequest("/__fake/reset", body: [:])
        let app = signIn()
        let power = app.switches["powerToggle"]
        XCTAssertEqual(power.value as? String, "1")
        power.tap()
        XCTAssertTrue(app.staticTexts["Lights are off"].waitForExistence(timeout: 10))
        app.buttons["brightness75"].tap()
        XCTAssertTrue(waitForSelected(app.buttons["brightness75"]))
        app.buttons["colorOrange"].tap()
        XCTAssertTrue(waitForSelected(app.buttons["colorOrange"]))
        app.swipeUp()
        let slider = app.sliders["temperatureSlider"]
        XCTAssertTrue(slider.waitForExistence(timeout: 5))
        slider.adjust(toNormalizedSliderPosition: 0.75)
        try await Task.sleep(for: .seconds(1))
        let state = try await mockRequest("/__fake/state")
        let devices = state["devices"] as! [String: [String: Any]]
        XCTAssertEqual(devices.count, 2)
        for device in devices.values {
            XCTAssertEqual(device["powerSwitch"] as? Int, 0)
            XCTAssertEqual(device["brightness"] as? Int, 75)
            XCTAssertEqual(device["colorRgb"] as? Int, 0xFF8000)
        }
        let requests = try await mockRequestArray("/__fake/requests")
        let temperatureRequests = requests.filter { $0["instance"] as? String == "colorTemperatureK" }
        XCTAssertEqual(temperatureRequests.count, 2, "Slider must send one room command when editing ends")
        attach(app, name: "Native living room controls")
    }

    func testFailedPowerCommandRollsBackAndRecovers() async throws {
        _ = try await mockRequest("/__fake/reset", body: [:])
        let app = signIn()
        _ = try await mockRequest("/__fake/fail", body: ["enabled": true])
        let power = app.switches["powerToggle"]
        power.tap()
        let error = app.staticTexts["errorMessage"]
        XCTAssertTrue(error.waitForExistence(timeout: 10))
        XCTAssertEqual(power.value as? String, "1")
        XCTAssertTrue(error.label.contains("didn't respond"))
        attach(app, name: "Failed command restored power state")
        _ = try await mockRequest("/__fake/fail", body: ["enabled": false])
        power.tap()
        XCTAssertTrue(app.staticTexts["Lights are off"].waitForExistence(timeout: 10))
    }

    private func signIn() -> XCUIApplication {
        let app = XCUIApplication()
        app.launchEnvironment["DEVICE_PANEL_BASE_URL"] = server
        app.launchEnvironment["DEVICE_PANEL_RESET"] = "1"
        app.launch()
        let password = app.secureTextFields["password"]
        XCTAssertTrue(password.waitForExistence(timeout: 10))
        password.tap()
        password.typeText("device-panel-test")
        app.buttons["signIn"].tap()
        XCTAssertTrue(app.staticTexts["Living Room"].waitForExistence(timeout: 20))
        return app
    }

    private func waitForSelected(_ button: XCUIElement) -> Bool {
        let expectation = XCTNSPredicateExpectation(predicate: NSPredicate(format: "selected == true"), object: button)
        return XCTWaiter.wait(for: [expectation], timeout: 10) == .completed
    }

    private func mockRequest(_ path: String, body: [String: Any]? = nil) async throws -> [String: Any] {
        var request = URLRequest(url: URL(string: mock + path)!)
        if let body {
            request.httpMethod = "POST"
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.httpBody = try JSONSerialization.data(withJSONObject: body)
        }
        let (data, _) = try await URLSession.shared.data(for: request)
        return try JSONSerialization.jsonObject(with: data) as! [String: Any]
    }

    private func mockRequestArray(_ path: String) async throws -> [[String: Any]] {
        let (data, _) = try await URLSession.shared.data(from: URL(string: mock + path)!)
        return try JSONSerialization.jsonObject(with: data) as! [[String: Any]]
    }

    private func attach(_ app: XCUIApplication, name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }
}
