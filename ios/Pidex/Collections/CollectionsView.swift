import SwiftUI

/// Where a collection opens: optionally with a search already made.
nonisolated struct CollectionRoute: Hashable {
    let workspace: Workspace
    var query: String?
}

/// The home screen: one card per collection (a workspace and its scenario),
/// search across all of them, and the camera one tap away.
struct CollectionsView: View {
    @Environment(AppModel.self) private var app
    @State private var overviews: [String: Pidex_V1_GetOverviewResponse] = [:]
    @State private var query = ""
    @State private var results: [(workspace: Workspace, response: Pidex_V1_SearchPhotosResponse)] = []
    @State private var searching = false
    @State private var creating = false
    @State private var showingSettings = false
    @State private var ask: AskRoute?
    /// The collection open over home. Pages are handed to the stack, not
    /// looked up by value: links inside a page opened by value do not open
    /// their own pages.
    @State private var opened: CollectionRoute?
    @State private var showingHistory = false

    private let columns = [GridItem(.flexible(), spacing: 14), GridItem(.flexible(), spacing: 14)]

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    if query.isEmpty || results.isEmpty && !searching {
                        if !query.isEmpty {
                            Text("No photos match.").foregroundStyle(.secondary).padding(.horizontal)
                        }
                        grid
                    } else {
                        searchResults
                    }
                }
                .padding(.vertical)
            }
            .overlay { if searching { ProgressView() } }
            // Room to scroll the last row above the camera button and the
            // upload progress, which float over the page.
            .contentMargins(.bottom, 110, for: .scrollContent)
            .overlay(alignment: .bottomTrailing) { cameraButton }
            .navigationTitle("Collections")
            .searchable(text: $query, prompt: Text("Search all collections"))
            .onSubmit(of: .search) { Task { await searchAll() } }
            .onChange(of: query) { _, q in if q.isEmpty { results = [] } }
            .toolbar {
                Button { ask = AskRoute(scope: allScope) } label: {
                    // Asking is talking to Pidex: its mark, as Grok shows its own.
                    PidexMark(height: 18)
                        .foregroundStyle(Theme.ink)
                }
                .disabled(app.workspaces.items.isEmpty)
                .accessibilityLabel(Text("Ask a question"))
                .accessibilityIdentifier("askButton")
                Menu {
                    Button { ask = AskRoute(scope: allScope) } label: {
                        Label("Ask a question", systemImage: "bubble.left.and.text.bubble.right")
                    }
                    .accessibilityIdentifier("askQuestion")
                    Button { showingHistory = true } label: {
                        Label("Conversations", systemImage: "clock.arrow.circlepath")
                    }
                    Divider()
                    Button { creating = true } label: { Label("New collection", systemImage: "plus") }
                    Button { showingSettings = true } label: { Label("Settings", systemImage: "gearshape") }
                } label: {
                    Image(systemName: "ellipsis")
                }
                .accessibilityIdentifier("homeMenu")
            }
            .navigationDestination(item: $opened) { route in
                if let api = app.api(for: route.workspace) {
                    CollectionView(workspace: route.workspace, api: api, initialQuery: route.query)
                }
            }
            .navigationDestination(item: $ask) { route in
                AskView(route: route)
            }
            .navigationDestination(isPresented: $showingHistory) {
                ConversationListView(scope: allScope)
            }
            .navigationDestination(isPresented: $creating) {
                if let api = app.api {
                    SetupChatView(mode: .create, api: api) { done in
                        // Empty when its camera opened, which says it.
                        if !done.isEmpty { app.show(done) }
                        creating = false
                        Task { await loadOverviews() }
                    }
                }
            }
            .sheet(isPresented: $showingSettings) { AppSettingsView() }
            // Again whenever a collection is added or removed.
            .task(id: app.workspaces.items.map(\.id)) { await loadOverviews() }
            .onChange(of: app.workspaces.items.map(\.id)) { before, after in
                // A collection deleted from its own page: back home.
                if let o = opened, !after.contains(o.workspace.id) { opened = nil }
            }
            .onChange(of: app.showingCamera) { _, showing in
                if !showing { Task { await loadOverviews() } }
            }
            .refreshable { await loadOverviews() }
        }
    }

    private var grid: some View {
        LazyVGrid(columns: columns, spacing: 18) {
            ForEach(app.workspaces.items) { w in
                Button { opened = CollectionRoute(workspace: w) } label: {
                    CollectionCard(name: w.name, overview: overviews[w.id], active: w.id == app.active?.id)
                }
                .buttonStyle(.plain)
                .accessibilityValue(w.id == app.active?.id ? Text("The camera shoots for this collection") : Text(verbatim: ""))
                .accessibilityIdentifier("collection-\(w.name)")
                .contextMenu {
                    Button { open(w) } label: { Label("Take photos", systemImage: "camera") }
                    Button(role: .destructive) { remove(w) } label: {
                        Label("Remove from this iPhone", systemImage: "minus.circle")
                    }
                }
            }
            Button { creating = true } label: { NewCollectionCard() }
                .buttonStyle(.plain)
                .disabled(app.api == nil)
                .accessibilityIdentifier("newCollection")
        }
        .padding(.horizontal)
    }

    private var searchResults: some View {
        VStack(alignment: .leading, spacing: 20) {
            ForEach(results, id: \.workspace.id) { r in
                Button { opened = CollectionRoute(workspace: r.workspace, query: query) } label: {
                    VStack(alignment: .leading, spacing: 8) {
                        HStack {
                            Text(verbatim: r.workspace.name).font(.title3.bold())
                            Image(systemName: "chevron.right").foregroundStyle(.secondary)
                            Spacer()
                            Text("\(r.response.photos.count) photos").foregroundStyle(.secondary)
                        }
                        .padding(.horizontal)
                        ScrollView(.horizontal, showsIndicators: false) {
                            HStack(spacing: 4) {
                                ForEach(r.response.photos.prefix(8), id: \.id) { p in
                                    Thumbnail(photo: p).frame(width: 110, height: 110)
                                        .clipShape(RoundedRectangle(cornerRadius: 8))
                                }
                            }
                            .padding(.horizontal)
                        }
                    }
                }
                .buttonStyle(.plain)
            }
        }
    }

    private var cameraButton: some View {
        Button {
            if let id = app.active?.id { open(id: id) }
        } label: {
            Image(systemName: "camera")
                .font(.system(size: 22, weight: .medium))
                .foregroundStyle(Theme.onInk)
                .frame(width: 62, height: 62)
                .background(Theme.ink, in: Circle())
                .shadow(color: .black.opacity(0.18), radius: 10, y: 4)
        }
        .padding(24)
        .disabled(app.active == nil)
        .accessibilityLabel(Text("Take photos for \(app.active?.name ?? "")"))
        .accessibilityIdentifier("cameraButton")
    }

    /// Asking from home asks every collection, or the only one.
    private var allScope: AskScope {
        let items = app.workspaces.items
        return items.count == 1 ? .collection(items[0].id) : .all
    }

    private func open(_ w: Workspace) { open(id: w.id) }

    private func open(id: String) {
        do {
            try app.openCamera(for: id)
        } catch {
            app.show(error.localizedDescription, tone: .problem)
        }
    }

    private func remove(_ w: Workspace) {
        do {
            try app.remove(w.id)
            overviews[w.id] = nil
        } catch {
            app.show(error.localizedDescription, tone: .problem)
        }
    }

    private func loadOverviews() async {
        await withTaskGroup(of: (String, Pidex_V1_GetOverviewResponse?).self) { group in
            for w in app.workspaces.items {
                guard let api = app.api(for: w) else { continue }
                group.addTask {
                    // Offline, as last seen.
                    let local = LocalLibrary.for(api)
                    guard let o = try? await api.overview() else { return (w.id, local?.kept(Pidex_V1_GetOverviewResponse.self, as: "overview")) }
                    try? local?.keep(o, as: "overview")
                    await PhotoLinks.shared.know(o.recent.map(\.id), in: api)
                    return (w.id, o)
                }
            }
            for await (id, o) in group {
                if let o { overviews[id] = o }
            }
        }
    }

    /// Every collection is searched in parallel; those with matches are
    /// listed, each opening with the same search.
    private func searchAll() async {
        let q = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !q.isEmpty else { return }
        searching = true
        defer { searching = false }
        var found: [(Workspace, Pidex_V1_SearchPhotosResponse)] = []
        await withTaskGroup(of: (Workspace, Pidex_V1_SearchPhotosResponse?).self) { group in
            for w in app.workspaces.items {
                guard let api = app.api(for: w) else { continue }
                group.addTask { (w, try? await api.searchPhotos(query: q)) }
            }
            for await (w, r) in group {
                if let r, !r.photos.isEmpty { found.append((w, r)) }
            }
        }
        let order = app.workspaces.items.map(\.id)
        results = found.sorted { (order.firstIndex(of: $0.0.id) ?? 0) < (order.firstIndex(of: $1.0.id) ?? 0) }
            .map { (workspace: $0.0, response: $0.1) }
    }
}

