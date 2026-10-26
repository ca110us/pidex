import SwiftUI

/// Setting a collection up by talking it through: describe what you
/// photograph, get a draft, ask for changes, then create it. The
/// conversation stays with the collection, apart from the photo
/// conversations; opening its setup later carries on where it left off.
struct SetupChatView: View {
    enum Mode {
        /// A new collection; drafts are made with the current one's client
        /// and the conversation is kept once the collection exists.
        case create
        /// The collection behind this client.
        case edit(name: String)
    }

    let mode: Mode
    let api: APIClient
    /// Called once the collection is created or changed, with a line to
    /// show.
    let onDone: (String) -> Void

    @Environment(AppModel.self) private var app
    @State private var messages: [Pidex_V1_SetupMessage] = []
    @State private var input = ""
    @State private var working: String?
    /// While a draft is made: which step to say it is at.
    @State private var stage = 0
    @State private var renaming = false
    @State private var newName = ""
    @State private var error: String?
    @State private var loading = false
    @State private var showingDetails: ScenarioConfig?
    @State private var canChange = true

    private var isCreate: Bool { if case .create = mode { true } else { false } }

    /// The latest draft, unless a version was published after it.
    private var openDraft: (index: Int, draft: Pidex_V1_DraftConfigResponse)? {
        guard let i = messages.lastIndex(where: { $0.role == .assistant && $0.hasDraft }) else { return nil }
        if messages[(i + 1)...].contains(where: { $0.role == .event && $0.configVersion > 0 }) { return nil }
        return (i, messages[i].draft)
    }

