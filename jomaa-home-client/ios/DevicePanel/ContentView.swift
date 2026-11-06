import SwiftUI

private let panelAccent = Color(red: 1, green: 0.71, blue: 0.30)

struct ContentView: View {
    @ObservedObject var panel: PanelModel
    @State private var showMatter = false

    var body: some View {
        ZStack {
            LinearGradient(colors: [Color(red: 0.10, green: 0.10, blue: 0.18),
                                    Color(red: 0.08, green: 0.13, blue: 0.24)],
                           startPoint: .topLeading, endPoint: .bottomTrailing)
                .ignoresSafeArea()
            if panel.signedIn {
                controls
            } else {
                signIn
            }
        }
        .tint(panelAccent)
        .animation(.easeInOut(duration: 0.2), value: panel.signedIn)
    }

    private var signIn: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 24) {
                Image(systemName: "lightbulb.2.fill")
                    .font(.system(size: 42)).foregroundStyle(panelAccent)
                VStack(alignment: .leading, spacing: 8) {
                    Text("Device Panel").font(.largeTitle.bold())
                    Text("Your home hub, one touch away.").foregroundStyle(.secondary)
                }
                VStack(alignment: .leading, spacing: 16) {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Hub address").font(.subheadline.weight(.medium))
                        TextField("http://192.168.1.10:8080", text: $panel.server)
                            .keyboardType(.URL).textInputAutocapitalization(.never)
                            .autocorrectionDisabled().accessibilityIdentifier("serverURL")
                            .fieldStyle()
                    }
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Hub password").font(.subheadline.weight(.medium))
                        SecureField("Password", text: $panel.password)
                            .textContentType(.password).submitLabel(.go)
                            .accessibilityIdentifier("password").fieldStyle()
                            .onSubmit { signInAction() }
                    }
                    if let error = panel.errorMessage { ErrorBanner(message: error) }
                    Button(action: signInAction) {
                        HStack {
                            if panel.busy { ProgressView().tint(Color.black) }
                            Text(panel.busy ? "Signing in…" : "Sign in").fontWeight(.semibold)
                        }
                        .frame(maxWidth: .infinity).padding(.vertical, 14)
                        .foregroundStyle(Color(red: 0.10, green: 0.10, blue: 0.18))
                        .background(panelAccent, in: RoundedRectangle(cornerRadius: 12))
                    }
                    .disabled(panel.busy || panel.password.isEmpty)
                    .accessibilityIdentifier("signIn")
                }
                .padding(24).cardStyle()
            }
            .padding(24).frame(maxWidth: 520)
            .padding(.top, 60)
            .frame(maxWidth: .infinity)
        }
        .scrollDismissesKeyboard(.interactively)
    }

    private var controls: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 20) {
                    HStack {
                        VStack(alignment: .leading, spacing: 6) {
                            Text(panel.selected?.name ?? "Home").font(.largeTitle.bold())
                                .accessibilityIdentifier("targetTitle")
                            Text("Make yourself at home.").foregroundStyle(.secondary)
                        }
                        Spacer()
                    }
                    HStack(spacing: 8) {
                        Circle().fill(panel.connected ? .green : .red).frame(width: 8, height: 8)
                        Text(panel.busy ? "Updating…" : panel.connected ? "Connected" : "Connection issue")
                            .font(.subheadline)
                            .accessibilityIdentifier("connectionStatus")
                        if panel.busy { ProgressView().scaleEffect(0.7) }
                    }
                    if panel.targets.count > 1 { TargetPicker(panel: panel) }
                    if let error = panel.errorMessage { ErrorBanner(message: error) }
                    if panel.selected != nil {
                        PowerCard(panel: panel)
                        if panel.features.brightness { BrightnessCard(panel: panel) }
                        if panel.features.color { ColorCard(panel: panel) }
                        if panel.features.colorTemperature { TemperatureCard(panel: panel) }
                    } else if !panel.busy {
                        Text("The hub has no devices yet. Add them to its config.json.")
                            .foregroundStyle(.secondary)
                    }
                }
                .padding(24).frame(maxWidth: 620)
                .frame(maxWidth: .infinity)
            }
            .background(Color.clear)
            .scrollContentBackground(.hidden)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button { showMatter = true } label: { Image(systemName: "plus") }
                        .accessibilityLabel("Add Matter device").accessibilityIdentifier("matterButton")
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        Task { await panel.refresh() }
                    } label: { Image(systemName: "arrow.clockwise") }
                    .accessibilityLabel("Refresh state").disabled(panel.busy)
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Sign out") { panel.signOut() }
                        .disabled(panel.busy)
                }
            }
            .toolbarBackground(.hidden, for: .navigationBar)
            .sheet(isPresented: $showMatter, onDismiss: panel.clearPairingResult) {
                MatterPairingView(panel: panel)
            }
        }
        .background(Color.clear)
    }

    private func signInAction() {
        Task {
            await panel.signIn(password: panel.password)
        }
    }
}

