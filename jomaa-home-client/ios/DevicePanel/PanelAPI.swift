import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif
#if canImport(Security)
import Security
#endif

// MARK: - Hub models
//
// The app only talks to the home hub. Every device looks the same here,
// whatever brand it is: the hub speaks Matter (and anything else) for us.

/// What a device or room can change. A Matter plug, for example, has none.
struct Features: Codable, Equatable {
    var brightness = false
    var colorTemperature = false
    var color = false
}

/// A device or room. `kind` is "room" or the driver behind a single device.
struct Target: Codable, Equatable, Identifiable {
    let id: String
    let name: String
    let kind: String
    var members: [String]? = nil
    var features = Features()

    var isRoom: Bool { kind == "room" }
}

struct TargetList: Decodable {
    let targets: [Target]
}

/// Brightness and colorTemperaturePct are 0-100; 0 is the warmest white.
struct RoomState: Codable, Equatable {
    var on: Bool
    var brightness: Int
    var colorTemperaturePct: Int
}

struct StateResponse: Decodable {
    let state: RoomState?
    let devices: [String: RoomState]
    let failed: [String: String]?
}

enum ColorPreset: String, CaseIterable, Identifiable {
    case red = "#ff0000"
    case orange = "#ff8000"

    var id: String { rawValue }
    var title: String { self == .red ? "Red" : "Orange" }
}

/// A partial update. Nil fields are left out of the JSON and left alone.
struct Command: Encodable, Equatable {
    var on: Bool? = nil
    var brightness: Int? = nil
    var colorTemperaturePct: Int? = nil
    var color: String? = nil
    var clientId = ""
    var revision = 0
}

struct DesiredState: Encodable {
    let clientId: String
    let revision: Int
    let on: Bool
    let brightness: Int?
    let colorTemperaturePct: Int?
}

struct SyncResponse: Decodable {
    let synchronized: Bool
    let stale: Bool
    let corrected: [String: [String]]?
    let failed: [String: String]?
}

struct MatterNode: Decodable, Equatable, Identifiable {
    let nodeId: UInt64
    let available: Bool
    var vendor: String? = nil
    var product: String? = nil
    var label: String? = nil

    var id: UInt64 { nodeId }
    var displayName: String {
        let name = [vendor, product, label].compactMap { $0 }.joined(separator: " ")
        return name.isEmpty ? "Matter device" : name
    }
}

private struct MatterNodeList: Decodable { let nodes: [MatterNode] }
private struct CommissionResponse: Decodable {
    struct Node: Decodable {
        let nodeId: UInt64?
        enum CodingKeys: String, CodingKey { case nodeId = "node_id" }
    }
    let node: Node?
}
private struct LoginResponse: Decodable { let token: String }
private struct ErrorResponse: Decodable {
    let error: String?
    let failed: [String: String]?
}

enum MatterSetupCode {
    /// Returns the cleaned code, or nil if it isn't a QR payload ("MT:...") or an 11/21-digit manual code.
    static func normalize(_ input: String) -> String? {
        let trimmed = input.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.hasPrefix("MT:") { return trimmed.count > 3 ? trimmed : nil }
        let digits = trimmed.replacingOccurrences(of: "-", with: "").replacingOccurrences(of: " ", with: "")
        guard [11, 21].contains(digits.count), digits.allSatisfy(\.isASCIIDigit) else { return nil }
        return digits
    }
}

private extension Character {
    var isASCIIDigit: Bool { ("0"..."9").contains(self) }
}

enum PanelAPIError: LocalizedError, Equatable {
    case invalidServer
    case incorrectPassword
    case signedOut
    case devicesFailed([String: String])
    case matterUnavailable
    case server(Int, String?)
    case invalidResponse

    var errorDescription: String? {
        switch self {
        case .invalidServer: return "Enter a valid hub address, like http://192.168.1.10:8080."
        case .incorrectPassword: return "Incorrect password."
        case .signedOut: return "Your session has ended. Sign in again."
        case .devicesFailed(let failed):
            return "\(failed.count) device(s) didn't respond: \(failed.keys.sorted().joined(separator: ", "))"
        case .matterUnavailable: return "The hub isn't connected to its Matter server."
        case .server(let status, let message): return message ?? "The hub couldn't complete the request (HTTP \(status))."
        case .invalidResponse: return "The hub returned an unexpected response."
        }
    }
}

// MARK: - Client

