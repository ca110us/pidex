import Foundation
import GRDB
import SwiftProtobuf
import Testing
@testable import Pidex

/// Finding photos on the phone: as the server would for picked tags, a
/// place, days and subjects; offline, a search in words read by the names
/// the phone knows.
struct LocalMatchingTests {
    let file = FileManager.default.temporaryDirectory.appending(path: "match-\(UUID().uuidString).sqlite")

    private func photo(_ id: String, day: Int, tags: [String] = [], subject: String = "", city: String = "",
                       description: String = "", fields: [String: String] = [:]) -> Pidex_V1_Photo {
        var p = Pidex_V1_Photo()
        p.id = id
        p.status = .processed
        p.role = .content
        p.capturedAt = Google_Protobuf_Timestamp(date: Date(timeIntervalSince1970: Double(day) * 86400))
        p.tags = tags.map { k in
            var t = Pidex_V1_Tag()
            t.key = k
            return t
        }
        p.subjectID = subject
        p.description_p = description
        p.extractedFields = fields
        if !city.isEmpty {
            p.place.country = "中国"
            p.place.city = city
        }
        return p
    }

    private func subject(_ id: String, _ name: String) -> Pidex_V1_Subject {
        var s = Pidex_V1_Subject()
        s.id = id
        s.displayName = name
        return s
    }

    private func library() throws -> LocalLibrary {
        let l = try LocalLibrary(file: file)
        var r = Pidex_V1_SyncLibraryResponse()
        r.photos = [
            photo("a", day: 1, tags: ["sleeping"], subject: "tutu", city: "大连市", description: "Tutu asleep on the sofa"),
            photo("b", day: 2, tags: ["sleeping", "outdoors"], subject: "mimi", city: "大连市"),
            photo("c", day: 3, tags: ["eating"], subject: "tutu", city: "杭州市", fields: ["bed": "12"]),
            photo("d", day: 4, tags: ["eating"], description: "100% cotton_blend"),
        ]
        r.subjects = [subject("tutu", "Tutu"), subject("mimi", "Mimi")]
        r.cursor = "c"
        try l.apply(r)
        return l
    }

    private func ids(_ l: LocalLibrary, _ edit: (inout Pidex_V1_SearchFilters) -> Void) throws -> [String] {
        var f = Pidex_V1_SearchFilters()
        edit(&f)
        return try l.photos(matching: f, limit: 50).map(\.id)
    }

    @Test func filtersAsTheServerDoes() throws {
        let l = try library()
        #expect(try ids(l) { $0.tags = ["sleeping"]; $0.tagsRequired = true } == ["b", "a"])
        #expect(try ids(l) { $0.tags = ["sleeping", "outdoors"]; $0.tagsRequired = true } == ["b"])
        #expect(try ids(l) { $0.place = "大连" } == ["b", "a"])
        #expect(try ids(l) { $0.place = "中国" } == ["c", "b", "a"])
        #expect(try ids(l) { $0.subjectIds = ["tutu"] } == ["c", "a"])
        #expect(try ids(l) { $0.subjectQuery = "mim" } == ["b"])
        #expect(try ids(l) {
            $0.from = Google_Protobuf_Timestamp(date: Date(timeIntervalSince1970: 2 * 86400))
            $0.to = Google_Protobuf_Timestamp(date: Date(timeIntervalSince1970: 4 * 86400))
        } == ["c", "b"])
        var f = Pidex_V1_SearchFilters()
        f.place = "大连"
        #expect(try l.count(matching: f) == 2)
        #expect(try l.tagCounts(matching: f) == ["sleeping": 2, "outdoors": 1])
    }

    @Test func wordsMatchWhatEachPhotoSays() throws {
        let l = try library()
        #expect(try ids(l) { $0.text = "sofa" } == ["a"])
        #expect(try ids(l) { $0.text = "12" } == ["c"])
        #expect(try ids(l) { $0.text = "tutu sofa" } == ["a"])
        // % and _ are words, not patterns.
        #expect(try ids(l) { $0.text = "100%" } == ["d"])
        #expect(try ids(l) { $0.text = "n_b" } == ["d"])
        #expect(try ids(l) { $0.text = "o_t" }.isEmpty)
    }

    @Test func whatThePhoneCanFindExactly() {
        var f = Pidex_V1_SearchFilters()
        f.tags = ["sleeping"]
        #expect(!LocalLibrary.matchesExactly(f), "tags only preferred rank a search by meaning")
        f.tagsRequired = true
        f.place = "大连"
        #expect(LocalLibrary.matchesExactly(f))
        f.text = "on the sofa"
        #expect(!LocalLibrary.matchesExactly(f))
        f.text = ""
        f.mode = .missing
        #expect(!LocalLibrary.matchesExactly(f))
    }

    @Test func offlineSearchReadsTheNamesItKnows() throws {
        let l = try library()
        let tags = [ScenarioConfig.Tag(key: "sleeping", name: "睡觉"), ScenarioConfig.Tag(key: "eating", name: "Eating")]
        let read = { (text: String) in
            OfflineSearch.filters(for: text, subjects: try l.subjects(), tags: tags, places: try l.placeNames())
        }
        let f = try read("大连睡觉的Tutu")
        #expect(f.place == "大连")
        #expect(f.tags == ["sleeping"] && f.tagsRequired)
        #expect(f.subjectIds == ["tutu"])
        #expect(f.text.isEmpty)
        #expect(try l.photos(matching: f, limit: 10).map(\.id) == ["a"])

        #expect(try read("mimi eating").subjectIds == ["mimi"])
        #expect(try read("mimi eating").tags == ["eating"])
        // Nothing it knows: the words as they are.
        let other = try read("cotton")
        #expect(other.text == "cotton" && other.subjectIds.isEmpty && other.tags.isEmpty)
        #expect(try l.photos(matching: other, limit: 10).map(\.id) == ["d"])
    }

    @Test func aCopyFromBeforeMatchingIsReadAgain() throws {
        // A copy kept by the first version, with no words to match on.
        let queue = try DatabaseQueue(path: file.path)
        try LocalLibrary.migrator.migrate(queue, upTo: "v1")
        let p = photo("old", day: 1, tags: ["sleeping"], city: "大连市", description: "on the sofa")
        let data: Data = try p.serializedData()
        try queue.write { db in
            try db.execute(sql: """
                INSERT INTO photo (id, at, shown, subject_id, country, city, city_in, area, area_in, data)
                VALUES ('old', 86400, 1, '', '中国', '大连市', '中国', '', '大连市', ?)
                """, arguments: [data])
        }
        try queue.close()
        let l = try LocalLibrary(file: file)
        #expect(try ids(l) { $0.text = "sofa" } == ["old"])
        #expect(try ids(l) { $0.tags = ["sleeping"]; $0.tagsRequired = true } == ["old"])
        #expect(try ids(l) { $0.place = "大连" } == ["old"])
    }
}
