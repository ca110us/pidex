import Foundation
import Observation
import UIKit

/// One shooting session: the camera, the shots kept on the phone until the
/// person sends them, who each group of shots shows, and the hand-off to the
/// upload queue.
@MainActor
@Observable
final class CaptureModel {
    enum CameraState: Equatable {
        case starting
        case ready
        case unavailable(String)
    }

    /// Whether a label is being looked up; what the banner says otherwise
    /// follows from the shots and picks.
    enum Banner: Equatable {
        case idle
        case reading
    }

    private(set) var cameraState: CameraState = .starting
    private(set) var banner: Banner = .idle
    private(set) var sessionID = UUID()
    /// Shots kept on the phone, oldest first, until the set is sent.
    private(set) var shots: [StagedShot] = []
    /// Small images of the shots, by shot ID.
    private(set) var thumbnails: [String: UIImage] = [:]
    private(set) var isShooting = false
    private(set) var isSending = false
    private(set) var lastError: String?
    /// Every subject is told apart by how it looks: there is no label to
    /// photograph first.
    private(set) var recognizesByLook = false
    /// The collection was just made: until its first shot, the camera says
    /// how to start.
    var firstShoot = false
    /// A pick made before the shots it applies to were taken.
    private var nextPick: StagedShot.Pick?
    /// Shots being looked up.
    private var reading = 0
    /// Shots already shown to the model to recognize by look: each once.
    private var tried: Set<String> = []

    let camera = CameraController()
    let api: APIClient
    private let uploads: UploadQueue
    private let store: ShotStore?

    init(api: APIClient, uploads: UploadQueue, staging: URL? = nil) {
        self.api = api
        self.uploads = uploads
        store = staging.flatMap { try? ShotStore(directory: $0) }
        // A shoot interrupted by the app closing carries on.
        if let store {
            shots = store.load()
            for shot in shots {
                let url = store.url(of: shot)
                Task { @MainActor [weak self] in
                    let image = await Self.thumbnail(url)
                    self?.thumbnails[shot.id] = image
                }
            }
        }
    }

    var shotCount: Int { shots.count }

    func startCamera() async {
        if let json = try? await api.activeConfig().configJson, let config = try? ScenarioConfig(json: json) {
            recognizesByLook = !config.subjectTypes.isEmpty && config.subjectTypes.allSatisfy(\.byAppearance)
        }
        do {
            try await camera.start()
            cameraState = .ready
        } catch {
            cameraState = .unavailable(error.localizedDescription)
        }
    }

    func stopCamera() { camera.stop() }

    /// Starts a new session: a new patient round, a new site visit.
    func newSession() {
        sessionID = UUID()
        nextPick = nil
        tried = []
        banner = .idle
    }

    // MARK: Who the shots show

    /// Who each shot goes to: an identity shot or a pick decides it for the
    /// shots after it, until the next one.
    var owners: [String: ShotOwner] {
        var out: [String: ShotOwner] = [:]
        var current = ShotOwner()
        for shot in shots {
            if let m = shot.mark {
                current = ShotOwner(subjectID: m.subjectID, name: m.name, mark: m.state, identityShotID: shot.id,
                                    likelyID: m.likelyID, likelyName: m.likelyName)
            }
            if let p = shot.pick {
                current = ShotOwner(subjectID: p.subjectID, name: p.name, chosen: true, mark: current.mark, identityShotID: current.identityShotID)
            }
            out[shot.id] = current
        }
        return out
    }

    /// Who the next shot will go to.
    var current: ShotOwner {
        if let p = nextPick { return ShotOwner(subjectID: p.subjectID, name: p.name, chosen: true) }
        guard let last = shots.last else { return ShotOwner() }
        return owners[last.id] ?? ShotOwner()
    }

    /// The shots of the current group: from the last identity shot on.
    private var groupStart: Int {
        shots.lastIndex { $0.mark != nil } ?? 0
    }

    /// Shots that do not say who they show: none may be sent, so nothing
    /// unidentified is filed wrongly or lost among the unfiled.
    var unidentified: [String] {
        let owners = owners
        return shots.filter { !(owners[$0.id]?.identified ?? false) }.map(\.id)
    }

    /// A label or a look is being read.
    var isIdentifying: Bool { reading > 0 }

    var canSend: Bool { !shots.isEmpty && unidentified.isEmpty && !isIdentifying && !isSending }

    /// The current group has shots but nothing says who they show.
    var needsIdentity: Bool {
        !shots.isEmpty && !current.identified && !isIdentifying
    }

    /// Says yes to who the current group looks like.
    func confirmLikely() {
        guard let id = current.likelyID, let name = current.likelyName else { return }
        var s = Pidex_V1_Subject()
        s.id = id
        s.displayName = name
        choose(s)
    }

