import SwiftUI

/// Finding and asking about photos as a conversation, with one collection or
/// all of them; the collection is picked at the top. Each message may build
/// on the ones before ("only the back ones", "and the tablet?").
struct AskView: View {
    @Environment(AppModel.self) private var app
    @State private var route: AskRoute
    /// The library of the collection this was opened from, so photos opened
    /// here and there stay in step.
    private let library: LibraryModel?
    @State private var model: AskModel?
    @State private var input = ""
    @State private var showingHistory = false
    @State private var loaded: UUID?

    init(route: AskRoute, library: LibraryModel? = nil) {
        _route = State(initialValue: route)
        self.library = library
    }

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 20) {
                    ForEach(model?.turns ?? []) { turn in
                        TurnView(turn: turn, many: model?.asksMany ?? false).id(turn.id)
                    }
                    if let error = model?.error {
                        Label(error, systemImage: "exclamationmark.triangle")
                            .font(.footnote)
                            .foregroundStyle(Theme.danger)
                            .padding(.horizontal)
                    }
                }
                .padding(.vertical)
            }
            .scrollDismissesKeyboard(.interactively)
            .onChange(of: model?.turns.count) { _, _ in
                guard let id = model?.turns.last?.id else { return }
                withAnimation { proxy.scrollTo(id, anchor: .top) }
            }
        }
        .overlay {
            if model?.loading ?? true {
                ProgressView()
            } else if model?.turns.isEmpty ?? false {
                welcome
            }
        }
        .safeAreaInset(edge: .bottom) {
            Composer(text: $input, busy: model?.busy ?? true, prompt: prompt) {
                Button { start(route.scope) } label: { Label("New conversation", systemImage: "square.and.pencil") }
                Button { showingHistory = true } label: { Label("Conversations", systemImage: "clock.arrow.circlepath") }
                if case .collection(let id) = route.scope {
                    Button { try? app.openCamera(for: id) } label: { Label("Take photos", systemImage: "camera") }
                }
            } onSend: { text in
                guard let model else { return }
                Task { await model.send(text) }
            }
        }
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .principal) { scopeMenu }
            ToolbarItem(placement: .topBarTrailing) {
                Button { start(route.scope) } label: { Image(systemName: "square.and.pencil") }
                    .disabled(model?.turns.isEmpty ?? true)
                    .accessibilityLabel(Text("New conversation"))
                    .accessibilityIdentifier("newConversation")
            }
        }
        .sheet(isPresented: $showingHistory) {
            NavigationStack {
                ConversationListView(scope: .all) { picked in
                    route = picked
                    showingHistory = false
                }
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button("Done") { showingHistory = false }
                    }
                }
            }
        }
        // Again on coming back from a photo, which must not start over.
        .task(id: route.id) {
            guard loaded != route.id else { return }
            loaded = route.id
            let m = app.askModel(for: route, library: library)
            model = m
            input = ""
            for p in m.parties { Task { await p.library.loadConfig() } }
            await m.load()
            if let first = route.firstMessage, m.turns.isEmpty {
                Task { await m.send(first) }
            }
        }
    }

    /// Which collections to ask, like picking a model in a chat app. Another
    /// pick starts a new conversation, since each collection keeps its own.
    private var scopeMenu: some View {
        Menu {
            Picker(selection: Binding(get: { route.scope }, set: { start($0) })) {
                if app.workspaces.items.count > 1 {
                    Label("All collections", systemImage: "square.stack.3d.up").tag(AskScope.all)
                }
                ForEach(app.workspaces.items) { w in
                    Text(verbatim: w.name).tag(AskScope.collection(w.id))
                }
            } label: {
                Text("Ask")
            }
            .pickerStyle(.inline)
        } label: {
            HStack(spacing: 4) {
                Text(verbatim: scopeName).font(.headline).lineLimit(1)
                Image(systemName: "chevron.down")
                    .font(.caption.weight(.bold))
                    .foregroundStyle(.secondary)
            }
            .foregroundStyle(.primary)
            .padding(.horizontal, 12)
            .padding(.vertical, 6)
            .contentShape(Capsule())
        }
        .tint(.primary)
        .accessibilityIdentifier("askScope")
    }

    private var scopeName: String {
        switch route.scope {
        case .all: String(localized: "All collections")
        case .collection(let id): app.workspaces.items.first { $0.id == id }?.name ?? ""
        }
    }

    private var prompt: LocalizedStringKey {
        route.scope == .all ? "Ask all collections" : "Ask or find photos"
    }

    private var welcome: some View {
        VStack(spacing: 10) {
            PidexMark(height: 40)
                .foregroundStyle(.primary)
            Group {
                if route.scope == .all {
                    Text("Ask all your collections")
                } else {
                    Text("Ask “\(scopeName)”")
                }
            }
            .font(.title2.bold())
            .multilineTextAlignment(.center)
            Text("Find photos, ask what they show, or what is not photographed yet. Follow-ups build on what came before.")
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
        }
        .padding(32)
        .allowsHitTesting(false)
    }

    private func start(_ scope: AskScope) {
        route = AskRoute(scope: scope)
    }
}

