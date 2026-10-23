import Foundation
import Testing
import UIKit
@testable import Pidex

/// Shots stay on the phone until sent: they can be deleted, a person can
/// say who they show, and only what is left is sent, each with its subject.
@MainActor
struct ShotTrayTests {
    let root = FileManager.default.temporaryDirectory.appending(path: "tray-\(UUID().uuidString)")

    func makeModel() throws -> (CaptureModel, UploadQueue) {
        let queue = try UploadQueue(directory: root.appending(path: "queue"), transport: nil, autoDrain: false)
        // Nothing here reaches a server: plain photos carry no label to look up.
        let api = APIClient(baseURL: URL(string: "http://127.0.0.1:9")!, token: "test")
        return (CaptureModel(api: api, uploads: queue, staging: root.appending(path: "staged")), queue)
    }

    func photo(_ color: UIColor) -> Data {
        UIGraphicsImageRenderer(size: CGSize(width: 64, height: 64)).jpegData(withCompressionQuality: 0.9) { ctx in
            color.setFill()
            ctx.fill(CGRect(x: 0, y: 0, width: 64, height: 64))
        }
    }

    func subject(_ id: String, _ name: String) -> Pidex_V1_Subject {
        var s = Pidex_V1_Subject()
        s.id = id
        s.displayName = name
        return s
    }

    @Test func keepsShotsUntilSentAndSendsOnlyWhatIsLeft() async throws {
        let (model, queue) = try makeModel()
        await model.importPhoto(photo(.red))
        await model.importPhoto(photo(.green))
        await model.importPhoto(photo(.blue))
        #expect(model.shots.count == 3)
        #expect(await queue.pending.isEmpty, "nothing is uploaded while shooting")
        #expect(model.needsIdentity, "no label photographed and nobody chosen")
        #expect(!model.canSend && model.unidentified.count == 3)
        #expect(await model.sendAll() == 0, "nothing goes out unidentified")

        model.delete(model.shots[1].id)
        #expect(model.shots.count == 2)

        // Chosen for the group: the shots already taken and those after.
        model.choose(subject("s-1", "Mimi"))
        await model.importPhoto(photo(.yellow))
        #expect(!model.needsIdentity)
        #expect(model.owners.values.allSatisfy { $0.subjectID == "s-1" && $0.chosen })

        // The shoot survives the app closing.
        let (reopened, _) = try makeModel()
        #expect(reopened.shots.map(\.id) == model.shots.map(\.id))

        #expect(await model.sendAll() == 3)
        let sent = await queue.pending
        #expect(sent.count == 3)
        #expect(sent.allSatisfy { $0.lockedSubjectID == "s-1" && $0.subjectChosen == true })
        #expect(Set(sent.map(\.id)).count == 3, "each keeps its shot's ID, for others to name")
        #expect(model.shots.isEmpty)
        #expect(try makeModel().0.shots.isEmpty, "sent shots leave the phone's staging area")
    }

    @Test func aPickMadeFirstAppliesToTheNextShots() async throws {
        let (model, _) = try makeModel()
        model.choose(subject("s-2", "Luna"))
        #expect(model.current.name == "Luna")
        await model.importPhoto(photo(.gray))
        await model.importPhoto(photo(.white))
        #expect(model.shots.first?.pick?.subjectID == "s-2")
        #expect(model.owners[model.shots[1].id]?.name == "Luna")

        // Deleting the shot that carried the pick keeps it for the group.
        model.delete(model.shots[0].id)
        #expect(model.owners[model.shots[0].id]?.subjectID == "s-2")
    }

    @Test func aChoiceCanBeTakenBack() async throws {
        let (model, _) = try makeModel()
        // Chosen before shooting, then taken back: the next shots go to nobody.
        model.choose(subject("s-3", "Tutu"))
        model.clearChoice()
        #expect(!model.current.chosen && model.current.subjectID == nil)

        // Chosen for shots already taken: they say nobody again, and none
        // may be sent until someone says who.
        await model.importPhoto(photo(.red))
        await model.importPhoto(photo(.green))
        model.choose(subject("s-3", "Tutu"))
        #expect(model.unidentified.isEmpty)
        model.clearChoice()
        #expect(model.shots.allSatisfy { $0.pick == nil })
        #expect(model.unidentified.count == 2 && !model.canSend)
        let (reopened, _) = try makeModel()
        #expect(reopened.shots.allSatisfy { $0.pick == nil }, "taken back for good, not only on screen")
    }

    @Test func discardingThrowsAwayEveryShot() async throws {
        let (model, queue) = try makeModel()
        await model.importPhoto(photo(.red))
        model.discardAll()
        #expect(model.shots.isEmpty)
        #expect(await queue.pending.isEmpty)
    }
}

/// One bar for a batch: uploading fills its first half, filing the second.
@MainActor
struct UploadBatchTests {
    @Test func uploadingIsTheFirstHalf() {
        let api = APIClient(baseURL: URL(string: "http://127.0.0.1:9")!, token: "test")
        let batch = UploadBatch(sessionID: UUID(), api: api, collection: "Pets", expected: 4)
        #expect(batch.phase(pending: 4) == .uploading(done: 0))
        #expect(batch.progress(pending: 2) == 0.25)
        #expect(batch.phase(pending: 0) == .filing(done: 0), "uploaded, not yet seen filed")
        #expect(batch.progress(pending: 0) == 0.5)
        batch.add(2)
        #expect(batch.expected == 6)
        #expect(batch.phase(pending: 3) == .uploading(done: 3))
    }
}
