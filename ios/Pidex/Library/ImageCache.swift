import SwiftUI
import UIKit

/// Photos' images by photo id: in memory, then on the phone (DiskImages),
/// then from the server. Signed links change on every fetch, so caching by
/// URL would never hit; a photo id is stable, so a conversation or
/// collection reopened shows its photos at once, offline too. A photo with
/// no link, or a link expired, gets a fresh one.
actor ImageCache {
    static let shared = ImageCache()

    private let memory = NSCache<NSString, UIImage>()
    /// Originals are large: a few in memory.
    private let originals = NSCache<NSString, UIImage>()
    private var inFlight: [String: Task<UIImage?, Never>] = [:]

    init() {
        memory.countLimit = 500
        originals.countLimit = 12
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

    /// A photo in full. keep: it may stay on the phone once fetched (not
    /// for high-privacy teams).
    func original(id: String, url: URL?, keep: Bool) async -> UIImage? {
        if let hit = originals.object(forKey: id as NSString) { return hit }
        if let data = DiskImages.originals.data(id), let image = UIImage(data: data) {
            originals.setObject(image, forKey: id as NSString)
            return image
        }
        guard let (data, image) = await download(id: id, url: url, original: true) else { return nil }
        if keep { DiskImages.originals.write(data, id: id) }
        originals.setObject(image, forKey: id as NSString)
        return image
    }

    func image(id: String, url: URL?) async -> UIImage? {
        if let hit = memory.object(forKey: id as NSString) { return hit }
        if let data = DiskImages.thumbnails.data(id), let image = UIImage(data: data) {
            memory.setObject(image, forKey: id as NSString)
            return image
        }
        if let pending = inFlight[id] { return await pending.value }
        let task = Task<UIImage?, Never> {
            guard let (data, image) = await self.download(id: id, url: url, original: false) else { return nil }
            DiskImages.thumbnails.write(data, id: id)
            return image
        }
        inFlight[id] = task
        let image = await task.value
        inFlight[id] = nil
        if let image { memory.setObject(image, forKey: id as NSString) }
        return image
    }

    /// Fetches thumbnails not on the phone yet, a few at a time, so a
    /// collection can be browsed offline.
    func prefetch(_ ids: [String]) async {
        let missing = ids.filter { memory.object(forKey: $0 as NSString) == nil && !DiskImages.thumbnails.contains($0) }
        await withTaskGroup(of: Void.self) { group in
            for (i, id) in missing.enumerated() {
                if i >= 4 { await group.next() }
                if Task.isCancelled { break }
                group.addTask { _ = await self.image(id: id, url: nil) }
            }
        }
    }

    /// Forgets photos deleted, or of a collection removed from the phone.
    func forget(_ ids: [String]) {
        for id in ids {
            memory.removeObject(forKey: id as NSString)
            originals.removeObject(forKey: id as NSString)
        }
        DiskImages.thumbnails.remove(ids)
        DiskImages.originals.remove(ids)
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
