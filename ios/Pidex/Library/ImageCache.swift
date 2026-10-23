import SwiftUI
import UIKit

/// Thumbnails by photo id, in memory and on disk. Signed links change on
/// every fetch, so caching by URL would never hit; a photo id is stable, so
/// a conversation or collection reopened shows its photos at once.
actor ImageCache {
    static let shared = ImageCache()

    private let memory = NSCache<NSString, UIImage>()
    private let directory: URL?
    private var inFlight: [String: Task<UIImage?, Never>] = [:]

    init() {
        directory = try? FileManager.default
            .url(for: .cachesDirectory, in: .userDomainMask, appropriateFor: nil, create: true)
            .appending(path: "Thumbnails", directoryHint: .isDirectory)
        if let directory {
            try? FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        }
        memory.countLimit = 500
    }

    /// Originals are large: a few are kept in memory, none on disk.
    private let originals: NSCache<NSString, UIImage> = {
        let c = NSCache<NSString, UIImage>()
        c.countLimit = 12
        return c
    }()

    func original(id: String, url: URL?) async -> UIImage? {
        if let hit = originals.object(forKey: id as NSString) { return hit }
        guard let url, let (data, response) = try? await URLSession.shared.data(from: url),
              (response as? HTTPURLResponse)?.statusCode == 200,
              let image = UIImage(data: data) else { return nil }
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
        guard let url else { return nil }
        if let pending = inFlight[id] { return await pending.value }
        let task = Task<UIImage?, Never> {
            guard let (data, response) = try? await URLSession.shared.data(from: url),
                  (response as? HTTPURLResponse)?.statusCode == 200,
                  let image = UIImage(data: data) else { return nil }
            if let file { try? data.write(to: file, options: .atomic) }
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
