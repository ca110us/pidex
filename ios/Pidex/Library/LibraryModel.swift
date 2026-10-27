import Foundation
import Observation
import SwiftProtobuf

/// A collection's photos as browsed: the newest, or those a picked tag
/// filters to; editing the filters searches again without a model call.
/// Photos come from the phone's copy of the collection, synced as it opens,
/// whenever the phone can find them as the server would (all of them, or
/// those with picked tags, a place, days, subjects): at once, and offline
/// too. A search by meaning goes to the server; offline, it is matched on
/// the phone by the words it knows.
@MainActor
@Observable
final class LibraryModel {
    let api: APIClient
    let local: LocalLibrary?
    /// The server could not be reached: what is shown is the phone's copy.
    private(set) var offline = false
    /// What is shown came from the phone's copy, so more come from it too.
    private var fromLocal = false
    /// A search the server should have read was matched on the phone,
    /// offline: by the names and words it knows, not by meaning.
    private(set) var matchedOffline = false
    private(set) var config: ScenarioConfig?
    private(set) var filters: Pidex_V1_SearchFilters?
    private(set) var photos: [Pidex_V1_Photo] = []
    private(set) var subjects: [Pidex_V1_Subject] = []
    private(set) var missing: [Pidex_V1_Subject] = []
    private(set) var subjectNotFound = false
    /// How many photos match in all; `photos` holds those loaded so far.
    private(set) var total = 0
    /// With tags picked: how many of the matches have each tag.
    private(set) var tagCounts: [String: Int] = [:]
    /// The places the collection's photos were taken in, most first, as
    /// counted before any place or tag was picked.
    private(set) var places: [Pidex_V1_PlaceCount] = []
    private var loadingMore = false
    private var known: [String: Pidex_V1_Subject] = [:]
    private(set) var working = false
    private(set) var error: String?

    init(api: APIClient) {
        self.api = api
        local = LocalLibrary.for(api)
    }

    var isFiltered: Bool {
        filters.map(Self.narrows) ?? false
    }

    private static func narrows(_ f: Pidex_V1_SearchFilters) -> Bool {
        f.mode == .missing || !f.subjectQuery.isEmpty || !f.subjectIds.isEmpty || !f.tags.isEmpty
            || f.hasFrom || f.hasTo || !f.text.isEmpty || !f.roles.isEmpty || !f.place.isEmpty
    }

    /// Loads the scenario and the newest photos; `force` refreshes them
    /// unless a search is showing.
    func load(force: Bool = false) async {
        if config == nil || force, let r = await remembered("config", { try await self.api.activeConfig() }) {
            config = try? ScenarioConfig(json: r.configJson)
        }
        guard (filters == nil && photos.isEmpty) || (force && !isFiltered) else { return }
        guard let local else {
            // Counted too, for the row of places to narrow by.
            await run { try await self.api.searchPhotos(filters: .init(), tagCounts: true) }
            return
        }
        // What the phone has, at once; then what changed.
        if local.hasSynced { await refreshLocal() }
        do {
            try await local.sync(with: api)
            offline = false
        } catch {
            offline = true
            if !local.hasSynced { self.error = error.localizedDescription }
        }
        await refreshLocal()
        if !offline { prefetchThumbnails() }
    }

    /// Shows the copy as synced, unless the server's results are showing.
    private func refreshLocal() async {
        if filters == nil || fromLocal { await showLocal() }
    }

    private var prefetching = false

    /// On Wi-Fi, the newest photos' thumbnails are fetched ahead, so the
    /// collection can be browsed offline later.
    private func prefetchThumbnails() {
        guard let local, !prefetching, Unmetered.shared.now,
              let ids = try? local.shownIDs(limit: Self.prefetched) else { return }
        prefetching = true
        let api = api
        Task.detached(priority: .utility) {
            await PhotoLinks.shared.know(ids, in: api)
            await ImageCache.shared.prefetch(ids)
            await MainActor.run { self.prefetching = false }
        }
    }

