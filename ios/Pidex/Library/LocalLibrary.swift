import CryptoKit
import Foundation
import GRDB
import os
import SwiftProtobuf

/// A collection's photos and subjects as kept on this phone, following the
/// server (which stays the truth): a collection opens at once, can be
/// browsed with no connection, and only what changed is fetched again.
/// One database file per collection, named from its token (never the token
/// itself), readable only while the phone is unlocked or the file is open.
nonisolated final class LocalLibrary: Sendable {
    private let db: DatabaseQueue
    private let syncing = OSAllocatedUnfairLock<Task<Void, Error>?>(initialState: nil)

    init(file: URL) throws {
        var config = Configuration()
        config.label = "LocalLibrary"
        // One connection: it is opened while the phone is unlocked and kept
        // open, so its files stay readable while it is.
        db = try DatabaseQueue(path: file.path, configuration: config)
        try Self.migrator.migrate(db)
    }

    static var migrator: DatabaseMigrator {
        var m = DatabaseMigrator()
        m.registerMigration("v1") { db in
            try db.execute(sql: """
                CREATE TABLE photo (
                    id TEXT PRIMARY KEY,
                    -- When it was taken, else received: newest first.
                    at REAL NOT NULL,
                    -- Processed and not junk, as search shows photos.
                    shown INTEGER NOT NULL,
                    subject_id TEXT NOT NULL,
                    -- Its place at each level the page groups by.
                    country TEXT NOT NULL, city TEXT NOT NULL, city_in TEXT NOT NULL,
                    area TEXT NOT NULL, area_in TEXT NOT NULL,
                    data BLOB NOT NULL
                );
                CREATE INDEX photo_shown_at ON photo (shown, at DESC, id DESC);
                CREATE TABLE subject (id TEXT PRIMARY KEY, data BLOB NOT NULL);
                -- The sync cursor, and responses kept for when offline.
                CREATE TABLE kept (key TEXT PRIMARY KEY, data BLOB NOT NULL);
                """)
        }
        m.registerMigration("v2: matching on the phone") { db in
            try db.execute(sql: """
                -- Tags as |key|key|; every part of its place; and its words
                -- (description, fields read, tags, place) to match a search.
                ALTER TABLE photo ADD COLUMN tags TEXT NOT NULL DEFAULT '';
                ALTER TABLE photo ADD COLUMN place_text TEXT NOT NULL DEFAULT '';
                ALTER TABLE photo ADD COLUMN search_text TEXT NOT NULL DEFAULT '';
                ALTER TABLE subject ADD COLUMN name TEXT NOT NULL DEFAULT '';
                ALTER TABLE subject ADD COLUMN type_key TEXT NOT NULL DEFAULT '';
                """)
            for data in try Data.fetchAll(db, sql: "SELECT data FROM photo") {
                if let p = try? Pidex_V1_Photo(serializedBytes: data) { try save(p, in: db) }
            }
            for data in try Data.fetchAll(db, sql: "SELECT data FROM subject") {
                if let s = try? Pidex_V1_Subject(serializedBytes: data) { try save(s, in: db) }
            }
        }
        return m
    }

    // MARK: Sync

    /// Brings the copy up to date, a page at a time until nothing is left;
    /// callers at the same time share one sync.
    func sync(with api: APIClient) async throws {
        let task = syncing.withLock { current in
            if let current { return current }
            let task = Task { try await self.pull(api) }
            current = task
            return task
        }
        defer { syncing.withLock { if $0 == task { $0 = nil } } }
        try await task.value
    }

    private func pull(_ api: APIClient) async throws {
        while true {
            let r = try await api.syncLibrary(cursor: cursor)
            try apply(r)
            if !r.more { return }
        }
    }

    /// Keeps a page of changes, with the cursor that follows it, at once:
    /// interrupted, the next sync asks for the same page again.
    func apply(_ r: Pidex_V1_SyncLibraryResponse) throws {
        try db.write { db in
            if r.startOver {
                try db.execute(sql: "DELETE FROM photo; DELETE FROM subject")
            }
            for p in r.photos {
                try Self.save(p, in: db)
            }
            for s in r.subjects {
                try Self.save(s, in: db)
            }
            for id in r.deletedPhotoIds {
                try db.execute(sql: "DELETE FROM photo WHERE id = ?", arguments: [id])
            }
            for id in r.deletedSubjectIds {
                try db.execute(sql: "DELETE FROM subject WHERE id = ?", arguments: [id])
            }
            try Self.keep(Data(r.cursor.utf8), as: "cursor", in: db)
        }
        // Deleted on the server: their images go from the phone too.
        if !r.deletedPhotoIds.isEmpty {
            let gone = r.deletedPhotoIds
            Task { await ImageCache.shared.forget(gone) }
        }
    }

    /// Every photo in the copy, shown or not.
    func photoIDs() throws -> [String] {
        try db.read { db in try String.fetchAll(db, sql: "SELECT id FROM photo") }
    }

    private static func save(_ p: Pidex_V1_Photo, in db: Database) throws {
        let at = p.hasCapturedAt ? p.capturedAt.date : p.createdAt.date
        let shown = p.status == .processed && p.role != .junk
        let pl = p.place
        let city = pl.city.isEmpty ? pl.region : pl.city
        let area = pl.name.isEmpty ? pl.district : pl.name
        let places = [pl.country, pl.region, pl.city, pl.district, pl.name].filter { !$0.isEmpty }
        let tags = p.tags.map(\.key)
        let words = ([p.description_p] + p.extractedFields.values.sorted() + tags + places).joined(separator: "\n")
        let data: Data = try p.serializedData()
        try db.execute(sql: """
            INSERT OR REPLACE INTO photo
                (id, at, shown, subject_id, country, city, city_in, area, area_in, tags, place_text, search_text, data)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """, arguments: [p.id, at.timeIntervalSince1970, shown, p.subjectID,
                             pl.country, city, pl.country, area, city,
                             tags.isEmpty ? "" : "|" + tags.joined(separator: "|") + "|",
                             places.joined(separator: "\n"), words, data])
    }

    private static func save(_ s: Pidex_V1_Subject, in db: Database) throws {
        let data: Data = try s.serializedData()
        try db.execute(sql: "INSERT OR REPLACE INTO subject (id, name, type_key, data) VALUES (?, ?, ?, ?)",
                       arguments: [s.id, s.displayName, s.typeKey, data])
    }

    /// The cursor to ask from; empty before the first sync.
    var cursor: String {
        (try? db.read { db in try Self.kept("cursor", in: db) }).map { String(decoding: $0, as: UTF8.self) } ?? ""
    }

    /// Whether a sync has ever completed.
    var hasSynced: Bool { !cursor.isEmpty }

    // MARK: Reading

    /// Shown photos, newest first.
    func photos(offset: Int = 0, limit: Int) throws -> [Pidex_V1_Photo] {
        try db.read { db in
            try Data.fetchAll(db, sql: "SELECT data FROM photo WHERE shown ORDER BY at DESC, id DESC LIMIT ? OFFSET ?",
                              arguments: [limit, offset])
                .compactMap { try? Pidex_V1_Photo(serializedBytes: $0) }
        }
    }

    /// The newest shown photos' ids.
    func shownIDs(limit: Int) throws -> [String] {
        try db.read { db in
            try String.fetchAll(db, sql: "SELECT id FROM photo WHERE shown ORDER BY at DESC, id DESC LIMIT ?", arguments: [limit])
        }
    }

    func shownCount() throws -> Int {
        try db.read { db in try Int.fetchOne(db, sql: "SELECT count(*) FROM photo WHERE shown") ?? 0 }
    }

    /// How many shown photos were taken in each place, at every level, as
    /// the server counts them.
    func placeCounts() throws -> [Pidex_V1_PlaceCount] {
        try db.read { db in
            try Row.fetchAll(db, sql: """
                SELECT 'country' AS level, country AS name, '' AS parent, count(*) AS n FROM photo
                    WHERE shown AND country <> '' GROUP BY country
                UNION ALL
                SELECT 'city', city, city_in, count(*) FROM photo WHERE shown AND city <> '' GROUP BY city, city_in
                UNION ALL
                SELECT 'area', area, area_in, count(*) FROM photo WHERE shown AND area <> '' GROUP BY area, area_in
                ORDER BY n DESC, name
                """).map { row in
                var c = Pidex_V1_PlaceCount()
                c.level = row["level"]
                c.name = row["name"]
                c.parent = row["parent"]
                c.count = Int32(row["n"] as Int)
                return c
            }
        }
    }

    func subjects() throws -> [Pidex_V1_Subject] {
        try db.read { db in
            try Data.fetchAll(db, sql: "SELECT data FROM subject").compactMap { try? Pidex_V1_Subject(serializedBytes: $0) }
        }
    }

    // MARK: Matching on the phone

    /// Whether the phone can find what filters ask for as the server
    /// would: picked tags, a place, days, subjects. A search by meaning
    /// (text, or tags only preferred), or for what is missing, is the
    /// server's.
    static func matchesExactly(_ f: Pidex_V1_SearchFilters) -> Bool {
        f.mode != .missing && f.text.isEmpty && f.roles.isEmpty && f.subjectType.isEmpty
            && (f.tags.isEmpty || f.tagsRequired)
    }

    /// Shown photos that match filters, newest first. Offline, text is
    /// matched as words in what the phone knows of each photo, and tags
    /// only preferred are required.
    func photos(matching f: Pidex_V1_SearchFilters, offset: Int = 0, limit: Int) throws -> [Pidex_V1_Photo] {
        let (sql, args) = Self.conditions(f)
        return try db.read { db in
            try Data.fetchAll(db, sql: "SELECT data FROM photo WHERE \(sql) ORDER BY at DESC, id DESC LIMIT ? OFFSET ?",
                              arguments: args + [limit, offset])
                .compactMap { try? Pidex_V1_Photo(serializedBytes: $0) }
        }
    }

    func count(matching f: Pidex_V1_SearchFilters) throws -> Int {
        let (sql, args) = Self.conditions(f)
        return try db.read { db in try Int.fetchOne(db, sql: "SELECT count(*) FROM photo WHERE \(sql)", arguments: args) ?? 0 }
    }

    /// How many of the matches have each tag.
    func tagCounts(matching f: Pidex_V1_SearchFilters) throws -> [String: Int] {
        let (sql, args) = Self.conditions(f)
        var out: [String: Int] = [:]
        for tags in try db.read({ db in try String.fetchAll(db, sql: "SELECT tags FROM photo WHERE \(sql)", arguments: args) }) {
            for key in tags.split(separator: "|") { out[String(key), default: 0] += 1 }
        }
        return out
    }

    /// Subjects whose names contain the words.
    func subjects(named words: String) throws -> [Pidex_V1_Subject] {
        try db.read { db in
            try Data.fetchAll(db, sql: "SELECT data FROM subject WHERE name LIKE ? ESCAPE '\\'", arguments: ["%" + Self.escaped(words) + "%"])
                .compactMap { try? Pidex_V1_Subject(serializedBytes: $0) }
        }
    }

    /// Every name a place of a shown photo goes by.
    func placeNames() throws -> Set<String> {
        let all = try db.read { db in try String.fetchAll(db, sql: "SELECT DISTINCT place_text FROM photo WHERE shown AND place_text <> ''") }
        return Set(all.flatMap { $0.split(separator: "\n").map(String.init) })
    }

    private static func conditions(_ f: Pidex_V1_SearchFilters) -> (String, StatementArguments) {
        var sql = ["shown"]
        var args: StatementArguments = []
        for tag in f.tags {
            sql.append("tags LIKE ? ESCAPE '\\'")
            args += ["%|" + escaped(tag) + "|%"]
        }
        if !f.place.isEmpty {
            sql.append("place_text LIKE ? ESCAPE '\\'")
            args += ["%" + escaped(f.place) + "%"]
        }
        if f.hasFrom {
            sql.append("at >= ?")
            args += [f.from.date.timeIntervalSince1970]
        }
        if f.hasTo {
            sql.append("at < ?")
            args += [f.to.date.timeIntervalSince1970]
        }
        if !f.subjectIds.isEmpty {
            sql.append("subject_id IN (" + f.subjectIds.map { _ in "?" }.joined(separator: ",") + ")")
            args += StatementArguments(f.subjectIds)
        }
        if !f.subjectQuery.isEmpty {
            sql.append("subject_id IN (SELECT id FROM subject WHERE name LIKE ? ESCAPE '\\')")
            args += ["%" + escaped(f.subjectQuery) + "%"]
        }
        for word in f.text.split(whereSeparator: \.isWhitespace) {
            sql.append("search_text LIKE ? ESCAPE '\\'")
            args += ["%" + escaped(String(word)) + "%"]
        }
        return (sql.joined(separator: " AND "), args)
    }

    private static func escaped(_ s: String) -> String {
        s.replacingOccurrences(of: "\\", with: "\\\\").replacingOccurrences(of: "%", with: "\\%").replacingOccurrences(of: "_", with: "\\_")
    }

    // MARK: Responses kept for offline

    func keep(_ message: some SwiftProtobuf.Message, as key: String) throws {
        let data: Data = try message.serializedData()
        try db.write { db in try Self.keep(data, as: "response." + key, in: db) }
    }

    func kept<M: SwiftProtobuf.Message>(_ type: M.Type, as key: String) -> M? {
        guard let data = try? db.read({ db in try Self.kept("response." + key, in: db) }) else { return nil }
        return try? M(serializedBytes: data)
    }

    private static func keep(_ data: Data, as key: String, in db: Database) throws {
        try db.execute(sql: "INSERT OR REPLACE INTO kept (key, data) VALUES (?, ?)", arguments: [key, data])
    }

    private static func kept(_ key: String, in db: Database) throws -> Data? {
        try Data.fetchOne(db, sql: "SELECT data FROM kept WHERE key = ?", arguments: [key])
    }

    // MARK: One per collection

    private static let open = OSAllocatedUnfairLock<[String: LocalLibrary]>(initialState: [:])

    /// The copy for a collection's client, opened once; nil if it cannot be
    /// (the phone then asks the server, as before).
    static func `for`(_ api: APIClient) -> LocalLibrary? {
        open.withLock { libraries in
            if let l = libraries[api.storeKey] { return l }
            guard let dir = directory, let l = try? LocalLibrary(file: dir.appending(path: api.storeKey + ".sqlite")) else { return nil }
            libraries[api.storeKey] = l
            return l
        }
    }

    /// Forgets the copy of a collection removed from this phone, with its
    /// photos' images.
    static func erase(token: String) {
        let key = APIClient.storeKey(for: token)
        open.withLock { libraries in
            guard let dir = directory else { return }
            let file = dir.appending(path: key + ".sqlite")
            let library = libraries[key] ?? (FileManager.default.fileExists(atPath: file.path) ? try? LocalLibrary(file: file) : nil)
            if let ids = try? library?.photoIDs(), !ids.isEmpty {
                Task { await ImageCache.shared.forget(ids) }
            }
            libraries[key] = nil
            for suffix in ["", "-wal", "-shm"] {
                try? FileManager.default.removeItem(at: dir.appending(path: key + ".sqlite" + suffix))
            }
        }
    }

    /// Application Support, which the system does not clear for space;
    /// left out of backups, since the server has it all; files in it
    /// readable only while the phone is unlocked or they are open.
    private static let directory: URL? = {
        guard var dir = try? FileManager.default
            .url(for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: true)
            .appending(path: "Library", directoryHint: .isDirectory) else { return nil }
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true,
                                                 attributes: [.protectionKey: FileProtectionType.completeUnlessOpen])
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try? dir.setResourceValues(values)
        return dir
    }()
}

extension APIClient {
    /// Names what is kept on the phone for a collection: the same token
    /// always gives the same name, and the name does not give the token.
    nonisolated static func storeKey(for token: String) -> String {
        SHA256.hash(data: Data(token.utf8)).prefix(12).map { String(format: "%02x", $0) }.joined()
    }
}
