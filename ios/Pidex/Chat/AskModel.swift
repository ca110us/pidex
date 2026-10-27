import Foundation
import Observation

/// Who a conversation asks: one collection, or all of them at once.
nonisolated enum AskScope: Hashable, Sendable {
    case collection(String)
    case all
}

/// A conversation to open: new, or carried on. `conversations` holds each
/// collection's conversation on the server, by workspace ID.
nonisolated struct AskRoute: Hashable, Identifiable, Sendable {
    var id = UUID()
    var scope: AskScope
    var conversations: [String: String] = [:]
    /// Several collections asked together are kept as one on this device.
    var groupID: String?
    var firstMessage: String?
}

/// A conversation with one collection or with all of them. Each collection
/// keeps its own conversation on its server; asking all of them sends every
/// message to each and shows their replies together.
@MainActor
@Observable
final class AskModel {
    struct Party {
        let workspace: Workspace
        let library: LibraryModel
        var conversationID: String?
    }

    /// One collection's reply to a message: on its way, made, or failed.
    struct Reply: Identifiable {
        var id: String { workspace.id }
        let workspace: Workspace
        let library: LibraryModel
        var message: Pidex_V1_ChatMessage?
        var error: String?
        var pending: Bool { message == nil && error == nil }
    }

    struct Turn: Identifiable {
        let id: String
        let text: String
        var replies: [Reply]
    }

    let scope: AskScope
    private(set) var parties: [Party]
    private(set) var turns: [Turn] = []
    private(set) var title = ""
    private(set) var loading = false
    private(set) var error: String?
    private var groupID: String?

    init(scope: AskScope, parties: [Party], groupID: String? = nil) {
        self.scope = scope
        self.parties = parties
        self.groupID = groupID
    }

    var busy: Bool { turns.last?.replies.contains { $0.pending } ?? false }
    var asksMany: Bool { parties.count > 1 }

    /// Loads the messages so far. Several collections' conversations are
    /// lined up by message, since one may have missed a message that failed.
    func load() async {
        let saved = parties.enumerated().filter { $0.element.conversationID != nil }
        guard turns.isEmpty, !saved.isEmpty else { return }
        loading = true
        defer { loading = false }
        var threads: [Int: [(text: String, reply: Pidex_V1_ChatMessage?)]] = [:]
        await withTaskGroup(of: (Int, Result<Pidex_V1_GetConversationResponse, Error>).self) { group in
            for (i, p) in saved {
                let api = p.library.api, id = p.conversationID!
                group.addTask {
                    do { return (i, .success(try await api.conversation(id))) } catch { return (i, .failure(error)) }
                }
            }
            for await (i, result) in group {
                switch result {
                case .success(let r):
                    if title.isEmpty { title = r.conversation.title }
                    threads[i] = Self.exchanges(r.messages)
                case .failure(let e):
                    error = e.localizedDescription
                }
            }
        }
        let longest = threads.values.max { $0.count < $1.count } ?? []
        var next = [Int: Int]()
        turns = longest.enumerated().map { n, exchange in
            Turn(id: "saved-\(n)", text: exchange.text, replies: parties.indices.compactMap { i in
                guard let thread = threads[i] else { return nil }
                let at = next[i, default: 0]
                guard at < thread.count, thread[at].text == exchange.text else { return nil }
                next[i] = at + 1
                return Reply(workspace: parties[i].workspace, library: parties[i].library,
                             message: thread[at].reply, error: thread[at].reply == nil ? String(localized: "No reply.") : nil)
            })
        }
        for p in parties {
            p.library.remember(turns.flatMap { $0.replies.flatMap { ($0.message?.subjects ?? []) + ($0.message?.missing ?? []) } })
        }
    }

    /// Sends a message to every collection asked, each building on its own
    /// conversation so far and told what the others replied last, so
    /// "their photos" after one named four cats is read by all as those.
    func send(_ text: String) async {
        let text = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, !busy, !parties.isEmpty else { return }
        error = nil
        let others = parties.indices.map(elsewhere(for:))
        let n = turns.count
        turns.append(Turn(id: UUID().uuidString, text: text, replies: parties.map {
            Reply(workspace: $0.workspace, library: $0.library)
        }))
        await withTaskGroup(of: (Int, Result<Pidex_V1_SendMessageResponse, Error>).self) { group in
            for (i, p) in parties.enumerated() {
                let api = p.library.api, id = p.conversationID, said = others[i]
                group.addTask {
                    do { return (i, .success(try await api.send(text, in: id, elsewhere: said))) } catch { return (i, .failure(error)) }
                }
            }
            for await (i, result) in group {
                switch result {
                case .success(let r):
                    parties[i].conversationID = r.conversation.id
                    if title.isEmpty { title = r.conversation.title }
                    turns[n].replies[i].message = r.reply
                    parties[i].library.remember(r.reply.subjects + r.reply.missing)
                case .failure(let e):
                    turns[n].replies[i].error = ScenarioText.explain(e)
                }
            }
        }
        if asksMany { saveGroup() }
    }

