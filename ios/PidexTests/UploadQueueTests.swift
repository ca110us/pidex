import Foundation
import Testing
@testable import Pidex

final class TestClock: @unchecked Sendable {
    var now = Date(timeIntervalSince1970: 1_800_000_000)
}

actor FakeTransport: UploadTransport {
    var calls: [String] = []
    var createFailures: [UploadFailure] = []
    var putFailures: [UploadFailure] = []
    var completeFailures: [UploadFailure] = []
    var alreadyUploaded = false
    var completed: [Pidex_V1_CompleteUploadRequest] = []
    var fileFailures: [UploadFailure] = []
    var filesReceived: [String] = []

    func failNextPut(_ f: UploadFailure) { putFailures.append(f) }
    func failNextComplete(_ f: UploadFailure) { completeFailures.append(f) }
    func failNextCreate(_ f: UploadFailure) { createFailures.append(f) }
    func setAlreadyUploaded(_ v: Bool) { alreadyUploaded = v }
    func failNextFile(_ f: UploadFailure) { fileFailures.append(f) }

    func createUpload(clientPhotoID: String, sizeBytes: Int64, sha256: String) async throws(UploadFailure) -> CreatedUpload {
        calls.append("create")
        if !createFailures.isEmpty { throw createFailures.removeFirst() }
        return CreatedUpload(photoID: "photo-" + clientPhotoID, uploadURL: URL(string: "https://s3.test/put"),
                             headers: ["Content-Type": "image/jpeg"], alreadyUploaded: alreadyUploaded)
    }

    func put(file: URL, to url: URL, headers: [String: String]) async throws(UploadFailure) {
        calls.append("put")
        if !putFailures.isEmpty { throw putFailures.removeFirst() }
        guard FileManager.default.fileExists(atPath: file.path()) else { throw .transient("file missing") }
    }

    func complete(_ request: Pidex_V1_CompleteUploadRequest) async throws(UploadFailure) {
        calls.append("complete")
        if !completeFailures.isEmpty { throw completeFailures.removeFirst() }
        completed.append(request)
    }

    func createFileUpload(photoID: String, file: PendingFile) async throws(UploadFailure) -> CreatedUpload {
        calls.append("create \(file.kind.rawValue)")
        if !fileFailures.isEmpty { throw fileFailures.removeFirst() }
        return CreatedUpload(photoID: photoID, uploadURL: URL(string: "https://s3.test/put"), headers: [:], alreadyUploaded: false)
    }

    func completeFileUpload(photoID: String, file: PendingFile) async throws(UploadFailure) {
        calls.append("complete \(file.kind.rawValue)")
        filesReceived.append("\(photoID) \(file.kind.rawValue) \(file.contentType) \(file.sizeBytes)")
    }
}

struct UploadQueueTests {
    let dir = FileManager.default.temporaryDirectory.appending(path: "uq-\(UUID().uuidString)")
    let clock = TestClock()
    let transport = FakeTransport()

    func makeQueue() throws -> UploadQueue {
        try UploadQueue(directory: dir, transport: transport, now: { [clock] in clock.now }, autoDrain: false)
    }

    func analysis() -> Pidex_V1_OnDeviceAnalysis {
        var a = Pidex_V1_OnDeviceAnalysis()
        var b = Pidex_V1_BarcodeObservation()
        b.symbology = "code128"
        b.payload = "12345678"
        a.barcodes = [b]
        a.perceptualHash = "f0f0f0f0f0f0f0f0"
        return a
    }

    func enqueue(_ q: UploadQueue, locked: String? = nil) async throws {
        try await q.enqueue(jpeg: Data(repeating: 7, count: 64), sessionID: "s1", capturedAt: clock.now,
                            onDevice: analysis(), lockedSubjectID: locked)
    }

    func jpegFiles() throws -> [URL] {
        try FileManager.default.contentsOfDirectory(at: dir, includingPropertiesForKeys: nil).filter { $0.pathExtension == "jpg" }
    }

