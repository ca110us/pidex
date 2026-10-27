import PhotosUI
import SwiftUI

struct CaptureView: View {
    let model: CaptureModel
    /// Subjects to choose from; made with the view so a sheet never opens
    /// before it exists.
    @State private var library: LibraryModel
    @Environment(AppModel.self) private var app
    @State private var showSummary = false
    /// Photos chosen from the library, added in the order chosen.
    @State private var pickerItems: [PhotosPickerItem] = []
    @State private var importing = 0
    @State private var switchProblem: String?
    @State private var choosing = false
    @State private var confirmingClose = false
    @State private var naming = false
    @State private var newName = ""

    init(model: CaptureModel) {
        self.model = model
        _library = State(initialValue: LibraryModel(api: model.api))
    }

    var body: some View {
        ZStack {
            Color.black.ignoresSafeArea()
            switch model.cameraState {
            case .ready:
                CameraPreview(session: model.camera.session).ignoresSafeArea()
            case .starting:
                ProgressView().tint(.white)
            case .unavailable(let reason):
                Text(reason)
                    .multilineTextAlignment(.center)
                    .foregroundStyle(.white)
                    .padding(32)
            }
            VStack(spacing: 0) {
                BannerView(model: model) { choose() } onName: {
                    newName = model.current.name ?? ""
                    naming = true
                }
                workspaceChip
                if let switchProblem {
                    Text(switchProblem)
                        .font(.footnote)
                        .foregroundStyle(.white)
                        .padding(8)
                        .background(.black.opacity(0.5), in: RoundedRectangle(cornerRadius: 10))
                        .padding(.horizontal, 12)
                        .padding(.top, 6)
                }
                Spacer()
                statusLine
                if !model.shots.isEmpty {
                    ShotTray(model: model)
                } else {
                    BatchPill()
                }
                controls
            }
        }
        .navigationTitle(Text(verbatim: ""))
        .toolbar(.hidden, for: .navigationBar)
        .task { await model.startCamera() }
        .onDisappear { model.stopCamera() }
        .sheet(isPresented: $choosing) {
            NavigationStack {
                SubjectPicker(model: library, current: model.current.subjectID ?? "", title: "Who is this?") { subject in
                    choosing = false
                    model.choose(subject)
                }
            }
        }
        .alert("Name it", isPresented: $naming) {
            TextField("Name", text: $newName)
            Button("Cancel", role: .cancel) {}
            // Read the field when Save is tapped, not when the alert opened.
            Button("Save") {
                let name = newName
                Task { await model.name(name) }
            }
        } message: {
            Text("A name you will recognize. If one has it already, these photos go to that one.")
        }
        .confirmationDialog(Text("\(model.shots.count) photos not uploaded yet"), isPresented: $confirmingClose, titleVisibility: .visible) {
            if model.canSend {
                Button("Upload \(model.shots.count) photos") {
                    Task {
                        if await send() > 0 { close() }
                    }
                }
            }
            Button("Discard \(model.shots.count) photos", role: .destructive) {
                model.discardAll()
                close()
            }
            Button("Keep them for later") { close() }
        }
        .navigationDestination(isPresented: $showSummary) {
            if let batch = app.batch {
                SessionSummaryView(batch: batch) {
                    model.newSession()
                    showSummary = false
                    close()
                }
            }
        }
        .onChange(of: pickerItems) { _, items in
            guard !items.isEmpty else { return }
            Task {
                importing = items.count
                // The original file, so its day and place come with it.
                for item in items {
                    if let data = try? await item.loadTransferable(type: Data.self) {
                        await model.importPhoto(data)
                    }
                    importing -= 1
                }
                importing = 0
                pickerItems = []
            }
        }
    }

