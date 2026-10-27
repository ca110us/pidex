import Foundation
import Testing
@testable import Pidex

struct DiskImagesTests {
    let root = FileManager.default.temporaryDirectory.appending(path: "images-\(UUID().uuidString)")

    private func id(_ n: Int) -> String { String(format: "00000000-0000-0000-0000-%012d", n) }

    @Test func keepsByPhotoIDAndDropsTheLeastRecentlySeen() throws {
        let images = DiskImages(name: "Test", budget: { 1000 }, root: root)
        for n in 1...3 { images.write(Data(repeating: UInt8(n), count: 400), id: id(n)) }
        // Seen in this order, the first last.
        for (n, ago) in [(1, 0.0), (2, 300), (3, 200)] {
            let file = try #require(images.directory).appending(path: id(n) + ".jpg")
            try FileManager.default.setAttributes([.modificationDate: Date.now.addingTimeInterval(-ago)], ofItemAtPath: file.path)
        }
        images.trim()
        #expect(images.contains(id(1)))
        #expect(!images.contains(id(2)))
        #expect(images.size <= 900)
        #expect(images.data(id(1)) == Data(repeating: 1, count: 400))
    }

    @Test func takesUploadedFilesAndForgets() throws {
        let images = DiskImages(name: "Test", budget: { 1 << 20 }, root: root)
        let upload = FileManager.default.temporaryDirectory.appending(path: "\(UUID().uuidString).jpg")
        try Data(repeating: 9, count: 10).write(to: upload)
        #expect(images.adopt(upload, id: id(7)))
        #expect(!FileManager.default.fileExists(atPath: upload.path))
        #expect(images.data(id(7)) == Data(repeating: 9, count: 10))
        images.remove([id(7)])
        #expect(!images.contains(id(7)))
        // Not a photo id: never a file name.
        images.write(Data([1]), id: "../escape")
        #expect(!images.contains("../escape"))
    }

    @Test func noBudgetKeepsNothing() throws {
        let images = DiskImages(name: "Test", budget: { 0 }, root: root)
        images.write(Data([1, 2, 3]), id: id(1))
        let upload = FileManager.default.temporaryDirectory.appending(path: "\(UUID().uuidString).jpg")
        try Data([1]).write(to: upload)
        #expect(!images.adopt(upload, id: id(2)))
        #expect(images.size == 0)
    }
}