/// A collection as a card: its newest photos as a cover, name and counts.
struct CollectionCard: View {
    let name: String
    let overview: Pidex_V1_GetOverviewResponse?
    let active: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            cover
                .aspectRatio(1, contentMode: .fit)
                .clipShape(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
                .overlay(alignment: .topTrailing) {
                    if active {
                        Image(systemName: "camera.circle.fill")
                            .font(.title3)
                            .symbolRenderingMode(.palette)
                            .foregroundStyle(.white, .black.opacity(0.45))
                            .padding(8)
                            .accessibilityLabel(Text("The camera shoots for this collection"))
                    }
                }
            Text(verbatim: name).font(.headline).lineLimit(1)
            if let o = overview {
                Text("\(o.photoCount) photos · \(o.subjectCount) subjects")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            } else {
                Text(verbatim: " ").font(.caption)
            }
        }
    }

    /// One large photo, or a mosaic of the four newest.
    @ViewBuilder
    private var cover: some View {
        let photos = overview?.recent ?? []
        if photos.count >= 4 {
            Grid(horizontalSpacing: 2, verticalSpacing: 2) {
                GridRow { Thumbnail(photo: photos[0]); Thumbnail(photo: photos[1]) }
                GridRow { Thumbnail(photo: photos[2]); Thumbnail(photo: photos[3]) }
            }
        } else if let first = photos.first {
            Thumbnail(photo: first)
        } else {
            Theme.surface
                .overlay {
                    Image(systemName: "photo.on.rectangle").font(.system(size: 30, weight: .light)).foregroundStyle(Theme.muted)
                }
        }
    }
}

struct NewCollectionCard: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            RoundedRectangle(cornerRadius: Theme.radius, style: .continuous)
                .strokeBorder(style: StrokeStyle(lineWidth: 1.5, dash: [5, 5]))
                .foregroundStyle(Theme.hairline)
                .background(Theme.surface.opacity(0.5), in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
                .aspectRatio(1, contentMode: .fit)
                .overlay {
                    VStack(spacing: 10) {
                        Image(systemName: "plus")
                            .font(.system(size: 20, weight: .medium))
                            .foregroundStyle(Theme.onInk)
                            .frame(width: 44, height: 44)
                            .background(Theme.ink, in: Circle())
                        Text("Describe what you photograph").font(.caption).foregroundStyle(Theme.muted).multilineTextAlignment(.center)
                    }
                    .padding()
                }
            Text("New collection").font(.headline)
            Text(verbatim: " ").font(.caption)
        }
    }
}
