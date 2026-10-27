import CryptoKit
import Foundation

/// One photo waiting to reach the server. Persisted, so nothing is lost if
/// the app is killed or the phone has no signal for a day.
nonisolated struct PendingUpload: Codable, Sendable, Identifiable, Equatable {
    /// Client photo ID; the server de-duplicates retries on it.
    var id: String
    var sessionID: String
    var capturedAt: Date
    var fileName: String
    var sizeBytes: Int64
    var sha256: String
    /// Serialized Pidex_V1_OnDeviceAnalysis.
    var onDevice: Data
    var lockedSubjectID: String?
    /// The person picked lockedSubjectID on the camera. Optional so queues
    /// written by older versions still decode.
    var subjectChosen: Bool?
    /// The shot that identifies this one, by its ID (which may be this
    /// one's), when no subject is set.
    var identityPhotoID: String?
    /// Where it was taken. Optional so older queues still decode.
    var place: ShotPlace?
    /// Once uploaded, the file stays on the phone as the photo's original
    /// rather than being deleted (not for high-privacy teams).
    var keepOriginal: Bool?

    var photoID: String?
    var bytesUploaded = false
    var attempts = 0
    var nextAttemptAt: Date?
    var rejectedReason: String?
}

nonisolated struct UploadStatus: Sendable, Equatable {
    var pending = 0
    var rejected = 0
    var pausedReason: String?
    var lastError: String?
    /// Photos still to upload, by capture session.
    var pendingBySession: [String: Int] = [:]
}

