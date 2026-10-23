import SwiftUI
import UIKit

/// Photos as on an iPhone: only the photo, zoomed with two fingers or a
/// double tap, the next one a swipe away, a tap hiding the bars. Its
/// details open on swiping up or from the menu, never in the way.
/// More of a list than has been loaded: how many it holds in all, and how
/// to load the next page, giving back the whole list so far.
struct MorePhotos {
    let total: Int
    let load: @MainActor () async -> [Pidex_V1_Photo]
}

struct PhotoViewer: View {
    let model: LibraryModel
    let more: MorePhotos?
    @State private var photos: [Pidex_V1_Photo]
    @State private var current: Int?
    /// How many the list holds in all, loaded or not.
    @State private var total: Int
    @State private var chrome = true
    @State private var zoomed = false
    @State private var showingInfo = false
    @State private var confirmingDelete = false
    @Environment(AppModel.self) private var app
    @Environment(\.dismiss) private var dismiss

    init(photos: [Pidex_V1_Photo], start: Int, model: LibraryModel, more: MorePhotos? = nil) {
        self.model = model
        self.more = more
        _photos = State(initialValue: photos)
        _current = State(initialValue: start)
        _total = State(initialValue: max(more?.total ?? 0, photos.count))
    }

    var body: some View {
        ScrollView(.horizontal) {
            LazyHStack(spacing: 0) {
                ForEach(photos.indices, id: \.self) { i in
                    PhotoPage(photo: photos[i], zoomed: $zoomed) {
                        withAnimation(.easeInOut(duration: 0.2)) { chrome.toggle() }
                    } onSwipeUp: {
                        showingInfo = true
                    } onSwipeDown: {
                        dismiss()
                    }
                    .containerRelativeFrame([.horizontal, .vertical])
                    .id(i)
                }
            }
            .scrollTargetLayout()
        }
        .scrollTargetBehavior(.paging)
        .scrollPosition(id: $current)
        .task(id: current) {
            // Near the end of what is loaded: the next page, so paging on
            // reaches every photo the count says there are.
            guard let i = current, let more, i >= photos.count - 3, photos.count < total else { return }
            let all = await more.load()
            if all.count > photos.count {
                photos = all
            } else {
                total = photos.count
            }
        }
        .scrollIndicators(.hidden)
        .scrollDisabled(zoomed)
        .ignoresSafeArea()
        .background(chrome ? Theme.paper : .black)
        .accessibilityIdentifier("photoViewer")
        .navigationTitle("Photo")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar(chrome ? .visible : .hidden, for: .navigationBar)
        .statusBarHidden(!chrome)
        .toolbar {
            // Where in the list this is, then when it was taken.
            ToolbarItem(placement: .principal) {
                VStack(spacing: 0) {
                    if let i = current, photos.count > 1 {
                        Text("\((i + 1).formatted()) of \(total.formatted())").font(.headline).monospacedDigit()
                            .accessibilityIdentifier("photoPosition")
                    } else {
                        Text("Photo").font(.headline)
                    }
                    if let photo, photo.hasCapturedAt {
                        Text(photo.capturedAt.date.formatted(date: .abbreviated, time: .shortened))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
            }
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    Button { showingInfo = true } label: { Label("Details and tags", systemImage: "info.circle") }
                    Button(role: .destructive) { confirmingDelete = true } label: { Label("Delete photo", systemImage: "trash") }
                        .accessibilityIdentifier("deletePhoto")
                } label: {
                    Image(systemName: "ellipsis")
                }
                .accessibilityLabel(Text("More"))
                .accessibilityIdentifier("photoMenu")
            }
        }
        .confirmationDialog("Delete this photo?", isPresented: $confirmingDelete, titleVisibility: .visible) {
            Button("Delete for good", role: .destructive) { Task { await deleteCurrent() } }
        } message: {
            Text("It is removed for everyone, with its tags and its place in conversations. This cannot be undone.")
        }
        .sheet(isPresented: $showingInfo) {
            if let i = current, photos.indices.contains(i) {
                NavigationStack {
                    PhotoInfoView(photo: Binding(get: { photos[i] }, set: { photos[i] = $0 }), model: model)
                }
                .presentationDetents([.medium, .large])
                .presentationDragIndicator(.visible)
            }
        }
    }