    @Test func uploadsAndCleansUp() async throws {
        let q = try makeQueue()
        try await enqueue(q, locked: "subject-1")
        #expect(try jpegFiles().count == 1)

        await q.drainOnce()

        #expect(await transport.calls == ["create", "put", "complete"])
        #expect(await q.pending.isEmpty)
        #expect(try jpegFiles().isEmpty)
        let sent = try #require(await transport.completed.first)
        #expect(sent.sessionID == "s1")
        #expect(sent.lockedSubjectID == "subject-1")
        #expect(sent.onDevice.barcodes.first?.payload == "12345678")
        #expect(sent.photoID.hasPrefix("photo-"))
    }

    @Test func uploadedShotsStayAsOriginalsWhenKept() async throws {
        let kept = FileManager.default.temporaryDirectory.appending(path: "kept-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: kept, withIntermediateDirectories: true)
        let q = try UploadQueue(directory: dir, transport: transport, now: { [clock] in clock.now }, autoDrain: false,
                                keep: { file, id in (try? FileManager.default.moveItem(at: file, to: kept.appending(path: id))) != nil })
        try await q.enqueue(jpeg: Data(repeating: 7, count: 64), sessionID: "s1", capturedAt: clock.now,
                            onDevice: analysis(), lockedSubjectID: "subject-1", id: "a", keepOriginal: true)
        // A high-privacy collection's shot is deleted once uploaded.
        try await q.enqueue(jpeg: Data(repeating: 8, count: 64), sessionID: "s1", capturedAt: clock.now,
                            onDevice: analysis(), lockedSubjectID: "subject-1", id: "b")
        await q.drainOnce()

        #expect(await q.pending.isEmpty)
        #expect(try jpegFiles().isEmpty)
        #expect(try FileManager.default.contentsOfDirectory(atPath: kept.path) == ["photo-a"])
    }

    /// A shot's files as the camera made them, staged where the camera
    /// keeps them.
    func staged(_ id: String) throws -> [StagedFile] {
        let shots = dir.deletingLastPathComponent().appending(path: "shots-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: shots, withIntermediateDirectories: true)
        let original = shots.appending(path: id + ".original"), motion = shots.appending(path: id + ".mov")
        try Data(repeating: 1, count: 300).write(to: original)
        try Data(repeating: 2, count: 500).write(to: motion)
        return [StagedFile(kind: .original, url: original, contentType: "image/heic"),
                StagedFile(kind: .live, url: motion, contentType: "video/quicktime")]
    }

    @Test func filesFollowOnceThePhotosAreIn() async throws {
        let q = try makeQueue()
        for id in ["a", "b"] {
            try await q.enqueue(jpeg: Data(repeating: 7, count: 64), sessionID: "s1", capturedAt: clock.now,
                                onDevice: analysis(), lockedSubjectID: "subject-1", id: id, files: try staged(id))
        }
        // Moved into the queue, not copied.
        #expect(try FileManager.default.contentsOfDirectory(atPath: dir.path).count == 7)
        await transport.failNextFile(.transient("offline"))
        await q.drainOnce()

        // Both photos went first; their files wait for the network.
        #expect(await transport.calls == ["create", "put", "complete", "create", "put", "complete", "create original"])
        let waiting = await q.pending
        #expect(waiting.count == 2 && waiting.allSatisfy { $0.completed == true })
        #expect(try jpegFiles().isEmpty)

        clock.now += UploadQueue.maxBackoff + 1
        await q.drainOnce()
        #expect(await q.pending.isEmpty)
        #expect(await transport.filesReceived == [
            "photo-a original image/heic 300", "photo-a live video/quicktime 500",
            "photo-b original image/heic 300", "photo-b live video/quicktime 500",
        ])
        #expect(try FileManager.default.contentsOfDirectory(atPath: dir.path) == ["index.json"])
    }

    @Test func aFileTheServerWillNotKeepIsDropped() async throws {
        let q = try makeQueue()
        try await q.enqueue(jpeg: Data(repeating: 7, count: 64), sessionID: "s1", capturedAt: clock.now,
                            onDevice: analysis(), lockedSubjectID: "subject-1", id: "a", files: try staged("a"))
        await transport.failNextFile(.rejected("this collection keeps no Live Photos"))
        await q.drainOnce()
        #expect(await q.pending.isEmpty)
        #expect(await transport.filesReceived == ["photo-a live video/quicktime 500"])
    }

