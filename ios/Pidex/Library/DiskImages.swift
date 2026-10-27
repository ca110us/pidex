import Foundation
import Network
import os

/// Images kept on the phone by photo id, in Application Support (which the
/// system does not clear for space, as it does Caches), out of backups and
/// readable only while the phone is unlocked or they are open. When they
/// outgrow their budget, the least recently seen go first.
nonisolated final class DiskImages: Sendable {
    /// Small, for grids: a thousand are about 40 MB.
    static let thumbnails = DiskImages(name: "Thumbnails", budget: { 1 << 30 })
    /// Originals seen in full, and photos taken on this phone, up to what
    /// the user allows (Settings).
    static let originals = DiskImages(name: "Originals", budget: { OriginalsBudget.current.bytes })

    let directory: URL?
    private let budget: @Sendable () -> Int64
    /// Bytes written since the budget was last checked.
    private let written = OSAllocatedUnfairLock(initialState: Int64(0))

    init(name: String, budget: @escaping @Sendable () -> Int64, root: URL? = nil) {
        self.budget = budget
        let files = FileManager.default
        let root = root ?? (try? files.url(for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: true))
        guard var dir = root?.appending(path: name, directoryHint: .isDirectory) else {
            directory = nil
            return
        }
        // Thumbnails were once kept in Caches.
        if let old = try? files.url(for: .cachesDirectory, in: .userDomainMask, appropriateFor: nil, create: false)
            .appending(path: name, directoryHint: .isDirectory),
           root == nil, !files.fileExists(atPath: dir.path), files.fileExists(atPath: old.path) {
            try? files.moveItem(at: old, to: dir)
        }
        try? files.createDirectory(at: dir, withIntermediateDirectories: true)
        try? files.setAttributes([.protectionKey: FileProtectionType.completeUnlessOpen], ofItemAtPath: dir.path)
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try? dir.setResourceValues(values)
        directory = dir
    }

    private func file(_ id: String) -> URL? {
        // Photo ids are UUIDs; anything else is not a name to trust.
        guard UUID(uuidString: id) != nil else { return nil }
        return directory?.appending(path: id.lowercased() + ".jpg")
    }

    func contains(_ id: String) -> Bool {
        file(id).map { FileManager.default.fileExists(atPath: $0.path) } ?? false
    }

    /// The image, marked as just seen.
    func data(_ id: String) -> Data? {
        guard let f = file(id), let data = try? Data(contentsOf: f) else { return nil }
        try? FileManager.default.setAttributes([.modificationDate: Date.now], ofItemAtPath: f.path)
        return data
    }

    func write(_ data: Data, id: String) {
        guard budget() > 0, let f = file(id), (try? data.write(to: f, options: [.atomic, .completeFileProtectionUnlessOpen])) != nil else { return }
        added(Int64(data.count))
    }

    /// Takes a file this phone already has (a photo it uploaded) rather
    /// than fetching it again. False if it is not kept.
    func adopt(_ source: URL, id: String) -> Bool {
        guard budget() > 0, let f = file(id) else { return false }
        let files = FileManager.default
        try? files.removeItem(at: f)
        guard (try? files.moveItem(at: source, to: f)) != nil else { return false }
        try? files.setAttributes([.protectionKey: FileProtectionType.completeUnlessOpen, .modificationDate: Date.now], ofItemAtPath: f.path)
        added((try? f.resourceValues(forKeys: [.fileSizeKey]).fileSize).flatMap { $0 }.map(Int64.init) ?? 0)
        return true
    }

    func remove(_ ids: [String]) {
        for id in ids {
            if let f = file(id) { try? FileManager.default.removeItem(at: f) }
        }
    }

    /// How much is kept, in bytes.
    var size: Int64 {
        entries().reduce(0) { $0 + $1.size }
    }

    func removeAll() {
        for e in entries() { try? FileManager.default.removeItem(at: e.url) }
    }

    /// Drops the least recently seen until what is kept fits the budget,
    /// with a tenth to spare so this does not run on every write.
    func trim() {
        written.withLock { $0 = 0 }
        let limit = budget()
        var all = entries()
        var total = all.reduce(0) { $0 + $1.size }
        guard total > limit else { return }
        all.sort { $0.seen < $1.seen }
        let target = limit - limit / 10
        for e in all where total > target {
            try? FileManager.default.removeItem(at: e.url)
            total -= e.size
        }
    }

    private func added(_ bytes: Int64) {
        let due = written.withLock { w in
            w += bytes
            return w > max(budget() / 20, 1 << 20)
        }
        if due { Task.detached(priority: .utility) { self.trim() } }
    }

    private func entries() -> [(url: URL, size: Int64, seen: Date)] {
        guard let directory, let urls = try? FileManager.default.contentsOfDirectory(
            at: directory, includingPropertiesForKeys: [.fileSizeKey, .contentModificationDateKey]) else { return [] }
        return urls.compactMap { u in
            guard let v = try? u.resourceValues(forKeys: [.fileSizeKey, .contentModificationDateKey]) else { return nil }
            return (u, Int64(v.fileSize ?? 0), v.contentModificationDate ?? .distantPast)
        }
    }
}

/// How much of the phone originals may take, as the user chose.
nonisolated enum OriginalsBudget: Int, CaseIterable, Identifiable, Sendable {
    case none = 0
    case halfGigabyte = 512
    case oneGigabyte = 1024
    case twoGigabytes = 2048
    case fiveGigabytes = 5120

    static let key = "originalsBudgetMB"

    static var current: OriginalsBudget {
        let raw = UserDefaults.standard.object(forKey: key) as? Int
        return raw.flatMap(OriginalsBudget.init) ?? .twoGigabytes
    }

    var id: Int { rawValue }
    var bytes: Int64 { Int64(rawValue) << 20 }
}

/// Whether the phone is on a network that costs nothing to use: Wi-Fi or
/// wired, not a phone's hotspot, not Low Data Mode.
nonisolated final class Unmetered: Sendable {
    static let shared = Unmetered()
    private let monitor = NWPathMonitor()
    private let state = OSAllocatedUnfairLock(initialState: false)

    private init() {
        monitor.pathUpdateHandler = { [state] path in
            state.withLock { $0 = path.status == .satisfied && !path.isExpensive && !path.isConstrained }
        }
        monitor.start(queue: .global(qos: .utility))
    }

    var now: Bool { state.withLock { $0 } }
}
