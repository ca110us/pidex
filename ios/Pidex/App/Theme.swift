import SwiftUI
import UIKit

/// The app's colours: black and white, the way Grok does it. Ink is the
/// accent (black in light mode, white in dark), surfaces are flat greys a
/// step off the page, and colour is kept for state alone: amber for what
/// needs a person, red for what failed, green for what is identified. The
/// asset catalog's AccentColor is ink too, so system controls follow.
enum Theme {
    /// Text and primary buttons.
    static let ink = Color(light: 0x0A0A0A, dark: 0xFAFAFA)
    /// What is drawn on ink: a primary button's label.
    static let onInk = Color(light: 0xFFFFFF, dark: 0x000000)
    /// The page.
    static let paper = Color(light: 0xFFFFFF, dark: 0x000000)
    /// Cards, bubbles, fields: one step off the page.
    static let surface = Color(light: 0xF4F4F5, dark: 0x161616)
    /// Pressed or chosen, one step further.
    static let raised = Color(light: 0xE8E8EA, dark: 0x262626)
    /// Edges, when a surface needs one.
    static let hairline = Color(light: 0x0A0A0A, dark: 0xFFFFFF, opacity: 0.1)
    /// Secondary text.
    static let muted = Color(light: 0x6B6B70, dark: 0x8E8E93)

    /// Something needs a person.
    static let attention = Color(light: 0xB45309, dark: 0xF5A524)
    /// Something failed.
    static let danger = Color(light: 0xDC2626, dark: 0xF87171)
    /// Identified, filed.
    static let success = Color(light: 0x16A34A, dark: 0x4ADE80)

    /// Large surfaces round this much.
    static let radius: CGFloat = 20
}

extension Color {
    /// A colour for each appearance, from 0xRRGGBB.
    init(light: UInt32, dark: UInt32, opacity: Double = 1) {
        func ui(_ v: UInt32) -> UIColor {
            UIColor(red: CGFloat(v >> 16 & 0xFF) / 255, green: CGFloat(v >> 8 & 0xFF) / 255, blue: CGFloat(v & 0xFF) / 255, alpha: opacity)
        }
        self.init(uiColor: UIColor { $0.userInterfaceStyle == .dark ? ui(dark) : ui(light) })
    }
}

/// The main action: an ink pill, full width or as wide as its label.
struct InkButtonStyle: ButtonStyle {
    var wide = false
    @Environment(\.isEnabled) private var enabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.body.weight(.semibold))
            .foregroundStyle(Theme.onInk)
            .padding(.vertical, 12)
            .padding(.horizontal, 20)
            .frame(maxWidth: wide ? .infinity : nil)
            .background(Theme.ink.opacity(enabled ? 1 : 0.3), in: Capsule())
            .opacity(configuration.isPressed ? 0.8 : 1)
            .scaleEffect(configuration.isPressed ? 0.98 : 1)
            .animation(.snappy(duration: 0.15), value: configuration.isPressed)
    }
}

/// A lesser action beside the main one: a surface pill.
struct SurfaceButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var enabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.body.weight(.medium))
            .foregroundStyle(Theme.ink.opacity(enabled ? 1 : 0.35))
            .padding(.vertical, 12)
            .padding(.horizontal, 18)
            .background(configuration.isPressed ? Theme.raised : Theme.surface, in: Capsule())
    }
}

extension ButtonStyle where Self == InkButtonStyle {
    static var ink: InkButtonStyle { InkButtonStyle() }
    static var inkWide: InkButtonStyle { InkButtonStyle(wide: true) }
}

extension ButtonStyle where Self == SurfaceButtonStyle {
    static var surface: SurfaceButtonStyle { SurfaceButtonStyle() }
}