private struct PowerCard: View {
    @ObservedObject var panel: PanelModel

    var body: some View {
        Toggle(isOn: Binding(get: { panel.room.on }, set: { value in
            Task { await panel.setPower(value) }
        })) {
            HStack(spacing: 14) {
                Image(systemName: "power").font(.title2).foregroundStyle(panelAccent)
                VStack(alignment: .leading, spacing: 4) {
                    Text("Power").font(.headline)
                    Text(panel.room.on ? "Lights are on" : "Lights are off")
                        .font(.subheadline).foregroundStyle(.secondary)
                }
            }
        }
        .accessibilityLabel("\(panel.selected?.name ?? "Room") power").accessibilityIdentifier("powerToggle")
        .disabled(panel.busy).padding(20).cardStyle()
    }
}

private struct BrightnessCard: View {
    @ObservedObject var panel: PanelModel

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Label("Brightness", systemImage: "sun.max").font(.headline)
            HStack(spacing: 10) {
                ForEach([10, 50, 75, 100], id: \.self) { value in
                    Button {
                        Task { await panel.setBrightness(value) }
                    } label: {
                        Text("\(value)%").font(.subheadline.weight(.semibold))
                            .frame(maxWidth: .infinity).padding(.vertical, 14)
                            .foregroundStyle(panel.room.brightness == value ? Color.black : Color.white)
                            .background(panel.room.brightness == value ? panelAccent : Color.white.opacity(0.08),
                                        in: RoundedRectangle(cornerRadius: 10))
                    }
                    .accessibilityLabel("Brightness \(value) percent")
                    .accessibilityIdentifier("brightness\(value)")
                    .accessibilityAddTraits(panel.room.brightness == value ? .isSelected : [])
                }
            }
            .disabled(panel.busy)
        }
        .padding(20).cardStyle()
    }
}

private struct ColorCard: View {
    @ObservedObject var panel: PanelModel

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Label("Color", systemImage: "paintpalette").font(.headline)
            HStack(spacing: 12) {
                ForEach(ColorPreset.allCases) { preset in
                    colorButton(preset, color: preset == .red ? .red : .orange)
                }
            }
            .disabled(panel.busy)
        }
        .padding(20).cardStyle()
    }

    private func colorButton(_ value: ColorPreset, color: Color) -> some View {
        Button {
            Task { await panel.setColor(value) }
        } label: {
            HStack(spacing: 8) {
                Circle().fill(color).frame(width: 14, height: 14)
                Text(value.title).fontWeight(.medium)
                if panel.selectedColor == value { Image(systemName: "checkmark") }
            }
            .frame(maxWidth: .infinity).padding(.vertical, 12)
            .background(Color.white.opacity(0.08), in: RoundedRectangle(cornerRadius: 10))
            .overlay(RoundedRectangle(cornerRadius: 10).stroke(panel.selectedColor == value ? color : .clear, lineWidth: 2))
        }
        .foregroundStyle(.white).accessibilityIdentifier("color\(value.title)")
        .accessibilityAddTraits(panel.selectedColor == value ? .isSelected : [])
    }
}

private struct TemperatureCard: View {
    @ObservedObject var panel: PanelModel

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Label("Color temperature", systemImage: "thermometer.medium").font(.headline)
            Slider(value: $panel.temperatureDraft, in: 0...100, step: 1) { editing in
                if !editing { Task { await panel.setTemperature(Int(panel.temperatureDraft)) } }
            }
            .accessibilityLabel("Color temperature").accessibilityIdentifier("temperatureSlider")
            .disabled(panel.busy)
            HStack {
                Text("Warm").foregroundStyle(panelAccent)
                Spacer()
                let signed = Int(panel.temperatureDraft) * 2 - 100
                Text("\(signed > 0 ? "+" : "")\(signed)%")
                    .fontWeight(.semibold).monospacedDigit().accessibilityIdentifier("temperatureValue")
                Spacer()
                Text("Cool").foregroundStyle(.cyan)
            }
            .font(.subheadline)
        }
        .padding(20).cardStyle()
    }
}