    private var photo: Pidex_V1_Photo? {
        current.flatMap { photos.indices.contains($0) ? photos[$0] : nil }
    }

    /// Deletes the photo showing; the next one shows, or the viewer
    /// closes when it was the last.
    private func deleteCurrent() async {
        guard let i = current, photos.indices.contains(i) else { return }
        let id = photos[i].id
        do {
            _ = try await model.api.deletePhotos([id])
            model.removed([id])
            photos.remove(at: i)
            total = max(total - 1, photos.count)
            app.show(String(localized: "Photo deleted"))
            if photos.isEmpty {
                dismiss()
            } else {
                current = min(i, photos.count - 1)
            }
        } catch {
            app.show(error.localizedDescription, tone: .problem)
        }
    }
}

/// One photo, its thumbnail at once and then the original.
private struct PhotoPage: View {
    let photo: Pidex_V1_Photo
    @Binding var zoomed: Bool
    let onTap: () -> Void
    let onSwipeUp: () -> Void
    let onSwipeDown: () -> Void
    @State private var image: UIImage?

    var body: some View {
        ZoomableImage(image: image, zoomed: $zoomed, onTap: onTap, onSwipeUp: onSwipeUp, onSwipeDown: onSwipeDown)
            .overlay { if image == nil { ProgressView() } }
            .accessibilityElement()
            .accessibilityLabel(Text(verbatim: photo.description_p))
            .accessibilityAddTraits(.isImage)
            .task(id: photo.id) {
                let thumb = URL(string: photo.thumbnailURL.isEmpty ? photo.viewURL : photo.thumbnailURL)
                if image == nil { image = await ImageCache.shared.image(id: photo.id, url: thumb) }
                if let full = await ImageCache.shared.original(id: photo.id, url: URL(string: photo.viewURL)) {
                    image = full
                }
            }
    }
}

/// The system's own zooming: a scroll view pinched, double-tapped and
/// panned, its image always centred.
private struct ZoomableImage: UIViewRepresentable {
    let image: UIImage?
    @Binding var zoomed: Bool
    let onTap: () -> Void
    let onSwipeUp: () -> Void
    let onSwipeDown: () -> Void

    func makeUIView(context: Context) -> ZoomView {
        ZoomView()
    }

    func updateUIView(_ view: ZoomView, context: Context) {
        view.onTap = onTap
        view.onSwipeUp = onSwipeUp
        view.onSwipeDown = onSwipeDown
        view.onZoom = { z in if zoomed != z { zoomed = z } }
        view.show(image)
    }
}

private final class ZoomView: UIScrollView, UIScrollViewDelegate, UIGestureRecognizerDelegate {
    var onTap: () -> Void = {}
    var onSwipeUp: () -> Void = {}
    var onSwipeDown: () -> Void = {}
    var onZoom: (Bool) -> Void = { _ in }
    private let imageView = UIImageView()
    private var laidOutFor: CGSize = .zero

