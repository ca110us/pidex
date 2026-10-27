import SwiftUI
import UIKit

/// Thumbnails by photo id, in memory and on disk. Signed links change on
/// every fetch, so caching by URL would never hit; a photo id is stable, so
/// a conversation or collection reopened shows its photos at once, offline
/// too. A photo with no link, or a link expired, gets a fresh one.
actor ImageCache {
    static let shared = ImageCache()

    private let memory = NSCache<NSString, UIImage>()
    private let directory: URL?
    private var inFlight: [String: Task<UIImage?, Never>] = [:]

    init() {
        // Application Support, which the system does not clear for space as
        // it does Caches: browsing offline needs them. Out of backups (the
        // server has them), readable only while the phone is unlocked.
        let files = FileManager.default
        directory = try? files
            .url(for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: true)
            .appending(path: "Thumbnails", directoryHint: .isDirectory)
        if var directory {
            let old = try? files.url(for: .cachesDirectory, in: .userDomainMask, appropriateFor: nil, create: false)
                .appending(path: "Thumbnails", directoryHint: .isDirectory)
            if let old, !files.fileExists(atPath: directory.path), files.fileExists(atPath: old.path) {
                try? files.moveItem(at: old, to: directory)
            }
            try? files.createDirectory(at: directory, withIntermediateDirectories: true)
            try? files.setAttributes([.protectionKey: FileProtectionType.completeUnlessOpen], ofItemAtPath: directory.path)
            var values = URLResourceValues()
            values.isExcludedFromBackup = true
            try? directory.setResourceValues(values)
        }
        memory.countLimit = 500
    }

    /// The image at url, else at a fresh link: none was given, or it expired.
    private func download(id: String, url: URL?, original: Bool) async -> (Data, UIImage)? {
        if let url, let got = await Self.get(url) { return got }
        guard let fresh = await PhotoLinks.shared.link(for: id, original: original), fresh != url else { return nil }
        return await Self.get(fresh)
    }

    private static func get(_ url: URL) async -> (Data, UIImage)? {
        guard let (data, response) = try? await URLSession.shared.data(from: url),
              (response as? HTTPURLResponse)?.statusCode == 200,
              let image = UIImage(data: data) else { return nil }
        return (data, image)
    }

    /// Originals are large: a few are kept in memory, none on disk.
    private let originals: NSCache<NSString, UIImage> = {
        let c = NSCache<NSString, UIImage>()
        c.countLimit = 12
        return c
    }()

    func original(id: String, url: URL?) async -> UIImage? {
        if let hit = originals.object(forKey: id as NSString) { return hit }
        guard let (_, image) = await download(id: id, url: url, original: true) else { return nil }
        originals.setObject(image, forKey: id as NSString)
        return image
    }

    func image(id: String, url: URL?) async -> UIImage? {
        if let hit = memory.object(forKey: id as NSString) { return hit }
        let file = directory?.appending(path: id + ".jpg")
        if let file, let data = try? Data(contentsOf: file), let image = UIImage(data: data) {
            memory.setObject(image, forKey: id as NSString)
            return image
        }
        if let pending = inFlight[id] { return await pending.value }
        let task = Task<UIImage?, Never> {
            guard let (data, image) = await self.download(id: id, url: url, original: false) else { return nil }
            if let file { try? data.write(to: file, options: [.atomic, .completeFileProtectionUnlessOpen]) }
            return image
        }
        inFlight[id] = task
        let image = await task.value
        inFlight[id] = nil
        if let image { memory.setObject(image, forKey: id as NSString) }
        return image
    }
}

/// A photo's thumbnail through the cache.
struct CachedThumbnail: View {
    let id: String
    let url: URL?
    @State private var image: UIImage?
    @State private var failed = false

    var body: some View {
        ZStack {
            if let image {
                Image(uiImage: image).resizable().scaledToFill()
            } else if failed {
                Image(systemName: "photo").foregroundStyle(.secondary)
            } else {
                ProgressView()
            }
        }
        .task(id: id) {
            image = await ImageCache.shared.image(id: id, url: url)
            failed = image == nil
        }
    }
}
