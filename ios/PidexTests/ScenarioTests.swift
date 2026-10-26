import Foundation
import Testing
import UIKit
@testable import Pidex

struct WorkspaceListTests {
    @Test func firstWorkspaceBecomesActive() {
        var list = WorkspaceList()
        list.upsert(Workspace(id: "a", name: "Ortho", token: "t1"), activate: false)
        #expect(list.active?.id == "a")
        list.upsert(Workspace(id: "b", name: "Plants", token: "t2"), activate: false)
        #expect(list.active?.id == "a")
        list.upsert(Workspace(id: "c", name: "IT", token: "t3"), activate: true)
        #expect(list.active?.id == "c")
        #expect(list.items.map(\.id) == ["a", "b", "c"])
    }

    @Test func sameTeamOrTokenReplacesTheEntry() {
        var list = WorkspaceList()
        list.upsert(Workspace(id: "a", name: "Ortho", token: "t1"), activate: true)
        list.upsert(Workspace(id: "a", name: "Ortho", token: "t9"), activate: true)
        #expect(list.items == [Workspace(id: "a", name: "Ortho", token: "t9")])
    }

    @Test func removingTheActiveOneFallsBack() {
        var list = WorkspaceList()
        list.upsert(Workspace(id: "a", name: "A", token: "t1"), activate: true)
        list.upsert(Workspace(id: "b", name: "B", token: "t2"), activate: true)
        list.remove("b")
        #expect(list.active?.id == "a")
        list.remove("a")
        #expect(list.active == nil)
    }

    /// A token saved before workspaces existed has no team ID until the
    /// server names it.
    @Test func migratedTokenLearnsItsTeam() {
        var list = WorkspaceList()
        list.upsert(Workspace(id: "", name: "Workspace", token: "old"), activate: true)
        list.upsert(Workspace(id: "p", name: "Plants", token: "t2"), activate: false)
        list.update(token: "old", id: "o", name: "Ortho")
        #expect(list.active == Workspace(id: "o", name: "Ortho", token: "old"))
        #expect(list.items.count == 2)
        list.update(token: "missing", id: "x", name: "X")
        #expect(list.items.count == 2)
    }

    @Test func survivesEncoding() throws {
        var list = WorkspaceList()
        list.upsert(Workspace(id: "a", name: "A", token: "t1"), activate: false)
        list.upsert(Workspace(id: "b", name: "B", token: "t2"), activate: true)
        let copy = try JSONDecoder().decode(WorkspaceList.self, from: JSONEncoder().encode(list))
        #expect(copy == list)
        #expect(copy.active?.id == "b")
    }
}

struct ScenarioConfigTests {
    /// A collection kept by place and time (a trip) has no subject types;
    /// the draft must still show, whether they come as [], null or not at all.
    @Test func aConfigWithoutSubjectTypesStillReads() throws {
        for types in [#""subject_types":[],"#, #""subject_types":null,"#, ""] {
            let json = #"{"name":"Dalian trip","industry":"personal","strictness":"lenient","privacy":"standard",\#(types)"tags":[{"key":"food","name":"Food"}]}"#
            let c = try ScenarioConfig(json: json)
            #expect(c.subjectTypes.isEmpty && c.tags.count == 1, "\(types)")
        }
    }

    /// The shape teamconfig.Config marshals to, including fields the app
    /// does not use.
    static let json = """
    {"format_version":1,"name":"我的植物日记","industry":"home","strictness":"lenient","privacy":"standard","date_order":"dmy",
     "subject_types":[{"key":"plant","name":"植物","carriers":"花盆里的标签","identity_fields":[
        {"key":"plant_name","name":"植物名称","kind":"text"},{"key":"pot_number","name":"编号","kind":"id","pattern":"^[0-9]{2}$"}],
      "display_fields":["plant_name"],"match":{"min_fields":1},
      "barcode":{"field":"pot_number","strong":true,"symbologies":["qr"]}},
      {"key":"bed","name":"花坛","parent":"plant","carriers":"木牌","identity_fields":[{"key":"bed_no","name":"编号","kind":"id"}],
      "display_fields":["bed_no"],"match":{"min_fields":1}}],
     "tags":[{"key":"yellow_leaves","name":"叶片发黄","group":"状况"},{"key":"pests","name":"可见害虫","group":"病虫害"},
      {"key":"wilting","name":"萎蔫","group":"状况"},{"key":"overview","name":"全景"}],
     "session":{"gap_minutes":30,"anchor_valid_minutes":0,"max_jump_meters":0}}
    """

