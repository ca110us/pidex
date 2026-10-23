import SwiftUI

/// The active workspace's scenario, its version history, and the way to
/// change it.
struct ScenarioView: View {
    let api: APIClient

    @State private var config: ScenarioConfig?
    @State private var versions: [Pidex_V1_ConfigVersion] = []
    @State private var canChange = false
    @Environment(AppModel.self) private var app
    @State private var error: String?
    @State private var restoring: Pidex_V1_ConfigVersion?
    @State private var editing = false

    var body: some View {
        List {
            if let error {
                Section {
                    Text(error).foregroundStyle(Theme.danger)
                }
            }
            if let config {
                ConfigSections(config: config)
            } else if error == nil {
                ProgressView()
            }
            if versions.count > 1 {
                Section {
                    ForEach(versions, id: \.version) { v in
                        HStack {
                            VStack(alignment: .leading) {
                                Text("Version \(v.version)")
                                Text(v.createdAt.date.formatted(date: .abbreviated, time: .shortened))
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                            Spacer()
                            if v.active {
                                Text("In use").foregroundStyle(.secondary)
                            } else if canChange {
                                Button("Restore") { restoring = v }
                                    .buttonStyle(.borderless)
                            }
                        }
                    }
                } header: {
                    Text("History")
                } footer: {
                    Text("Restoring publishes a copy of that version as the newest one. Photos keep the version they were filed with.")
                }
            }
        }
        .navigationTitle("Scenario")
        .toolbar {
            Button { editing = true } label: {
                Label("Setup conversation", systemImage: "bubble.left.and.text.bubble.right")
            }
            .accessibilityIdentifier("setupConversation")
        }
        .navigationDestination(isPresented: $editing) {
            SetupChatView(mode: .edit(name: config?.name ?? ""), api: api) { done in
                app.show(done)
                editing = false
                Task { await load() }
            }
        }
        .confirmationDialog("Restore version \(restoring?.version ?? 0)?", isPresented: Binding(get: { restoring != nil }, set: { if !$0 { restoring = nil } }), titleVisibility: .visible) {
            Button("Restore") {
                if let v = restoring { Task { await restore(v.version) } }
                restoring = nil
            }
            Button("Cancel", role: .cancel) { restoring = nil }
        }
        .task { await load() }
        .refreshable { await load() }
    }

    private func load() async {
        do {
            async let active = api.activeConfig()
            async let history = api.configVersions()
            async let team = api.team()
            config = try ScenarioConfig(json: try await active.configJson)
            versions = try await history
            canChange = ["owner", "admin"].contains(try await team.role)
            error = nil
        } catch {
            self.error = ScenarioText.explain(error)
        }
    }

    private func restore(_ version: Int32) async {
        do {
            let v = try await api.restoreConfig(version: version)
            app.show(String(localized: "Restored version \(version) as version \(v)."))
            await load()
        } catch {
            self.error = ScenarioText.explain(error)
        }
    }
}
