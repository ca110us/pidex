// Renders the app's icon and marks from the SVG originals in icon/, so the
// SVGs stay the only source. Run from the repository root after changing
// them:
//
//     swift design/brand/render.swift
//
// It writes into the iOS asset catalog:
//   AppIcon      the home screen icon: default, dark and tinted (iOS 18+)
//   PidexIcon    the icon as a vector, for showing it inside the app
//   PidexMark    the P alone, a template image that takes any colour
//   LaunchMark   the P on the launch screen, ink in light mode, paper in dark
//
// The SVGs are drawn in order: a background rect, then groups of paths, each
// group one fill. A group filled with the background colour cuts into what is
// under it; on transparent marks it is cut out instead.

import CoreGraphics
import Foundation
import ImageIO
import UniformTypeIdentifiers

let root = URL(fileURLWithPath: FileManager.default.currentDirectoryPath)
let icons = root.appending(path: "design/brand/icon")
let assets = root.appending(path: "ios/Pidex/Resources/Assets.xcassets")

struct Icon {
    var background: CGColor
    /// Paths in drawing order, each with its fill.
    var layers: [(fill: CGColor, path: CGPath)]
    /// The SVG's own coordinate space.
    var box: CGRect

    /// What is drawn, without the background and the cuts.
    var glyph: [CGPath] { layers.filter { !$0.fill.same(as: background) }.map(\.path) }
    var cuts: [CGPath] { layers.filter { $0.fill.same(as: background) }.map(\.path) }
    var glyphBounds: CGRect { glyph.reduce(CGRect.null) { $0.union($1.boundingBoxOfPath) } }
}

extension CGColor {
    static func hex(_ s: String) -> CGColor {
        let v = UInt32(s.trimmingCharacters(in: CharacterSet(charactersIn: "#")), radix: 16) ?? 0
        return CGColor(srgbRed: CGFloat(v >> 16 & 0xFF) / 255, green: CGFloat(v >> 8 & 0xFF) / 255, blue: CGFloat(v & 0xFF) / 255, alpha: 1)
    }

    func same(as other: CGColor) -> Bool {
        guard let a = components, let b = other.components, a.count == b.count else { return false }
        return zip(a, b).allSatisfy { abs($0 - $1) < 0.002 }
    }
}

func attribute(_ name: String, in tag: Substring) -> String? {
    guard let r = tag.range(of: "\(name)=\"") else { return nil }
    let rest = tag[r.upperBound...]
    return rest.firstIndex(of: "\"").map { String(rest[..<$0]) }
}

/// The path commands these SVGs use: M L H V C Z, absolute or relative.
func path(_ d: String) -> CGPath {
    let token = try! NSRegularExpression(pattern: "[MmLlHhVvCcZz]|-?(?:\\d+\\.?\\d*|\\.\\d+)(?:e-?\\d+)?")
    let tokens = token.matches(in: d, range: NSRange(d.startIndex..., in: d)).map { String(d[Range($0.range, in: d)!]) }
    let p = CGMutablePath()
    var i = 0
    var command = "M"
    var at = CGPoint.zero
    func number() -> CGFloat { defer { i += 1 }; return CGFloat(Double(tokens[i])!) }
    while i < tokens.count {
        if tokens[i].first!.isLetter { command = tokens[i]; i += 1 }
        let relative = command == command.lowercased()
        let origin = relative ? at : .zero
        switch command.uppercased() {
        case "M":
            at = CGPoint(x: origin.x + number(), y: origin.y + number())
            p.move(to: at)
            command = relative ? "l" : "L"
        case "L":
            at = CGPoint(x: origin.x + number(), y: origin.y + number())
            p.addLine(to: at)
        case "H":
            at.x = (relative ? at.x : 0) + number()
            p.addLine(to: at)
        case "V":
            at.y = (relative ? at.y : 0) + number()
            p.addLine(to: at)
        case "C":
            let c1 = CGPoint(x: origin.x + number(), y: origin.y + number())
            let c2 = CGPoint(x: origin.x + number(), y: origin.y + number())
            at = CGPoint(x: origin.x + number(), y: origin.y + number())
            p.addCurve(to: at, control1: c1, control2: c2)
        case "Z":
            p.closeSubpath()
            at = p.currentPoint
        default:
            fatalError("unsupported path command \(command)")
        }
    }
    return p
}

