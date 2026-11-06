import Foundation
import Combine

@MainActor
final class PanelModel: ObservableObject {
    @Published var server = ProcessInfo.processInfo.environment["DEVICE_PANEL_BASE_URL"]
        ?? UserDefaults.standard.string(forKey: "panelServer") ?? "http://127.0.0.1:8787"
    @Published var password = ""
    @Published var signedIn = false
    @Published var busy = false
    @Published var connected = false
    @Published var errorMessage: String?
    @Published var room = RoomState(on: false, brightness: 50, colorTemperaturePct: 50)
    @Published var temperatureDraft: Double = 50
    @Published var selectedColor: String?

    private var api: PanelAPI?
    private let clientId = UUID().uuidString
    private var revision = 0
    private var reconciliation: Task<Void, Never>?

    func signIn(password: String) async {
        guard !busy else { return }
        busy = true
        errorMessage = nil
        defer { busy = false }
        do {
            let url = try PanelAPI.serverURL(server)
            let client = PanelAPI(baseURL: url)
            try await client.signIn(password: password)
            room = try await client.state()
            temperatureDraft = Double(room.colorTemperaturePct)
            selectedColor = nil
            api = client
            server = url.absoluteString
            UserDefaults.standard.set(server, forKey: "panelServer")
            self.password = ""
            signedIn = true
            connected = true
        } catch {
            handle(error)
        }
    }

    func signOut() {
        reconciliation?.cancel()
        reconciliation = nil
        api = nil // The ephemeral URLSession's cookies disappear with the session.
        password = ""
        signedIn = false
        connected = false
        errorMessage = nil
    }

    func refresh() async {
        guard !busy, let api else { return }
        busy = true
        reconciliation?.cancel()
        defer { busy = false }
        do {
            room = try await api.state()
            temperatureDraft = Double(room.colorTemperaturePct)
            selectedColor = nil
            connected = true
            errorMessage = nil
        } catch { handle(error) }
    }

    func setPower(_ on: Bool) async {
        await apply { room.on = on } command: { api, revision in
            try await api.power(on, clientId: self.clientId, revision: revision)
        }
    }

    func setBrightness(_ value: Int) async {
        await apply { room.brightness = value } command: { api, revision in
            try await api.brightness(value, clientId: self.clientId, revision: revision)
        }
    }

    func setTemperature(_ value: Int) async {
        await apply {
            room.colorTemperaturePct = value
            temperatureDraft = Double(value)
            selectedColor = nil
        } command: { api, revision in
            try await api.temperature(value, clientId: self.clientId, revision: revision)
        }
    }

    func setColor(_ value: String) async {
        await apply { selectedColor = value } command: { api, revision in
            try await api.color(value, clientId: self.clientId, revision: revision)
        }
    }

    private func apply(_ optimistic: () -> Void, command: (PanelAPI, Int) async throws -> Void) async {
        guard !busy, let api else { return }
        reconciliation?.cancel()
        let previousRoom = room
        let previousColor = selectedColor
        busy = true
        errorMessage = nil
        revision += 1
        optimistic()
        do {
            try await command(api, revision)
            connected = true
        } catch {
            room = previousRoom
            temperatureDraft = Double(previousRoom.colorTemperaturePct)
            selectedColor = previousColor
            handle(error)
        }
        busy = false
        if signedIn { scheduleReconciliation() }
    }

    private func scheduleReconciliation() {
        let expectedRevision = revision
        reconciliation = Task { [weak self] in
            for delay in [3, 6, 10, 15] {
                do {
                    try await Task.sleep(for: .seconds(delay))
                    guard let self, !Task.isCancelled, self.revision == expectedRevision,
                          let api = self.api else { return }
                    let desired = DesiredState(clientId: self.clientId, revision: expectedRevision,
                                               on: self.room.on, brightness: self.room.brightness,
                                               colorTemperaturePct: self.selectedColor == nil ? self.room.colorTemperaturePct : nil)
                    let result = try await api.sync(desired)
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

    private func handle(_ error: Error) {
        connected = false
        errorMessage = error.localizedDescription
        if error as? PanelAPIError == .signedOut {
            signedIn = false
            api = nil
            reconciliation?.cancel()
        }
    }
}