/// Moves photos from the phone to the server, in order, one at a time.
/// Each photo is written to disk before anything else happens; the file is
/// deleted, or kept as the photo's original, only after the server confirms
/// it has the photo.
actor UploadQueue {
    private let directory: URL
    private var transport: UploadTransport?
    /// Keeps an uploaded file as the original of a photo, by its id; false
    /// if it was not kept.
    private let keep: (@Sendable (URL, String) -> Bool)?
    private let onChange: @Sendable (UploadStatus) -> Void
    private let now: @Sendable () -> Date
    private var items: [PendingUpload] = []
    private var draining = false
    private let autoDrain: Bool
    private var pausedReason: String?
    private var lastError: String?

    static let maxBackoff: TimeInterval = 300

    init(
        directory: URL,
        transport: UploadTransport?,
        now: @escaping @Sendable () -> Date = Date.init,
        autoDrain: Bool = true,
        keep: (@Sendable (URL, String) -> Bool)? = nil,
        onChange: @escaping @Sendable (UploadStatus) -> Void = { _ in }
    ) throws {
        self.directory = directory
        self.keep = keep
        self.autoDrain = autoDrain
        self.transport = transport
        self.now = now
        self.onChange = onChange
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let index = directory.appending(path: "index.json")
        if let data = try? Data(contentsOf: index) {
            items = (try? JSONDecoder().decode([PendingUpload].self, from: data)) ?? []
        }
    }

    var pending: [PendingUpload] { items }

    /// Stores the photo and queues it. Returns once the photo is safely on
    /// disk; uploading continues in the background.
    /// Every photo says who it shows: `lockedSubjectID`, or else
    /// `identityPhotoID`, the ID of the shot that identifies it (by default
    /// this one). `id` keeps a shot's own ID, so others can name it.
    func enqueue(jpeg: Data, sessionID: String, capturedAt: Date, onDevice: Pidex_V1_OnDeviceAnalysis,
                 lockedSubjectID: String?, subjectChosen: Bool = false, id: String? = nil, identityPhotoID: String? = nil,
                 place: ShotPlace? = nil, keepOriginal: Bool = false) throws {
        let id = id ?? UUID().uuidString.lowercased()
        let fileName = id + ".jpg"
        try jpeg.write(to: directory.appending(path: fileName), options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
        let item = PendingUpload(
            id: id, sessionID: sessionID, capturedAt: capturedAt, fileName: fileName,
            sizeBytes: Int64(jpeg.count), sha256: SHA256.hash(data: jpeg).map { String(format: "%02x", $0) }.joined(),
            onDevice: try onDevice.serializedData(), lockedSubjectID: lockedSubjectID,
            subjectChosen: subjectChosen ? true : nil,
            identityPhotoID: lockedSubjectID == nil ? (identityPhotoID ?? id) : nil,
            place: place,
            keepOriginal: keepOriginal ? true : nil
        )
        items.append(item)
        try persist()
        publish()
        kick()
    }

    /// Points the queue at a new server or token. There is one queue per
    /// device, so photos taken before a settings change still go out.
    func setTransport(_ transport: UploadTransport?) {
        self.transport = transport
        resume()
    }

    /// Clears a pause after the user fixes their settings, and retries now.
    func resume() {
        pausedReason = nil
        for i in items.indices { items[i].nextAttemptAt = nil }
        publish()
        kick()
    }

    /// Starts draining unless a drain is already running.
    func kick() {
        guard autoDrain, !draining else { return }
        draining = true
        Task { await self.drainLoop() }
    }

    private func drainLoop() async {
        defer { draining = false }
        while true {
            await drainOnce()
            guard pausedReason == nil, let wait = nextWakeDelay() else { return }
            try? await Task.sleep(for: .seconds(wait))
        }
    }

    /// Tries every item that is due, in capture order. Stops at the first
    /// network failure: if one upload cannot get through, the rest won't
    /// either, and hammering a dead connection drains the battery.
    func drainOnce() async {
        guard let transport else { return }
        var index = 0
        while index < items.count, pausedReason == nil {
            let item = items[index]
            if item.rejectedReason != nil || (item.nextAttemptAt.map { $0 > now() } ?? false) {
                index += 1
                continue
            }
            let outcome = await attempt(item, via: transport)
            // The actor may have run enqueue() during the await; find the
            // item again by ID rather than trusting the old position.
            guard let i = items.firstIndex(where: { $0.id == item.id }) else { continue }
            switch outcome {
            case .done:
                // As updated during the attempt: it now has the server's id.
                let done = items.remove(at: i)
                let file = directory.appending(path: done.fileName)
                // The phone has the photo already: no need to fetch it again.
                let kept = done.keepOriginal == true && done.photoID.map { keep?(file, $0) ?? false } ?? false
                if !kept { try? FileManager.default.removeItem(at: file) }
                lastError = nil
                index = i
            case .retryLater(let updated, let reason):
                items[i] = updated
                lastError = reason
                save()
                return
            case .skip(let updated):
                items[i] = updated
                index = i + 1
            case .pause(let reason):
                pausedReason = reason
            }
            save()
        }
    }

    private enum Outcome {
        case done
        case retryLater(PendingUpload, String)
        case skip(PendingUpload)
        case pause(String)
    }

    private func attempt(_ original: PendingUpload, via transport: UploadTransport) async -> Outcome {
        var item = original
        do {
            if !item.bytesUploaded {
                let created = try await transport.createUpload(clientPhotoID: item.id, sizeBytes: item.sizeBytes, sha256: item.sha256)
                item.photoID = created.photoID
                if !created.alreadyUploaded {
                    guard let url = created.uploadURL else { throw UploadFailure.transient("no upload URL") }
                    try await transport.put(file: directory.appending(path: item.fileName), to: url, headers: created.headers)
                }
                item.bytesUploaded = true
                // Remember the bytes are up, so a crash before Complete does
                // not upload them again.
                if let i = items.firstIndex(where: { $0.id == item.id }) { items[i] = item }
                save()
            }
            try await transport.complete(completeRequest(for: item))
            return .done
        } catch let failure as UploadFailure {
            switch failure {
            case .notReceived:
                item.bytesUploaded = false
                return backoff(item, reason: "upload was not received; retrying")
            case .transient(let reason):
                return backoff(item, reason: reason)
            case .unauthorized(let reason):
                return .pause(String(localized: "Sign-in problem: \(reason)"))
            case .rejected(let reason):
                item.rejectedReason = reason
                return .skip(item)
            }
        } catch {
            return backoff(item, reason: error.localizedDescription)
        }
    }

    private func backoff(_ original: PendingUpload, reason: String) -> Outcome {
        var item = original
        item.attempts += 1
        let delay = min(Self.maxBackoff, pow(2, Double(min(item.attempts, 10))))
        item.nextAttemptAt = now().addingTimeInterval(delay)
        return .retryLater(item, reason)
    }

    private func completeRequest(for item: PendingUpload) throws -> Pidex_V1_CompleteUploadRequest {
        var r = Pidex_V1_CompleteUploadRequest()
        r.photoID = item.photoID ?? ""
        r.sessionID = item.sessionID
        r.capturedAt = .init(date: item.capturedAt)
        r.onDevice = try Pidex_V1_OnDeviceAnalysis(serializedBytes: item.onDevice)
        if let locked = item.lockedSubjectID {
            r.lockedSubjectID = locked
            r.subjectChosen = item.subjectChosen ?? false
        } else {
            // Queues written before this was kept: each shot stood for itself.
            r.identityClientPhotoID = item.identityPhotoID ?? item.id
        }
        if let place = item.place {
            r.location = place.geoPoint
            if let named = place.place { r.place = named }
        }
        return r
    }

    private func nextWakeDelay() -> TimeInterval? {
        let due = items.filter { $0.rejectedReason == nil }.compactMap(\.nextAttemptAt)
        guard let soonest = due.min() else { return nil }
        return max(1, soonest.timeIntervalSince(now()))
    }

    private func save() {
        do { try persist() } catch { lastError = "could not save upload queue: \(error.localizedDescription)" }
        publish()
    }

    private func persist() throws {
        let data = try JSONEncoder().encode(items)
        try data.write(to: directory.appending(path: "index.json"), options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
    }

    private func publish() {
        let waiting = items.filter { $0.rejectedReason == nil }
        onChange(UploadStatus(
            pending: waiting.count,
            rejected: items.filter { $0.rejectedReason != nil }.count,
            pausedReason: pausedReason,
            lastError: lastError,
            pendingBySession: Dictionary(grouping: waiting, by: \.sessionID).mapValues(\.count)
        ))
    }
}