    /// About 40 MB of thumbnails.
    static let prefetched = 1000

    /// The photos the filters ask for, from the phone's copy.
    private func showLocal() async {
        guard let local else { return }
        let f = filters ?? .init()
        do {
            let page = try local.photos(matching: f, limit: max(120, fromLocal ? photos.count : 0))
            let subs = try local.subjects()
            await PhotoLinks.shared.know(page.map(\.id), in: api)
            // Filters changed while this was read: theirs will show.
            guard (filters ?? .init()) == f else { return }
            filters = f
            photos = page
            total = try local.count(matching: f)
            if Self.narrows(f) {
                tagCounts = try local.tagCounts(matching: f)
            } else {
                places = try local.placeCounts()
            }
            subjects = []
            missing = []
            subjectNotFound = false
            remember(subs)
            fromLocal = true
            error = nil
        } catch {
            self.error = error.localizedDescription
        }
    }

    /// A response from the server, kept on the phone; offline, the one
    /// kept last time.
    func remembered<M: SwiftProtobuf.Message>(_ key: String, _ fetch: () async throws -> M) async -> M? {
        do {
            let m = try await fetch()
            try? local?.keep(m, as: key)
            return m
        } catch {
            return local?.kept(M.self, as: key)
        }
    }

    /// Only the scenario, for naming tags in a conversation.
    func loadConfig() async {
        guard config == nil, let json = try? await api.activeConfig().configJson else { return }
        config = try? ScenarioConfig(json: json)
    }

    /// Searches again with the given filters, as the user edited them.
    func apply(_ f: Pidex_V1_SearchFilters) async {
        let synced = local?.hasSynced == true
        // Found on the phone exactly as the server would.
        if synced && (!Self.narrows(f) || LocalLibrary.matchesExactly(f)) {
            matchedOffline = false
            filters = f
            await showLocal()
            return
        }
        // Known to be offline: no waiting for the server.
        if synced && offline && f.mode != .missing {
            await showOffline(f)
            return
        }
        let counts = (f.tagsRequired || !f.place.isEmpty) && f.text.isEmpty
        if await !run({ try await self.api.searchPhotos(filters: f, tagCounts: counts) }), synced, f.mode != .missing {
            await showOffline(f)
        }
    }

    /// A search matched on the phone, the server out of reach.
    private func showOffline(_ f: Pidex_V1_SearchFilters) async {
        offline = true
        matchedOffline = true
        filters = f
        await showLocal()
    }

    /// Tags picked to narrow by: a photo must have every one.
    var pickedTags: [String] {
        guard let f = filters, f.tagsRequired else { return [] }
        return f.tags
    }

    /// The place picked to narrow by, if any.
    var pickedPlace: String? {
        guard let p = filters?.place, !p.isEmpty else { return nil }
        return p
    }

    /// Narrows to photos taken in a place, or drops it, keeping picked tags.
    func toggle(place: String) async {
        var f = filters.map { $0.tagsRequired || $0.tags.isEmpty ? $0 : Pidex_V1_SearchFilters() } ?? .init()
        f.place = f.place == place ? "" : place
        await apply(f)
    }

    /// Picks a tag too, or drops it, keeping the other filters.
    func toggle(tag: String) async {
        var f = filters.map { $0.tagsRequired || $0.tags.isEmpty ? $0 : Pidex_V1_SearchFilters() } ?? .init()
        if let i = f.tags.firstIndex(of: tag) {
            f.tags.remove(at: i)
        } else {
            f.tags.append(tag)
        }
        f.tagsRequired = !f.tags.isEmpty
        await apply(f)
    }

    func clearTags() async {
        var f = filters ?? .init()
        f.tags = []
        f.tagsRequired = false
        await apply(f)
    }