    /// The draft the next change builds on: the latest one, applied or not.
    private var latestDraft: Pidex_V1_DraftConfigResponse? {
        messages.last { $0.role == .assistant && $0.hasDraft }?.draft
    }

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 14) {
                    intro
                    ForEach(Array(messages.enumerated()), id: \.offset) { i, m in
                        message(m, index: i).id(i)
                    }
                    if let working {
                        AssistantBubble {
                            HStack(spacing: 10) {
                                ProgressView()
                                Text(verbatim: working)
                                    .foregroundStyle(Theme.muted)
                                    .contentTransition(.opacity)
                                    .animation(.easeInOut, value: working)
                            }
                            .font(.subheadline)
                        }
                        .id("working")
                    }
                    if let error {
                        AssistantBubble {
                            Label(error, systemImage: "exclamationmark.triangle").foregroundStyle(Theme.danger)
                        }
                    }
                }
                .padding(.vertical)
            }
            .scrollDismissesKeyboard(.interactively)
            .onChange(of: messages.count) { _, n in
                withAnimation { proxy.scrollTo(n - 1, anchor: .top) }
            }
            .onChange(of: working) { _, w in
                if w != nil { withAnimation { proxy.scrollTo("working", anchor: .bottom) } }
            }
        }
        .overlay { if loading { ProgressView() } }
        .safeAreaInset(edge: .bottom) {
            if canChange {
                // The draft card opens the full details; nothing else to offer.
                Composer(text: $input, busy: working != nil, prompt: latestDraft == nil && isCreate ? "Describe what you photograph" : "What should change?") {
                    EmptyView()
                } onSend: { text in
                    Task { await send(text) }
                }
            }
        }
        .navigationTitle(isCreate ? Text("New collection") : Text("Setup"))
        .navigationBarTitleDisplayMode(.inline)
        .sheet(item: Binding(get: { showingDetails.map(Details.init) }, set: { showingDetails = $0?.config })) { d in
            NavigationStack {
                List { ConfigSections(config: d.config) }
                    .navigationTitle(Text(verbatim: d.config.name))
                    .navigationBarTitleDisplayMode(.inline)
                    .toolbar {
                        ToolbarItem(placement: .confirmationAction) {
                            Button("Done") { showingDetails = nil }
                        }
                    }
            }
        }
        .alert("Name", isPresented: $renaming) {
            TextField("Name", text: $newName)
            Button("Cancel", role: .cancel) {}
            Button("Save") { rename(to: newName) }
        }
        .task { await load() }
    }

    struct Details: Identifiable {
        let config: ScenarioConfig
        var id: String { config.name }
    }

    // MARK: Messages

    /// What to say first. A new collection starts from a question and a few
    /// examples, not from choices: how subjects are told apart is worked
    /// out from what people say, and shown on the draft to change.
    @ViewBuilder
    private var intro: some View {
        switch mode {
        case .create where latestDraft == nil && working == nil:
            VStack(alignment: .leading, spacing: 6) {
                Text("What do you photograph?").font(.title2.bold())
                Text("Say it in your own words. Pidex sets the collection up, and you can change anything later.")
                    .font(.subheadline)
                    .foregroundStyle(Theme.muted)
            }
            .padding(.horizontal)
            .padding(.top, 8)
            VStack(spacing: 8) {
                ForEach(Self.examples) { ex in
                    Button { Task { await send(String(localized: ex.description)) } } label: {
                        HStack(spacing: 12) {
                            Image(systemName: ex.icon)
                                .font(.system(size: 16, weight: .medium))
                                .frame(width: 36, height: 36)
                                .background(Theme.raised, in: Circle())
                            Text(ex.title).font(.subheadline.weight(.medium))
                            Spacer(minLength: 4)
                            Image(systemName: "arrow.up.right").font(.caption.weight(.semibold)).foregroundStyle(Theme.muted)
                        }
                        .foregroundStyle(Theme.ink)
                        .padding(.vertical, 10)
                        .padding(.horizontal, 12)
                        .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("example-\(ex.id)")
                }
            }
            .padding(.horizontal)
        case .create:
            EmptyView()
        case .edit(let name):
            AssistantBubble {
                Text("This is where “\(name)” was set up. Say what to change, such as a missing tag or another way to tell them apart; nothing changes until you apply it.")
            }
        }
        if !canChange {
            Text("Only owners and admins can change this collection's scenario.")
                .font(.footnote)
                .foregroundStyle(.secondary)
                .padding(.horizontal)
        }
    }

    /// Ways in, for someone looking at a blank page: each says enough for a
    /// good first draft.
    struct Example: Identifiable {
        let id: String
        let icon: String
        let title: LocalizedStringResource
        let description: LocalizedStringResource
    }

    static let examples = [
        Example(id: "pets", icon: "pawprint", title: "My pets at home",
                description: "My cats at home. File every photo under the right cat, and let me find them later by what they are doing."),
        Example(id: "clinic", icon: "cross.case", title: "Wounds at a clinic",
                description: "Wound photos at our clinic. Each patient wears a wristband with a name and a record number; we follow healing over weeks."),
        Example(id: "site", icon: "building.2", title: "Units on a renovation site",
                description: "Renovation work: each project has units with door numbers, and we photograph stages such as waterproofing and tiling."),
        Example(id: "trip", icon: "airplane", title: "Photos from a trip",
                description: "Photos from my trip to Dalian. Keep them by where and when they were taken, and let me find them by place, day or what they show."),
        Example(id: "gear", icon: "wrench.and.screwdriver", title: "Equipment with serial numbers",
                description: "Equipment maintenance: each device has a label with its serial number; we photograph its condition and every repair."),
    ]

    @ViewBuilder
    private func message(_ m: Pidex_V1_SetupMessage, index: Int) -> some View {
        switch m.role {
        case .user:
            UserBubble(text: m.text)
        case .event:
            Text(verbatim: m.text)
                .font(.caption)
                .foregroundStyle(.secondary)
                .frame(maxWidth: .infinity)
        default:
            if m.hasDraft, let config = try? ScenarioConfig(json: m.draft.configJson) {
                draftBubble(m.draft, config, open: openDraft?.index == index)
            } else {
                AssistantBubble { Text(verbatim: m.text) }
            }
        }
    }

    /// The draft as one card: what it is called, how each kind of subject
    /// is told apart, its tags, and one button. Everything else is a tap
    /// away (Details), or a sentence away (the composer).
    private func draftBubble(_ d: Pidex_V1_DraftConfigResponse, _ config: ScenarioConfig, open: Bool) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            DraftPreview(
                config: config, summary: d.summary, warnings: d.warnings, editable: open && canChange && working == nil,
                onRename: { newName = config.name; renaming = true },
                onIdentifyBy: { st, byLook in
                    Task {
                        await send(byLook
                                   ? String(localized: "Tell \(st.name) apart by how they look, not by a label.")
                                   : String(localized: "Tell \(st.name) apart by a label or number on them, not by how they look."))
                    }
                },
                onDetails: { showingDetails = config }
            ) {
                if open && canChange {
                    Button {
                        Task { await commit(d, config) }
                    } label: {
                        Label(isCreate ? "Create and shoot" : "Apply changes", systemImage: isCreate ? "camera" : "checkmark")
                    }
                    .buttonStyle(.inkWide)
                    .disabled(working != nil)
                    .accessibilityIdentifier("commitSetup")
                }
            }
            .padding(.horizontal)
        }
    }

    // MARK: Actions

    private func load() async {
        guard case .edit = mode else { return }
        loading = true
        defer { loading = false }
        do {
            async let conversation = api.setupConversation()
            async let team = api.team()
            messages = try await conversation
            canChange = ["owner", "admin"].contains(try await team.role)
        } catch {
            self.error = error.localizedDescription
        }
    }

    /// What is happening while a draft is made, a step at a time: the model
    /// takes a while, and a spinner alone says nothing.
    private static let stages: [LocalizedStringResource] = [
        "Understanding what you photograph…",
        "Working out how to tell them apart…",
        "Choosing tags you will search by…",
        "Naming it and checking it…",
        "Almost there…",
    ]

    private func send(_ text: String) async {
        var asked = Pidex_V1_SetupMessage()
        asked.role = .user
        asked.text = text
        messages.append(asked)
        input = ""
        error = nil
        stage = 0
        // The draft comes into view, not under the keyboard.
        UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
        working = String(localized: Self.stages[0])
        let ticker = Task { @MainActor in
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(3.5))
                guard !Task.isCancelled, working != nil else { return }
                stage = min(stage + 1, Self.stages.count - 1)
                working = String(localized: Self.stages[stage])
            }
        }
        defer {
            ticker.cancel()
            working = nil
        }
        do {
            // How subjects are told apart is worked out from the words;
            // the draft shows it, to change.
            let draft = try await api.draftConfig(
                description: text, base: latestDraft?.configJson, forNewTeam: isCreate, identifyBy: ""
            )
            var answer = Pidex_V1_SetupMessage()
            answer.role = .assistant
            answer.text = draft.summary
            answer.draft = draft
            messages.append(answer)
            // A collection that exists keeps every exchange as it happens.
            if !isCreate { try await keep([asked, answer], with: api) }
        } catch {
            self.error = ScenarioText.explain(error)
        }
    }

    private func commit(_ draft: Pidex_V1_DraftConfigResponse, _ config: ScenarioConfig) async {
        working = isCreate ? String(localized: "Creating the collection…") : String(localized: "Publishing…")
        error = nil
        defer { working = nil }
        do {
            if isCreate {
                let (team, token) = try await api.createTeam(name: config.name, configJSON: draft.configJson)
                let switched = app.add(team: team, token: token)
                // The conversation moves into the collection it made.
                if let created = app.workspaces.items.first(where: { $0.id == team.id }), let client = app.api(for: created) {
                    var event = Pidex_V1_SetupMessage()
                    event.role = .event
                    event.text = String(localized: "Created “\(team.name)”")
                    event.configVersion = 1
                    try? await keep(messages + [event], with: client)
                }
                // Straight to the first photos, which is where it files
                // itself; the camera says how, so no toast says it again.
                if switched, (try? app.openCamera(for: team.id, firstShoot: true)) != nil {
                    onDone("")
                } else {
                    onDone(String(localized: "Created “\(team.name)”. Its camera opens once your photos finish uploading."))
                }
            } else {
                let version = try await api.publishConfig(json: draft.configJson)
                var event = Pidex_V1_SetupMessage()
                event.role = .event
                event.text = String(localized: "Published version \(version)")
                event.configVersion = version
                messages.append(event)
                try await keep([event], with: api)
                onDone(String(localized: "Published version \(version). New photos use it from now on."))
            }
        } catch {
            self.error = ScenarioText.explain(error)
        }
    }

    /// A new name for the open draft, kept in it: no model call.
    private func rename(to name: String) {
        let name = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !name.isEmpty, let i = openDraft?.index,
              var json = try? JSONSerialization.jsonObject(with: Data(messages[i].draft.configJson.utf8)) as? [String: Any] else { return }
        json["name"] = name
        guard let data = try? JSONSerialization.data(withJSONObject: json), let text = String(data: data, encoding: .utf8) else { return }
        messages[i].draft.configJson = text
    }

    private func keep(_ list: [Pidex_V1_SetupMessage], with client: APIClient) async throws {
        _ = try await client.addSetupMessages(list)
    }
}