private struct TargetPicker: View {
    @ObservedObject var panel: PanelModel

    var body: some View {
        // Rooms first, then single devices.
        let ordered = panel.targets.filter(\.isRoom) + panel.targets.filter { !$0.isRoom }
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                ForEach(ordered) { target in
                    let selected = target.id == panel.selected?.id
                    Button {
                        Task { await panel.select(target) }
                    } label: {
                        Label(target.name, systemImage: target.isRoom ? "square.split.2x2" : "lightbulb")
                            .font(.subheadline.weight(.medium))
                            .padding(.horizontal, 14).padding(.vertical, 9)
                            .foregroundStyle(selected ? Color.black : Color.white)
                            .background(selected ? panelAccent : Color.white.opacity(0.08), in: Capsule())
                    }
                    .accessibilityIdentifier("target-\(target.id)")
                    .accessibilityAddTraits(selected ? .isSelected : [])
                }
            }
        }
        .disabled(panel.busy)
    }
}

struct MatterPairingView: View {
    @ObservedObject var panel: PanelModel
    @Environment(\.dismiss) private var dismiss
    @State private var code = ""
    @State private var networkOnly = false

    var body: some View {
        NavigationStack {
            Form {
                Section("Paired devices") {
                    if panel.matterNodes.isEmpty {
                        Text("No Matter devices paired yet.").foregroundStyle(.secondary)
                    }
                    ForEach(panel.matterNodes) { node in
                        HStack {
                            Text(node.displayName)
                            Spacer()
                            Text("node \(node.nodeId)").foregroundStyle(.secondary).monospacedDigit()
                            Circle().fill(node.available ? .green : .red).frame(width: 8, height: 8)
                        }
                    }
                }
                Section {
                    TextField("MT:… or 1234-567-8901", text: $code)
                        .textInputAutocapitalization(.characters).autocorrectionDisabled()
                        .accessibilityIdentifier("pairingCode")
                    Toggle("Already on my network", isOn: $networkOnly)
                    Button {
                        Task { await panel.pair(code: code, networkOnly: networkOnly) }
                    } label: {
                        HStack {
                            if panel.pairing { ProgressView() }
                            Text(panel.pairing ? "Pairing…" : "Pair")
                        }
                    }
                    .disabled(code.isEmpty || panel.pairing)
                    .accessibilityIdentifier("pairButton")
                } header: {
                    Text("Add a device")
                } footer: {
                    Text("Use the setup code on the device or its box. The hub does the pairing, so keep the device near it.")
                }
                if let node = panel.pairedNodeId {
                    Text("Paired as node \(node). Add it to a room in the hub's config.json.")
                        .foregroundStyle(.green).accessibilityIdentifier("pairingResult")
                }
                if let error = panel.matterError {
                    Text(error).foregroundStyle(.red).accessibilityIdentifier("pairingError")
                }
            }
            .navigationTitle("Matter devices")
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }.disabled(panel.pairing)
                }
            }
            .task { await panel.loadMatterNodes() }
            .onChange(of: panel.pairedNodeId) { _, node in if node != nil { code = "" } }
        }
        .interactiveDismissDisabled(panel.pairing)
    }
}

private struct ErrorBanner: View {
    let message: String

    var body: some View {
        Label(message, systemImage: "exclamationmark.circle")
            .font(.subheadline).foregroundStyle(Color(red: 1, green: 0.54, blue: 0.54))
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(12).background(Color.red.opacity(0.10), in: RoundedRectangle(cornerRadius: 10))
            .accessibilityIdentifier("errorMessage")
    }
}

private extension View {
    func cardStyle() -> some View {
        background(Color.white.opacity(0.05), in: RoundedRectangle(cornerRadius: 18))
            .overlay(RoundedRectangle(cornerRadius: 18).stroke(Color.white.opacity(0.12), lineWidth: 1))
    }

    func fieldStyle() -> some View {
        padding(14).background(Color.white.opacity(0.08), in: RoundedRectangle(cornerRadius: 10))
            .overlay(RoundedRectangle(cornerRadius: 10).stroke(Color.white.opacity(0.15), lineWidth: 1))
    }
}
