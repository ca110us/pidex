import SwiftUI

/// How a request was understood, one removable chip per filter. Removing
/// one hands back the filters without it.
struct FilterChips: View {
    let filters: Pidex_V1_SearchFilters
    let model: LibraryModel
    let onChange: (Pidex_V1_SearchFilters) -> Void

    private struct Chip: Identifiable {
        let id: String
        let label: String
        let icon: String
        let remove: (inout Pidex_V1_SearchFilters) -> Void
    }

    var body: some View {
        if !chips.isEmpty {
            row
        }
    }

    private var row: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                ForEach(chips) { chip in
                    Button {
                        var f = filters
                        chip.remove(&f)
                        onChange(f)
                    } label: {
                        HStack(spacing: 4) {
                            Image(systemName: chip.icon).font(.caption2)
                            Text(verbatim: chip.label).lineLimit(1)
                            Image(systemName: "xmark").font(.caption2.weight(.bold)).foregroundStyle(.secondary)
                        }
                        .font(.footnote)
                        .padding(.vertical, 6)
                        .padding(.horizontal, 10)
                        .background(Theme.surface, in: Capsule())
                        .overlay(Capsule().strokeBorder(Theme.hairline))
                    }
                    .buttonStyle(.plain)
                    .accessibilityHint(Text("Removes this filter"))
                }
            }
            .padding(.horizontal)
        }
    }

    private var chips: [Chip] {
        var out: [Chip] = []
        if filters.mode == .missing {
            out.append(Chip(id: "mode", label: String(localized: "Not photographed yet"), icon: "questionmark.square.dashed") { $0.mode = .photos })
        }
        if !filters.subjectIds.isEmpty || !filters.subjectQuery.isEmpty {
            let names = filters.subjectIds.compactMap { model.subject($0)?.displayName }
            let label = names.isEmpty ? filters.subjectQuery : names.joined(separator: ", ")
            out.append(Chip(id: "subject", label: label, icon: "person.text.rectangle") {
                $0.subjectIds = []
                $0.subjectQuery = ""
            })
        }
        if !filters.subjectType.isEmpty, let st = model.config?.subjectTypes.first(where: { $0.key == filters.subjectType }) {
            out.append(Chip(id: "type", label: st.name, icon: "square.stack") { $0.subjectType = "" })
        }
        for key in filters.tagsRequired ? [] : filters.tags {
            out.append(Chip(id: "tag-" + key, label: model.tagName(key), icon: "tag") { $0.tags.removeAll { $0 == key } })
        }
        if filters.hasFrom || filters.hasTo {
            out.append(Chip(id: "date", label: dateLabel, icon: "calendar") {
                $0.clearFrom()
                $0.clearTo()
            })
        }
        if !filters.text.isEmpty {
            out.append(Chip(id: "text", label: "“\(filters.text)”", icon: "text.magnifyingglass") { $0.text = "" })
        }
        return out
    }

    private var dateLabel: String {
        let style = Date.FormatStyle(date: .abbreviated, time: .omitted)
        let from = filters.hasFrom ? filters.from.date.formatted(style) : "…"
        // "to" is exclusive; show the last day included.
        let to = filters.hasTo ? filters.to.date.addingTimeInterval(-1).formatted(style) : "…"
        return from == to ? from : "\(from) – \(to)"
    }
}

/// A square thumbnail from a signed URL.
struct Thumbnail: View {
    let photo: Pidex_V1_Photo

    var body: some View {
        Theme.surface
            .aspectRatio(1, contentMode: .fit)
            .overlay {
                CachedThumbnail(id: photo.id, url: URL(string: photo.thumbnailURL.isEmpty ? photo.viewURL : photo.thumbnailURL))
            }
            .clipped()
            .accessibilityLabel(Text(verbatim: photo.description_p))
    }
}

/// A three-column grid of photos that open in detail.
struct PhotoGrid: View {
    let photos: [Pidex_V1_Photo]
    let model: LibraryModel
    /// Numbers photos [1], [2]… as an answer cites them.
    var numbered = false
    /// While set, a tap marks a photo rather than opening it.
    var selection: Binding<Set<String>>?
    /// The whole list these photos are part of (one day of it, say): the
    /// viewer pages through all of it.
    var album: [Pidex_V1_Photo]?
    /// The rest of the list, not loaded yet.
    var more: MorePhotos?

    var body: some View {
        LazyVGrid(columns: Array(repeating: GridItem(.flexible(), spacing: 2), count: 3), spacing: 2) {
            ForEach(Array(photos.enumerated()), id: \.element.id) { i, photo in
                if let selection {
                    let on = selection.wrappedValue.contains(photo.id)
                    Button {
                        if on { selection.wrappedValue.remove(photo.id) } else { selection.wrappedValue.insert(photo.id) }
                    } label: {
                        Thumbnail(photo: photo)
                            .overlay { if on { Color.black.opacity(0.25) } }
                            .overlay(alignment: .bottomTrailing) {
                                Image(systemName: on ? "checkmark.circle.fill" : "circle")
                                    .font(.title3)
                                    .symbolRenderingMode(.palette)
                                    .foregroundStyle(on ? Theme.onInk : .white, on ? Theme.ink : Color.black.opacity(0.3))
                                    .padding(6)
                            }
                    }
                    .buttonStyle(.plain)
                    .accessibilityAddTraits(on ? .isSelected : [])
                    .accessibilityIdentifier("photo-\(i)")
                } else {
                NavigationLink {
                    let all = album ?? photos
                    PhotoViewer(photos: all, start: all.firstIndex { $0.id == photo.id } ?? i, model: model, more: more)
                } label: {
                    Thumbnail(photo: photo)
                        .overlay(alignment: .topLeading) {
                            if numbered {
                                Text(verbatim: "\(i + 1)")
                                    .font(.caption.bold())
                                    .padding(5)
                                    .background(.black.opacity(0.6), in: Circle())
                                    .foregroundStyle(.white)
                                    .padding(4)
                            }
                        }
                }
                .accessibilityIdentifier("photo-\(i)")
                }
            }
        }
    }
}
