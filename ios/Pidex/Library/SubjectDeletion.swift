import SwiftUI

extension View {
    /// Asks how to delete a subject, whose photos are either kept, filed
    /// under nothing, or deleted with it; then deletes it.
    func subjectDeletion(_ subject: Binding<Pidex_V1_Subject?>, api: APIClient, onDeleted: @escaping (Pidex_V1_Subject) -> Void) -> some View {
        modifier(SubjectDeletion(subject: subject, api: api, onDeleted: onDeleted))
    }
}

private struct SubjectDeletion: ViewModifier {
    @Binding var subject: Pidex_V1_Subject?
    let api: APIClient
    let onDeleted: (Pidex_V1_Subject) -> Void
    @Environment(AppModel.self) private var app

    func body(content: Content) -> some View {
        content.confirmationDialog(
            Text("Delete “\(subject?.displayName ?? "")”?"),
            isPresented: Binding(get: { subject != nil }, set: { if !$0 { subject = nil } }),
            titleVisibility: .visible,
            presenting: subject
        ) { s in
            if s.photoCount == 0 {
                Button("Delete", role: .destructive) { Task { await delete(s, withPhotos: false) } }
                    .accessibilityIdentifier("deleteSubjectKeepPhotos")
            } else {
                Button("Delete it, keep its \(s.photoCount) photos") { Task { await delete(s, withPhotos: false) } }
                    .accessibilityIdentifier("deleteSubjectKeepPhotos")
                Button("Delete it and its \(s.photoCount) photos", role: .destructive) { Task { await delete(s, withPhotos: true) } }
                    .accessibilityIdentifier("deleteSubjectWithPhotos")
            }
        } message: { s in
            Text("Kept photos stay in the collection, filed under nothing, and can be filed again. Deleted photos cannot be recovered.")
        }
    }

    private func delete(_ s: Pidex_V1_Subject, withPhotos: Bool) async {
        do {
            let r = try await api.deleteSubject(s.id, withPhotos: withPhotos)
            onDeleted(s)
            app.show(withPhotos
                     ? String(localized: "Deleted “\(s.displayName)” and \(r.deleted) photos")
                     : String(localized: "Deleted “\(s.displayName)”; \(r.unfiled) photos kept"))
        } catch {
            app.show(error.localizedDescription, tone: .problem)
        }
    }
}
