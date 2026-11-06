import Foundation

/// The hub's session token. It stays valid until the hub password changes;
/// the password itself is never stored.
public struct Session: Codable, Equatable, Sendable {
    public let token: String

    public init(token: String) {
        self.token = token
    }
}

struct SignInRequest: Encodable, Sendable {
    let password: String
}

struct SignInResponse: Decodable, Sendable {
    let token: String
}
