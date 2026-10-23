import SwiftUI

/// Server address and tokens. Adding a token adds its team as a workspace
/// and switches to it.
struct SettingsView: View {
    @Environment(AppModel.self) private var app
    @Environment(\.dismiss) private var dismiss
    @State private var serverURL = ""
    @State private var token = ""
    @State private var checking = false
    @State private var message: String?

    var body: some View {
        Form {
            if app.active == nil {
                // First run: what this is, before how to connect it.
                Section {
                    BrandHeader()
                }
                .listRowBackground(Color.clear)
            }
            Section {
                TextField(String("http://192.168.1.10:8080"), text: $serverURL)
                    .textContentType(.URL)
                    .keyboardType(.URL)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                SecureField("API token (cbk_…)", text: $token)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
            } header: {
                Text("Server")
            } footer: {
                if app.active == nil {
                    Text("Ask your team admin for a token. It is stored in the Keychain on this device only.")
                } else {
                    Text("Add another team's token to add its collection. Tokens are stored in the Keychain on this device only.")
                }
            }
            Section {
                Button { Task { await save() } } label: {
                    if checking {
                        Text("Checking…")
                    } else if app.active == nil {
                        Text("Connect")
                    } else {
                        Text("Add collection")
                    }
                }
                    .disabled(checking || serverURL.isEmpty || token.isEmpty)
                if let message {
                    Text(message).font(.footnote).foregroundStyle(.secondary)
                }
            }
        }
        // On first run the brand header heads the page.
        .navigationTitle(app.active == nil ? Text(verbatim: "") : Text("Server"))
        .onAppear {
            serverURL = app.serverURL
        }
    }

    private func save() async {
        guard let url = URL(string: serverURL.trimmingCharacters(in: .whitespaces)), url.scheme != nil, url.host() != nil else {
            message = String(localized: "Enter a full URL, for example http://192.168.1.10:8080")
            return
        }
        checking = true
        defer { checking = false }
        do {
            try await app.connect(serverURL: url.absoluteString, token: token.trimmingCharacters(in: .whitespacesAndNewlines))
            token = ""
            message = String(localized: "Connected to “\(app.active?.name ?? "")”.")
            dismiss()
        } catch {
            message = String(localized: "Could not connect: \(error.localizedDescription)")
        }
    }
}