actor PanelAPI {
    let baseURL: URL
    private(set) var token: String?
    private let session: URLSession

    init(baseURL: URL, token: String? = nil, session: URLSession? = nil) {
        self.baseURL = baseURL
        self.token = token
        if let session {
            self.session = session
        } else {
            let configuration = URLSessionConfiguration.ephemeral
            configuration.timeoutIntervalForRequest = 20
            // Pairing a Matter device can take a few minutes; the hub allows five.
            configuration.timeoutIntervalForResource = 330
            configuration.requestCachePolicy = .reloadIgnoringLocalCacheData
            configuration.httpShouldSetCookies = false
            self.session = URLSession(configuration: configuration)
        }
    }

    /// Accepts https anywhere, and plain http only for addresses on the home
    /// network, where the hub normally runs without TLS.
    static func serverURL(_ value: String) throws -> URL {
        guard var components = URLComponents(string: value.trimmingCharacters(in: .whitespacesAndNewlines)),
              let scheme = components.scheme?.lowercased(), ["http", "https"].contains(scheme),
              let host = components.host, !host.isEmpty,
              components.user == nil, components.password == nil,
              components.query == nil, components.fragment == nil,
              components.path.isEmpty || components.path == "/" else {
            throw PanelAPIError.invalidServer
        }
        if scheme == "http" && !isLocalHost(host) { throw PanelAPIError.invalidServer }
        components.path = ""
        guard let url = components.url else { throw PanelAPIError.invalidServer }
        return url
    }

    static func isLocalHost(_ host: String) -> Bool {
        let h = host.lowercased().trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        if h == "localhost" || h.hasSuffix(".local") { return true }
        if h.contains(":") { return h == "::1" || h.hasPrefix("fe80:") || h.hasPrefix("fd") || h.hasPrefix("fc") }
        let parts = h.split(separator: ".", omittingEmptySubsequences: false)
        if parts.count == 1 { return true } // single-label name, like "hub"
        let octets = parts.compactMap { Int($0) }
        guard octets.count == 4, parts.count == 4, octets.allSatisfy({ (0...255).contains($0) }) else { return false }
        let (a, b) = (octets[0], octets[1])
        return a == 10 || a == 127 || (a == 172 && (16...31).contains(b)) || (a == 192 && b == 168) || (a == 169 && b == 254)
    }

    /// Exchanges the hub password for a session token.
    @discardableResult
    func signIn(password: String) async throws -> String {
        let response: LoginResponse = try await request("POST", ["api", "login"], body: ["password": password],
                                                        authorized: false, unauthorized: .incorrectPassword)
        token = response.token
        return response.token
    }

    func signOut() async {
        let _: [String: Bool]? = try? await request("POST", ["api", "logout"], body: [String: String]())
        token = nil
    }

    func targets() async throws -> [Target] {
        let list: TargetList = try await request("GET", ["api", "targets"])
        return list.targets
    }

    func state(of target: String) async throws -> StateResponse {
        let response: StateResponse = try await request("GET", ["api", "targets", target, "state"])
        if let state = response.state {
            guard (0...100).contains(state.brightness), (0...100).contains(state.colorTemperaturePct) else {
                throw PanelAPIError.invalidResponse
            }
        }
        return response
    }

    func send(_ command: Command, to target: String) async throws {
        if let brightness = command.brightness, !(1...100).contains(brightness) { throw PanelAPIError.invalidResponse }
        if let temperature = command.colorTemperaturePct, !(0...100).contains(temperature) { throw PanelAPIError.invalidResponse }
        let _: [String: Bool] = try await request("POST", ["api", "targets", target, "command"], body: command)
    }

    func sync(_ desired: DesiredState, target: String) async throws -> SyncResponse {
        try await request("POST", ["api", "targets", target, "sync"], body: desired)
    }

    func matterNodes() async throws -> [MatterNode] {
        let list: MatterNodeList = try await request("GET", ["api", "matter", "nodes"])
        return list.nodes
    }

    /// Pairs a Matter device through the hub. Returns the new node id.
    func commission(code: String, networkOnly: Bool) async throws -> UInt64? {
        struct Body: Encodable { let code: String; let networkOnly: Bool }
        let response: CommissionResponse = try await request("POST", ["api", "matter", "commission"],
                                                             body: Body(code: code, networkOnly: networkOnly),
                                                             timeout: 330)
        return response.node?.nodeId
    }

    private func request<Response: Decodable>(
        _ method: String,
        _ path: [String],
        body: (any Encodable)? = nil,
        authorized: Bool = true,
        unauthorized: PanelAPIError = .signedOut,
        timeout: TimeInterval? = nil
    ) async throws -> Response {
        var url = baseURL
        for segment in path { url.appendPathComponent(segment) }
        var request = URLRequest(url: url)
        request.httpMethod = method
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let timeout { request.timeoutInterval = timeout }
        if authorized {
            guard let token else { throw PanelAPIError.signedOut }
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        if let body {
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.httpBody = try JSONEncoder().encode(body)
        }
        let (data, response) = try await session.data(for: request)
        guard let response = response as? HTTPURLResponse else { throw PanelAPIError.invalidResponse }
        guard (200..<300).contains(response.statusCode) else {
            let payload = try? JSONDecoder().decode(ErrorResponse.self, from: data)
            switch response.statusCode {
            case 401: throw unauthorized
            case 502 where !(payload?.failed ?? [:]).isEmpty: throw PanelAPIError.devicesFailed(payload!.failed!)
            case 503: throw PanelAPIError.matterUnavailable
            default: throw PanelAPIError.server(response.statusCode, payload?.error)
            }
        }
        guard response.value(forHTTPHeaderField: "Content-Type")?.lowercased().contains("application/json") == true else {
            throw PanelAPIError.invalidResponse
        }
        do { return try JSONDecoder().decode(Response.self, from: data) }
        catch { throw PanelAPIError.invalidResponse }
    }
}

// MARK: - Token storage

/// Keeps the hub session token in the Keychain so the app stays signed in.
/// The password itself is never stored.
enum TokenStore {
    private static let service = "com.jomaa.DevicePanel.hub"

    static func load(for server: String) -> String? {
        #if canImport(Security)
        var query = baseQuery(server)
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var item: CFTypeRef?
        guard SecItemCopyMatching(query as CFDictionary, &item) == errSecSuccess,
              let data = item as? Data else { return nil }
        return String(data: data, encoding: .utf8)
        #else
        return nil
        #endif
    }

    static func save(_ token: String, for server: String) {
        #if canImport(Security)
        clear(for: server)
        var query = baseQuery(server)
        query[kSecValueData as String] = Data(token.utf8)
        query[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        SecItemAdd(query as CFDictionary, nil)
        #endif
    }

    static func clear(for server: String) {
        #if canImport(Security)
        SecItemDelete(baseQuery(server) as CFDictionary)
        #endif
    }

    #if canImport(Security)
    private static func baseQuery(_ server: String) -> [String: Any] {
        [kSecClass as String: kSecClassGenericPassword,
         kSecAttrService as String: service,
         kSecAttrAccount as String: server]
    }
    #endif
}