    /// Which collection the photos go to, picked here, and the way back.
    private var workspaceChip: some View {
        HStack {
            Menu {
                Picker(selection: Binding(get: { app.active?.id ?? "" }, set: shoot(for:))) {
                    ForEach(app.workspaces.items) { w in
                        Text(verbatim: w.name).tag(w.id)
                    }
                } label: {
                    Text("Shoot for")
                }
                .pickerStyle(.inline)
                .disabled(!app.canSwitch)
                if !app.canSwitch {
                    Text("Another collection once \(app.uploadStatus.unfinished) photos are uploaded")
                }
            } label: {
                HStack(spacing: 4) {
                    Label(app.active?.name ?? String(localized: "Workspace"), systemImage: "square.stack.3d.up")
                        .lineLimit(1)
                    Image(systemName: "chevron.down").font(.caption2.weight(.bold))
                }
                .font(.caption.weight(.semibold))
                .padding(.vertical, 6)
                .padding(.horizontal, 10)
                .background(.black.opacity(0.5), in: Capsule())
                .foregroundStyle(.white)
            }
            .accessibilityIdentifier("workspaceChip")
            Spacer()
            Button {
                if model.shots.isEmpty { close() } else { confirmingClose = true }
            } label: {
                Image(systemName: "xmark")
                    .font(.callout.weight(.bold))
                    .padding(10)
                    .background(.black.opacity(0.5), in: Circle())
            }
            .foregroundStyle(.white)
            .accessibilityLabel(Text("Close camera"))
            .accessibilityIdentifier("closeCamera")
        }
        .padding(.horizontal, 12)
        .padding(.top, 8)
    }

    private func close() {
        model.stopCamera()
        app.showingCamera = false
    }

    /// Sends the shots kept and follows them until filed, here and after
    /// the camera closes.
    private func send() async -> Int {
        let n = await model.sendAll()
        if n > 0 { app.track(sent: n, session: model.sessionID, api: model.api) }
        return n
    }

    private func choose() {
        choosing = true
    }

    /// Shooting for another collection: a new capture model, whose camera
    /// starts as this view gives way to a new one.
    private func shoot(for id: String) {
        do {
            try app.switchTo(id)
            switchProblem = nil
        } catch {
            switchProblem = error.localizedDescription
        }
    }

    private var statusLine: some View {
        Group {
            if let error = model.lastError {
                Text(error)
            } else if let paused = app.uploadStatus.pausedReason {
                Text("Uploads paused. \(paused)")
            } else if !model.unidentified.isEmpty {
                // Nothing is sent that does not say who it shows.
                Text("\(model.unidentified.count) photos not identified: choose who, or delete them")
                    .foregroundStyle(Theme.attention)
            } else if !model.shots.isEmpty {
                Text("\(model.shots.count) kept on this phone · Upload when done")
            } else if app.uploadStatus.pending > 0 {
                Text("\(app.uploadStatus.pending) uploading")
            } else {
                Text("Nothing is uploaded until you tap Upload")
            }
        }
        .font(.footnote)
        .foregroundStyle(.white)
        .padding(.vertical, 6)
        .padding(.horizontal, 12)
        .background(.black.opacity(0.5), in: Capsule())
        .padding(.bottom, 12)
        .environment(\.colorScheme, .dark)
    }

    private var controls: some View {
        HStack {
            PhotosPicker(selection: $pickerItems, maxSelectionCount: 200, selectionBehavior: .ordered, matching: .images,
                         preferredItemEncoding: .current) {
                Group {
                    if importing > 0 {
                        Text(verbatim: "\(importing)").font(.headline.monospacedDigit())
                    } else {
                        Image(systemName: "photo.on.rectangle").font(.title2)
                    }
                }
                .frame(width: 56, height: 56)
            }
            .disabled(importing > 0)
            .accessibilityLabel("Add photos from the library")
            Spacer()
            Button {
                Task { await model.shoot() }
            } label: {
                Circle()
                    .strokeBorder(.white, lineWidth: 4)
                    .background(Circle().fill(model.isShooting ? .gray : .white).padding(6))
                    .frame(width: 76, height: 76)
            }
            .disabled(model.cameraState != .ready || model.isShooting)
            .accessibilityLabel("Take photo")
            Spacer()
            Button {
                Task {
                    if await send() > 0 { showSummary = true }
                }
            } label: {
                VStack(spacing: 2) {
                    Image(systemName: "arrow.up.circle.fill").font(.title2)
                    Text("Upload").font(.caption.weight(.semibold))
                }
                .frame(width: 56, height: 56)
                .overlay(alignment: .topTrailing) {
                    if !model.shots.isEmpty {
                        Text(verbatim: "\(model.shots.count)")
                            .font(.caption2.weight(.bold))
                            .padding(.horizontal, 5)
                            .padding(.vertical, 1)
                            .foregroundStyle(.black)
                            .background(.white, in: Capsule())
                            .offset(x: 6, y: -4)
                    }
                }
            }
            .disabled(!model.canSend)
            .opacity(model.canSend ? 1 : 0.4)
            .accessibilityIdentifier("sendShots")
        }
        .foregroundStyle(.white)
        .padding(.horizontal, 28)
        .padding(.bottom, 24)
    }
}

