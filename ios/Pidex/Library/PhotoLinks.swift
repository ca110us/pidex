import Foundation

/// Fresh links to photos' images. A photo from the phone's copy carries no
/// links, and links from a search expire after a quarter of an hour; this
/// asks the server for them, the photos asked for at about the same time in
/// one request.
actor PhotoLinks {
    static let shared = PhotoLinks()

    /// The collection each photo shown is in.
    private var owners: [String: APIClient] = [:]
    private var links: [String: (links: Pidex_V1_PhotoLinks, until: Date)] = [:]
    private var pending: [ObjectIdentifier: Batch] = [:]

    private struct Batch {
        let api: APIClient
        var ids: Set<String> = []
        var done: Task<Void, Never>?
    }

    /// Notes which collection photos are in, before they are shown.
    func know(_ ids: [String], in api: APIClient) {
        for id in ids { owners[id] = api }
    }

    /// A link to a photo's original, or its thumbnail (the original until it
    /// has one); nil for a photo not known or not found.
    func link(for id: String, original: Bool) async -> URL? {
        if links[id].map({ $0.until < .now }) ?? true {
            guard let api = owners[id] else { return nil }
            await fetch(id, api)
        }
        guard let l = links[id]?.links else { return nil }
        return URL(string: original || l.thumbnailURL.isEmpty ? l.viewURL : l.thumbnailURL)
    }

    private func fetch(_ id: String, _ api: APIClient) async {
        let key = ObjectIdentifier(api)
        if pending[key] == nil {
            var batch = Batch(api: api)
            batch.done = Task {
                // Grids ask for a screenful at once: wait for the rest.
                try? await Task.sleep(for: .milliseconds(30))
                await self.flush(key)
            }
            pending[key] = batch
        }
        pending[key]?.ids.insert(id)
        await pending[key]?.done?.value
    }

    private func flush(_ key: ObjectIdentifier) async {
        guard let batch = pending.removeValue(forKey: key) else { return }
        let ids = Array(batch.ids)
        // Signed for 15 minutes: trusted for 10.
        let until = Date.now.addingTimeInterval(600)
        for start in stride(from: 0, to: ids.count, by: 200) {
            guard let got = try? await batch.api.photoLinks(Array(ids[start ..< min(start + 200, ids.count)])) else { continue }
            for (id, l) in got { links[id] = (l, until) }
        }
    }
}
