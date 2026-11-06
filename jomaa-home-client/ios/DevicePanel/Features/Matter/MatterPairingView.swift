import DevicePanelCore
import SwiftUI

/// Pairs a Matter device with the hub and lists the ones already paired.
struct MatterPairingView: View {
    @ObservedObject var controller: RoomController
    @Environment(\.dismiss) private var dismiss
    @State private var code = ""
    @State private var networkOnly = false

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("Setup code", text: $code)
                        .textInputAutocapitalization(.characters)
                        .autocorrectionDisabled()
                        .disabled(controller.matter.isPairing)
                        .accessibilityIdentifier("setupCodeField")
                    Toggle("Already on my network", isOn: $networkOnly)
                        .disabled(controller.matter.isPairing)
                    Button(action: pair) {
                        HStack {
                            if controller.matter.isPairing {
                                ProgressView()
                            }
                            Text(controller.matter.isPairing ? "Pairing…" : "Pair")
                        }
                    }
                    .disabled(!canPair)
                    .accessibilityIdentifier("pairButton")
                } header: {
                    Text("Add a device")
                } footer: {
                    Text("Enter the 11-digit code or the MT: code printed by the QR label. Pairing can take a few minutes.")
                }

                if let nodeID = controller.matter.pairedNodeID {
                    Section {
                        Label("Paired as node \(String(nodeID))", systemImage: "checkmark.circle.fill")
                            .foregroundStyle(AppTheme.success)
                    }
                }

                if let error = controller.matter.error {
                    Section {
                        Label(error, systemImage: "exclamationmark.triangle.fill")
                            .foregroundStyle(AppTheme.failure)
                            .accessibilityIdentifier("matterError")
                    }
                }

                Section("Paired devices") {
                    if controller.matter.isLoading {
                        ProgressView()
                    } else if controller.matter.nodes.isEmpty {
                        Text("No Matter devices paired yet.")
                            .foregroundStyle(.secondary)
                    } else {
                        ForEach(controller.matter.nodes) { node in
                            LabeledContent(node.displayName, value: node.available ? "Online" : "Offline")
                        }
                    }
                }
            }
            .navigationTitle("Matter Devices")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                        .disabled(controller.matter.isPairing)
                }
            }
            .task {
                await controller.loadMatterNodes()
            }
            .onDisappear {
                controller.clearPairingResult()
            }
        }
        .interactiveDismissDisabled(controller.matter.isPairing)
    }

    private var canPair: Bool {
        !code.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && !controller.matter.isPairing
    }

    private func pair() {
        Task { await controller.pair(code: code, networkOnly: networkOnly) }
    }
}