/// Who the shots show, as the phone knows it now, and what to do when
/// nothing says. Tapping it picks who by hand.
struct BannerView: View {
    let model: CaptureModel
    let onChoose: () -> Void
    /// Names a new one recognized by its look, instead of the name made
    /// from how it looks.
    let onName: () -> Void

    var body: some View {
        HStack(spacing: 8) {
            Button(action: { if !model.filesByPlace { onChoose() } }) {
                HStack(spacing: 10) {
                    // State is the icon's colour; the banner stays dark glass.
                    Image(systemName: icon)
                        .foregroundStyle(color)
                    Text(text).lineLimit(2).multilineTextAlignment(.leading)
                    Spacer(minLength: 4)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("whoBanner")
            if model.canNameNew {
                pill("Name it", action: onName).accessibilityIdentifier("nameNew")
            }
            if model.current.likelyName != nil, !model.current.chosen {
                pill("Yes", primary: true, action: model.confirmLikely).accessibilityIdentifier("confirmLikely")
            }
            if !model.filesByPlace {
                pill(model.current.subjectID == nil ? "Choose" : "Change", action: onChoose)
            }
            if model.current.chosen {
                Button(action: model.clearChoice) {
                    Image(systemName: "xmark")
                        .font(.caption.weight(.bold))
                        .frame(width: 26, height: 26)
                        .background(.white.opacity(0.16), in: Circle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel(Text("Clear the choice"))
                .accessibilityIdentifier("clearChoice")
            }
        }
        .font(.subheadline.weight(.semibold))
        .foregroundStyle(.white)
        .padding(.vertical, 10)
        .padding(.leading, 16)
        .padding(.trailing, 10)
        .frame(maxWidth: .infinity)
        .background(.black.opacity(0.6), in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Theme.radius, style: .continuous).strokeBorder(.white.opacity(0.08)))
        .padding(.horizontal, 10)
        .padding(.top, 4)
        .animation(.default, value: text)
        // Over the camera it is always dark: theme colours take their dark
        // forms, whatever the phone's appearance.
        .environment(\.colorScheme, .dark)
    }

    /// The one answer that settles it is white; the rest are glass.
    private func pill(_ title: LocalizedStringKey, primary: Bool = false, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Text(title)
                .font(.caption.weight(.bold))
                .foregroundStyle(primary ? .black : .white)
                .padding(.vertical, 6)
                .padding(.horizontal, 12)
                .background(primary ? AnyShapeStyle(.white) : AnyShapeStyle(.white.opacity(0.16)), in: Capsule())
        }
        .buttonStyle(.plain)
    }

    private var text: String {
        let c = model.current
        if model.banner == .reading {
            return model.recognizesByLook ? String(localized: "Recognizing…") : String(localized: "Reading…")
        }
        if let name = c.name, c.chosen { return String(localized: "For: \(name)") }
        switch c.mark {
        case .matched: return String(localized: "Current: \(c.name ?? "")")
        case .new: return String(localized: "New: \(c.name ?? "")")
        case .ambiguous:
            if let likely = c.likelyName { return String(localized: "Looks like \(likely): is it?") }
            return String(localized: "ID seen — will ask you to confirm")
        case .offline: return String(localized: "ID seen offline — will match when online")
        case nil: break
        }
        if model.filesByPlace {
            // Nobody to name: where and when it is taken says it.
            return model.shots.isEmpty
                ? String(localized: "Just shoot, or add photos from the library: each is kept by where and when it was taken")
                : String(localized: "\(model.shots.count) photos, kept by where and when they were taken")
        }
        if model.firstShoot && model.shots.isEmpty {
            // A face says most about who it is; a label, all of it.
            return model.recognizesByLook
                ? String(localized: "First photos: each one close up, face on")
                : String(localized: "First photos: start with the label or ID")
        }
        if model.needsIdentity {
            return model.recognizesByLook
                ? String(localized: "Not recognized: shoot it again closer, or choose who this is")
                : String(localized: "No ID photo yet: photograph the label, or choose who this is")
        }
        return model.recognizesByLook
            ? String(localized: "Just shoot: each one is recognized by its look")
            : String(localized: "Photograph a label or ID to start")
    }

    private var icon: String {
        let c = model.current
        if model.filesByPlace { return "mappin.and.ellipse" }
        if model.banner == .reading { return "text.viewfinder" }
        if c.chosen { return "hand.point.up.left" }
        switch c.mark {
        case .matched: return "person.text.rectangle"
        case .new: return "plus.circle"
        case .ambiguous, .offline: return "questionmark.circle"
        case nil: return model.needsIdentity ? "exclamationmark.triangle" : (model.recognizesByLook ? "eye" : "viewfinder")
        }
    }

    private var color: Color {
        let c = model.current
        if c.chosen || c.mark == .matched || c.mark == .new { return Theme.success }
        if c.mark == .ambiguous || c.mark == .offline || model.needsIdentity { return Theme.attention }
        return .white
    }
}

/// The shots kept so far, newest on the right, like a story being made.
/// The shot that tells who a group shows is framed: green when known.
struct ShotTray: View {
    let model: CaptureModel

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 6) {
                    let owners = model.owners
                    let unknown = Set(model.unidentified)
                    ForEach(Array(model.shots.enumerated()), id: \.element.id) { i, shot in
                        if i > 0 && (shot.mark != nil || shot.pick != nil) {
                            // A new group starts here.
                            Rectangle().fill(.white.opacity(0.5)).frame(width: 2, height: 40)
                        }
                        tile(shot, index: i, owner: owners[shot.id], unknown: unknown.contains(shot.id))
                            .id(shot.id)
                    }
                }
                .padding(.horizontal, 12)
                .padding(.top, 8)
            }
            .onChange(of: model.shots.count) { _, _ in
                withAnimation { proxy.scrollTo(model.shots.last?.id, anchor: .trailing) }
            }
        }
        .frame(height: 84)
        .environment(\.colorScheme, .dark)
    }

    private func tile(_ shot: StagedShot, index: Int, owner: ShotOwner?, unknown: Bool) -> some View {
        let decides = shot.mark != nil || shot.pick != nil
        let frame: Color? = switch (shot.pick, shot.mark?.state) {
        case (.some, _), (nil, .matched), (nil, .new): Theme.success
        case (nil, .ambiguous), (nil, .offline): Theme.attention
        default: unknown ? Theme.attention : nil
        }
        return ZStack(alignment: .topTrailing) {
            Group {
                if let image = model.thumbnails[shot.id] {
                    Image(uiImage: image).resizable().scaledToFill()
                } else {
                    Color.gray.opacity(0.4)
                }
            }
            .frame(width: 60, height: 60)
            .clipShape(RoundedRectangle(cornerRadius: 9, style: .continuous))
            .overlay {
                if let frame {
                    RoundedRectangle(cornerRadius: 9, style: .continuous).strokeBorder(frame, lineWidth: 3)
                }
            }
            .overlay(alignment: .bottom) {
                if decides, let name = owner?.name, !name.isEmpty {
                    Text(verbatim: name)
                        .font(.system(size: 9, weight: .semibold))
                        .lineLimit(1)
                        .padding(.horizontal, 4)
                        .padding(.vertical, 2)
                        .frame(maxWidth: 60)
                        .background((frame ?? .black).opacity(0.85))
                        .foregroundStyle(.white)
                        .clipShape(UnevenRoundedRectangle(bottomLeadingRadius: 9, bottomTrailingRadius: 9))
                }
            }
            .overlay(alignment: .bottomLeading) {
                if unknown && !decides {
                    Image(systemName: "questionmark.circle.fill")
                        .font(.system(size: 14, weight: .bold))
                        .symbolRenderingMode(.palette)
                        .foregroundStyle(.white, Theme.attention)
                        .padding(4)
                }
            }
            .padding(.top, 6)
            .padding(.trailing, 6)
            .accessibilityElement()
            .accessibilityLabel(decides && owner?.name != nil ? Text("Identifies \(owner?.name ?? "")")
                                : unknown ? Text("Photo \(index + 1), not identified") : Text("Photo \(index + 1)"))
            .accessibilityIdentifier("shot-\(index)")

            Button {
                withAnimation(.snappy) { model.delete(shot.id) }
            } label: {
                Image(systemName: "xmark")
                    .font(.system(size: 10, weight: .bold))
                    .foregroundStyle(.white)
                    .frame(width: 20, height: 20)
                    .background(.black.opacity(0.75), in: Circle())
                    .overlay(Circle().strokeBorder(.white.opacity(0.8), lineWidth: 1))
            }
            .accessibilityLabel(Text("Delete photo \(index + 1)"))
            .accessibilityIdentifier("deleteShot-\(index)")
        }
    }
}
