import Foundation

/// The room or device the app last showed, cached for the widget, Shortcuts,
/// and offline starts.
public struct CachedTarget: Codable, Equatable, Sendable {
    public let id: String
    public let name: String
    public let features: Features
    public let state: RoomState?

    public init(id: String, name: String, features: Features, state: RoomState?) {
        self.id = id
        self.name = name
        self.features = features
        self.state = state
    }

    public init(target: Target, state: RoomState?) {
        self.init(id: target.id, name: target.name, features: target.features, state: state)
    }

    public func with(state: RoomState?) -> CachedTarget {
        CachedTarget(id: id, name: name, features: features, state: state)
    }
}

public actor SharedPreferences {
    public static let appGroup = "group.com.danieljomaa.devicepanel"

    private enum Key {
        static let serverURL = "serverURL"
        static let clientID = "clientID"
        static let revision = "revision"
        static let selectedTarget = "selectedTarget"
    }

    private let defaults: UserDefaults
    private let encoder: JSONEncoder
    private let decoder: JSONDecoder

    public init(suiteName: String? = SharedPreferences.appGroup) {
        if let suiteName, let sharedDefaults = UserDefaults(suiteName: suiteName) {
            defaults = sharedDefaults
        } else {
            defaults = .standard
        }
        encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
    }

    public func serverConfiguration() throws -> ServerConfiguration? {
        guard let value = defaults.string(forKey: Key.serverURL) else {
            return nil
        }
        return try ServerConfiguration(urlString: value)
    }

    public func saveServerConfiguration(_ configuration: ServerConfiguration) {
        defaults.set(configuration.baseURL.absoluteString, forKey: Key.serverURL)
    }

    public func clientID() -> UUID {
        if let rawValue = defaults.string(forKey: Key.clientID), let value = UUID(uuidString: rawValue) {
            return value
        }
        let value = UUID()
        defaults.set(value.uuidString, forKey: Key.clientID)
        return value
    }

    public func currentRevision() -> Int64 {
        Int64(defaults.string(forKey: Key.revision) ?? "0") ?? 0
    }

    /// One rising counter for every command this install sends, as the hub expects.
    public func nextRevision(now: Date = Date()) -> Int64 {
        let current = currentRevision()
        let timestamp = Int64(now.timeIntervalSince1970 * 1_000_000)
        let next = max(current + 1, timestamp)
        defaults.set(String(next), forKey: Key.revision)
        return next
    }

    public func selectedTarget() -> CachedTarget? {
        guard let data = defaults.data(forKey: Key.selectedTarget) else {
            return nil
        }
        return try? decoder.decode(CachedTarget.self, from: data)
    }

    public func saveSelectedTarget(_ target: CachedTarget) {
        // Encoding these plain values can't fail.
        if let data = try? encoder.encode(target) {
            defaults.set(data, forKey: Key.selectedTarget)
        }
    }

    public func cachedRoomState() -> RoomState? {
        selectedTarget()?.state
    }

    public func clearSelectedTarget() {
        defaults.removeObject(forKey: Key.selectedTarget)
    }

    public func reset() {
        defaults.removeObject(forKey: Key.serverURL)
        defaults.removeObject(forKey: Key.clientID)
        defaults.removeObject(forKey: Key.revision)
        defaults.removeObject(forKey: Key.selectedTarget)
    }
}