/// A message and the replies to it: one collection's, or each collection's
/// under its name, those with nothing to show folded into one bubble.
struct TurnView: View {
    let turn: AskModel.Turn
    let many: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            UserBubble(text: turn.text)
            if many {
                // Only collections that found something are shown; when none
                // did, one line says so.
                ForEach(turn.replies.filter { $0.message.map { !Self.isEmpty($0) } ?? false }) { r in
                    VStack(alignment: .leading, spacing: 4) {
                        NavigationLink {
                            CollectionPage(workspace: r.workspace)
                        } label: {
                            Label { Text(verbatim: r.workspace.name) } icon: { Image(systemName: "rectangle.stack") }
                                .font(.caption.weight(.semibold))
                                .foregroundStyle(.secondary)
                        }
                        .buttonStyle(.plain)
                        .padding(.horizontal, 24)
                        ReplyView(reply: r)
                    }
                }
                let pending = turn.replies.filter(\.pending)
                if !pending.isEmpty {
                    Looking(text: String(localized: "Looking through \(pending.map(\.workspace.name).formatted())…"))
                }
                let found = turn.replies.contains { $0.message.map { !Self.isEmpty($0) } ?? false }
                let answered = turn.replies.contains { $0.message != nil }
                let unrelated = turn.replies.contains { $0.message?.kind == .unrelated }
                // Every collection has answered and none has anything.
                let nothing = pending.isEmpty && answered && !found
                let failed = turn.replies.filter { $0.error != nil }
                if nothing || !failed.isEmpty {
                    AssistantBubble {
                        if nothing {
                            Text(unrelated ? MessageView.unrelated : String(localized: "No photos found."))
                                .foregroundStyle(.secondary)
                        }
                        ForEach(failed) { r in
                            Label(String(localized: "\(r.workspace.name): \(r.error ?? "")"), systemImage: "exclamationmark.triangle")
                                .foregroundStyle(Theme.danger)
                        }
                    }
                    .font(.subheadline)
                }
            } else {
                ForEach(turn.replies) { ReplyView(reply: $0) }
            }
        }
    }

    /// A reply with no photos to show (or not about photos at all), which a
    /// collection asked among others does not show.
    static func isEmpty(_ m: Pidex_V1_ChatMessage) -> Bool {
        m.kind != .missing && m.photos.isEmpty
    }
}

struct ReplyView: View {
    let reply: AskModel.Reply

    var body: some View {
        if let m = reply.message {
            MessageView(message: m, library: reply.library)
        } else if let error = reply.error {
            AssistantBubble {
                Label(error, systemImage: "exclamationmark.triangle").foregroundStyle(Theme.danger)
            }
        } else {
            Looking(text: String(localized: "Looking through the photos…"))
        }
    }
}

private struct Looking: View {
    let text: String

    var body: some View {
        AssistantBubble {
            HStack(spacing: 8) {
                ProgressView()
                Text(verbatim: text).foregroundStyle(.secondary)
            }
            .font(.subheadline)
        }
    }
}

/// Where a message is written or spoken, as in chat apps: a "+" for more on
/// the left, then the text with the microphone and send.
struct Composer<Actions: View>: View {
    @Binding var text: String
    var busy = false
    var prompt: LocalizedStringKey = "Ask or find photos"
    @ViewBuilder let actions: () -> Actions
    let onSend: (String) -> Void
    @State private var dictation = Dictation()
    @State private var problem: String?
    /// Counts dictations: words recognized after a message was sent belong
    /// to an older one and must not refill the field.
    @State private var dictationRound = 0

    /// One line of text, and the buttons beside it, share this height, so
    /// they line up on its middle.
    private let rowHeight: CGFloat = 46

    private var trimmed: String { text.trimmingCharacters(in: .whitespacesAndNewlines) }
    private var listening: Bool { dictation.state == .listening }

