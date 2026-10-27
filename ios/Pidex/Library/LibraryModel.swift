import Foundation
import Observation
import SwiftProtobuf

/// A collection's photos as browsed: the newest, or those a picked tag
/// filters to; editing the filters searches again without a model call.
/// Unfiltered, photos come from the phone's copy of the collection, synced
/// as it opens: at once, and offline too.
@MainActor
@Observable
final class LibraryModel {
    let api: APIClient
    let local: LocalLibrary?
    /// The server could not be reached: what is shown is the phone's copy.
    private(set) var offline = false
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
        if local.hasSynced { await showLocal() }
        do {
            try await local.sync(with: api)
            offline = false
        } catch {
            offline = true
            if !local.hasSynced { self.error = error.localizedDescription }
        }
        await showLocal()
    }

    /// The newest photos from the phone's copy, unless a search is showing.
    private func showLocal() async {
        guard let local, !isFiltered else { return }
        do {
            let page = try local.photos(limit: max(120, photos.count))
            let subs = try local.subjects()
            await PhotoLinks.shared.know(page.map(\.id), in: api)
            guard !isFiltered else { return }
            filters = filters ?? .init()
            photos = page
            total = try local.shownCount()
            places = try local.placeCounts()
            remember(subs)
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
        if local != nil && !Self.narrows(f) {
            filters = f
            await showLocal()
            return
        }
        let counts = (f.tagsRequired || !f.place.isEmpty) && f.text.isEmpty
        await run { try await self.api.searchPhotos(filters: f, tagCounts: counts) }
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
        if let local, !isFiltered {
            do {
                let next = try local.photos(offset: photos.count, limit: 120)
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
    func search(_ text: String) async {
        await run { try await self.api.searchPhotos(query: text) }
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

    private func run(_ call: @escaping () async throws -> Pidex_V1_SearchPhotosResponse) async {
        working = true
        error = nil
        defer { working = false }
        do {
            let r = try await call()
            await PhotoLinks.shared.know(r.photos.map(\.id), in: api)
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
        } catch {
            self.error = error.localizedDescription
        }
    }
}
