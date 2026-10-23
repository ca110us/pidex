import SwiftUI

/// List sections describing a scenario in plain words: what photos are
/// filed under, how those are recognized, and the tags.
struct ConfigSections: View {
    let config: ScenarioConfig

    var body: some View {
        Section {
            LabeledContent("Name", value: config.name)
            Text(config.strictnessExplanation)
                .font(.footnote)
                .foregroundStyle(.secondary)
            if config.privacy == "high" {
                Label("High privacy: names and IDs stay out of photo descriptions.", systemImage: "lock")
                    .font(.footnote)
            }
        } header: {
            Text("Scenario")
        }

        ForEach(config.subjectTypes) { st in
            Section {
                if st.byAppearance {
                    Label("Recognized by how it looks. No labels needed: unsure matches ask you to confirm.", systemImage: "eye")
                        .font(.footnote)
                    LabeledContent("What tells them apart") { Text(st.carriers) }
                } else {
                    Text(st.carriers)
                }
                ForEach(st.byAppearance ? [] : st.identityFields) { f in
                    LabeledContent(f.name, value: Self.kindLabel(f.kind))
                }
                if let b = st.barcode {
                    Label(b.strong ? "A barcode or QR code alone identifies it" : "Barcodes and QR codes are read",
                          systemImage: "barcode.viewfinder")
                        .font(.footnote)
                }
                if st.match.minFields > 1 {
                    Label("\(st.match.minFields) of these must match before a photo is filed", systemImage: "checkmark.shield")
                        .font(.footnote)
                }
            } header: {
                Text("Photos are filed by \(st.name)")
            } footer: {
                if let parent = st.parent, !parent.isEmpty, let p = config.subjectTypes.first(where: { $0.key == parent }) {
                    Text("Each \(st.name) belongs to a \(p.name).")
                }
            }
        }

        Section {
            ForEach(config.tagGroups, id: \.group) { g in
                VStack(alignment: .leading, spacing: 4) {
                    if !g.group.isEmpty {
                        Text(verbatim: Self.heading(g.group)).font(.caption).foregroundStyle(.secondary)
                    }
                    Text(g.tags.map(\.name).joined(separator: " · "))
                }
            }
        } header: {
            Text("Tags (\(config.tags.count))")
        }
    }

    /// Group names are meant to be headings, but older configs may hold a
    /// key such as "photo_type"; show that as "Photo type".
    nonisolated static func heading(_ group: String) -> String {
        guard group.contains("_") || group == group.lowercased() else { return group }
        let words = group.replacingOccurrences(of: "_", with: " ")
        return words.prefix(1).uppercased() + words.dropFirst()
    }

    nonisolated static func kindLabel(_ kind: String) -> String {
        switch kind {
        case "person_name": String(localized: "Person name", comment: "Kind of identity field")
        case "date": String(localized: "Date", comment: "Kind of identity field")
        case "id": String(localized: "Exact code", comment: "Kind of identity field")
        case "address": String(localized: "Address", comment: "Kind of identity field")
        default: String(localized: "Text", comment: "Kind of identity field")
        }
    }
}
