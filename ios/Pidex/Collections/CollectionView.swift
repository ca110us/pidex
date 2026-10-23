import SwiftUI

/// One collection: its photos by day or its subjects, the tags used most as
/// one-tap filters, and its camera; its menu starts a conversation to find
/// and ask about photos.
struct CollectionView: View {
    let workspace: Workspace
    @State private var model: LibraryModel
    @State private var overview: Pidex_V1_GetOverviewResponse?
    @State private var subjects: [Pidex_V1_Subject] = []
    @State private var section = Section.photos
    @State private var showingScenario = false
    @State private var showingSetup = false
    @State private var deleting = false
    /// The user's role in this collection; only its owner may delete it.
    @State private var role = ""
    @State private var showingHistory = false
    @State private var ask: AskRoute?
    @State private var query = ""
    /// Photos marked for deleting; nil when not selecting.
    @State private var selected: Set<String>?
    @State private var confirmingDelete = false
    @State private var renaming: Pidex_V1_Subject?
    @State private var deletingSubject: Pidex_V1_Subject?
    @State private var shownSubject: Pidex_V1_Subject?
    @State private var newName = ""
    @Environment(AppModel.self) private var app
    private let initialQuery: String?

    enum Section: Hashable {
        case photos
        case subjects
    }

    init(workspace: Workspace, api: APIClient, initialQuery: String? = nil) {
        self.workspace = workspace
        self.initialQuery = initialQuery
        _model = State(initialValue: LibraryModel(api: api))
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 12) {
                if let o = overview, model.isFiltered {
                    // Clearing sits here, not in the tag row, so the tags
                    // never move.
                    HStack {
                        Text("\(model.total) of \(o.photoCount) photos")
                            .foregroundStyle(.secondary)
                        Spacer()
                        if !model.pickedTags.isEmpty {
                            Button("Clear tags") { Task { await model.clearTags() } }
                        }
                    }
                    .font(.subheadline)
                    .padding(.horizontal)
                } else if let o = overview {
                    Text("\(o.photoCount) photos · \(o.subjectCount) subjects")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .padding(.horizontal)
                }
                if !(overview?.tags.isEmpty ?? true) {
                    tagRow
                }
                ForEach([model.error].compactMap { $0 }, id: \.self) { text in
                    Label(text, systemImage: "exclamationmark.triangle")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                        .padding(.horizontal)
                }
                if model.isFiltered {
                    if let f = model.filters {
                        FilterChips(filters: f, model: model) { edited in
                            Task { await model.apply(edited) }
                        }
                    }
                    if model.photos.isEmpty && !model.working {
                        emptyState(String(localized: "No photos match."))
                    } else {
                        PhotoGrid(photos: model.photos, model: model, selection: selectionBinding, more: morePhotos)
                        moreSentinel
                    }
                } else {
                    Picker("Show", selection: $section) {
                        Text("Photos").tag(Section.photos)
                        Text("Subjects").tag(Section.subjects)
                    }
                    .pickerStyle(.segmented)
                    .padding(.horizontal)
                    if section == .photos {
                        if model.photos.isEmpty && !model.working {
                            emptyState(String(localized: "No photos yet. Tap the camera to start."))
                        } else {
                            DayGroupedGrid(photos: model.photos, model: model, selection: selectionBinding, more: morePhotos)
                            moreSentinel
                        }
                    } else {
                        subjectList
                    }
                }
            }
            .padding(.vertical, 8)
        }
        .contentMargins(.bottom, 80, for: .scrollContent)
        .subjectDeletion($deletingSubject, api: model.api) { s in
            subjects.removeAll { $0.id == s.id }
            Task { await load() }
        }
        .safeAreaInset(edge: .bottom) {
            if let selected {
                HStack {
                    Button("Cancel") { self.selected = nil }
                    Spacer()
                    Text(selected.isEmpty ? String(localized: "Select photos") : String(localized: "\(selected.count) selected"))
                        .font(.subheadline.weight(.semibold))
                    Spacer()
                    Button(role: .destructive) { confirmingDelete = true } label: { Image(systemName: "trash") }
                        .disabled(selected.isEmpty)
                        .accessibilityLabel(Text("Delete"))
                        .accessibilityIdentifier("deleteSelected")
                }
                .padding(.horizontal, 20)
                .padding(.vertical, 14)
                .glassSurface(Capsule())
                .padding(.horizontal, 12)
                .padding(.bottom, 4)
            }
        }
        .confirmationDialog(Text("Delete \(selected?.count ?? 0) photos?"), isPresented: $confirmingDelete, titleVisibility: .visible) {
            Button("Delete for good", role: .destructive) { Task { await deleteSelected() } }
        } message: {
            Text("They are removed for everyone, with their tags and their place in conversations. This cannot be undone.")
        }
        .overlay {
            if model.working {
                ProgressView().padding().background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12))
            }
        }
        .alert("Rename", isPresented: Binding(get: { renaming != nil }, set: { if !$0 { renaming = nil } })) {
            TextField("Name", text: $newName)
            Button("Cancel", role: .cancel) {}
            // The alert clears `renaming` as it closes; keep it first.
            Button("Save") { [renaming] in Task { await rename(renaming) } }
        }
        .navigationTitle(Text(verbatim: workspace.name))
        .searchable(text: $query, prompt: Text("Search this collection"))
        .onSubmit(of: .search) { Task { await search() } }
        .onChange(of: query) { _, q in
            if q.isEmpty, model.isFiltered { Task { await model.clear() } }
        }
        .toolbar {
            ToolbarItemGroup(placement: .topBarTrailing) {
                Button { openCamera() } label: { Image(systemName: "camera") }
                    .accessibilityLabel(Text("Take photos"))
                    .accessibilityIdentifier("collectionCamera")
                Button { ask = AskRoute(scope: .collection(workspace.id)) } label: {
                    PidexMark(height: 18)
                        .foregroundStyle(Theme.ink)
                }
                .accessibilityLabel(Text("Ask a question"))
                .accessibilityIdentifier("askButton")
                Menu {
                    Button { ask = AskRoute(scope: .collection(workspace.id)) } label: {
                        Label("Ask a question", systemImage: "bubble.left.and.text.bubble.right")
                    }
                    .accessibilityIdentifier("askQuestion")
                    Button { showingHistory = true } label: {
                        Label("Conversations", systemImage: "clock.arrow.circlepath")
                    }
                    .accessibilityIdentifier("conversations")
                    Divider()
                    Button { selected = [] } label: {
                        Label("Select photos", systemImage: "checkmark.circle")
                    }
                    .disabled(model.photos.isEmpty)
                    .accessibilityIdentifier("selectPhotos")
                    Button { showingSetup = true } label: {
                        Label("Setup conversation", systemImage: "slider.horizontal.3")
                    }
                    .accessibilityIdentifier("setupConversation")
                    Button { showingScenario = true } label: { Label("Scenario", systemImage: "info.circle") }
                    if role == "owner" {
                        Divider()
                        Button(role: .destructive) { deleting = true } label: {
                            Label("Delete collection…", systemImage: "trash")
                        }
                        .accessibilityIdentifier("deleteCollection")
                    }
                } label: {
                    Image(systemName: "ellipsis")
                }
                .accessibilityLabel(Text("More"))
                .accessibilityIdentifier("collectionMenu")
            }
        }
        .navigationDestination(item: $ask) { route in
            AskView(route: route, library: model)
        }
        .navigationDestination(isPresented: $showingHistory) {
            ConversationListView(scope: .collection(workspace.id))
        }
        .sheet(isPresented: $deleting) {
            NavigationStack {
                DeleteCollectionView(workspace: workspace, api: model.api, photoCount: Int(overview?.photoCount ?? 0),
                                     uploading: workspace.id == app.active?.id ? app.uploadStatus.pending : 0) {
                    deleting = false
                    app.forgetDeleted(workspace)
                    // Home notices the collection is gone and goes back to it.
                    app.show(String(localized: "Deleted “\(workspace.name)” and everything in it."))
                }
            }
        }
        .navigationDestination(item: $shownSubject) { s in
            SubjectPhotosView(subject: s, model: model)
        }
        .navigationDestination(isPresented: $showingSetup) {
            SetupChatView(mode: .edit(name: workspace.name), api: model.api) { _ in
                Task { await load() }
            }
        }
        .navigationDestination(isPresented: $showingScenario) {
            ScenarioView(api: model.api)
        }
        .task {
            await load()
            if let q = initialQuery, !q.isEmpty, ask == nil {
                ask = AskRoute(scope: .collection(workspace.id), firstMessage: q)
            }
        }
        .refreshable { await load() }
        .onChange(of: app.showingCamera) { _, showing in
            // Back from shooting: show what was just taken.
            if !showing { Task { await load() } }
        }
    }

    /// The tags used most, as filters to combine: each picked one narrows
    /// the photos further, and the counts say what picking another would
    /// leave. Every tag keeps its place; one that would leave nothing is
    /// greyed out rather than removed, so nothing seems to vanish.
    private var tagRow: some View {
        let picked = model.pickedTags
        return ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                ForEach(overview?.tags ?? [], id: \.key) { t in
                    let on = picked.contains(t.key)
                    let count = picked.isEmpty ? Int(t.count) : model.tagCounts[t.key] ?? 0
                    Button {
                        Task { await model.toggle(tag: t.key) }
                    } label: {
                        HStack(spacing: 4) {
                            if on { Image(systemName: "checkmark").font(.caption2.weight(.bold)) }
                            Text(verbatim: model.tagName(t.key))
                            Text(verbatim: "\(count)").foregroundStyle(on ? Theme.onInk.opacity(0.7) : Theme.muted)
                        }
                        .font(.footnote)
                        .padding(.vertical, 6)
                        .padding(.horizontal, 12)
                        .background(on ? Theme.ink : Theme.surface, in: Capsule())
                        .foregroundStyle(on ? Theme.onInk : Theme.ink)
                    }
                    .buttonStyle(.plain)
                    .disabled(!on && count == 0)
                    .opacity(!on && count == 0 ? 0.4 : 1)
                    .accessibilityIdentifier("tag-\(t.key)")
                    .accessibilityAddTraits(on ? .isSelected : [])
                }
            }
            .padding(.horizontal)
        }
    }

    /// The photos not loaded yet, for the viewer to page on into.
    private var morePhotos: MorePhotos {
        MorePhotos(total: model.total) { [model] in
            await model.more()
            return model.photos
        }
    }

    /// Loads the next page when the end of the grid comes into view.
    @ViewBuilder
    private var moreSentinel: some View {
        if model.photos.count < model.total {
            ProgressView()
                .frame(maxWidth: .infinity)
                .padding()
                .task(id: model.photos.count) { await model.more() }
        }
    }

    private var subjectList: some View {
        LazyVStack(alignment: .leading, spacing: 0) {
            if subjects.isEmpty {
                emptyState(String(localized: "Nothing filed yet"))
            }
            ForEach(subjects, id: \.id) { s in
                // Opened from outside the lazy list, where the stack always
                // sees its destination.
                Button { shownSubject = s } label: {
                    HStack(spacing: 12) {
                        SubjectAvatar(subject: s)
                        VStack(alignment: .leading) {
                            Text(verbatim: s.displayName)
                            if let type = model.config?.subjectTypes.first(where: { $0.key == s.typeKey }) {
                                Text(verbatim: type.name).font(.caption).foregroundStyle(.secondary)
                            }
                        }
                        Spacer()
                        Text("\(s.photoCount) photos").foregroundStyle(.secondary)
                        Image(systemName: "chevron.right").foregroundStyle(.tertiary)
                    }
                    .padding(.horizontal)
                    .padding(.vertical, 10)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .contextMenu {
                    Button { newName = s.displayName; renaming = s } label: { Label("Rename", systemImage: "pencil") }
                    Button(role: .destructive) { deletingSubject = s } label: { Label("Delete…", systemImage: "trash") }
                }
                .accessibilityIdentifier("subjectRow-\(s.displayName)")
                Divider().padding(.leading)
            }
        }
    }

    private func emptyState(_ text: String) -> some View {
        Text(verbatim: text)
            .foregroundStyle(.secondary)
            .multilineTextAlignment(.center)
            .frame(maxWidth: .infinity)
            .padding(.top, 60)
            .padding(.horizontal)
    }

    private func load() async {
        await model.load(force: true)
        async let o = try? model.api.overview()
        async let s = try? model.api.subjects()
        async let t = try? model.api.team()
        overview = await o
        subjects = await s ?? []
        model.remember(subjects)
        role = await t?.role ?? ""
    }

    private var selectionBinding: Binding<Set<String>>? {
        guard selected != nil else { return nil }
        return Binding(get: { selected ?? [] }, set: { selected = $0 })
    }

    private func deleteSelected() async {
        guard let ids = selected, !ids.isEmpty else { return }
        do {
            let gone = try await model.api.deletePhotos(Array(ids))
            model.removed(ids)
            selected = nil
            app.show(String(localized: "Deleted \(gone.photos) photos"))
            await load()
        } catch {
            app.show(error.localizedDescription, tone: .problem)
        }
    }

    /// A subject's name finds its photos at once; anything else is read
    /// by the search model ("Tutu asleep last week").
    private func search() async {
        let q = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !q.isEmpty else { return }
        let named = subjects.filter { $0.displayName.localizedCaseInsensitiveCompare(q) == .orderedSame }
        if let s = named.first {
            var f = Pidex_V1_SearchFilters()
            f.subjectIds = [s.id]
            await model.apply(f)
        } else {
            await model.search(q)
        }
    }

    private func rename(_ subject: Pidex_V1_Subject?) async {
        guard let s = subject else { return }
        let name = newName.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !name.isEmpty, name != s.displayName else { return }
        do {
            let updated = try await model.api.renameSubject(s.id, to: name)
            if let i = subjects.firstIndex(where: { $0.id == s.id }) { subjects[i].displayName = updated.displayName }
            model.remember([updated])
        } catch {
            app.show(error.localizedDescription, tone: .problem)
        }
    }

    private func openCamera() {
        do {
            try app.openCamera(for: workspace.id)
        } catch {
            app.show(error.localizedDescription, tone: .problem)
        }
    }
}

