import SwiftUI

/// Deleting a collection for good, said plainly before it happens: every
/// photo, subject and conversation goes, for everyone in it, and none of it
/// can be recovered. Typing its name turns the button on.
struct DeleteCollectionView: View {
    let workspace: Workspace
    let api: APIClient
    let photoCount: Int
    /// Photos still uploading to it from this phone; deleting waits for them.
    var uploading = 0
    let onDeleted: () -> Void
    @State private var typed = ""
    @State private var working = false
    @State private var error: String?
    @Environment(\.dismiss) private var dismiss

    private var confirmed: Bool { typed.trimmingCharacters(in: .whitespaces) == workspace.name }

    var body: some View {
        Form {
            Section {
                VStack(alignment: .leading, spacing: 12) {
                    Label("This cannot be undone", systemImage: "exclamationmark.triangle.fill")
                        .font(.headline)
                        .foregroundStyle(Theme.danger)
                    Text("Deleting “\(workspace.name)” removes all \(photoCount) photos, every subject filed in it, its conversations, its setup and its history, for everyone who uses it. Nothing can be recovered afterwards, not even by support.")
                    Text("To keep the photos, export or save them before deleting.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
                .padding(.vertical, 4)
            }
            Section {
                TextField(text: $typed) { Text(verbatim: workspace.name) }
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .accessibilityIdentifier("confirmDeleteName")
            } header: {
                Text("Type the collection's name to confirm")
            }
            if uploading > 0 {
                Section {
                    Text("\(uploading) photos are still uploading to this collection. Delete it once they are done, so none are left waiting on this phone.")
                        .foregroundStyle(Theme.attention)
                }
            }
            if let error {
                Section { Text(error).foregroundStyle(Theme.danger) }
            }
            Section {
                Button(role: .destructive) {
                    Task { await delete() }
                } label: {
                    HStack {
                        Spacer()
                        if working { ProgressView() } else { Text("Delete for good").bold() }
                        Spacer()
                    }
                }
                .disabled(!confirmed || working || uploading > 0)
                .accessibilityIdentifier("confirmDelete")
            }
        }
        .navigationTitle("Delete collection")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .cancellationAction) {
                Button("Cancel") { dismiss() }.disabled(working)
            }
        }
        .interactiveDismissDisabled(working)
    }

    private func delete() async {
        working = true
        defer { working = false }
        do {
            _ = try await api.deleteTeam(confirmName: workspace.name)
            onDeleted()
        } catch {
            self.error = error.localizedDescription
        }
    }
}
