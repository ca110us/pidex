@preconcurrency import AVFoundation
import Foundation
import Photos

/// A Live Photo picked from the library, as its two files: the still as the
/// camera made it, and its motion with where it was taken left out.
nonisolated enum LiveImport {
    static func files(of live: PHLivePhoto) async -> (still: Data, motion: URL)? {
        let resources = PHAssetResource.assetResources(for: live)
        guard let photo = resources.first(where: { $0.type == .photo }),
              let video = resources.first(where: { $0.type == .pairedVideo }) else { return nil }
        let dir = FileManager.default.temporaryDirectory.appending(path: "LiveImport-" + UUID().uuidString, directoryHint: .isDirectory)
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let stillFile = dir.appending(path: "still"), movieFile = dir.appending(path: "motion.mov")
        guard await write(photo, to: stillFile), await write(video, to: movieFile),
              let still = try? Data(contentsOf: stillFile),
              let motion = await withoutLocation(movieFile, in: dir) else {
            try? FileManager.default.removeItem(at: dir)
            return nil
        }
        // The still is read; the motion's file is moved by whoever keeps it.
        try? FileManager.default.removeItem(at: stillFile)
        return (still, motion)
    }

    private static func write(_ resource: PHAssetResource, to file: URL) async -> Bool {
        await withCheckedContinuation { cont in
            let options = PHAssetResourceRequestOptions()
            options.isNetworkAccessAllowed = true
            PHAssetResourceManager.default().writeData(for: resource, toFile: file, options: options) { error in
                cont.resume(returning: error == nil)
            }
        }
    }

    /// The motion with any location taken out, copied as it is otherwise;
    /// the same file when it has none.
    static func withoutLocation(_ movie: URL, in dir: URL) async -> URL? {
        let asset = AVURLAsset(url: movie)
        guard let metadata = try? await asset.load(.metadata) else { return nil }
        let located: (AVMetadataItem) -> Bool = { item in
            item.identifier == .quickTimeMetadataLocationISO6709 || item.identifier == .commonIdentifierLocation
                || item.identifier == .quickTimeUserDataLocationISO6709
        }
        guard metadata.contains(where: located) else { return movie }
        guard let export = AVAssetExportSession(asset: asset, presetName: AVAssetExportPresetPassthrough) else { return nil }
        let out = dir.appending(path: "motion-clean.mov")
        export.metadata = metadata.filter { !located($0) }
        if #available(iOS 18, *) {
            guard (try? await export.export(to: out, as: .mov)) != nil else { return nil }
        } else {
            export.outputURL = out
            export.outputFileType = .mov
            await export.export()
            guard export.status == .completed else { return nil }
        }
        try? FileManager.default.removeItem(at: movie)
        return out
    }
}
