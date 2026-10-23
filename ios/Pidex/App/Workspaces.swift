import Foundation

/// A team this device can shoot for. Each team has its own scenario
/// (config), subjects and photos, and its own token.
nonisolated struct Workspace: Codable, Hashable, Identifiable, Sendable {
    /// The team ID, or "" for a token added before the team was known.
    var id: String
    var name: String
    var token: String
}

/// The workspaces saved on this device and which one is in use. Pure value
/// logic; AppModel stores it in the Keychain because it holds tokens.
nonisolated struct WorkspaceList: Codable, Equatable, Sendable {
    private(set) var items: [Workspace] = []
    private(set) var activeID: String?

    var active: Workspace? {
        items.first { $0.id == activeID } ?? items.first
    }

    /// Adds a workspace, or replaces the one with the same team or token.
    mutating func upsert(_ w: Workspace, activate: Bool) {
        items.removeAll { $0.id == w.id || $0.token == w.token }
        items.append(w)
        if activate || activeID == nil {
            activeID = w.id
        }
    }

    mutating func activate(_ id: String) {
        if items.contains(where: { $0.id == id }) {
            activeID = id
        }
    }

    mutating func remove(_ id: String) {
        items.removeAll { $0.id == id }
        if activeID == id {
            activeID = items.first?.id
        }
    }

    /// Records the team ID and name the server reports for a token.
    mutating func update(token: String, id: String, name: String) {
        guard items.contains(where: { $0.token == token }) else { return }
        let wasActive = active?.token == token
        // An older token for the same team is replaced by this one.
        items.removeAll { $0.id == id && $0.token != token }
        let i = items.firstIndex { $0.token == token }!
        items[i].id = id
        items[i].name = name
        if wasActive {
            activeID = id
        }
    }
}