/// Photos in sections by the day they were taken, newest first.
struct DayGroupedGrid: View {
    let photos: [Pidex_V1_Photo]
    let model: LibraryModel
    var selection: Binding<Set<String>>?
    var more: MorePhotos?

    var body: some View {
        LazyVStack(alignment: .leading, spacing: 8, pinnedViews: .sectionHeaders) {
            ForEach(days, id: \.day) { group in
                SwiftUI.Section {
                    PhotoGrid(photos: group.photos, model: model, selection: selection, album: photos, more: more)
                } header: {
                    Text(group.day.formatted(date: .complete, time: .omitted))
                        .font(.subheadline.weight(.semibold))
                        .padding(.horizontal)
                        .padding(.vertical, 6)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(.bar)
                }
            }
        }
    }

    private var days: [(day: Date, photos: [Pidex_V1_Photo])] {
        let cal = Calendar.current
        var order: [Date] = []
        var byDay: [Date: [Pidex_V1_Photo]] = [:]
        for p in photos {
            let day = cal.startOfDay(for: p.hasCapturedAt ? p.capturedAt.date : .now)
            if byDay[day] == nil { order.append(day) }
            byDay[day, default: []].append(p)
        }
        return order.map { ($0, byDay[$0]!) }
    }
}

/// A subject shown by a photo of it, or an icon when it has none yet.
struct SubjectAvatar: View {
    let subject: Pidex_V1_Subject
    var size: CGFloat = 44

    var body: some View {
        Group {
            if subject.hasCover {
                Thumbnail(photo: subject.cover)
            } else {
                Theme.surface
                    .overlay { Image(systemName: "photo").fontWeight(.light).foregroundStyle(Theme.muted) }
            }
        }
        .frame(width: size, height: size)
        .clipShape(RoundedRectangle(cornerRadius: size / 4.5, style: .continuous))
    }
}

/// A collection opened from elsewhere, by its workspace.
struct CollectionPage: View {
    let workspace: Workspace
    @Environment(AppModel.self) private var app

    var body: some View {
        if let api = app.api(for: workspace) {
            CollectionView(workspace: workspace, api: api)
        }
    }
}
