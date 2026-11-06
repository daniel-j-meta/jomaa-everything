import XCTest
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif
@testable import DevicePanel

final class StubURLProtocol: URLProtocol {
    static var handler: ((URLRequest) throws -> (HTTPURLResponse, Data))?
    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
    override func startLoading() {
        do {
            let (response, data) = try Self.handler!(request)
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch { client?.urlProtocol(self, didFailWithError: error) }
    }
    override func stopLoading() {}
}

final class PanelAPITests: XCTestCase {
    private let base = URL(string: "http://hub.test")!
    private var session: URLSession!

    override func setUp() {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [StubURLProtocol.self]
        session = URLSession(configuration: configuration)
    }

    override func tearDown() { StubURLProtocol.handler = nil }

    private func api(token: String? = "tok") -> PanelAPI { PanelAPI(baseURL: base, token: token, session: session) }

    func testSignInPostsJSONPasswordAndKeepsToken() async throws {
        StubURLProtocol.handler = { request in
            XCTAssertEqual(request.url?.path, "/api/login")
            XCTAssertEqual(request.httpMethod, "POST")
            XCTAssertNil(request.value(forHTTPHeaderField: "Authorization"))
            let body = try JSONSerialization.jsonObject(with: Self.body(request)) as! [String: Any]
            XCTAssertEqual(body["password"] as? String, "a+b& %é")
            return Self.json(request, "{\"token\":\"abc\"}")
        }
        let client = api(token: nil)
        let token = try await client.signIn(password: "a+b& %é")
        XCTAssertEqual(token, "abc")
        let kept = await client.token
        XCTAssertEqual(kept, "abc")
    }

    func testWrongPasswordIsReported() async {
        StubURLProtocol.handler = { request in Self.json(request, "{\"error\":\"wrong password\"}", status: 401) }
        do { try await api(token: nil).signIn(password: "wrong"); XCTFail("Expected incorrectPassword") }
        catch { XCTAssertEqual(error as? PanelAPIError, .incorrectPassword) }
    }

    func testTargetsAndStateUseBearerToken() async throws {
        StubURLProtocol.handler = { request in
            XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer tok")
            if request.url?.path == "/api/targets" {
                return Self.json(request, """
                {"targets":[{"id":"living-room","name":"Living Room","kind":"room","members":["a"],
                  "features":{"brightness":true,"colorTemperature":true,"color":false}},
                 {"id":"plug","name":"Plug","kind":"matter","features":{"brightness":false,"colorTemperature":false,"color":false}}]}
                """)
            }
            XCTAssertEqual(request.url?.path, "/api/targets/living-room/state")
            return Self.json(request, "{\"state\":{\"on\":true,\"brightness\":40,\"colorTemperaturePct\":10},\"devices\":{}}")
        }
        let targets = try await api().targets()
        XCTAssertTrue(targets[0].isRoom)
        XCTAssertFalse(targets[1].features.brightness)
        let state = try await api().state(of: "living-room")
        XCTAssertEqual(state.state, RoomState(on: true, brightness: 40, colorTemperaturePct: 10))
    }

    func testCommandSendsOnlyChangedFields() async throws {
        StubURLProtocol.handler = { request in
            XCTAssertEqual(request.url?.path, "/api/targets/living-room/command")
            let body = try JSONSerialization.jsonObject(with: Self.body(request)) as! [String: Any]
            XCTAssertEqual(Set(body.keys), ["color", "clientId", "revision"])
            XCTAssertEqual(body["color"] as? String, "#ff8000")
            XCTAssertEqual(body["revision"] as? Int, 13)
            return Self.json(request, "{\"ok\":true}")
        }
        try await api().send(Command(color: ColorPreset.orange.rawValue, clientId: "phone", revision: 13), to: "living-room")
    }

    func testPartialFailureNamesDevices() async {
        StubURLProtocol.handler = { request in
            Self.json(request, "{\"ok\":false,\"failed\":{\"lamp-b\":\"timeout\"}}", status: 502)
        }
        do { try await api().send(Command(on: true), to: "living-room"); XCTFail("Expected devicesFailed") }
        catch { XCTAssertEqual(error as? PanelAPIError, .devicesFailed(["lamp-b": "timeout"])) }
    }

    func testColorSyncOmitsTemperature() async throws {
        StubURLProtocol.handler = { request in
            let body = try JSONSerialization.jsonObject(with: Self.body(request)) as! [String: Any]
            XCTAssertNil(body["colorTemperaturePct"])
            XCTAssertEqual(body["brightness"] as? Int, 50)
            return Self.json(request, "{\"synchronized\":true,\"stale\":false,\"corrected\":{},\"failed\":{}}")
        }
        let sync = try await api().sync(DesiredState(clientId: "c", revision: 3, on: true, brightness: 50,
                                                     colorTemperaturePct: nil), target: "living-room")
        XCTAssertTrue(sync.synchronized)
    }

