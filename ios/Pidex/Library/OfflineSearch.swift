import Foundation

/// A search in words read on the phone, offline, with no model to read it:
/// the names the phone knows in the words (subjects, tags, places) become
/// filters, as the search model would make them; with none, the words are
/// matched as they are against what each photo says.
nonisolated enum OfflineSearch {
    static func filters(for text: String, subjects: [Pidex_V1_Subject], tags: [ScenarioConfig.Tag],
                        places: Set<String>) -> Pidex_V1_SearchFilters {
        let words = text.lowercased()
        var f = Pidex_V1_SearchFilters()
        f.mode = .photos
        // Names are kept long before short, so "Tutu Jr" is not read as "Tutu".
        var left = words
        func take(_ name: String) -> Bool {
            let n = name.lowercased().trimmingCharacters(in: .whitespaces)
            guard !n.isEmpty, let r = left.range(of: n) else { return false }
            left.replaceSubrange(r, with: " ")
            return true
        }
        for s in subjects.sorted(by: { $0.displayName.count > $1.displayName.count }) where take(s.displayName) {
            f.subjectIds.append(s.id)
        }
        for t in tags.sorted(by: { $0.name.count > $1.name.count }) where take(t.name) || take(t.key) {
            f.tags.append(t.key)
        }
        f.tagsRequired = !f.tags.isEmpty
        // A city as people say it: "大连" for "大连市".
        let named = places.union(places.map(PlaceNames.city)).sorted { $0.count > $1.count }
        if let place = named.first(where: take) { f.place = place }
        if f.subjectIds.isEmpty && f.tags.isEmpty && f.place.isEmpty {
            f.text = text.trimmingCharacters(in: .whitespacesAndNewlines)
        }
        return f
    }
}