    init() {
        super.init(frame: .zero)
        delegate = self
        minimumZoomScale = 1
        maximumZoomScale = 6
        showsVerticalScrollIndicator = false
        showsHorizontalScrollIndicator = false
        contentInsetAdjustmentBehavior = .never
        decelerationRate = .fast
        imageView.contentMode = .scaleAspectFit
        addSubview(imageView)

        let double = UITapGestureRecognizer(target: self, action: #selector(doubleTapped(_:)))
        double.numberOfTapsRequired = 2
        addGestureRecognizer(double)
        let single = UITapGestureRecognizer(target: self, action: #selector(tapped))
        single.require(toFail: double)
        addGestureRecognizer(single)
        for direction in [UISwipeGestureRecognizer.Direction.up, .down] {
            let swipe = UISwipeGestureRecognizer(target: self, action: #selector(swiped(_:)))
            swipe.direction = direction
            swipe.delegate = self
            addGestureRecognizer(swipe)
        }
    }

    required init?(coder: NSCoder) { fatalError("init(coder:) is not used") }

    func show(_ image: UIImage?) {
        guard image !== imageView.image else { return }
        // The original replaces the thumbnail: same shape, same zoom.
        imageView.image = image
        if zoomScale <= minimumZoomScale { fit() }
    }

    override func layoutSubviews() {
        super.layoutSubviews()
        if bounds.size != laidOutFor {
            laidOutFor = bounds.size
            setZoomScale(minimumZoomScale, animated: false)
            fit()
        }
    }

    /// The image as large as fits, centred.
    private func fit() {
        guard let size = imageView.image?.size, size.width > 0, size.height > 0, bounds.width > 0 else { return }
        let scale = min(bounds.width / size.width, bounds.height / size.height)
        let fitted = CGSize(width: size.width * scale, height: size.height * scale)
        imageView.frame = CGRect(origin: .zero, size: fitted)
        contentSize = fitted
        center()
    }

    private func center() {
        let x = max((bounds.width - contentSize.width) / 2, 0)
        let y = max((bounds.height - contentSize.height) / 2, 0)
        contentInset = UIEdgeInsets(top: y, left: x, bottom: y, right: x)
    }

    func viewForZooming(in scrollView: UIScrollView) -> UIView? { imageView }

    func scrollViewDidZoom(_ scrollView: UIScrollView) {
        center()
        onZoom(zoomScale > minimumZoomScale + 0.01)
    }

    @objc private func doubleTapped(_ g: UITapGestureRecognizer) {
        if zoomScale > minimumZoomScale + 0.01 {
            setZoomScale(minimumZoomScale, animated: true)
        } else {
            let point = g.location(in: imageView)
            let scale = min(maximumZoomScale, 2.5)
            let size = CGSize(width: bounds.width / scale, height: bounds.height / scale)
            zoom(to: CGRect(x: point.x - size.width / 2, y: point.y - size.height / 2, width: size.width, height: size.height), animated: true)
        }
    }

    @objc private func tapped() { onTap() }

    @objc private func swiped(_ g: UISwipeGestureRecognizer) {
        // Zoomed in, a swipe moves around the photo instead.
        guard zoomScale <= minimumZoomScale + 0.01 else { return }
        g.direction == .up ? onSwipeUp() : onSwipeDown()
    }

    func gestureRecognizer(_ g: UIGestureRecognizer, shouldRecognizeSimultaneouslyWith other: UIGestureRecognizer) -> Bool {
        true
    }
}

/// A photo's details, apart from the photo: when it was taken, what it is
/// filed under, what the AI saw, and its tags, which can be corrected. A
/// correction re-indexes the photo for search.
struct PhotoInfoView: View {
    @Binding var photo: Pidex_V1_Photo
    let model: LibraryModel
    @State private var error: String?
    @State private var saving = false
    @State private var choosing = false
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        List {
            Section {
                if photo.hasCapturedAt {
                    LabeledContent("Taken", value: photo.capturedAt.date.formatted(date: .abbreviated, time: .shortened))
                }
                if let s = model.subject(photo.subjectID) {
                    NavigationLink {
                        SubjectPhotosView(subject: s, model: model)
                    } label: {
                        LabeledContent("Filed under") { Text(verbatim: s.displayName) }
                    }
                } else {
                    LabeledContent("Filed under", value: String(localized: "Not filed"))
                }
                Button(photo.subjectID.isEmpty ? "File under…" : "File under another…") { choosing = true }
                    .disabled(saving)
                    .accessibilityIdentifier("refile")
                if !photo.description_p.isEmpty {
                    Text(verbatim: photo.description_p)
                        .foregroundStyle(.secondary)
                }
            }
            Section {
                ForEach(photo.tags, id: \.key) { tag in
                    HStack {
                        Text(verbatim: model.tagName(tag.key))
                        Spacer()
                        if tag.source == .user {
                            Image(systemName: "person.fill.checkmark").foregroundStyle(.secondary)
                                .accessibilityLabel(Text("Added by a person"))
                        } else {
                            Text(verbatim: "\(Int(tag.confidence * 100))%").foregroundStyle(.secondary).monospacedDigit()
                        }
                    }
                    .swipeActions {
                        Button("Remove", role: .destructive) { Task { await edit(remove: tag.key) } }
                    }
                }
                Menu {
                    ForEach(addable, id: \.group) { group in
                        Section(group.group.isEmpty ? "" : ConfigSections.heading(group.group)) {
                            ForEach(group.tags) { t in
                                Button(t.name) { Task { await edit(add: t.key) } }
                            }
                        }
                    }
                } label: {
                    Label("Add a tag", systemImage: "plus")
                }
                .disabled(saving || addable.isEmpty)
            } header: {
                Text("Tags")
            } footer: {
                Text("Swipe a tag to remove it. Corrections are kept and the photo is found by them from now on.")
            }
            if let error {
                Section { Text(error).foregroundStyle(Theme.danger) }
            }
        }
        .navigationTitle("Details")
        .navigationBarTitleDisplayMode(.inline)
        .sheet(isPresented: $choosing) {
            NavigationStack {
                SubjectPicker(model: model, current: photo.subjectID) { subject in
                    choosing = false
                    Task { await file(under: subject) }
                }
            }
        }
        .toolbar {
            ToolbarItem(placement: .confirmationAction) {
                Button("Done") { dismiss() }
            }
        }
    }

    private var addable: [(group: String, tags: [ScenarioConfig.Tag])] {
        let have = Set(photo.tags.map(\.key))
        return (model.config?.tagGroups ?? []).compactMap { g in
            let rest = g.tags.filter { !have.contains($0.key) }
            return rest.isEmpty ? nil : (g.group, rest)
        }
    }

    /// A person's correction: the photo, and for subjects known by their
    /// look, a new example of what that one looks like.
    private func file(under subject: Pidex_V1_Subject) async {
        saving = true
        defer { saving = false }
        do {
            if let updated = try await model.api.assign(photoIDs: [photo.id], to: subject.id).first {
                photo = updated
                model.replace(updated)
            }
            model.remember([subject])
            error = nil
        } catch {
            self.error = error.localizedDescription
        }
    }

    private func edit(add: String? = nil, remove: String? = nil) async {
        saving = true
        defer { saving = false }
        do {
            photo = try await model.api.updateTags(photoID: photo.id, add: add.map { [$0] } ?? [], remove: remove.map { [$0] } ?? [])
            model.replace(photo)
            error = nil
        } catch {
            self.error = error.localizedDescription
        }
    }
}

/// Every photo filed under one subject, newest day first.
struct SubjectPhotosView: View {
    @State private var subject: Pidex_V1_Subject
    let model: LibraryModel
    @State private var photos: [Pidex_V1_Photo] = []
    @State private var loading = true
    @State private var error: String?
    @State private var renaming = false
    @State private var newName = ""
    @State private var merging = false
    @State private var deleting: Pidex_V1_Subject?
    @State private var editingLooks = false
    @Environment(\.dismiss) private var dismiss

    init(subject: Pidex_V1_Subject, model: LibraryModel) {
        _subject = State(initialValue: subject)
        self.model = model
    }

    var body: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 8, pinnedViews: .sectionHeaders) {
                if let error {
                    Text(error).foregroundStyle(Theme.danger).padding()
                } else if photos.isEmpty && !loading {
                    Text("No photos yet.").foregroundStyle(.secondary).frame(maxWidth: .infinity).padding(.top, 60)
                }
                if showsLooks {
                    LooksCard(subject: subject) { editingLooks = true }
                        .padding(.horizontal)
                        .padding(.vertical, 8)
                }
                ForEach(days, id: \.day) { group in
                    Section {
                        PhotoGrid(photos: group.photos, model: model, album: photos)
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
        .overlay { if loading { ProgressView() } }
        .navigationTitle(Text(verbatim: subject.displayName))
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .principal) {
                Button { newName = subject.displayName; renaming = true } label: {
                    HStack(spacing: 4) {
                        Text(verbatim: subject.displayName).font(.headline).lineLimit(1)
                        Image(systemName: "pencil").font(.caption.weight(.semibold)).foregroundStyle(.secondary)
                    }
                    .foregroundStyle(.primary)
                }
                .accessibilityLabel(Text("Rename \(subject.displayName)"))
                .accessibilityIdentifier("renameSubject")
            }
        }
        .toolbar {
            Menu {
                Button { newName = subject.displayName; renaming = true } label: { Label("Rename", systemImage: "pencil") }
                Button { merging = true } label: { Label("Same as another…", systemImage: "arrow.triangle.merge") }
                Button(role: .destructive) { deleting = subject } label: { Label("Delete…", systemImage: "trash") }
                    .accessibilityIdentifier("deleteSubject")
            } label: {
                Image(systemName: "ellipsis")
            }
            .accessibilityLabel(Text("More"))
            .accessibilityIdentifier("subjectMenu")
        }
        .subjectDeletion($deleting, api: model.api) { _ in dismiss() }
        .sheet(isPresented: $editingLooks) {
            LooksEditor(subject: subject, api: model.api) { updated in
                subject = updated
                model.remember([updated])
            }
        }
        .alert("Rename", isPresented: $renaming) {
            TextField("Name", text: $newName)
            Button("Cancel", role: .cancel) {}
            Button("Save") { Task { await rename() } }
        }
        .sheet(isPresented: $merging) {
            NavigationStack {
                SubjectPicker(model: model, current: subject.id, typeKey: subject.typeKey, allowsNew: false,
                              title: "Which one is it?",
                              footer: "Its photos move there, and this one is removed.") { other in
                    merging = false
                    Task { await merge(into: other) }
                }
            }
        }
        .task {
            var f = Pidex_V1_SearchFilters()
            f.subjectIds = [subject.id]
            do {
                photos = try await model.api.searchPhotos(filters: f).photos
            } catch {
                self.error = error.localizedDescription
            }
            loading = false
        }
    }

    /// Only subjects told apart by look have a profile.
    private var showsLooks: Bool {
        !subject.profile.isEmpty || model.config?.subjectTypes.first { $0.key == subject.typeKey }?.byAppearance == true
    }

    private func rename() async {
        let name = newName.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !name.isEmpty, name != subject.displayName else { return }
        do {
            subject = try await model.api.renameSubject(subject.id, to: name)
            model.remember([subject])
        } catch {
            self.error = error.localizedDescription
        }
    }

    /// The same one filed twice: its photos join the other, and this page,
    /// now empty, closes.
    private func merge(into other: Pidex_V1_Subject) async {
        do {
            let merged = try await model.api.mergeSubjects([subject.id], into: other.id)
            model.remember([merged])
            dismiss()
        } catch {
            self.error = error.localizedDescription
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

/// Picks the subject a photo shows, or adds one by name when subjects are
/// known by their look and have no label to read.
struct SubjectPicker: View {
    let model: LibraryModel
    var current: String = ""
    /// Only subjects of this type; all types when empty.
    var typeKey: String = ""
    var allowsNew = true
    var title: LocalizedStringKey = "File under"
    var footer: LocalizedStringKey?
    let onPick: (Pidex_V1_Subject) -> Void
    @State private var subjects: [Pidex_V1_Subject] = []
    @State private var query = ""
    @State private var newName = ""
    @State private var loading = true
    @State private var error: String?
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        List {
            if let error {
                Text(error).foregroundStyle(Theme.danger)
            }
            ForEach(types, id: \.key) { t in
                Section {
                    if allowsNew && t.byAppearance && query.isEmpty {
                        HStack {
                            TextField("New \(t.name): name", text: $newName)
                                .accessibilityIdentifier("newSubjectName")
                            Button("Add") { Task { await add(t) } }
                                .disabled(newName.trimmingCharacters(in: .whitespaces).isEmpty)
                        }
                    }
                    ForEach(shown.filter { $0.typeKey == t.key }, id: \.id) { s in
                        Button {
                            onPick(s)
                        } label: {
                            HStack(spacing: 12) {
                                SubjectAvatar(subject: s, size: 36)
                                Text(verbatim: s.displayName)
                                Spacer()
                                if s.id == current {
                                    Image(systemName: "checkmark").foregroundStyle(.tint)
                                }
                            }
                        }
                        .tint(.primary)
                        .disabled(s.id == current)
                        .accessibilityIdentifier("subject-\(s.displayName)")
                    }
                } header: {
                    if types.count > 1 { Text(verbatim: t.name) }
                } footer: {
                    if let footer { Text(footer) }
                }
            }
        }
        .overlay { if loading { ProgressView() } }
        .searchable(text: $query)
        .navigationTitle(title)
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .cancellationAction) {
                Button("Cancel") { dismiss() }
            }
        }
        .task {
            do {
                await model.loadConfig()
                subjects = try await model.api.subjects()
                model.remember(subjects)
            } catch {
                self.error = error.localizedDescription
            }
            loading = false
        }
    }

    /// Types with something to pick, or where one can be added by name.
    private var types: [ScenarioConfig.SubjectType] {
        (model.config?.subjectTypes ?? []).filter { t in
            (typeKey.isEmpty || t.key == typeKey)
                && ((allowsNew && t.byAppearance) || shown.contains { $0.typeKey == t.key })
        }
    }

    private var shown: [Pidex_V1_Subject] {
        let q = query.trimmingCharacters(in: .whitespaces)
        return subjects.filter { q.isEmpty || $0.displayName.localizedCaseInsensitiveContains(q) }
    }

    private func add(_ t: ScenarioConfig.SubjectType) async {
        do {
            let s = try await model.api.createSubject(typeKey: t.key, name: newName.trimmingCharacters(in: .whitespaces))
            onPick(s)
        } catch {
            self.error = error.localizedDescription
        }
    }
}

/// What a subject looks like, one feature a line: written from its photos,
/// or by a person. Searching finds it by these ("the cat with the black
/// nose"), even in photos that do not show them.
struct LooksCard: View {
    let subject: Pidex_V1_Subject
    let onEdit: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("Looks").font(.subheadline.weight(.semibold))
                Spacer()
                Button(subject.profile.isEmpty ? "Add" : "Edit", action: onEdit)
                    .font(.subheadline.weight(.medium))
                    .accessibilityIdentifier("editLooks")
            }
            if subject.profile.isEmpty {
                Text("Written from its photos once there are a few. Add what you know, such as a black nose, and searches find it.")
                    .font(.footnote)
                    .foregroundStyle(Theme.muted)
            } else {
                VStack(alignment: .leading, spacing: 4) {
                    ForEach(Array(lines.enumerated()), id: \.offset) { _, line in
                        HStack(alignment: .firstTextBaseline, spacing: 8) {
                            Circle().fill(Theme.muted).frame(width: 4, height: 4)
                            Text(verbatim: line).font(.footnote)
                        }
                    }
                }
                Text(subject.profileByPerson ? "Written by you" : "Written from its photos")
                    .font(.caption)
                    .foregroundStyle(Theme.muted)
            }
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.radius, style: .continuous))
    }

    private var lines: [String] {
        subject.profile.split(separator: "\n").map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty }
    }
}