    @Test func photosStillToSendAreToldFromFiles() async throws {
        let log = StatusLog()
        let q = try UploadQueue(directory: dir, transport: transport, now: { [clock] in clock.now }, autoDrain: false,
                                onChange: { log.add($0) })
        try await q.enqueue(jpeg: Data(repeating: 7, count: 64), sessionID: "s1", capturedAt: clock.now,
                            onDevice: analysis(), lockedSubjectID: "subject-1", id: "a", files: try staged("a"))
        #expect(log.last?.pending == 1 && log.last?.files == 0)
        await transport.failNextFile(.transient("offline"))
        await q.drainOnce()
        // The batch counts the photo in; the collection still has its files to send.
        #expect(log.last?.pending == 0 && log.last?.files == 1 && log.last?.unfinished == 1)
        #expect(log.last?.pendingBySession["s1"] == nil)
    }

    @Test func networkFailureKeepsPhotoAndBacksOff() async throws {
        let q = try makeQueue()
        try await enqueue(q)
        await transport.failNextPut(.transient("offline"))

        await q.drainOnce()
        let item = try #require(await q.pending.first)
        #expect(item.attempts == 1)
        #expect(item.nextAttemptAt == clock.now.addingTimeInterval(2))
        #expect(try jpegFiles().count == 1)

        await q.drainOnce() // not due yet: nothing happens
        #expect(await transport.calls == ["create", "put"])

        clock.now.addTimeInterval(3)
        await q.drainOnce()
        #expect(await q.pending.isEmpty)
        #expect(await transport.calls == ["create", "put", "create", "put", "complete"])
    }

    @Test func alreadyUploadedSkipsThePut() async throws {
        let q = try makeQueue()
        try await enqueue(q)
        await transport.setAlreadyUploaded(true)
        await q.drainOnce()
        #expect(await transport.calls == ["create", "complete"])
    }

    @Test func bytesAreNotSentTwiceAfterAFailedComplete() async throws {
        let q = try makeQueue()
        try await enqueue(q)
        await transport.failNextComplete(.transient("timeout"))
        await q.drainOnce()
        #expect(try #require(await q.pending.first).bytesUploaded)

        clock.now.addTimeInterval(10)
        await q.drainOnce()
        #expect(await transport.calls == ["create", "put", "complete", "complete"])
    }

    @Test func serverMissingBytesTriggersReupload() async throws {
        let q = try makeQueue()
        try await enqueue(q)
        await transport.failNextComplete(.notReceived)
        await q.drainOnce()
        #expect(try #require(await q.pending.first).bytesUploaded == false)

        clock.now.addTimeInterval(10)
        await q.drainOnce()
        #expect(await transport.calls == ["create", "put", "complete", "create", "put", "complete"])
    }

    @Test func authProblemPausesUntilResumed() async throws {
        let q = try makeQueue()
        try await enqueue(q)
        try await enqueue(q)
        await transport.failNextCreate(.unauthorized("invalid token"))

        await q.drainOnce()
        await q.drainOnce()
        #expect(await transport.calls == ["create"])
        #expect(await q.pending.count == 2)

        await q.resume()
        await q.drainOnce()
        #expect(await q.pending.isEmpty)
    }

    @Test func rejectedPhotoIsKeptAndDoesNotBlockOthers() async throws {
        let q = try makeQueue()
        try await enqueue(q)
        try await enqueue(q)
        await transport.failNextCreate(.rejected("size_bytes too large"))

        await q.drainOnce()
        let left = await q.pending
        #expect(left.count == 1)
        #expect(left.first?.rejectedReason == "size_bytes too large")
        #expect(try jpegFiles().count == 1, "a rejected photo is never deleted")
    }

    @Test func queueSurvivesRestart() async throws {
        let first = try makeQueue()
        try await enqueue(first)
        try await enqueue(first)

        let second = try makeQueue()
        #expect(await second.pending.count == 2)
        await second.drainOnce()
        #expect(await second.pending.isEmpty)
    }
}

final class StatusLog: @unchecked Sendable {
    private let lock = NSLock()
    private var statuses: [UploadStatus] = []

    func add(_ s: UploadStatus) { lock.withLock { statuses.append(s) } }
    var last: UploadStatus? { lock.withLock { statuses.last } }
}
