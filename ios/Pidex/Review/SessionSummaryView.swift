import SwiftUI

/// After the shoot: one bar for how far the photos got, where they went,
/// and the few that need a person. Confirming a suggestion is one tap; any
/// subject can be renamed here, as one named from its look often is.
struct SessionSummaryView: View {
    let batch: UploadBatch
    let onDone: () -> Void
    @Environment(AppModel.self) private var app
    @State private var busy: Set<String> = []
    @State private var error: String?
    /// The photo being filed by hand: a wrong suggestion, or none.
    @State private var picking: Picking?
    @State private var renaming: Pidex_V1_Subject?
    @State private var newName = ""
    @State private var library: LibraryModel

    struct Picking: Identifiable {
        let ids: [String]
        var id: String { ids.joined(separator: ",") }
    }

    init(batch: UploadBatch, onDone: @escaping () -> Void) {
        self.batch = batch
        self.onDone = onDone
        _library = State(initialValue: LibraryModel(api: batch.api))
    }

    private var pending: Int { app.uploadStatus.pendingBySession[batch.sessionID.uuidString.lowercased()] ?? 0 }

    var body: some View {
        List {
            Section {
                BatchProgress(batch: batch, pending: pending, paused: app.uploadStatus.pausedReason)
            }
            if let summary = batch.summary, !summary.groups.isEmpty {
                Section("Filed") {
                    ForEach(summary.groups, id: \.subject.id) { g in
                        HStack {
                            Text(verbatim: g.subject.displayName)
                            Spacer()
                            Text(verbatim: "\(g.photoCount)").monospacedDigit().foregroundStyle(.secondary)
                            Button {
                                newName = g.subject.displayName
                                renaming = g.subject
                            } label: {
                                Image(systemName: "pencil")
                            }
                            .buttonStyle(.borderless)
                            .accessibilityLabel(Text("Rename \(g.subject.displayName)"))
                        }
                    }
                }
            }
            if let summary = batch.summary, !summary.needsReview.isEmpty {
                // Photos sent together show one subject: asked once per
                // subject suggested, not once per photo.
                Section("Needs you") {
                    ForEach(Self.groups(summary.needsReview), id: \.suggested) { g in
                        reviewRow(g.photos, suggested: summary.suggestedSubjects.first { $0.id == g.suggested })
                    }
                }
            }
            if let error = error ?? batch.error {
                Text(error).foregroundStyle(Theme.danger)
            }
        }
        .navigationTitle(Text(verbatim: batch.collection))
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .confirmationAction) {
                Button("Done", action: onDone)
            }
        }
        .alert("Rename", isPresented: Binding(get: { renaming != nil }, set: { if !$0 { renaming = nil } })) {
            TextField("Name", text: $newName)
            Button("Cancel", role: .cancel) {}
            // The alert clears `renaming` as it closes; keep it first.
            Button("Save") { [renaming] in Task { await rename(renaming) } }
        }
        .sheet(item: $picking) { p in
            NavigationStack {
                SubjectPicker(model: library) { subject in
                    picking = nil
                    Task { await confirm(p.ids, subject: subject.id) }
                }
            }
        }
        .refreshable { await batch.refresh() }
        .task { await batch.refresh() }
    }

    /// Photos needing a person, by the subject suggested for them; those
    /// with none come last, together.
    static func groups(_ photos: [Pidex_V1_Photo]) -> [(suggested: String, photos: [Pidex_V1_Photo])] {
        var order: [String] = []
        var by: [String: [Pidex_V1_Photo]] = [:]
        for p in photos {
            if by[p.suggestedSubjectID] == nil { order.append(p.suggestedSubjectID) }
            by[p.suggestedSubjectID, default: []].append(p)
        }
        order.sort { a, b in !a.isEmpty && b.isEmpty }
        return order.map { ($0, by[$0]!) }
    }

    @ViewBuilder
    private func reviewRow(_ photos: [Pidex_V1_Photo], suggested: Pidex_V1_Subject?) -> some View {
        let ids = photos.map(\.id)
        let working = ids.contains { busy.contains($0) }
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 4) {
                ForEach(photos.prefix(4), id: \.id) { p in
                    Thumbnail(photo: p)
                        .frame(width: 56, height: 56)
                        .clipShape(RoundedRectangle(cornerRadius: 6))
                }
                if photos.count > 4 {
                    Text(verbatim: "+\(photos.count - 4)").font(.footnote.weight(.semibold)).foregroundStyle(.secondary)
                        .padding(.leading, 4)
                }
            }
            HStack {
                if let s = suggested {
                    VStack(alignment: .leading) {
                        Text(photos.count == 1 ? "Is this \(s.displayName)?" : "Are these \(photos.count) \(s.displayName)?")
                        Text("Confidence \(Int((photos.map(\.assignmentConfidence).max() ?? 0) * 100))%")
                            .font(.caption).foregroundStyle(.secondary)
                    }
                    Spacer()
                    Button("Yes") { Task { await confirm(ids, subject: s.id) } }
                        .buttonStyle(.ink)
                        .disabled(working)
                        .accessibilityIdentifier("confirmGroup")
                    Button("Other") { picking = Picking(ids: ids) }
                        .buttonStyle(.surface)
                        .disabled(working)
                } else {
                    Text(photos.count == 1 ? "Not filed" : "\(photos.count) photos not filed")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                    Spacer()
                    Button("File…") { picking = Picking(ids: ids) }
                        .buttonStyle(.surface)
                        .disabled(working)
                }
            }
        }
        .padding(.vertical, 4)
    }

    private func confirm(_ photoIDs: [String], subject: String) async {
        busy.formUnion(photoIDs)
        defer { busy.subtract(photoIDs) }
        do {
            _ = try await batch.api.assign(photoIDs: photoIDs, to: subject)
            await batch.refresh()
        } catch {
            self.error = error.localizedDescription
        }
    }

    private func rename(_ subject: Pidex_V1_Subject?) async {
        guard let s = subject else { return }
        let name = newName.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !name.isEmpty, name != s.displayName else { return }
        do {
            let renamed = try await batch.api.renameSubject(s.id, to: name)
            batch.renamed(renamed)
            await batch.refresh()
        } catch {
            self.error = error.localizedDescription
        }
    }
}