/// A draft at a glance: its name, how each kind of subject is told apart
/// (each changeable), the tags it will search by, and what to act on.
struct DraftPreview<Actions: View>: View {
    let config: ScenarioConfig
    /// What the model says it made, in a few lines; Details has the rest.
    let summary: String
    let warnings: [Pidex_V1_ConfigChange]
    let editable: Bool
    let onRename: () -> Void
    let onIdentifyBy: (ScenarioConfig.SubjectType, Bool) -> Void
    let onDetails: () -> Void
    @ViewBuilder let actions: () -> Actions

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack(spacing: 8) {
                Text(verbatim: config.name).font(.title3.bold()).lineLimit(2)
                if editable {
                    Button(action: onRename) {
                        Image(systemName: "pencil").font(.subheadline.weight(.semibold)).foregroundStyle(Theme.muted)
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel(Text("Rename"))
                }
                Spacer(minLength: 0)
            }
            if !summary.isEmpty {
                Text(verbatim: summary)
                    .font(.footnote)
                    .foregroundStyle(Theme.muted)
                    .lineLimit(3)
                    .padding(.top, -6)
            }
            VStack(alignment: .leading, spacing: 8) {
                ForEach(config.subjectTypes) { st in
                    HStack(spacing: 10) {
                        Image(systemName: st.byAppearance ? "eye" : "barcode.viewfinder")
                            .font(.system(size: 14, weight: .medium))
                            .frame(width: 30, height: 30)
                            .background(Theme.raised, in: Circle())
                        VStack(alignment: .leading, spacing: 1) {
                            Text(verbatim: st.name).font(.subheadline.weight(.semibold))
                            Text(st.byAppearance
                                 ? String(localized: "Told apart by how they look")
                                 : String(localized: "By \(st.identityFields.map(\.name).formatted())"))
                                .font(.caption)
                                .foregroundStyle(Theme.muted)
                        }
                        Spacer(minLength: 4)
                        if editable {
                            Menu {
                                Button { onIdentifyBy(st, true) } label: { Label("By how they look", systemImage: "eye") }
                                    .disabled(st.byAppearance)
                                Button { onIdentifyBy(st, false) } label: { Label("By a label or number", systemImage: "barcode.viewfinder") }
                                    .disabled(!st.byAppearance)
                            } label: {
                                Text("Change").font(.caption.weight(.semibold))
                                    .padding(.vertical, 5).padding(.horizontal, 10)
                                    .background(Theme.raised, in: Capsule())
                            }
                            .foregroundStyle(Theme.ink)
                        }
                    }
                }
            }
            if !config.tags.isEmpty {
                TagFlow(names: config.tags.prefix(10).map(\.name), more: max(0, config.tags.count - 10))
            }
            ForEach(Array(warnings.enumerated()), id: \.offset) { _, w in
                Label(ScenarioText.describe(w), systemImage: "exclamationmark.triangle")
                    .font(.footnote)
                    .foregroundStyle(Theme.attention)
            }
            actions()
            Button("Details", action: onDetails)
                .font(.footnote.weight(.medium))
                .foregroundStyle(Theme.muted)
                .frame(maxWidth: .infinity)
        }
        .padding(16)
        .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
    }
}

