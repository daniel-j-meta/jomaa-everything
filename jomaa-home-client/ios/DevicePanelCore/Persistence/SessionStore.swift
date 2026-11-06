import Foundation
#if canImport(Security)
import Security
#endif

public protocol SessionStoring: Sendable {
    func load() async throws -> Session?
    func save(_ session: Session) async throws
    func clear() async throws
}

#if canImport(Security)
/// Keeps the hub session token in the Keychain, shared with the widget.
public actor KeychainSessionStore: SessionStoring {
    private let service: String
    private let account: String
    private let accessGroup: String?
    private let encoder = JSONEncoder()
    private let decoder = JSONDecoder()

    public init(
        service: String = "com.danieljomaa.devicepanel.session",
        account: String = "current-session",
        accessGroup: String? = nil
    ) {
        self.service = service
        self.account = account
        self.accessGroup = accessGroup
    }

    public func load() throws -> Session? {
        var query = baseQuery
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne

        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        if status == errSecItemNotFound {
            return nil
        }
        guard status == errSecSuccess, let data = result as? Data else {
            throw DevicePanelError.keychain(status: status)
        }
        do {
            return try decoder.decode(Session.self, from: data)
        } catch {
            throw DevicePanelError.storage
        }
    }

    public func save(_ session: Session) throws {
        let data: Data
        do {
            data = try encoder.encode(session)
        } catch {
            throw DevicePanelError.storage
        }

        let attributes: [String: Any] = [kSecValueData as String: data]
        let updateStatus = SecItemUpdate(baseQuery as CFDictionary, attributes as CFDictionary)
        if updateStatus == errSecSuccess {
            return
        }
        guard updateStatus == errSecItemNotFound else {
            throw DevicePanelError.keychain(status: updateStatus)
        }

        var addQuery = baseQuery
        addQuery[kSecValueData as String] = data
        addQuery[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        let addStatus = SecItemAdd(addQuery as CFDictionary, nil)
        guard addStatus == errSecSuccess else {
            throw DevicePanelError.keychain(status: addStatus)
        }
    }

    public func clear() throws {
        let status = SecItemDelete(baseQuery as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw DevicePanelError.keychain(status: status)
        }
    }

    private var baseQuery: [String: Any] {
        var query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
        if let accessGroup, !accessGroup.isEmpty {
            query[kSecAttrAccessGroup as String] = accessGroup
        }
        return query
    }
}
#endif

public actor InMemorySessionStore: SessionStoring {
    private var session: Session?

    public init(session: Session? = nil) {
        self.session = session
    }

    public func load() -> Session? {
        session
    }

    public func save(_ session: Session) {
        self.session = session
    }

    public func clear() {
        session = nil
    }
}
