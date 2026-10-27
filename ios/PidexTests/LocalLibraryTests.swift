import Foundation
import SwiftProtobuf
import Testing
@testable import Pidex

struct LocalLibraryTests {
    let file = FileManager.default.temporaryDirectory.appending(path: "library-\(UUID().uuidString).sqlite")

    private func photo(_ id: String, day: Int, status: Pidex_V1_PhotoStatus = .processed, role: Pidex_V1_PhotoRole = .content,
                       city: String = "", country: String = "", name: String = "") -> Pidex_V1_Photo {
        var p = Pidex_V1_Photo()
        p.id = id
        p.status = status
        p.role = role
        p.capturedAt = Google_Protobuf_Timestamp(date: Date(timeIntervalSince1970: Double(day) * 86400))
        if !city.isEmpty || !country.isEmpty {
            p.place.city = city
            p.place.country = country
            p.place.name = name
        }
        return p
    }

    private func changes(_ photos: [Pidex_V1_Photo] = [], deleted: [String] = [], subjects: [String] = [],
                         cursor: String, startOver: Bool = false) -> Pidex_V1_SyncLibraryResponse {
        var r = Pidex_V1_SyncLibraryResponse()
        r.photos = photos
        r.deletedPhotoIds = deleted
        r.subjects = subjects.map { id in
            var s = Pidex_V1_Subject()
            s.id = id
            s.displayName = "Tutu"
            return s
        }
        r.cursor = cursor
        r.startOver = startOver
        return r
    }

    @Test func keepsWhatSearchShowsNewestFirst() throws {
        let l = try LocalLibrary(file: file)
        #expect(!l.hasSynced)
        try l.apply(changes([
            photo("a", day: 1), photo("b", day: 3), photo("c", day: 2),
            photo("junk", day: 4, role: .junk), photo("waiting", day: 5, status: .processing),
        ], subjects: ["s1"], cursor: "c1"))
        #expect(l.cursor == "c1")
        #expect(try l.photos(limit: 10).map(\.id) == ["b", "c", "a"])
        #expect(try l.photos(offset: 1, limit: 1).map(\.id) == ["c"])
        #expect(try l.shownCount() == 3)
        #expect(try l.subjects().map(\.displayName) == ["Tutu"])

        // Processed later, then deleted, then changed: the copy follows.
        try l.apply(changes([photo("waiting", day: 5), photo("a", day: 6)], deleted: ["b"], cursor: "c2"))
        #expect(try l.photos(limit: 10).map(\.id) == ["a", "waiting", "c"])
        #expect(l.cursor == "c2")

        // Starting over drops what is not sent again.
        try l.apply(changes([photo("c", day: 2)], cursor: "c3", startOver: true))
        #expect(try l.photos(limit: 10).map(\.id) == ["c"])
        #expect(try l.subjects().isEmpty)
    }

    @Test func countsPlacesAtEveryLevel() throws {
        let l = try LocalLibrary(file: file)
        try l.apply(changes([
            photo("1", day: 1, city: "大连市", country: "中国", name: "星海广场"),
            photo("2", day: 2, city: "大连市", country: "中国", name: "星海广场"),
            photo("3", day: 3, city: "Tokyo", country: "Japan"),
            photo("4", day: 4, role: .junk, city: "Tokyo", country: "Japan"),
            photo("5", day: 5),
        ], cursor: "c"))
        let counts = Dictionary(uniqueKeysWithValues: try l.placeCounts().map { ("\($0.level):\($0.name)", "\($0.count) in \($0.parent)") })
        #expect(counts == [
            "country:中国": "2 in ", "country:Japan": "1 in ",
            "city:大连市": "2 in 中国", "city:Tokyo": "1 in Japan",
            "area:星海广场": "2 in 大连市",
        ])
    }

    @Test func keepsResponsesForOffline() throws {
        let l = try LocalLibrary(file: file)
        var o = Pidex_V1_GetOverviewResponse()
        o.photoCount = 42
        try l.keep(o, as: "overview")
        #expect(l.kept(Pidex_V1_GetOverviewResponse.self, as: "overview")?.photoCount == 42)
        #expect(l.kept(Pidex_V1_Team.self, as: "team") == nil)
        // Reopened, it is all still there.
        let again = try LocalLibrary(file: file)
        #expect(again.kept(Pidex_V1_GetOverviewResponse.self, as: "overview")?.photoCount == 42)
    }

    @Test func oneCopyPerCollectionErasedWithIt() throws {
        let token = "cbk_test_\(UUID().uuidString)"
        let api = APIClient(baseURL: URL(string: "http://127.0.0.1:1")!, token: token)
        let l = try #require(LocalLibrary.for(api))
        #expect(LocalLibrary.for(api) === l)
        #expect(!api.storeKey.contains(token) && api.storeKey.count == 24)
        try l.apply(changes([photo("a", day: 1)], cursor: "c"))
        LocalLibrary.erase(token: token)
        let fresh = try #require(LocalLibrary.for(api))
        #expect(fresh !== l)
        #expect(!fresh.hasSynced)
        LocalLibrary.erase(token: token)
    }
}
