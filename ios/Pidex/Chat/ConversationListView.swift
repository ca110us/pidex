import SwiftUI

/// Past conversations, newest first: one collection's, or every
/// collection's with questions put to all of them shown once. Each opens to
/// carry on.
struct ConversationListView: View {
    @Environment(AppModel.self) private var app
    let scope: AskScope
    /// Opens a conversation in place; without it, one opens as a page.
    var open: ((AskRoute) -> Void)?
    @State private var rows: [Row] = []
    @State private var loading = true
    @State private var error: String?

    struct Row: Identifiable {
        let id: String
        let title: String
        let updatedAt: Date
        /// The collection's name, or none when only one is listed.
        let place: String?
        let route: AskRoute
        /// What deleting removes: each collection's part on its server.
        let parts: [(workspace: Workspace, conversationID: String)]
    }

    var body: some View {
        List {
            if let error {
                Text(error).foregroundStyle(Theme.danger)
            }
            if rows.isEmpty && !loading {
                Text("No conversations yet.").foregroundStyle(.secondary)
            }
            ForEach(rows) { row in
                Group {
                    if let open {
                        Button { open(row.route) } label: { label(row) }
                            .tint(.primary)
                    } else {
                        NavigationLink { AskView(route: row.route) } label: { label(row) }
                    }
                }
                .swipeActions {
                    Button("Delete", role: .destructive) { Task { await delete(row) } }
                }
            }
        }
        .overlay { if loading { ProgressView() } }
        .navigationTitle("Conversations")
        .toolbar {
            ToolbarItem(placement: .primaryAction) {
                if let open {
                    Button { open(AskRoute(scope: scope)) } label: { Image(systemName: "square.and.pencil") }
                        .accessibilityLabel(Text("New conversation"))
                } else {
                    NavigationLink { AskView(route: AskRoute(scope: scope)) } label: { Image(systemName: "square.and.pencil") }
                        .accessibilityLabel(Text("New conversation"))
                }
            }
        }
        .task { await load() }
        .refreshable { await load() }
    }

    private func label(_ row: Row) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(verbatim: row.title).lineLimit(2)
            HStack(spacing: 4) {
                if let place = row.place {
                    Text(verbatim: place)
                    Text(verbatim: "·")
                }
                Text(row.updatedAt.formatted(.relative(presentation: .named)))
            }
            .font(.caption)
            .foregroundStyle(.secondary)
        }
    }

    private var workspaces: [Workspace] {
        switch scope {
        case .all: app.workspaces.items
        case .collection(let id): app.workspaces.items.filter { $0.id == id }
        }
    }

    private func load() async {
        var found: [String: [Pidex_V1_Conversation]] = [:]
        var failed: String?
        await withTaskGroup(of: (String, Result<[Pidex_V1_Conversation], Error>).self) { group in
            for w in workspaces {
                guard let api = app.api(for: w) else { continue }
                group.addTask {
                    do { return (w.id, .success(try await api.conversations())) } catch { return (w.id, .failure(error)) }
                }
            }
            for await (id, result) in group {
                switch result {
                case .success(let list): found[id] = list
                case .failure(let e): failed = e.localizedDescription
                }
            }
        }
        let byID = Dictionary(workspaces.map { ($0.id, $0) }, uniquingKeysWith: { a, _ in a })
        var out: [Row] = []
        var grouped = Set<String>()
        if scope == .all {
            for g in AskGroups.all() {
                let parts = g.conversations.compactMap { wid, cid -> (Workspace, Pidex_V1_Conversation)? in
                    guard let w = byID[wid], let c = found[wid]?.first(where: { $0.id == cid }) else { return nil }
                    return (w, c)
                }
                // Parts deleted elsewhere leave nothing to open.
                guard !parts.isEmpty else { continue }
                grouped.formUnion(parts.map(\.1.id))
                out.append(Row(id: g.id, title: g.title, updatedAt: parts.map(\.1.updatedAt.date).max() ?? g.updatedAt,
                               place: String(localized: "All collections"),
                               route: AskRoute(scope: .all, conversations: g.conversations, groupID: g.id),
                               parts: parts.map { ($0.0, $0.1.id) }))
            }
        }
        for w in workspaces {
            for c in found[w.id] ?? [] where !grouped.contains(c.id) {
                out.append(Row(id: c.id, title: c.title, updatedAt: c.updatedAt.date,
                               place: workspaces.count > 1 ? w.name : nil,
                               route: AskRoute(scope: .collection(w.id), conversations: [w.id: c.id]),
                               parts: [(w, c.id)]))
            }
        }
        rows = out.sorted { $0.updatedAt > $1.updatedAt }
        error = failed
        loading = false
    }

    private func delete(_ row: Row) async {
        do {
            for p in row.parts {
                try await app.api(for: p.workspace)?.deleteConversation(p.conversationID)
            }
            if row.route.groupID != nil { AskGroups.remove(row.id) }
            rows.removeAll { $0.id == row.id }
        } catch {
            self.error = error.localizedDescription
        }
    }
}
