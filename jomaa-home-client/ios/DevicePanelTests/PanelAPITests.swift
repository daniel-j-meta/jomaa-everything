import XCTest
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
    private let base = URL(string: "http://panel.test")!
    private var api: PanelAPI!

    override func setUp() {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [StubURLProtocol.self]
        api = PanelAPI(baseURL: base, session: URLSession(configuration: configuration))
    }

    override func tearDown() { StubURLProtocol.handler = nil }

    func testLoginPercentEncodesSpecialCharacters() async throws {
        StubURLProtocol.handler = { request in
            XCTAssertEqual(request.httpMethod, "POST")
            XCTAssertEqual(request.value(forHTTPHeaderField: "Content-Type"), "application/x-www-form-urlencoded")
            XCTAssertEqual(String(data: Self.body(request), encoding: .utf8), "password=a%2Bb%26%20%25%C3%A9")
            return (HTTPURLResponse(url: self.base.appendingPathComponent("home"), statusCode: 200,
                                    httpVersion: nil, headerFields: ["Content-Type": "text/html"])!, Data())
        }
        try await api.signIn(password: "a+b& %é")
    }

    func testWrongPasswordIsReported() async {
        StubURLProtocol.handler = { request in
            (HTTPURLResponse(url: request.url!, statusCode: 401, httpVersion: nil, headerFields: nil)!, Data())
        }
        do { try await api.signIn(password: "wrong"); XCTFail("Expected incorrectPassword") }
        catch { XCTAssertEqual(error as? PanelAPIError, .incorrectPassword) }
    }

    func testExpiredSessionRedirectCannotLookLikeSuccessfulCommand() async {
        StubURLProtocol.handler = { _ in
            (HTTPURLResponse(url: self.base.appendingPathComponent("login"), statusCode: 200,
                             httpVersion: nil, headerFields: ["Content-Type": "text/html"])!, Data("<html>Login</html>".utf8))
        }
        do { try await api.power(true, clientId: "test", revision: 1); XCTFail("Expected signedOut") }
        catch { XCTAssertEqual(error as? PanelAPIError, .signedOut) }
    }

    func testPowerAndTemperatureSendClientRevision() async throws {
        StubURLProtocol.handler = { request in
            let components = URLComponents(url: request.url!, resolvingAgainstBaseURL: false)!
            if request.httpMethod == "GET" {
                XCTAssertEqual(components.path, "/turnOffLivingRoom")
                XCTAssertEqual(components.queryItems, [URLQueryItem(name: "clientId", value: "native-client"),
                                                       URLQueryItem(name: "revision", value: "12")])
            } else {
                XCTAssertEqual(components.path, "/setLivingRoomColorTemp")
                let body = try JSONSerialization.jsonObject(with: Self.body(request)) as! [String: Any]
                XCTAssertEqual(body["clientId"] as? String, "native-client")
                XCTAssertEqual(body["revision"] as? Int, 13)
                XCTAssertEqual(body["pct"] as? Int, 75)
            }
            return Self.json(request.url!, "{\"status\":200,\"body\":\"ok\"}")
        }
        try await api.power(false, clientId: "native-client", revision: 12)
        try await api.temperature(75, clientId: "native-client", revision: 13)
    }

    func testColorSyncOmitsTemperatureToPreserveRGBMode() async throws {
        StubURLProtocol.handler = { request in
            let body = try JSONSerialization.jsonObject(with: Self.body(request)) as! [String: Any]
            XCTAssertNil(body["colorTemperaturePct"])
            XCTAssertEqual(body["brightness"] as? Int, 50)
            return Self.json(request.url!, "{\"synchronized\":true,\"corrected\":[],\"stale\":false}")
        }
        let sync = try await api.sync(DesiredState(clientId: "rgb-client", revision: 3, on: true,
                                                   brightness: 50, colorTemperaturePct: nil))
        XCTAssertTrue(sync.synchronized)
    }

    func testServerFailureAndMalformedStateAreRejected() async {
        StubURLProtocol.handler = { request in
            (HTTPURLResponse(url: request.url!, statusCode: 500, httpVersion: nil, headerFields: nil)!, Data())
        }
        do { _ = try await api.state(); XCTFail("Expected server failure") }
        catch { XCTAssertEqual(error as? PanelAPIError, .server(500)) }
        StubURLProtocol.handler = { request in
            Self.json(request.url!, "{\"status\":200,\"body\":{\"on\":true,\"brightness\":999,\"colorTemperaturePct\":50}}")
        }
        do { _ = try await api.state(); XCTFail("Expected invalid state") }
        catch { XCTAssertEqual(error as? PanelAPIError, .invalidResponse) }
    }

    func testServerURLValidation() throws {
        XCTAssertEqual(try PanelAPI.serverURL(" http://127.0.0.1:8787/ ").absoluteString, "http://127.0.0.1:8787")
        for value in ["panel.test", "file:///tmp", "https://user:secret@panel.test", "https://panel.test/home", "https://panel.test?q=1"] {
            XCTAssertThrowsError(try PanelAPI.serverURL(value))
        }
    }

    private static func json(_ url: URL, _ value: String) -> (HTTPURLResponse, Data) {
        (HTTPURLResponse(url: url, statusCode: 200, httpVersion: nil,
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
