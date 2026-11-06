import DevicePanelCore
import SwiftUI

struct RoomControlView: View {
    @ObservedObject var controller: RoomController
    @State private var showingSettings = false
    @State private var showingPairing = false
    @State private var temperatureDraft = 50.0
    @State private var isEditingTemperature = false
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass

    var body: some View {
        NavigationStack {
            ZStack {
                AppBackground()

                ScrollView {
                    VStack(spacing: 18) {
                        statusSection

                        if controller.targets.count > 1 {
                            TargetPicker(
                                targets: controller.targets,
                                selectedID: controller.selectedTarget?.id
                            ) { id in
                                Task { await controller.select(id) }
                            }
                        }

                        if let state = controller.roomState {
                            let features = controller.features

                            PowerControl(
                                name: controller.targetName,
                                isOn: state.on,
                                isBusy: controller.isPowerCommandInFlight
                            ) {
                                Task { await controller.setPower(!state.on) }
                            }

                            if features.color {
                                ColorSection(selectedMode: state.colorMode) { color in
                                    Task { await controller.setColor(color) }
                                }
                            }

                            if features.brightness {
                                BrightnessSection(
                                    brightness: state.brightness,
                                    wideLayout: horizontalSizeClass == .regular
                                ) { value in
                                    Task { await controller.setBrightness(value) }
                                }
                            }

                            if features.colorTemperature {
                                TemperatureSection(
                                    value: $temperatureDraft,
                                    isEditing: $isEditingTemperature
                                ) { value in
                                    Task { await controller.setColorTemperature(Int(value.rounded())) }
                                }
                            }
                        } else if controller.connection == .loading {
                            ProgressView("Loading \(controller.targetName)")
                                .tint(.white)
                                .padding(32)
                        }

                        if let error = controller.lastError {
                            ErrorBanner(
                                message: error,
                                canRetryCommand: controller.canRetryLastCommand,
                                retry: {
                                    Task {
                                        if controller.canRetryLastCommand {
                                            await controller.retryLastCommand()
                                        } else {
                                            await controller.refresh()
                                        }
                                    }
                                },
                                dismiss: controller.clearError
                            )
                        }
                    }
                    .frame(maxWidth: 760)
                    .padding(.horizontal, 18)
                    .padding(.vertical, 12)
                    .frame(maxWidth: .infinity)
                }
                .refreshable {
                    await controller.refresh()
                }
            }
            .navigationTitle(controller.targetName)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Add Matter Device", systemImage: "plus") {
                        showingPairing = true
                    }
                    .accessibilityIdentifier("addMatterDeviceButton")
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Refresh", systemImage: "arrow.clockwise") {
                        Task { await controller.refresh() }
                    }
                    .accessibilityIdentifier("refreshButton")
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Settings", systemImage: "gearshape") {
                        showingSettings = true
                    }
                }
            }
            .sheet(isPresented: $showingSettings) {
                SettingsView(controller: controller)
            }
            .sheet(isPresented: $showingPairing) {
                MatterPairingView(controller: controller)
            }
            .onAppear(perform: syncTemperatureDraft)
            .onChange(of: controller.roomState?.colorTemperaturePct) { _, _ in
                guard !isEditingTemperature else { return }
                syncTemperatureDraft()
            }
            .sensoryFeedback(.success, trigger: controller.connection) { oldValue, newValue in
                oldValue == .applying && newValue == .connected
            }
        }
    }

    private var statusSection: some View {
        ConnectionStatusView(
            phase: controller.connection,
            observedAt: controller.roomState?.observedAt
        )
    }

    private func syncTemperatureDraft() {
        if let value = controller.roomState?.colorTemperaturePct {
            temperatureDraft = Double(value)
        }
    }
}