func load(_ name: String) -> Icon {
    let svg = try! String(contentsOf: icons.appending(path: name), encoding: .utf8)
    let viewBox = attribute("viewBox", in: svg[...])!.split(separator: " ").map { CGFloat(Double($0)!) }
    let rect = svg[svg.range(of: "<rect")!.lowerBound...]
    var icon = Icon(background: .hex(attribute("fill", in: rect)!), layers: [],
                    box: CGRect(x: viewBox[0], y: viewBox[1], width: viewBox[2], height: viewBox[3]))
    for group in svg.components(separatedBy: "<g ").dropFirst() {
        let fill = CGColor.hex(attribute("fill", in: group[...])!)
        for tag in group.components(separatedBy: "<path ").dropFirst() {
            icon.layers.append((fill, path(attribute("d", in: tag[...])!)))
        }
    }
    return icon
}

/// SVG coordinates (y down) onto a context of the given size (y up),
/// showing `area` of the SVG.
func place(_ ctx: CGContext, _ area: CGRect, in size: CGSize) {
    ctx.translateBy(x: 0, y: size.height)
    ctx.scaleBy(x: size.width / area.width, y: -size.height / area.height)
    ctx.translateBy(x: -area.minX, y: -area.minY)
}

/// The icon as painted: background, then each layer over it.
func paint(_ icon: Icon, _ ctx: CGContext, background: CGColor? = nil, glyph: CGColor? = nil) {
    ctx.setFillColor(background ?? icon.background)
    ctx.fill(icon.box)
    for layer in icon.layers {
        let cut = layer.fill.same(as: icon.background)
        ctx.setFillColor(cut ? background ?? icon.background : glyph ?? layer.fill)
        ctx.addPath(layer.path)
        ctx.fillPath(using: .evenOdd)
    }
}

/// The glyph alone on nothing, its cuts cut out.
func paintMark(_ icon: Icon, _ ctx: CGContext, color: CGColor) {
    let outside = CGMutablePath()
    outside.addRect(icon.box.insetBy(dx: -icon.box.width, dy: -icon.box.height))
    for cut in icon.cuts { outside.addPath(cut) }
    ctx.addPath(outside)
    ctx.clip(using: .evenOdd)
    ctx.setFillColor(color)
    for g in icon.glyph {
        ctx.addPath(g)
        ctx.fillPath(using: .evenOdd)
    }
}

/// An opaque PNG: home screen icons may not be transparent.
func png(_ to: URL, pixels: Int, draw: (CGContext) -> Void) {
    let ctx = CGContext(data: nil, width: pixels, height: pixels, bitsPerComponent: 8, bytesPerRow: 0,
                        space: CGColorSpace(name: CGColorSpace.sRGB)!, bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue)!
    draw(ctx)
    let dest = CGImageDestinationCreateWithURL(to as CFURL, UTType.png.identifier as CFString, 1, nil)!
    CGImageDestinationAddImage(dest, ctx.makeImage()!, nil)
    precondition(CGImageDestinationFinalize(dest), "could not write \(to.path)")
}

func pdf(_ to: URL, size: CGSize, draw: (CGContext) -> Void) {
    var box = CGRect(origin: .zero, size: size)
    let ctx = CGContext(to as CFURL, mediaBox: &box, nil)!
    ctx.beginPDFPage(nil)
    draw(ctx)
    ctx.endPDFPage()
    ctx.closePDF()
}

func imageset(_ name: String) -> URL {
    let dir = assets.appending(path: name)
    try! FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
    return dir
}

func contents(_ dir: URL, _ json: String) {
    try! json.write(to: dir.appending(path: "Contents.json"), atomically: true, encoding: .utf8)
}

