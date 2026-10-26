import Foundation

/// The parts of a team config the app shows. The server owns the format;
/// unknown fields are ignored so a newer server never breaks this view.
nonisolated struct ScenarioConfig: Decodable, Sendable {
    var name: String
    var industry: String
    var description: String?
    var strictness: String
    var privacy: String
    var subjectTypes: [SubjectType]
    var tags: [Tag]

    private enum CodingKeys: String, CodingKey {
        case name, industry, description, strictness, privacy, subjectTypes, tags
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        name = try c.decode(String.self, forKey: .name)
        industry = try c.decode(String.self, forKey: .industry)
        description = try c.decodeIfPresent(String.self, forKey: .description)
        strictness = try c.decode(String.self, forKey: .strictness)
        privacy = try c.decode(String.self, forKey: .privacy)
        // A collection kept by place and time has no subject types; a
        // server may write that as null or leave it out.
        subjectTypes = try c.decodeIfPresent([SubjectType].self, forKey: .subjectTypes) ?? []
        tags = try c.decodeIfPresent([Tag].self, forKey: .tags) ?? []
    }

    struct SubjectType: Decodable, Sendable, Identifiable {
        var key: String
        var name: String
        var parent: String?
        /// "code" or "appearance"; older configs leave it out (code).
        var identifyBy: String?
        var carriers: String
        var identityFields: [Field]
        var match: Match
        var barcode: Barcode?
        var id: String { key }
        /// Told apart by how it looks: pets, mugs, plants nobody labels.
        var byAppearance: Bool { identifyBy == "appearance" }
    }

    struct Field: Decodable, Sendable, Identifiable {
        var key: String
        var name: String
        var kind: String
        var id: String { key }
    }

    struct Match: Decodable, Sendable {
        var minFields: Int
    }

    struct Barcode: Decodable, Sendable {
        var field: String
        var strong: Bool
    }

    struct Tag: Decodable, Sendable, Identifiable {
        var key: String
        var name: String
        var group: String?
        var id: String { key }
    }

    init(json: String) throws {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        self = try decoder.decode(Self.self, from: Data(json.utf8))
    }

    /// Tags grouped in the order their groups first appear.
    var tagGroups: [(group: String, tags: [Tag])] {
        var order: [String] = []
        var byGroup: [String: [Tag]] = [:]
        for t in tags {
            let g = t.group ?? ""
            if byGroup[g] == nil { order.append(g) }
            byGroup[g, default: []].append(t)
        }
        return order.map { ($0, byGroup[$0]!) }
    }

    var strictnessExplanation: String {
        switch strictness {
        case "strict": String(localized: "Strict: files a photo only when the ID is certain, otherwise asks you.")
        case "lenient": String(localized: "Lenient: files photos readily; mistakes are easy to fix.")
        default: String(localized: "Standard: files photos when the ID is clear, asks when in doubt.")
        }
    }
}