    /// What the other collections replied to the last few messages, oldest
    /// first: their answers, or whose photos they showed, as far back as a
    /// collection reads its own conversation. Said to a model, so in English.
    private func elsewhere(for i: Int) -> [Pidex_V1_ElsewhereReply] {
        guard asksMany else { return [] }
        return turns.suffix(Self.turnsRead).flatMap { turn in
            turn.replies.compactMap { r in
                guard r.workspace.id != parties[i].workspace.id, let m = r.message, let said = Self.said(m) else { return nil }
                var e = Pidex_V1_ElsewhereReply()
                e.request = turn.text
                e.collection = r.workspace.name
                e.reply = said
                return e
            }
        }
    }

    /// How many earlier exchanges the server reads a message with.
    static let turnsRead = 4

    static func said(_ m: Pidex_V1_ChatMessage) -> String? {
        if !m.text.isEmpty { return m.text }
        guard !m.photos.isEmpty else { return nil }
        let names = m.subjects.filter { s in m.photos.contains { $0.subjectID == s.id } }.map(\.displayName)
        return names.isEmpty ? "Showed \(m.photos.count) photos" : "Showed \(m.photos.count) photos of \(names.joined(separator: ", "))"
    }

    /// A message and the reply that follows it, for each message sent.
    private static func exchanges(_ messages: [Pidex_V1_ChatMessage]) -> [(text: String, reply: Pidex_V1_ChatMessage?)] {
        var out: [(text: String, reply: Pidex_V1_ChatMessage?)] = []
        for m in messages {
            if m.role == .user {
                out.append((m.text, nil))
            } else if !out.isEmpty, out[out.count - 1].reply == nil {
                out[out.count - 1].reply = m
            }
        }
        return out
    }

    private func saveGroup() {
        let id = groupID ?? UUID().uuidString
        groupID = id
        var conversations: [String: String] = [:]
        for p in parties {
            if let c = p.conversationID { conversations[p.workspace.id] = c }
        }
        AskGroups.save(AskGroup(id: id, title: title.isEmpty ? turns.first?.text ?? "" : title,
                                updatedAt: .now, conversations: conversations))
    }
}

/// A question put to several collections at once. Each collection keeps its
/// part on its own server; this device remembers which parts go together.
nonisolated struct AskGroup: Codable, Identifiable, Sendable {
    let id: String
    var title: String
    var updatedAt: Date
    var conversations: [String: String]
}

nonisolated enum AskGroups {
    private static let key = "askGroups"

    static func all() -> [AskGroup] {
        guard let data = UserDefaults.standard.data(forKey: key) else { return [] }
        return (try? JSONDecoder().decode([AskGroup].self, from: data)) ?? []
    }

    static func save(_ group: AskGroup) {
        var list = all().filter { $0.id != group.id }
        list.insert(group, at: 0)
        write(list)
    }

    static func remove(_ id: String) {
        write(all().filter { $0.id != id })
    }

    private static func write(_ list: [AskGroup]) {
        if let data = try? JSONEncoder().encode(list) {
            UserDefaults.standard.set(data, forKey: key)
        }
    }
}

extension AppModel {
    /// The collections a scope asks, each with a library for showing its
    /// photos; `library` is reused for the collection it belongs to.
    func askModel(for route: AskRoute, library: LibraryModel? = nil) -> AskModel {
        let workspaces: [Workspace]
        switch route.scope {
        case .collection(let id): workspaces = self.workspaces.items.filter { $0.id == id }
        case .all: workspaces = self.workspaces.items
        }
        let parties = workspaces.compactMap { w -> AskModel.Party? in
            guard let api = api(for: w) else { return nil }
            let lib = library.flatMap { $0.api === api ? $0 : nil } ?? LibraryModel(api: api)
            return AskModel.Party(workspace: w, library: lib, conversationID: route.conversations[w.id])
        }
        return AskModel(scope: route.scope, parties: parties, groupID: route.groupID)
    }
}
