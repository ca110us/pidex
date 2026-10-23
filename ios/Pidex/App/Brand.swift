import SwiftUI

/// Pidex's icon and mark, as the app shows them. Both are rendered from the
/// SVG originals in design/brand by design/brand/render.swift.
enum Brand {
    static let name = "Pidex"
    static var tagline: String { String(localized: "Snap it. It files itself.") }

    /// "Version 1.0 (12)", from the bundle.
    static var version: String {
        let info = Bundle.main.infoDictionary
        let short = info?["CFBundleShortVersionString"] as? String ?? "1.0"
        let build = info?["CFBundleVersion"] as? String ?? "1"
        return String(localized: "Version \(short) (\(build))")
    }
}

/// The app icon, with the corners the home screen gives it.
struct PidexIcon: View {
    var size: CGFloat = 60

    var body: some View {
        let shape = RoundedRectangle(cornerRadius: size * 0.2237, style: .continuous)
        Image("PidexIcon")
            .resizable()
            .interpolation(.high)
            .frame(width: size, height: size)
            .clipShape(shape)
            // Its edge, against a dark page.
            .overlay { shape.strokeBorder(.white.opacity(0.12), lineWidth: 0.5) }
            .accessibilityHidden(true)
    }
}

/// The P alone, in the current foreground style.
struct PidexMark: View {
    var height: CGFloat = 24

    var body: some View {
        Image("PidexMark")
            .resizable()
            .scaledToFit()
            .frame(height: height)
            .accessibilityHidden(true)
    }
}

/// The icon, the name and what it does: where someone first meets the app.
struct BrandHeader: View {
    var body: some View {
        VStack(spacing: 10) {
            PidexIcon(size: 76)
                .shadow(color: .black.opacity(0.18), radius: 12, y: 6)
            Text(verbatim: Brand.name)
                .font(.largeTitle.bold())
            Text(Brand.tagline)
                .font(.subheadline)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 8)
        .accessibilityElement(children: .combine)
    }
}