// The home screen icon is the dark one, white P on black, in light and dark
// mode alike; tinted mode wants grayscale, which the system colours.
let dark = load("pidex-dark.svg")
let light = load("pidex-light.svg")
let side = 1024
let full = CGSize(width: side, height: side)

try! FileManager.default.createDirectory(at: assets, withIntermediateDirectories: true)
contents(assets, #"{ "info" : { "author" : "xcode", "version" : 1 } }"#)

let appIcon = imageset("AppIcon.appiconset")
png(appIcon.appending(path: "AppIcon.png"), pixels: side) { place($0, dark.box, in: full); paint(dark, $0) }
png(appIcon.appending(path: "AppIcon-Dark.png"), pixels: side) { place($0, dark.box, in: full); paint(dark, $0) }
png(appIcon.appending(path: "AppIcon-Tinted.png"), pixels: side) {
    place($0, dark.box, in: full)
    paint(dark, $0, background: .hex("#000000"), glyph: .hex("#FFFFFF"))
}
contents(appIcon, """
{
  "images" : [
    { "filename" : "AppIcon.png", "idiom" : "universal", "platform" : "ios", "size" : "1024x1024" },
    { "appearances" : [ { "appearance" : "luminosity", "value" : "dark" } ], "filename" : "AppIcon-Dark.png", "idiom" : "universal", "platform" : "ios", "size" : "1024x1024" },
    { "appearances" : [ { "appearance" : "luminosity", "value" : "tinted" } ], "filename" : "AppIcon-Tinted.png", "idiom" : "universal", "platform" : "ios", "size" : "1024x1024" }
  ],
  "info" : { "author" : "xcode", "version" : 1 }
}
""")

// In the app, the icon as a vector square; views round its corners.
let pidexIcon = imageset("PidexIcon.imageset")
pdf(pidexIcon.appending(path: "pidex-icon.pdf"), size: dark.box.size) { place($0, dark.box, in: dark.box.size); paint(dark, $0) }
contents(pidexIcon, """
{
  "images" : [ { "filename" : "pidex-icon.pdf", "idiom" : "universal" } ],
  "info" : { "author" : "xcode", "version" : 1 },
  "properties" : { "preserves-vector-representation" : true }
}
""")

// The P alone, tight to its edges, for any colour.
let bounds = dark.glyphBounds
let mark = imageset("PidexMark.imageset")
pdf(mark.appending(path: "pidex-mark.pdf"), size: bounds.size) {
    place($0, bounds, in: bounds.size)
    paintMark(dark, $0, color: .hex("#000000"))
}
contents(mark, """
{
  "images" : [ { "filename" : "pidex-mark.pdf", "idiom" : "universal" } ],
  "info" : { "author" : "xcode", "version" : 1 },
  "properties" : { "preserves-vector-representation" : true, "template-rendering-intent" : "template" }
}
""")

// The launch screen shows its image as is: ink on light, paper on dark,
// the colours of the light and dark icons.
let launch = imageset("LaunchMark.imageset")
let launchSize = CGSize(width: 120, height: 120)
pdf(launch.appending(path: "launch-ink.pdf"), size: launchSize) {
    place($0, light.box, in: launchSize)
    paintMark(light, $0, color: light.layers.first { !$0.fill.same(as: light.background) }!.fill)
}
pdf(launch.appending(path: "launch-paper.pdf"), size: launchSize) {
    place($0, dark.box, in: launchSize)
    paintMark(dark, $0, color: dark.layers.first { !$0.fill.same(as: dark.background) }!.fill)
}
contents(launch, """
{
  "images" : [
    { "filename" : "launch-ink.pdf", "idiom" : "universal" },
    { "appearances" : [ { "appearance" : "luminosity", "value" : "dark" } ], "filename" : "launch-paper.pdf", "idiom" : "universal" }
  ],
  "info" : { "author" : "xcode", "version" : 1 },
  "properties" : { "preserves-vector-representation" : true }
}
""")

print("rendered AppIcon, PidexIcon, PidexMark and LaunchMark into \(assets.path)")