    @Test func decodesServerConfig() throws {
        let c = try ScenarioConfig(json: Self.json)
        #expect(c.name == "我的植物日记")
        #expect(c.subjectTypes.map(\.key) == ["plant", "bed"])
        #expect(c.subjectTypes[0].identityFields.map(\.kind) == ["text", "id"])
        #expect(c.subjectTypes[0].barcode?.strong == true)
        #expect(c.subjectTypes[1].barcode == nil)
        #expect(c.subjectTypes[1].parent == "plant")
    }

    @Test func groupsTagsInFirstSeenOrder() throws {
        let groups = try ScenarioConfig(json: Self.json).tagGroups
        #expect(groups.map(\.group) == ["状况", "病虫害", ""])
        #expect(groups[0].tags.map(\.key) == ["yellow_leaves", "wilting"])
    }
}

struct MultilingualLabelTests {
    /// A handwritten-style plant tag: the ID must come through even though
    /// the rest of the label is Chinese.
    @Test func readsIDOnChineseLabel() throws {
        let size = CGSize(width: 900, height: 400)
        let image = UIGraphicsImageRenderer(size: size).image { ctx in
            UIColor.white.setFill()
            ctx.fill(CGRect(origin: .zero, size: size))
            ("绿萝 03" as NSString).draw(at: CGPoint(x: 80, y: 130), withAttributes: [
                .font: UIFont.systemFont(ofSize: 120, weight: .semibold), .foregroundColor: UIColor.black,
            ])
        }
        let analysis = OnDeviceAnalyzer.analyze(try #require(image.cgImage))
        let text = analysis.text.map(\.text).joined(separator: " ")
        #expect(text.contains("03"), "read: \(text)")
        #expect(text.contains("绿萝"), "read: \(text)")
    }
}

struct ConfigSectionsTests {
    @Test func groupKeysReadAsHeadings() {
        #expect(ConfigSections.heading("photo_type") == "Photo type")
        #expect(ConfigSections.heading("stage") == "Stage")
        #expect(ConfigSections.heading("Body site") == "Body site")
        #expect(ConfigSections.heading("病虫害") == "病虫害")
    }
}

struct DictationLocaleTests {
    static let supported: Set<Locale> = ["en-US", "en-GB", "en-AU", "zh-CN", "zh-TW", "zh-HK", "es-ES", "es-MX"].map(Locale.init(identifier:)).reduce(into: []) { $0.insert($1) }

    @Test func chineseScriptPicksMainlandOrTaiwan() {
        #expect(Dictation.speechLocale(for: "zh-Hans", region: "US", supported: Self.supported).identifier == "zh-CN")
        #expect(Dictation.speechLocale(for: "zh-Hant", region: "US", supported: Self.supported).identifier == "zh-TW")
        #expect(Dictation.speechLocale(for: "zh-Hant", region: "HK", supported: Self.supported).identifier == "zh-HK")
        #expect(Dictation.speechLocale(for: "zh-Hans", region: "TW", supported: Self.supported).identifier == "zh-CN")
    }

    @Test func englishFollowsTheUsersRegion() {
        #expect(Dictation.speechLocale(for: "en", region: "GB", supported: Self.supported).identifier == "en-GB")
        #expect(Dictation.speechLocale(for: "en", region: "CN", supported: Self.supported).identifier == "en-US")
        #expect(Dictation.speechLocale(for: "es", region: "MX", supported: Self.supported).identifier == "es-MX")
    }

    @Test func unsupportedLanguageFallsBackToEnglish() {
        #expect(Dictation.speechLocale(for: "ko", region: "KR", supported: Self.supported).identifier == "en-US")
    }
}

struct DictationChoiceTests {
    @Test func offersTheUsersLanguagesThenAllSupported() {
        let supported: Set<Locale> = ["en-US", "en-GB", "zh-CN", "ja-JP", "es-MX"].map(Locale.init(identifier:)).reduce(into: []) { $0.insert($1) }
        let (mine, all) = DictationLanguagePicker.choices(preferred: ["zh-Hans-CN", "en-GB", "en-US", "ko-KR"], supported: supported)
        #expect(mine.map(\.id) == ["zh-CN", "en-GB", "en-US"])
        #expect(Set(all.map(\.id)) == Set(supported.map(\.identifier)))
        #expect(Dictation.speechLocale(for: "es-MX", region: "US", supported: supported).identifier == "es-MX")
    }
}