/// One bar: uploading is its first half, filing the second.
struct BatchProgress: View {
    let batch: UploadBatch
    let pending: Int
    var paused: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text(status).font(.headline)
                Spacer()
                if batch.phase(pending: pending) == .done {
                    Image(systemName: "checkmark.circle.fill").foregroundStyle(Theme.ink)
                }
            }
            ProgressView(value: batch.progress(pending: pending))
                .tint(Theme.ink)
                .animation(.default, value: batch.progress(pending: pending))
            if let paused, batch.phase(pending: pending) != .done {
                Text("Uploads paused. \(paused)").font(.caption).foregroundStyle(Theme.attention)
            } else if batch.needsReview > 0 {
                Text("\(batch.needsReview) need you").font(.caption).foregroundStyle(Theme.attention)
            }
        }
        .padding(.vertical, 4)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("batchProgress")
    }

    private var status: String {
        switch batch.phase(pending: pending) {
        case .uploading(let n): String(localized: "Uploading \(n) of \(batch.expected)")
        case .filing(let n): String(localized: "Filing \(n) of \(batch.expected)")
        case .done: String(localized: "All \(batch.expected) filed")
        }
    }
}

/// The batch in progress, minimized: a small floating capsule with a ring
/// for how far it got, out of the way of the page. Tapping it opens the
/// details. Once everything is filed and nothing needs a person, it says so
/// in a toast and goes.
struct BatchPill: View {
    @Environment(AppModel.self) private var app

    var body: some View {
        if let batch = app.batch {
            let pending = app.uploadStatus.pendingBySession[batch.sessionID.uuidString.lowercased()] ?? 0
            let phase = batch.phase(pending: pending)
            let settled = phase == .done && batch.needsReview == 0
            Button { app.showingBatch = true } label: {
                HStack(spacing: 10) {
                    ZStack {
                        Circle().stroke(Theme.hairline, lineWidth: 3)
                        Circle()
                            .trim(from: 0, to: batch.progress(pending: pending))
                            .stroke(Theme.ink, style: StrokeStyle(lineWidth: 3, lineCap: .round))
                            .rotationEffect(.degrees(-90))
                        if phase == .done {
                            Image(systemName: "checkmark").font(.system(size: 9, weight: .bold)).foregroundStyle(Theme.ink)
                        }
                    }
                    .frame(width: 20, height: 20)
                    .animation(.default, value: batch.progress(pending: pending))
                    Text(short(phase)).font(.footnote.weight(.semibold)).monospacedDigit()
                    if batch.needsReview > 0 {
                        Text(verbatim: "\(batch.needsReview)")
                            .font(.caption2.weight(.bold))
                            .foregroundStyle(.white)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 2)
                            .background(Theme.attention, in: Capsule())
                            .accessibilityLabel(Text("\(batch.needsReview) need you"))
                    }
                }
                .padding(.vertical, 10)
                .padding(.horizontal, 14)
                .glassSurface(Capsule())
                .shadow(color: .black.opacity(0.12), radius: 10, y: 3)
            }
            .buttonStyle(.plain)
            .transition(.move(edge: .bottom).combined(with: .opacity))
            .accessibilityIdentifier("batchPill")
            .task(id: settled) {
                // Done and nothing to check: a toast says so, and it goes.
                guard settled, !app.showingCamera, !app.showingBatch else { return }
                try? await Task.sleep(for: .seconds(1.5))
                guard app.batch === batch, !app.showingCamera, !app.showingBatch else { return }
                app.show(String(localized: "\(batch.expected) photos filed in “\(batch.collection)”"))
                app.dismissBatch()
            }
        }
    }

    private func short(_ phase: UploadBatch.Phase) -> String {
        switch phase {
        case .uploading(let n): String(localized: "Uploading \(n) of \(app.batch?.expected ?? 0)")
        case .filing(let n): String(localized: "Filing \(n) of \(app.batch?.expected ?? 0)")
        case .done: String(localized: "All \(app.batch?.expected ?? 0) filed")
        }
    }
}