/// Tag names as small chips, wrapping to as many lines as they need.
struct TagFlow: View {
    let names: [String]
    var more = 0

    var body: some View {
        Flow(spacing: 6) {
            ForEach(Array(names.enumerated()), id: \.offset) { _, n in
                chip(Text(verbatim: n))
            }
            if more > 0 { chip(Text(verbatim: "+\(more)")).foregroundStyle(Theme.muted) }
        }
    }

    private func chip(_ t: Text) -> some View {
        t.font(.caption)
            .padding(.vertical, 5)
            .padding(.horizontal, 10)
            .background(Theme.paper, in: Capsule())
            .overlay(Capsule().strokeBorder(Theme.hairline))
    }
}

/// Lays its views out in rows, wrapping when a row is full.
struct Flow: Layout {
    var spacing: CGFloat = 6

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let rows = rows(subviews, width: proposal.width ?? .infinity)
        let height = rows.map(\.height).reduce(0, +) + spacing * CGFloat(max(0, rows.count - 1))
        return CGSize(width: proposal.width ?? rows.map(\.width).max() ?? 0, height: height)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        var y = bounds.minY
        for row in rows(subviews, width: bounds.width) {
            var x = bounds.minX
            for i in row.indices {
                let size = subviews[i].sizeThatFits(.unspecified)
                subviews[i].place(at: CGPoint(x: x, y: y), proposal: ProposedViewSize(size))
                x += size.width + spacing
            }
            y += row.height + spacing
        }
    }

    private struct Row {
        var indices: [Int] = []
        var width: CGFloat = 0
        var height: CGFloat = 0
    }

    private func rows(_ subviews: Subviews, width: CGFloat) -> [Row] {
        var rows: [Row] = []
        var row = Row()
        for i in subviews.indices {
            let size = subviews[i].sizeThatFits(.unspecified)
            if !row.indices.isEmpty && row.width + spacing + size.width > width {
                rows.append(row)
                row = Row()
            }
            row.width += (row.indices.isEmpty ? 0 : spacing) + size.width
            row.height = max(row.height, size.height)
            row.indices.append(i)
        }
        if !row.indices.isEmpty { rows.append(row) }
        return rows
    }
}
