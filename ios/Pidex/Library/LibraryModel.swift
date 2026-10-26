import Foundation
import Observation

/// A collection's photos as browsed: the newest, or those a picked tag
/// filters to; editing the filters searches again without a model call.
@MainActor
@Observable
final class LibraryModel {
    let api: APIClient
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
    }

    var isFiltered: Bool {
        guard let f = filters else { return false }
        return f.mode == .missing || !f.subjectQuery.isEmpty || !f.subjectIds.isEmpty || !f.tags.isEmpty
            || f.hasFrom || f.hasTo || !f.text.isEmpty || !f.roles.isEmpty || !f.place.isEmpty
    }

    /// Loads the scenario and the newest photos; `force` refreshes them
    /// unless a search is showing.
    func load(force: Bool = false) async {
        if config == nil || force, let json = try? await api.activeConfig().configJson {
            config = try? ScenarioConfig(json: json)
        }
        if (filters == nil && photos.isEmpty) || (force && !isFiltered) {
            // Counted too, for the row of places to narrow by.
            await run { try await self.api.searchPhotos(filters: .init(), tagCounts: true) }
        }
    }

    /// Only the scenario, for naming tags in a conversation.
    func loadConfig() async {
        guard config == nil, let json = try? await api.activeConfig().configJson else { return }
        config = try? ScenarioConfig(json: json)
    }

    /// Searches again with the given filters, as the user edited them.
    func apply(_ f: Pidex_V1_SearchFilters) async {
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
        await run { try await self.api.searchPhotos(filters: .init()) }
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
