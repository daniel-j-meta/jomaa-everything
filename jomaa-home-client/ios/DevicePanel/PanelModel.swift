import Foundation
import Combine

@MainActor
final class PanelModel: ObservableObject {
    @Published var server = ProcessInfo.processInfo.environment["DEVICE_PANEL_BASE_URL"]
        ?? UserDefaults.standard.string(forKey: "panelServer") ?? "http://hub.local:8080"
    @Published var password = ""
    @Published var signedIn = false
    @Published var busy = false
    @Published var connected = false
    @Published var errorMessage: String?
    @Published var targets: [Target] = []
    @Published var selected: Target?
    @Published var room = RoomState(on: false, brightness: 50, colorTemperaturePct: 50)
    @Published var temperatureDraft: Double = 50
    @Published var selectedColor: ColorPreset?

    // Matter pairing
    @Published var matterNodes: [MatterNode] = []
    @Published var pairing = false
    @Published var pairedNodeId: UInt64?
    @Published var matterError: String?

    private var api: PanelAPI?
    private let clientId = UUID().uuidString
    private var revision = 0
    private var reconciliation: Task<Void, Never>?

    var features: Features { selected?.features ?? Features() }

    init() {
        // Stay signed in across launches when the Keychain has a token for this hub.
        if ProcessInfo.processInfo.environment["DEVICE_PANEL_RESET"] != nil {
            TokenStore.clear(for: server)
        } else if let url = try? PanelAPI.serverURL(server), let token = TokenStore.load(for: url.absoluteString) {
            api = PanelAPI(baseURL: url, token: token)
            signedIn = true
            Task { await refresh() }
        }
    }

    func signIn(password: String) async {
        guard !busy else { return }
        busy = true
        errorMessage = nil
        defer { busy = false }
        do {
            let url = try PanelAPI.serverURL(server)
            let client = PanelAPI(baseURL: url)
            let token = try await client.signIn(password: password)
            api = client
            server = url.absoluteString
            UserDefaults.standard.set(server, forKey: "panelServer")
            TokenStore.save(token, for: server)
            self.password = ""
            try await load(select: nil, using: client)
            signedIn = true
            connected = true
        } catch {
            handle(error)
        }
    }

    func signOut() {
        reconciliation?.cancel()
        reconciliation = nil
        if let api { Task { await api.signOut() } }
        TokenStore.clear(for: server)
        api = nil
        password = ""
        signedIn = false
        connected = false
        errorMessage = nil
        targets = []
        selected = nil
    }

    func refresh() async {
        guard !busy, let api else { return }
        busy = true
        reconciliation?.cancel()
        defer { busy = false }
        do {
            try await load(select: selected?.id, using: api)
            connected = true
            errorMessage = nil
        } catch { handle(error) }
    }

    func select(_ target: Target) async {
        guard !busy, let api, target.id != selected?.id else { return }
        busy = true
        reconciliation?.cancel()
        defer { busy = false }
        do {
            try await loadState(of: target, using: api)
            connected = true
            errorMessage = nil
        } catch { handle(error) }
    }

    /// Loads the device list, then the chosen target: the given id if it still
    /// exists, else the first room, else the first device.
    private func load(select id: String?, using api: PanelAPI) async throws {
        let list = try await api.targets()
        targets = list
        guard let target = list.first(where: { $0.id == id }) ?? list.first(where: \.isRoom) ?? list.first else {
            selected = nil
            return
        }
        try await loadState(of: target, using: api)
    }

    private func loadState(of target: Target, using api: PanelAPI) async throws {
        let response = try await api.state(of: target.id)
        guard let state = response.state else { throw PanelAPIError.devicesFailed(response.failed ?? [:]) }
        selected = target
        room = state
        temperatureDraft = Double(state.colorTemperaturePct)
        selectedColor = nil
        if let failed = response.failed, !failed.isEmpty {
            errorMessage = PanelAPIError.devicesFailed(failed).localizedDescription
        }
    }

    func setPower(_ on: Bool) async {
        await apply(Command(on: on)) { room.on = on }
    }

    func setBrightness(_ value: Int) async {
        await apply(Command(brightness: value)) { room.brightness = value }
    }

    func setTemperature(_ value: Int) async {
        await apply(Command(colorTemperaturePct: value)) {
            room.colorTemperaturePct = value
            temperatureDraft = Double(value)
            selectedColor = nil
        }
    }

    func setColor(_ value: ColorPreset) async {
        await apply(Command(color: value.rawValue)) { selectedColor = value }
    }

    private func apply(_ command: Command, optimistic: () -> Void) async {
        guard !busy, let api, let target = selected else { return }
        reconciliation?.cancel()
        let previousRoom = room
        let previousColor = selectedColor
        busy = true
        errorMessage = nil
        revision += 1
        var command = command
        command.clientId = clientId
        command.revision = revision
        optimistic()
        do {
            try await api.send(command, to: target.id)
            connected = true
        } catch {
            room = previousRoom
            temperatureDraft = Double(previousRoom.colorTemperaturePct)
            selectedColor = previousColor
            handle(error)
        }
        busy = false
        if signedIn { scheduleReconciliation(target: target) }
    }

    private func scheduleReconciliation(target: Target) {
        let expectedRevision = revision
        reconciliation = Task { [weak self] in
            for delay in [3, 6, 10, 15] {
                do {
                    try await Task.sleep(for: .seconds(delay))
                    guard let self, !Task.isCancelled, self.revision == expectedRevision,
                          let api = self.api else { return }
                    let room = self.room
                    let features = target.features
                    // After picking a color, leave white temperature alone so
                    // sync doesn't replace the color with white light.
                    let desired = DesiredState(
                        clientId: self.clientId, revision: expectedRevision, on: room.on,
                        brightness: room.on && features.brightness && room.brightness > 0 ? room.brightness : nil,
                        colorTemperaturePct: room.on && features.colorTemperature && self.selectedColor == nil
                            ? room.colorTemperaturePct : nil)
                    let result = try await api.sync(desired, target: target.id)
                    guard !Task.isCancelled, self.revision == expectedRevision else { return }
                    self.connected = true
                    if result.stale || result.synchronized { return }
                } catch is CancellationError { return }
                catch {
                    guard let self, !Task.isCancelled else { return }
                    self.handle(error)
                    if !self.signedIn { return }
                }
            }
        }
    }

    // MARK: Matter

    func loadMatterNodes() async {
        guard let api else { return }
        matterError = nil
        do { matterNodes = try await api.matterNodes() }
        catch { matterError = error.localizedDescription; signOutIfExpired(error) }
    }

    func pair(code: String, networkOnly: Bool) async {
        guard let api, !pairing else { return }
        guard let normalized = MatterSetupCode.normalize(code) else {
            matterError = "Enter the 11-digit setup code or the MT: code from the QR label."
            return
        }
        pairing = true
        pairedNodeId = nil
        matterError = nil
        defer { pairing = false }
        do {
            pairedNodeId = try await api.commission(code: normalized, networkOnly: networkOnly)
            await loadMatterNodes()
        } catch {
            matterError = error.localizedDescription
            signOutIfExpired(error)
        }
    }

    func clearPairingResult() {
        pairedNodeId = nil
        matterError = nil
    }

    private func handle(_ error: Error) {
        connected = false
        errorMessage = error.localizedDescription
        signOutIfExpired(error)
    }

    private func signOutIfExpired(_ error: Error) {
        guard error as? PanelAPIError == .signedOut else { return }
        TokenStore.clear(for: server)
        signedIn = false
        api = nil
        reconciliation?.cancel()
    }
}