    /// The next page of matches, as the grid scrolls to its end.
    func more() async {
        guard !working, !loadingMore, photos.count < total else { return }
        loadingMore = true
        defer { loadingMore = false }
        if let local, fromLocal {
            do {
                let next = try local.photos(matching: filters ?? .init(), offset: photos.count, limit: 120)
                await PhotoLinks.shared.know(next.map(\.id), in: api)
                photos += next
                if next.isEmpty { total = photos.count }
            } catch {
                self.error = error.localizedDescription
            }
            return
        }
        do {
            let r = try await api.searchPhotos(filters: filters ?? .init(), offset: photos.count, pageSize: 120)
            let known = Set(photos.map(\.id))
            let new = r.photos.filter { !known.contains($0.id) }
            photos += new
            remember(r.subjects)
            // Fewer than counted (photos deleted meanwhile): stop asking.
            if new.isEmpty { total = photos.count }
        } catch {
            self.error = error.localizedDescription
        }
    }

    /// Searches in the user's own words: a model reads them into filters.
    /// Offline, the phone reads the names it knows in them.
    func search(_ text: String) async {
        // Asked of the server even when it was out of reach last time: it is
        // what reads meaning.
        if await !run({ try await self.api.searchPhotos(query: text) }), let local, local.hasSynced {
            let f = OfflineSearch.filters(for: text, subjects: (try? local.subjects()) ?? [], tags: config?.tags ?? [],
                                          places: (try? local.placeNames()) ?? [])
            await showOffline(f)
        }
    }

    func clear() async {
        await apply(.init())
    }

    /// Photos deleted elsewhere: no longer shown or counted.
    func removed(_ ids: Set<String>) {
        let before = photos.count
        photos.removeAll { ids.contains($0.id) }
        total = max(0, total - (before - photos.count))
    }

    /// A photo changed elsewhere (tags edited): show the new version.
    func replace(_ photo: Pidex_V1_Photo) {
        if let i = photos.firstIndex(where: { $0.id == photo.id }) {
            photos[i] = photo
        }
    }

    func subject(_ id: String) -> Pidex_V1_Subject? {
        subjects.first { $0.id == id } ?? missing.first { $0.id == id } ?? known[id]
    }

    /// Subjects seen elsewhere (a conversation, the subject list), so a
    /// photo opened from there shows what it is filed under.
    func remember(_ list: [Pidex_V1_Subject]) {
        for s in list { known[s.id] = s }
    }

    func tagName(_ key: String) -> String {
        config?.tags.first { $0.key == key }?.name ?? key
    }

    /// Shows what the server found; false if it could not be reached (a
    /// refusal shows as an error).
    @discardableResult
    private func run(_ call: @escaping () async throws -> Pidex_V1_SearchPhotosResponse) async -> Bool {
        working = true
        error = nil
        defer { working = false }
        do {
            let r = try await call()
            await PhotoLinks.shared.know(r.photos.map(\.id), in: api)
            fromLocal = false
            matchedOffline = false
            offline = false
            filters = r.filters
            photos = r.photos
            subjects = r.subjects
            missing = r.missing
            subjectNotFound = r.subjectNotFound
            total = max(Int(r.total), r.photos.count)
            tagCounts = Dictionary(r.tagCounts.map { ($0.key, Int($0.count)) }, uniquingKeysWith: { a, _ in a })
            if r.filters.place.isEmpty && r.filters.tags.isEmpty && r.filters.text.isEmpty && r.filters.subjectQuery.isEmpty {
                places = r.placeCounts
            }
            return true
        } catch {
            let unreachable = (error as? APIError).map(Self.unreachable) ?? true
            // Out of reach with the phone's copy to fall back on: the caller
            // shows that instead.
            if !unreachable || local?.hasSynced != true {
                self.error = error.localizedDescription
            }
            return !unreachable
        }
    }

    /// The server could not be reached, as opposed to refusing the request.
    private static func unreachable(_ e: APIError) -> Bool {
        [.unavailable, .deadlineExceeded, .unknown, .canceled].contains(e.code)
    }
}
