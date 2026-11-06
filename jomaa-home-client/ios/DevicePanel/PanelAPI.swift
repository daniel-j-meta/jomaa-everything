import Foundation

struct RoomState: Codable, Equatable {
    var on: Bool
    var brightness: Int
    var colorTemperaturePct: Int
}

struct StateResponse: Decodable {
    let status: Int
    let body: RoomState
}

struct DesiredState: Encodable {
    let clientId: String
    let revision: Int
    let on: Bool
    let brightness: Int
    let colorTemperaturePct: Int?
}

struct SyncResponse: Decodable {
    let synchronized: Bool
    let corrected: [String]
    let stale: Bool
}

struct CommandResponse: Decodable {
    let status: Int
    let body: String
}

enum PanelAPIError: LocalizedError, Equatable {
    case invalidServer
    case incorrectPassword
    case signedOut
    case server(Int)
    case invalidResponse

    var errorDescription: String? {
        switch self {
        case .invalidServer: return "Enter a valid http or https server URL."
        case .incorrectPassword: return "Incorrect password."
        case .signedOut: return "Your session has ended. Sign in again."
        case .server(let status): return "The server could not complete the request (HTTP \(status))."
        case .invalidResponse: return "The server returned an unexpected response."
        }
    }
}

actor PanelAPI {
    let baseURL: URL
    private let session: URLSession

    init(baseURL: URL, session: URLSession? = nil) {
        self.baseURL = baseURL
        if let session {
            self.session = session
        } else {
            let configuration = URLSessionConfiguration.ephemeral
            configuration.timeoutIntervalForRequest = 15
            configuration.timeoutIntervalForResource = 30
            configuration.requestCachePolicy = .reloadIgnoringLocalCacheData
            self.session = URLSession(configuration: configuration)
        }
    }

    static func serverURL(_ value: String) throws -> URL {
        guard var components = URLComponents(string: value.trimmingCharacters(in: .whitespacesAndNewlines)),
              ["http", "https"].contains(components.scheme?.lowercased() ?? ""),
              components.host?.isEmpty == false,
              components.user == nil, components.password == nil,
              components.query == nil, components.fragment == nil,
              components.path.isEmpty || components.path == "/" else {
            throw PanelAPIError.invalidServer
        }
        components.path = ""
        guard let url = components.url else { throw PanelAPIError.invalidServer }
        return url
    }

    func signIn(password: String) async throws {
        var request = URLRequest(url: baseURL.appendingPathComponent("login"))
        request.httpMethod = "POST"
        request.setValue("application/x-www-form-urlencoded", forHTTPHeaderField: "Content-Type")
        // Percent-encode every byte except the form's unreserved characters.
        let safe = CharacterSet(charactersIn: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~")
        guard let encoded = password.addingPercentEncoding(withAllowedCharacters: safe) else {
            throw PanelAPIError.invalidResponse
        }
        request.httpBody = Data("password=\(encoded)".utf8)
        let (_, response) = try await session.data(for: request)
        guard let response = response as? HTTPURLResponse else { throw PanelAPIError.invalidResponse }
        if response.statusCode == 401 { throw PanelAPIError.incorrectPassword }
        guard (200..<300).contains(response.statusCode) else { throw PanelAPIError.server(response.statusCode) }
        // URLSession follows /login -> /home and keeps the HttpOnly auth cookie.
        guard response.url?.path == "/home" else { throw PanelAPIError.signedOut }
    }

    func state() async throws -> RoomState {
        let response: StateResponse = try await jsonRequest(path: "getLivingRoomState")
        guard response.status == 200, (1...100).contains(response.body.brightness),
              (0...100).contains(response.body.colorTemperaturePct) else {
            throw PanelAPIError.invalidResponse
        }
        return response.body
    }

    func power(_ on: Bool, clientId: String, revision: Int) async throws {
        try await command(path: on ? "turnOnLivingRoom" : "turnOffLivingRoom",
                          query: revisionQuery(clientId, revision))
    }

    func brightness(_ value: Int, clientId: String, revision: Int) async throws {
        guard [10, 50, 75, 100].contains(value) else { throw PanelAPIError.invalidResponse }
        try await command(path: "setLivingRoomBrightness\(value)", query: revisionQuery(clientId, revision))
    }

    func temperature(_ value: Int, clientId: String, revision: Int) async throws {
        guard (0...100).contains(value) else { throw PanelAPIError.invalidResponse }
        try await command(path: "setLivingRoomColorTemp", body: ["pct": value, "clientId": clientId, "revision": revision])
    }

    func color(_ value: String, clientId: String, revision: Int) async throws {
        guard ["red", "orange"].contains(value) else { throw PanelAPIError.invalidResponse }
        try await command(path: "setLivingRoomColor", body: ["color": value, "clientId": clientId, "revision": revision])
    }

    func sync(_ desired: DesiredState) async throws -> SyncResponse {
        try await jsonRequest(path: "syncLivingRoom", body: JSONEncoder().encode(desired))
    }

    private func revisionQuery(_ clientId: String, _ revision: Int) -> [URLQueryItem] {
        [URLQueryItem(name: "clientId", value: clientId), URLQueryItem(name: "revision", value: String(revision))]
    }

    private func command(path: String, query: [URLQueryItem] = [], body: [String: Any]? = nil) async throws {
        let encoded = try body.map { try JSONSerialization.data(withJSONObject: $0) }
        let response: CommandResponse = try await jsonRequest(path: path, query: query, body: encoded)
        guard response.status == 200 else { throw PanelAPIError.server(response.status) }
    }

    private func jsonRequest<Response: Decodable>(path: String, query: [URLQueryItem] = [], body: Data? = nil) async throws -> Response {
        var components = URLComponents(url: baseURL.appendingPathComponent(path), resolvingAgainstBaseURL: false)!
        if !query.isEmpty { components.queryItems = query }
        var request = URLRequest(url: components.url!)
        request.httpMethod = body == nil ? "GET" : "POST"
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let body {
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.httpBody = body
        }
        let (data, response) = try await session.data(for: request)
        guard let response = response as? HTTPURLResponse else { throw PanelAPIError.invalidResponse }
        if response.url?.path == "/login" || response.statusCode == 401 { throw PanelAPIError.signedOut }
        guard (200..<300).contains(response.statusCode) else { throw PanelAPIError.server(response.statusCode) }
        guard response.value(forHTTPHeaderField: "Content-Type")?.lowercased().contains("application/json") == true else {
            throw PanelAPIError.invalidResponse
        }
        do { return try JSONDecoder().decode(Response.self, from: data) }
        catch { throw PanelAPIError.invalidResponse }
    }
}