/// Writing what a subject looks like. What a person writes is kept: photos
/// no longer rewrite it, until they hand it back.
struct LooksEditor: View {
    let subject: Pidex_V1_Subject
    let api: APIClient
    let onSaved: (Pidex_V1_Subject) -> Void
    @State private var text: String
    @State private var saving = false
    @State private var error: String?
    @Environment(\.dismiss) private var dismiss

    init(subject: Pidex_V1_Subject, api: APIClient, onSaved: @escaping (Pidex_V1_Subject) -> Void) {
        self.subject = subject
        self.api = api
        self.onSaved = onSaved
        _text = State(initialValue: subject.profile)
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextEditor(text: $text)
                        .frame(minHeight: 180)
                        .accessibilityIdentifier("looksText")
                } footer: {
                    Text("One feature a line, such as “black nose” or “a notch in the left ear”. Searches find it by these. What you write here is kept; new photos no longer rewrite it.")
                }
                if subject.profileByPerson {
                    Section {
                        Button("Let its photos write it") { Task { await save("") } }
                    }
                }
                if let error {
                    Section { Text(error).foregroundStyle(Theme.danger) }
                }
            }
            .navigationTitle(Text("Looks of \(subject.displayName)"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") { Task { await save(text) } }
                        .disabled(saving || text.trimmingCharacters(in: .whitespacesAndNewlines) == subject.profile)
                }
            }
        }
        .presentationDetents([.medium, .large])
    }

    private func save(_ profile: String) async {
        saving = true
        defer { saving = false }
        do {
            onSaved(try await api.setSubjectProfile(subject.id, to: profile.trimmingCharacters(in: .whitespacesAndNewlines)))
            dismiss()
        } catch {
            self.error = error.localizedDescription
        }
    }
}