    var body: some View {
        VStack(spacing: 4) {
            if let problem {
                Text(problem).font(.caption).foregroundStyle(.secondary)
            }
            HStack(alignment: .bottom, spacing: 8) {
                // Without actions there is no menu: a button that opens
                // nothing is clutter.
                if Actions.self != EmptyView.self {
                    Menu {
                        actions()
                    } label: {
                        Image(systemName: "plus")
                            .font(.system(size: 19, weight: .medium))
                            .foregroundStyle(.primary)
                            .frame(width: 46, height: 46)
                            .contentShape(Circle())
                    }
                    .tint(.primary)
                    .glassSurface(Circle())
                    .accessibilityLabel(Text("More"))
                    .accessibilityIdentifier("composerMore")
                }

                // Bottom-aligned, each button in a row-high frame: on one line
                // everything is centred, and with more lines the buttons stay
                // by the last one.
                HStack(alignment: .bottom, spacing: 2) {
                    TextField(prompt, text: $text, axis: .vertical)
                        .lineLimit(1...6)
                        .padding(.leading, 16)
                        .padding(.vertical, 12)
                        .frame(minHeight: rowHeight)
                        .submitLabel(.send)
                        .onSubmit(send)
                        .accessibilityIdentifier("chatInput")
                    Button {
                        Task { await toggleDictation() }
                    } label: {
                        Image(systemName: listening ? "stop.circle.fill" : "mic")
                            .font(.system(size: 18))
                            .foregroundStyle(listening ? Theme.danger : Theme.muted)
                            .frame(width: 36, height: rowHeight)
                            .contentShape(Rectangle())
                    }
                    .contextMenu { DictationLanguagePicker() }
                    .accessibilityLabel(listening ? Text("Stop") : Text("Speak"))
                    .accessibilityHint(Text("Touch and hold to choose the language"))
                    if !trimmed.isEmpty {
                        Button(action: send) {
                            Image(systemName: "arrow.up")
                                .font(.system(size: 16, weight: .semibold))
                                .foregroundStyle(Theme.onInk)
                                .frame(width: 34, height: 34)
                                .background(Theme.ink.opacity(busy ? 0.3 : 1), in: Circle())
                                .frame(height: rowHeight)
                        }
                        .disabled(busy)
                        .accessibilityLabel(Text("Send"))
                        .accessibilityIdentifier("chatSend")
                        .transition(.scale.combined(with: .opacity))
                    }
                }
                .padding(.trailing, 6)
                .glassSurface(RoundedRectangle(cornerRadius: 23, style: .continuous))
                .animation(.snappy(duration: 0.2), value: trimmed.isEmpty)
            }
        }
        .padding(.horizontal, 12)
        .padding(.top, 6)
        .padding(.bottom, 8)
        .sensoryFeedback(.impact(weight: .light), trigger: listening)
    }

    private func send() {
        let t = trimmed
        guard !t.isEmpty, !busy else { return }
        // The recognizer's last words arrive after it stops; they belong to
        // the message just sent.
        dictationRound += 1
        if listening { dictation.stop() }
        text = ""
        onSend(t)
    }

    /// Speaking fills in the text to read over before sending.
    private func toggleDictation() async {
        if listening {
            dictation.stop()
            return
        }
        let language = UserDefaults.standard.string(forKey: "dictationLanguage").flatMap { $0.isEmpty ? nil : $0 }
            ?? Bundle.main.preferredLocalizations.first ?? "en"
        problem = nil
        dictationRound += 1
        let round = dictationRound
        await dictation.start(locale: Dictation.speechLocale(for: language)) { transcript in
            if round == dictationRound { text = transcript }
        }
        if case .unavailable(let message) = dictation.state {
            problem = message
        }
    }
}

extension View {
    /// Liquid Glass where the system has it, a material before that.
    @ViewBuilder
    func glassSurface<S: Shape>(_ shape: S) -> some View {
        if #available(iOS 26, *) {
            glassEffect(.regular.interactive(), in: shape)
        } else {
            background(.regularMaterial, in: shape)
                .overlay(shape.stroke(Color(.separator).opacity(0.5), lineWidth: 0.5))
        }
    }
}

/// The user's message, on the right.
struct UserBubble: View {
    let text: String

    var body: some View {
        HStack {
            Spacer(minLength: 56)
            Text(verbatim: text)
                .foregroundStyle(Theme.onInk)
                .padding(.vertical, 11)
                .padding(.horizontal, 16)
                .background(Theme.ink, in: RoundedRectangle(cornerRadius: 22, style: .continuous))
                .textSelection(.enabled)
        }
        .padding(.horizontal)
    }
}

