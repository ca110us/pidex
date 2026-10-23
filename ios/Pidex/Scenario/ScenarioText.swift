import SwiftUI

/// How scenario drafts and their failures are worded for the user.
enum ScenarioText {
    /// Words a change in the user's language. Names come from the config,
    /// in the team's language.
    static func describe(_ c: Pidex_V1_ConfigChange) -> String {
        let type = c.subjectType, field = c.field
        switch c.kind {
        case .subjectTypeRemoved:
            return String(localized: "“\(type)” is removed. Its subjects keep their photos, but new photos will not be filed under them.")
        case .fieldRemoved:
            return String(localized: "“\(field)” of \(type) is no longer read or matched. Existing subjects keep the value.")
        case .fieldKindChanged:
            return String(localized: "“\(field)” of \(type) is now matched as \(ConfigSections.kindLabel(c.to)) instead of \(ConfigSections.kindLabel(c.from)).")
        case .fieldPatternChanged:
            return String(localized: "“\(field)” of \(type) gets a new format rule; values that do not fit it are ignored.")
        case .minFieldsRaised:
            return String(localized: "\(type) now needs \(c.to) matching fields instead of \(c.from), so more photos will ask you to confirm.")
        case .barcodeRemoved:
            return String(localized: "Barcodes no longer identify \(type) by “\(field)”.")
        case .parentChanged:
            return String(localized: "\(type) moves from “\(c.from)” to “\(c.to)”. Existing subjects keep their old parent.")
        case .tagRemoved:
            return String(localized: "Tag “\(c.tag)” is removed. Photos that have it keep it.")
        case .strictnessChanged:
            return String(localized: "Filing changes from \(strictnessName(c.from)) to \(strictnessName(c.to)).")
        case .privacyLowered:
            return String(localized: "Privacy is lowered: photo descriptions may now include names and IDs.")
        case .identifyByChanged:
            return c.to == "appearance"
                ? String(localized: "\(type) will be recognized by how it looks instead of by a label. Photos already filed stay where they are.")
                : String(localized: "\(type) will be recognized by a label or code instead of by how it looks. Photos already filed stay where they are.")
        default:
            return c.message
        }
    }

    private static func strictnessName(_ s: String) -> String {
        switch s {
        case "strict": String(localized: "strict", comment: "Strictness level")
        case "lenient": String(localized: "lenient", comment: "Strictness level")
        default: String(localized: "standard", comment: "Strictness level")
        }
    }

    static func explain(_ error: Error) -> String {
        guard let e = error as? APIError else { return error.localizedDescription }
        switch e.code {
        case .aborted: return String(localized: "The model could not produce a valid scenario. Try describing it differently.") + "\n\n" + e.message
        case .failedPrecondition: return String(localized: "This server has no model set up for designing scenarios.")
        case .permissionDenied: return String(localized: "Only owners and admins can change this collection's scenario.")
        case .deadlineExceeded, .unavailable: return String(localized: "The model did not answer in time. Try again.") + "\n\n" + e.message
        default: return e.message
        }
    }
}
