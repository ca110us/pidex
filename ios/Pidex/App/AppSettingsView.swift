import SwiftUI

/// App settings, opened from the collections menu: the server and tokens
/// (each token adds a collection), and the app's language.
struct AppSettingsView: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.openURL) private var openURL

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
            .toolbar {
                Button("Done") { dismiss() }
            }
        }
    }

    /// The language the app is shown in, named in that language.
    static var appLanguage: String {
        let id = Bundle.main.preferredLocalizations.first ?? "en"
        return Locale(identifier: id).localizedString(forIdentifier: id) ?? id
    }
}
