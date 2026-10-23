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
    /// How subjects are told apart, asked before the first draft.
    @State private var identifyBy = ""
    @State private var working: String?
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
                            HStack(spacing: 8) {
                                ProgressView()
                                Text(verbatim: working).foregroundStyle(.secondary)
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
        .task { await load() }
    }

    struct Details: Identifiable {
        let config: ScenarioConfig
        var id: String { config.name }
    }

    // MARK: Messages

    /// What to say first, and for a new collection the one choice that
    /// shapes everything else.
    @ViewBuilder
    private var intro: some View {
        AssistantBubble {
            switch mode {
            case .create:
                Text("Tell me what you photograph, what you will want to find later, and how you tell one from another. I will set the collection up, and you can keep changing it here, now or later.")
            case .edit(let name):
                Text("This is where “\(name)” was set up. Say what to change, such as a missing tag or another way to tell them apart; nothing changes until you apply it.")
            }
        }
        if isCreate && latestDraft == nil {
            // Every choice in view at once, each saying what it means.
            VStack(alignment: .leading, spacing: 8) {
                Text("How do you tell them apart?").font(.footnote.weight(.semibold)).foregroundStyle(.secondary)
                option("By how they look", detail: "Pets, mugs, plants: no labels on them.", tag: "appearance", icon: "eye")
                option("Labels, tags or numbers", detail: "Wristbands, serial numbers, door numbers, asset tags.", tag: "code", icon: "barcode.viewfinder")
                option("Auto", detail: "Worked out from what you describe.", tag: "", icon: "sparkles")
            }
            .padding(.horizontal)
        }
        if !canChange {
            Text("Only owners and admins can change this collection's scenario.")
                .font(.footnote)
                .foregroundStyle(.secondary)
                .padding(.horizontal)
        }
    }

    private func option(_ title: LocalizedStringKey, detail: LocalizedStringKey, tag: String, icon: String) -> some View {
        let on = identifyBy == tag
        return Button { identifyBy = tag } label: {
            HStack(spacing: 12) {
                Image(systemName: icon)
                    .font(.system(size: 17, weight: .medium))
                    .foregroundStyle(on ? Theme.onInk : Theme.ink)
                    .frame(width: 38, height: 38)
                    .background(on ? Theme.ink : Theme.raised, in: Circle())
                // Two lines kept for every description, so the cards are one
                // height whatever the language.
                VStack(alignment: .leading, spacing: 2) {
                    Text(title).font(.subheadline.weight(.semibold)).lineLimit(1)
                    Text(detail).font(.caption).foregroundStyle(Theme.muted)
                        .lineLimit(2, reservesSpace: true)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                Image(systemName: on ? "checkmark.circle.fill" : "circle")
                    .font(.title3)
                    .foregroundStyle(on ? Theme.ink : Theme.muted.opacity(0.6))
            }
            .padding(14)
            .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
            .overlay {
                RoundedRectangle(cornerRadius: Theme.radius, style: .continuous)
                    .strokeBorder(on ? Theme.ink : Theme.hairline, lineWidth: on ? 1.5 : 1)
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityAddTraits(on ? .isSelected : [])
        .accessibilityIdentifier("identifyBy-\(tag.isEmpty ? "auto" : tag)")
    }

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

    private func draftBubble(_ d: Pidex_V1_DraftConfigResponse, _ config: ScenarioConfig, open: Bool) -> some View {
        AssistantBubble {
            Text(verbatim: d.summary)
            DraftCard(config: config) { showingDetails = config }
            ForEach(Array(d.warnings.enumerated()), id: \.offset) { _, w in
                Label(ScenarioText.describe(w), systemImage: "exclamationmark.triangle")
                    .font(.footnote)
                    .foregroundStyle(Theme.attention)
            }
            if !d.testTips.isEmpty {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Try it").font(.footnote.weight(.semibold))
                    ForEach(Array(d.testTips.enumerated()), id: \.offset) { i, tip in
                        Text(verbatim: "\(i + 1). \(tip)").font(.footnote)
                    }
                }
                .foregroundStyle(.secondary)
            }
            if open && canChange {
                Button {
                    Task { await commit(d, config) }
                } label: {
                    Text(isCreate ? "Create collection" : "Apply changes")
                }
                .buttonStyle(.inkWide)
                .disabled(working != nil)
                .accessibilityIdentifier("commitSetup")
            }
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

    private func send(_ text: String) async {
        var asked = Pidex_V1_SetupMessage()
        asked.role = .user
        asked.text = text
        messages.append(asked)
        working = String(localized: "Designing the scenario. This takes about 20 seconds.")
        error = nil
        defer { working = nil }
        do {
            let draft = try await api.draftConfig(
                description: text, base: latestDraft?.configJson, forNewTeam: isCreate,
                identifyBy: latestDraft == nil ? identifyBy : ""
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
                onDone(switched
                       ? String(localized: "Created “\(team.name)”. The camera now shoots for it.")
                       : String(localized: "Created “\(team.name)”. Its camera opens once your photos finish uploading."))
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

    private func keep(_ list: [Pidex_V1_SetupMessage], with client: APIClient) async throws {
        _ = try await client.addSetupMessages(list)
    }
}

/// A draft at a glance: what photos are filed under and how each is
/// recognized, and the tags.
struct DraftCard: View {
    let config: ScenarioConfig
    let onDetails: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(verbatim: config.name).font(.headline)
            ForEach(config.subjectTypes) { st in
                Label {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(verbatim: st.name).font(.subheadline.weight(.semibold))
                        Text(st.byAppearance
                             ? String(localized: "By how it looks")
                             : st.identityFields.map(\.name).joined(separator: " · "))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                } icon: {
                    Image(systemName: st.byAppearance ? "eye" : "barcode.viewfinder")
                }
            }
            Text(verbatim: config.tags.prefix(8).map(\.name).joined(separator: " · ") + (config.tags.count > 8 ? " …" : ""))
                .font(.caption)
                .foregroundStyle(.secondary)
            Button("Full details", action: onDetails)
                .font(.footnote)
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Theme.paper, in: RoundedRectangle(cornerRadius: 16, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 16, style: .continuous).strokeBorder(Theme.hairline))
    }
}