/// A reply's bubble, on the left.
struct AssistantBubble<Content: View>: View {
    @ViewBuilder let content: Content

    var body: some View {
        HStack {
            VStack(alignment: .leading, spacing: 8) { content }
                .padding(.vertical, 12)
                .padding(.horizontal, 16)
                .background(Theme.surface, in: RoundedRectangle(cornerRadius: 22, style: .continuous))
            Spacer(minLength: 40)
        }
        .padding(.horizontal)
    }
}

/// A reply: a bubble saying what was found, the answer, or what is missing,
/// and the photos in a block of their own. A reply never holds more than a
/// few photos; the rest open as an album.
struct MessageView: View {
    let message: Pidex_V1_ChatMessage
    let library: LibraryModel

    /// Subjects named in the bubble; the rest open on their own page.
    private let namedMissing = 6

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            AssistantBubble {
                if message.subjectNotFound && message.kind != .missing && message.kind != .unrelated {
                    Label(String(localized: "Nothing is filed under “\(message.filters.subjectQuery)”, so these photos match the words instead."),
                          systemImage: "info.circle")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
                switch message.kind {
                case .answer:
                    Text(verbatim: message.text)
                        .textSelection(.enabled)
                        .accessibilityIdentifier("answer")
                case .missing:
                    missing
                case .unrelated:
                    Text(Self.unrelated).foregroundStyle(.secondary)
                default:
                    Text(total == 0 ? String(localized: "No photos match.") : String(localized: "Found \(total) photos"))
                        .font(.body.weight(.medium))
                }
                if !understood.isEmpty, message.kind != .unrelated {
                    Label { Text(verbatim: understood) } icon: { Image(systemName: "line.3.horizontal.decrease.circle") }
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            switch message.kind {
            case .answer:
                CitedPhotos(photos: message.photos, library: library)
            case .missing:
                EmptyView()
            default:
                if !message.photos.isEmpty {
                    PhotoPreview(message: message, total: total, library: library)
                }
            }
        }
    }

    private var total: Int { max(Int(message.photoTotal), message.photos.count) }

    /// What a request not about the photos is told.
    static let unrelated = String(localized: "I can only find your photos and answer questions about them.")

    private var missing: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(message.missing.isEmpty ? String(localized: "Every one has a matching photo.")
                 : String(localized: "Not photographed yet: \(message.missing.count)"))
                .font(.body.weight(.medium))
            ForEach(message.missing.prefix(namedMissing), id: \.id) { s in
                NavigationLink {
                    SubjectPhotosView(subject: s, model: library)
                } label: {
                    Label { Text(verbatim: s.displayName) } icon: { Image(systemName: "person.text.rectangle") }
                        .font(.subheadline)
                }
            }
            if message.missing.count > namedMissing {
                NavigationLink {
                    List(message.missing, id: \.id) { s in
                        NavigationLink(s.displayName) { SubjectPhotosView(subject: s, model: library) }
                    }
                    .navigationTitle(Text("Not photographed yet: \(message.missing.count)"))
                } label: {
                    Text("See all \(message.missing.count)").font(.subheadline.weight(.medium))
                }
            }
        }
    }

    /// How the message was read: what a follow-up can adjust.
    private var understood: String {
        let f = message.filters
        var parts: [String] = []
        if !f.subjectQuery.isEmpty { parts.append(f.subjectQuery) }
        parts += f.tags.map { library.tagName($0) }
        if f.hasFrom || f.hasTo {
            let style = Date.FormatStyle(date: .abbreviated, time: .omitted)
            let from = f.hasFrom ? f.from.date.formatted(style) : "…"
            let to = f.hasTo ? f.to.date.addingTimeInterval(-1).formatted(style) : "…"
            parts.append(from == to ? from : "\(from) – \(to)")
        }
        return parts.joined(separator: " · ")
    }
}

/// A few of the photos found, as one rounded block: one, two or three side
/// by side, or four with how many more on the last. Everything found opens
/// as an album.
struct PhotoPreview: View {
    let message: Pidex_V1_ChatMessage
    let total: Int
    let library: LibraryModel

