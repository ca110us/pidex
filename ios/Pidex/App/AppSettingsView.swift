import SwiftUI

/// App settings, opened from the collections menu: the server and tokens
/// (each token adds a collection), what is kept on the phone, and the app's
/// language.
struct AppSettingsView: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.openURL) private var openURL
    @AppStorage(OriginalsBudget.key) private var budget = OriginalsBudget.twoGigabytes.rawValue
    @State private var used: (thumbnails: Int64, originals: Int64)?
    @State private var confirmingClear = false

    var body: some View {
        NavigationStack {
            List {
                Section {
                    HStack(spacing: 14) {
                        PidexIcon(size: 56)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(verbatim: Brand.name).font(.title3.bold())
                            Text(Brand.tagline).font(.subheadline).foregroundStyle(.secondary)
                            Text(Brand.version).font(.caption).foregroundStyle(.tertiary)
                        }
                    }
                    .padding(.vertical, 4)
                    .accessibilityElement(children: .combine)
                }
                Section {
                    NavigationLink("Server and tokens") { SettingsView() }
                } footer: {
                    Text("Adding a team's token adds its collection to this iPhone.")
                }
                Section {
                    Picker("Originals kept", selection: $budget) {
                        ForEach(OriginalsBudget.allCases) { b in
                            if b == .none {
                                Text("None").tag(b.rawValue)
                            } else {
                                Text(verbatim: b.bytes.formatted(.byteCount(style: .file))).tag(b.rawValue)
                            }
                        }
                    }
                    if let used {
                        LabeledContent("Originals", value: used.originals.formatted(.byteCount(style: .file)))
                        LabeledContent("Thumbnails", value: used.thumbnails.formatted(.byteCount(style: .file)))
                    }
                    Button("Remove photos kept on this iPhone", role: .destructive) { confirmingClear = true }
                } header: {
                    Text("Storage on this iPhone")
                } footer: {
                    Text("Photos you take, and photos you open in full, stay on this iPhone up to this size, so they open at once and offline; the least recently seen go first. High-privacy collections keep none. Thumbnails take up to 1 GB.")
                }
                Section {
                    // iOS keeps a per-app language; this opens Pidex's
                    // page in Settings, where it is changed.
                    Button {
                        if let url = URL(string: UIApplication.openSettingsURLString) { openURL(url) }
                    } label: {
                        LabeledContent("Language", value: Self.appLanguage)
                    }
                    .foregroundStyle(.primary)
                } footer: {
                    Text("The app follows your iPhone's language unless you pick one for Pidex in Settings. Scenario names and tags stay in the language they were written in.")
                }
            }
            .navigationTitle("Settings")
            .navigationBarTitleDisplayMode(.inline)
            .task(id: budget) {
                // Smaller than what is kept: the least recently seen go now.
                await Task.detached(priority: .utility) { DiskImages.originals.trim() }.value
                await measure()
            }
            .confirmationDialog("Remove photos kept on this iPhone?", isPresented: $confirmingClear, titleVisibility: .visible) {
                Button("Remove", role: .destructive) {
                    Task {
                        await Task.detached(priority: .utility) {
                            DiskImages.originals.removeAll()
                            DiskImages.thumbnails.removeAll()
                        }.value
                        await measure()
                    }
                }
            } message: {
                Text("They stay on the server and are fetched again when shown.")
            }
            .toolbar {
                Button("Done") { dismiss() }
            }
        }
    }

    private func measure() async {
        used = await Task.detached(priority: .utility) {
            (DiskImages.thumbnails.size, DiskImages.originals.size)
        }.value
    }

    /// The language the app is shown in, named in that language.
    static var appLanguage: String {
        let id = Bundle.main.preferredLocalizations.first ?? "en"
        return Locale(identifier: id).localizedString(forIdentifier: id) ?? id
    }
}