    /// The current group shows one recognized by its look that is not on
    /// file yet: it can be given its real name.
    var canNameNew: Bool {
        guard recognizesByLook, !current.chosen, current.mark == .new, groupMark?.typeKey != nil else { return false }
        return true
    }

    private var groupMark: StagedShot.Mark? {
        shots.last { $0.mark != nil }?.mark
    }

    /// Names the new one this group shows. A name already on file means
    /// that one; otherwise it is added by that name, and the shots go to it.
    func name(_ text: String) async {
        let name = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !name.isEmpty, let typeKey = groupMark?.typeKey else { return }
        do {
            let known = try await api.subjects().first {
                $0.typeKey == typeKey && $0.displayName.localizedCaseInsensitiveCompare(name) == .orderedSame
            }
            let subject: Pidex_V1_Subject
            if let known {
                subject = known
            } else {
                subject = try await api.createSubject(typeKey: typeKey, name: name)
            }
            choose(subject)
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// The person says who the current group shows, and the shots after it.
    func choose(_ subject: Pidex_V1_Subject) {
        let pick = StagedShot.Pick(subjectID: subject.id, name: subject.displayName)
        if shots.isEmpty {
            nextPick = pick
        } else {
            for i in groupStart..<shots.count { shots[i].pick = nil }
            shots[groupStart].pick = pick
            nextPick = nil
            persist()
        }
        banner = .idle
    }

    /// Takes back who the person said the current group shows: it goes back
    /// to what its shots say, or to nobody yet, and the next shots too.
    func clearChoice() {
        nextPick = nil
        if !shots.isEmpty {
            for i in groupStart..<shots.count { shots[i].pick = nil }
            persist()
        }
        banner = .idle
        if recognizesByLook { recognizeByLook() }
    }

    /// Removes a shot before it is sent. A pick it carried moves to the
    /// next shot of its group.
    func delete(_ id: String) {
        guard let i = shots.firstIndex(where: { $0.id == id }) else { return }
        let shot = shots.remove(at: i)
        if let pick = shot.pick, i < shots.count, shots[i].mark == nil, shots[i].pick == nil {
            shots[i].pick = pick
        } else if let pick = shot.pick, shots.isEmpty || i == shots.count {
            nextPick = pick
        }
        store?.delete(shot)
        thumbnails[id] = nil
        persist()
    }

    // MARK: Shooting

    func shoot() async {
        guard !isShooting else { return }
        isShooting = true
        defer { isShooting = false }
        do {
            let data = try await camera.capture()
            await handle(photoData: data, capturedAt: Date())
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// Feeds a picked image through the same path as a camera shot. Used on
    /// the simulator, and for photos taken before the app was opened.
    func importPhoto(_ data: Data) async {
        await handle(photoData: data, capturedAt: Date())
    }

    private func handle(photoData: Data, capturedAt: Date) async {
        let prepared: (jpeg: Data, analysis: Pidex_V1_OnDeviceAnalysis)
        do {
            prepared = try await Self.prepare(photoData)
        } catch {
            lastError = String(localized: "Could not read the photo: \(error.localizedDescription)")
            return
        }
        let id = UUID().uuidString.lowercased()
        var shot: StagedShot
        do {
            guard let store else { throw CocoaError(.fileNoSuchFile) }
            let name = try store.write(prepared.jpeg, id: id)
            shot = StagedShot(id: id, fileName: name, capturedAt: capturedAt, onDevice: try prepared.analysis.serializedData())
            if let p = nextPick, shots.isEmpty || owners[shots.last!.id]?.subjectID != p.subjectID {
                shot.pick = p
            }
            nextPick = nil
            shots.append(shot)
            persist()
            thumbnails[id] = await Self.thumbnail(store.url(of: shot))
            lastError = nil
        } catch {
            lastError = String(localized: "Could not save the photo: \(error.localizedDescription)")
            return
        }
        let jpeg = prepared.jpeg
        if recognizesByLook {
            recognizeByLook()
        } else if OnDeviceAnalyzer.mayCarryIdentity(prepared.analysis) {
            // In the background: the next shot need not wait for a model.
            Task { await identify(shotID: id, from: prepared.analysis, jpeg: jpeg) }
        }
    }

    @concurrent
    private static func prepare(_ data: Data) async throws -> (jpeg: Data, analysis: Pidex_V1_OnDeviceAnalysis) {
        let (jpeg, image) = try ImageFeatures.uprightJPEG(from: data)
        return (jpeg, OnDeviceAnalyzer.analyze(image))
    }

    /// A copy small enough to send quickly, and plenty for reading a label.
    @concurrent
    private static func downscale(_ jpeg: Data) async -> Data? {
        try? ImageFeatures.uprightJPEG(from: jpeg, maxEdge: 1280, quality: 0.8).jpeg
    }

    @concurrent
    private static func thumbnail(_ url: URL) async -> UIImage? {
        ShotStore.thumbnail(of: url)
    }

    /// A group is one subject: once it is recognized, the rest follow. Until
    /// then the newest shot not yet tried is shown to the model, one at a
    /// time; a shot taken while another is being recognized waits its turn
    /// rather than being skipped, so shooting closer always gets a look.
    private func recognizeByLook() {
        guard recognizesByLook, reading == 0, !current.identified, current.mark == nil, let store else { return }
        guard let shot = shots[groupStart...].last(where: { !tried.contains($0.id) }),
              let jpeg = try? Data(contentsOf: store.url(of: shot)),
              let analysis = try? Pidex_V1_OnDeviceAnalysis(serializedBytes: shot.onDevice) else { return }
        tried.insert(shot.id)
        Task {
            await identify(shotID: shot.id, from: analysis, jpeg: jpeg)
            recognizeByLook()
        }
    }

    /// Looks up a label or code in a shot: first from what the phone read,
    /// then, when that names nobody, by having the server's model read the
    /// shot (a serial number among other text on a screen). The shot
    /// becomes the group's identity shot when it names someone.
    private func identify(shotID: String, from analysis: Pidex_V1_OnDeviceAnalysis, jpeg: Data) async {
        reading += 1
        banner = .reading
        defer {
            reading -= 1
            if reading == 0 { banner = .idle }
        }
        var mark: StagedShot.Mark?
        do {
            // Codes: the phone's own reading first. Looks: only a model can.
            var r = recognizesByLook ? Pidex_V1_ResolveAnchorResponse() : try await api.resolveAnchor(analysis)
            if r.state == .notAnchor || recognizesByLook, let small = await Self.downscale(jpeg) {
                r = try await api.identifyShot(jpeg: small, onDevice: analysis, clientPhotoID: shotID)
            }
            switch r.state {
            case .matched:
                mark = .init(state: .matched, subjectID: r.subject.id, name: r.subject.displayName)
            case .newSubject:
                mark = .init(state: .new, name: r.extractedIdentity.values.sorted().joined(separator: " · "),
                             typeKey: r.subjectTypeKey)
            case .ambiguous where recognizesByLook:
                // Looks like one on file, not surely: asked, with one tap to
                // say yes. With nobody it looks like, the next shot tries.
                if let likely = r.candidates.first {
                    mark = .init(state: .ambiguous, typeKey: r.subjectTypeKey, likelyID: likely.id, likelyName: likely.displayName)
                }
            case .ambiguous:
                mark = .init(state: .ambiguous)
            default:
                break
            }
            if recognizesByLook { lastError = nil }
        } catch {
            // Offline: an ID barcode means the subject changed, even if we
            // cannot tell to whom.
            if !analysis.barcodes.isEmpty { mark = .init(state: .offline) }
            if recognizesByLook {
                lastError = String(localized: "Could not recognize this shot: \(ScenarioText.explain(error))")
            }
        }
        guard let mark, let i = shots.firstIndex(where: { $0.id == shotID }) else { return }
        // The look-up may finish after the person chose who this is; their
        // choice stays, as a pick outranks a read on the same shot.
        shots[i].mark = mark
        persist()
    }

    // MARK: Sending

    /// Sends every shot kept, in order, each with who it shows. Returns how
    /// many were handed to the upload queue.
    @discardableResult
    func sendAll() async -> Int {
        guard canSend, let store else { return 0 }
        isSending = true
        defer { isSending = false }
        let owners = owners
        let session = sessionID.uuidString.lowercased()
        var sent = 0
        while let shot = shots.first {
            do {
                let jpeg = try Data(contentsOf: store.url(of: shot))
                let owner = owners[shot.id]
                try await uploads.enqueue(
                    jpeg: jpeg, sessionID: session, capturedAt: shot.capturedAt,
                    onDevice: try Pidex_V1_OnDeviceAnalysis(serializedBytes: shot.onDevice),
                    lockedSubjectID: owner?.subjectID, subjectChosen: owner?.chosen ?? false,
                    id: shot.id, identityPhotoID: owner?.identityShotID
                )
            } catch {
                lastError = String(localized: "Could not send the photo: \(error.localizedDescription)")
                return sent
            }
            shots.removeFirst()
            store.delete(shot)
            thumbnails[shot.id] = nil
            persist()
            sent += 1
        }
        lastError = nil
        return sent
    }

    /// Throws away every shot kept.
    func discardAll() {
        for shot in shots { store?.delete(shot) }
        shots = []
        thumbnails = [:]
        nextPick = nil
        persist()
    }

    private func persist() {
        do {
            try store?.save(shots)
        } catch {
            lastError = String(localized: "Could not save the photo: \(error.localizedDescription)")
        }
    }
}