    func testMatterPairingAndNodes() async throws {
        StubURLProtocol.handler = { request in
            if request.url?.path == "/api/matter/commission" {
                let body = try JSONSerialization.jsonObject(with: Self.body(request)) as! [String: Any]
                XCTAssertEqual(body["code"] as? String, "34970112332")
                XCTAssertEqual(body["networkOnly"] as? Bool, true)
                return Self.json(request, "{\"node\":{\"node_id\":7,\"available\":true}}")
            }
            return Self.json(request, """
            {"nodes":[{"nodeId":18446744065119617024,"available":false,"isBridge":false,"vendor":"Acme","product":"Bulb"}]}
            """)
        }
        let node = try await api().commission(code: "34970112332", networkOnly: true)
        XCTAssertEqual(node, 7)
        let nodes = try await api().matterNodes()
        XCTAssertEqual(nodes.first?.nodeId, 18446744065119617024)
        XCTAssertEqual(nodes.first?.displayName, "Acme Bulb")
    }

    func testMatterServerDownAndExpiredSession() async {
        StubURLProtocol.handler = { request in Self.json(request, "{\"error\":\"not connected\"}", status: 503) }
        do { _ = try await api().matterNodes(); XCTFail("Expected matterUnavailable") }
        catch { XCTAssertEqual(error as? PanelAPIError, .matterUnavailable) }
        StubURLProtocol.handler = { request in Self.json(request, "{\"error\":\"sign in first\"}", status: 401) }
        do { _ = try await api().targets(); XCTFail("Expected signedOut") }
        catch { XCTAssertEqual(error as? PanelAPIError, .signedOut) }
    }

    func testHTMLAndBadStateAreRejected() async {
        StubURLProtocol.handler = { request in
            (HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil,
                             headerFields: ["Content-Type": "text/html"])!, Data("<html></html>".utf8))
        }
        do { _ = try await api().targets(); XCTFail("Expected invalidResponse") }
        catch { XCTAssertEqual(error as? PanelAPIError, .invalidResponse) }
        StubURLProtocol.handler = { request in
            Self.json(request, "{\"state\":{\"on\":true,\"brightness\":999,\"colorTemperaturePct\":50},\"devices\":{}}")
        }
        do { _ = try await api().state(of: "x"); XCTFail("Expected invalidResponse") }
        catch { XCTAssertEqual(error as? PanelAPIError, .invalidResponse) }
    }

    func testServerURLAllowsHTTPOnlyOnTheHomeNetwork() throws {
        XCTAssertEqual(try PanelAPI.serverURL(" http://192.168.1.10:8080/ ").absoluteString, "http://192.168.1.10:8080")
        for value in ["http://10.0.0.5", "http://hub.local:8080", "http://hub:8080", "http://127.0.0.1:8797", "https://hub.example.com"] {
            XCTAssertNoThrow(try PanelAPI.serverURL(value), value)
        }
        for value in ["hub.test", "file:///tmp", "http://hub.example.com", "http://8.8.8.8", "https://user:secret@hub.test",
                      "https://hub.test/home", "https://hub.test?q=1", "http://fdroid.example.com"] {
            XCTAssertThrowsError(try PanelAPI.serverURL(value), value)
        }
    }

    func testSetupCodes() {
        XCTAssertEqual(MatterSetupCode.normalize(" 3497-011-2332 "), "34970112332")
        XCTAssertEqual(MatterSetupCode.normalize("MT:Y.K9042C00KA0648G00"), "MT:Y.K9042C00KA0648G00")
        XCTAssertNil(MatterSetupCode.normalize("1234"))
        XCTAssertNil(MatterSetupCode.normalize("MT:"))
    }

    private static func json(_ request: URLRequest, _ value: String, status: Int = 200) -> (HTTPURLResponse, Data) {
        (HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: nil,
                         headerFields: ["Content-Type": "application/json"])!, Data(value.utf8))
    }

    private static func body(_ request: URLRequest) -> Data {
        if let data = request.httpBody { return data }
        guard let stream = request.httpBodyStream else { return Data() }
        stream.open()
        defer { stream.close() }
        var output = Data()
        var buffer = [UInt8](repeating: 0, count: 1024)
        while stream.hasBytesAvailable {
            let count = stream.read(&buffer, maxLength: buffer.count)
            if count <= 0 { break }
            output.append(contentsOf: buffer.prefix(count))
        }
        return output
    }
}