    private let width: CGFloat = 264

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            let shown = Array(message.photos.prefix(total > 4 ? 4 : 3))
            Group {
                if shown.count == 4 {
                    Grid(horizontalSpacing: 2, verticalSpacing: 2) {
                        GridRow { tile(0, shown); tile(1, shown) }
                        GridRow { tile(2, shown); tile(3, shown) }
                    }
                } else {
                    HStack(spacing: 2) {
                        ForEach(shown.indices, id: \.self) { tile($0, shown) }
                    }
                }
            }
            .frame(width: shown.count == 1 ? width * 0.75 : width)
            .clipShape(RoundedRectangle(cornerRadius: 20, style: .continuous))
            if total > shown.count {
                NavigationLink {
                    AlbumView(message: message, total: total, library: library)
                } label: {
                    Label("See all \(total) photos", systemImage: "square.grid.3x3")
                        .font(.subheadline.weight(.medium))
                        .padding(.vertical, 8)
                        .padding(.horizontal, 14)
                        .background(Theme.surface, in: Capsule())
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("seeAllPhotos")
            }
        }
        .padding(.horizontal)
    }

    private func tile(_ i: Int, _ shown: [Pidex_V1_Photo]) -> some View {
        let more = i == 3 ? total - 3 : 0
        return NavigationLink {
            if more > 0 {
                AlbumView(message: message, total: total, library: library)
            } else {
                PhotoViewer(photos: message.photos, start: i, model: library)
            }
        } label: {
            Thumbnail(photo: shown[i])
                .overlay {
                    if more > 0 {
                        ZStack {
                            Color.black.opacity(0.45)
                            Text(verbatim: "+\(more.formatted())")
                                .font(.title2.weight(.semibold))
                                .foregroundStyle(.white)
                        }
                    }
                }
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("photo-\(i)")
    }
}

/// The photos an answer cites, numbered as it cites them, in a row.
struct CitedPhotos: View {
    let photos: [Pidex_V1_Photo]
    let library: LibraryModel

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 6) {
                ForEach(Array(photos.enumerated()), id: \.element.id) { i, photo in
                    NavigationLink {
                        PhotoViewer(photos: photos, start: i, model: library)
                    } label: {
                        Thumbnail(photo: photo)
                            .frame(width: 96, height: 96)
                            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                            .overlay(alignment: .topLeading) {
                                Text(verbatim: "\(i + 1)")
                                    .font(.caption2.bold())
                                    .frame(minWidth: 18, minHeight: 18)
                                    .background(.black.opacity(0.6), in: Circle())
                                    .foregroundStyle(.white)
                                    .padding(5)
                            }
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("photo-\(i)")
                }
            }
            .padding(.horizontal)
        }
    }
}

/// Everything a reply found, like an album: the photos it holds at once,
/// the rest a page at a time as the grid scrolls.
struct AlbumView: View {
    let message: Pidex_V1_ChatMessage
    let total: Int
    let library: LibraryModel
    @State private var photos: [Pidex_V1_Photo]
    @State private var loading = false
    @State private var atEnd = false
    @State private var error: String?

    private static let page = 120

    init(message: Pidex_V1_ChatMessage, total: Int, library: LibraryModel) {
        self.message = message
        self.total = total
        self.library = library
        _photos = State(initialValue: message.photos)
    }

    var body: some View {
        ScrollView {
            LazyVGrid(columns: Array(repeating: GridItem(.flexible(), spacing: 2), count: 4), spacing: 2) {
                ForEach(Array(photos.enumerated()), id: \.element.id) { i, photo in
                    NavigationLink {
                        PhotoViewer(photos: photos, start: i, model: library, more: MorePhotos(total: total) {
                            await more()
                            return photos
                        })
                    } label: {
                        Thumbnail(photo: photo)
                    }
                    .buttonStyle(.plain)
                    .onAppear {
                        if i >= photos.count - 20 { Task { await more() } }
                    }
                }
            }
            if loading {
                ProgressView().padding()
            }
            if let error {
                Label(error, systemImage: "exclamationmark.triangle")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .padding()
            }
        }
        .navigationTitle(Text("\(total) photos"))
        .navigationBarTitleDisplayMode(.inline)
    }

    /// The next page, found by searching again as the reply was read.
    private func more() async {
        guard !loading, !atEnd, photos.count < total else { return }
        loading = true
        defer { loading = false }
        do {
            let r = try await library.api.searchPhotos(filters: message.filters, offset: photos.count, pageSize: Self.page)
            let known = Set(photos.map(\.id))
            let new = r.photos.filter { !known.contains($0.id) }
            photos += new
            atEnd = new.isEmpty
            error = nil
        } catch {
            self.error = error.localizedDescription
        }
    }
}
