import Foundation
import UIKit

/// A shot kept on the phone until the person sends the set: it can still be
/// deleted, and nothing reaches the server until then.
nonisolated struct StagedShot: Codable, Sendable, Identifiable, Equatable {
    var id: String
    var fileName: String
    var capturedAt: Date
    /// Serialized Pidex_V1_OnDeviceAnalysis.
    var onDevice: Data
    /// What this shot identified, when it showed a label or code.
    var mark: Mark?
    /// Who the person said this shot and the ones after it show.
    var pick: Pick?
    /// Where it was taken, when the phone knew; named once the phone has
    /// asked. Optional so shots kept by older versions still load.
    var place: ShotPlace?
    /// The file as the camera made it, by name and type, when it says more
    /// than the working copy (full size, HEIC). Optional for older shots.
    var original: String?
    var originalType: String?
    /// A Live Photo's motion (QuickTime), when the collection keeps it.
    var motion: String?

    /// The files sent after the photo, beside its working copy.
    func files(in store: ShotStore) -> [StagedFile] {
        var out: [StagedFile] = []
        if let original, let originalType {
            out.append(StagedFile(kind: .original, url: store.directory.appending(path: original), contentType: originalType))
        }
        if let motion {
            out.append(StagedFile(kind: .live, url: store.directory.appending(path: motion), contentType: "video/quicktime"))
        }
        return out
    }

    struct Mark: Codable, Sendable, Equatable {
        enum State: String, Codable, Sendable {
            /// An existing subject.
            case matched
            /// Identity read that matches nobody yet; the server adds it.
            case new
            /// Several subjects fit; someone confirms after the shoot.
            case ambiguous
            /// A code was seen without a connection to look it up.
            case offline
        }
        var state: State
        var subjectID: String?
        /// The subject's name, or what was read for a new one.
        var name: String?
        /// The kind of subject, for naming a new one.
        var typeKey: String?
        /// Who it looks like, when the look was not enough to be sure.
        var likelyID: String?
        var likelyName: String?
    }

    struct Pick: Codable, Sendable, Equatable {
        var subjectID: String
        var name: String
    }
}

/// A file handed to the upload queue beside a shot's working copy.
nonisolated struct StagedFile: Sendable {
    var kind: PendingFile.Kind
    var url: URL
    var contentType: String
}

/// Who a shot goes to, from the identity shots and picks before it.
nonisolated struct ShotOwner: Equatable, Sendable {
    var subjectID: String?
    var name: String?
    /// Picked by hand rather than read off a label.
    var chosen = false
    /// The identity shot or pick that decided it.
    var mark: StagedShot.Mark.State?
    /// The shot that identifies it, when no subject is known yet.
    var identityShotID: String?
    /// Who it looks like, to confirm with one tap.
    var likelyID: String?
    var likelyName: String?
    /// Whether it says who it shows: a subject, or a shot that names a new
    /// one (or a code read offline, looked up once it is sent).
    var identified: Bool { subjectID != nil || mark == .new || mark == .offline }
}

/// Staged shots on disk, per collection, so a shoot survives the app being
/// closed. Photos and an index sit in one directory.
nonisolated struct ShotStore: Sendable {
    let directory: URL

    init(directory: URL) throws {
        self.directory = directory
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    }

    func load() -> [StagedShot] {
        guard let data = try? Data(contentsOf: directory.appending(path: "index.json")),
              let shots = try? JSONDecoder().decode([StagedShot].self, from: data) else { return [] }
        return shots.filter { FileManager.default.fileExists(atPath: url(of: $0).path) }
    }

    func save(_ shots: [StagedShot]) throws {
        try JSONEncoder().encode(shots).write(to: directory.appending(path: "index.json"), options: .atomic)
    }

    func write(_ jpeg: Data, id: String, as ext: String = "jpg") throws -> String {
        let name = id + "." + ext
        try jpeg.write(to: directory.appending(path: name), options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
        return name
    }

    /// Takes a file made elsewhere (a Live Photo's motion), by moving it.
    func adopt(_ file: URL, id: String, as ext: String) throws -> String {
        let name = id + "." + ext
        let to = directory.appending(path: name)
        try? FileManager.default.removeItem(at: to)
        try FileManager.default.moveItem(at: file, to: to)
        return name
    }

    func url(of shot: StagedShot) -> URL { directory.appending(path: shot.fileName) }

    /// The shot's files; those already handed to the upload queue are gone.
    func delete(_ shot: StagedShot) {
        for name in [shot.fileName, shot.original, shot.motion].compactMap({ $0 }) {
            try? FileManager.default.removeItem(at: directory.appending(path: name))
        }
    }

    /// A small image for the tray.
    static func thumbnail(of url: URL) -> UIImage? {
        guard let source = CGImageSourceCreateWithURL(url as CFURL, nil),
              let image = CGImageSourceCreateThumbnailAtIndex(source, 0, [
                  kCGImageSourceCreateThumbnailFromImageAlways: true,
                  kCGImageSourceThumbnailMaxPixelSize: 240,
                  kCGImageSourceCreateThumbnailWithTransform: true,
              ] as CFDictionary) else { return nil }
        return UIImage(cgImage: image)
    }
}
