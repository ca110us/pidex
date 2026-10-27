import CoreGraphics
import Foundation
import ImageIO
import Testing
import UniformTypeIdentifiers
@testable import Pidex

private func makeImage(width: Int, height: Int, draw: (CGContext) -> Void) -> CGImage {
    let ctx = CGContext(data: nil, width: width, height: height, bitsPerComponent: 8, bytesPerRow: 0,
                        space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
    draw(ctx)
    return ctx.makeImage()!
}

private func gradient(width: Int = 320, height: Int = 240, shift: CGFloat = 0) -> CGImage {
    makeImage(width: width, height: height) { ctx in
        for x in 0..<width {
            let v = min(1, max(0, CGFloat(x) / CGFloat(width) + shift))
            ctx.setFillColor(CGColor(red: v, green: v * 0.8, blue: 1 - v, alpha: 1))
            ctx.fill(CGRect(x: x, y: 0, width: 1, height: height))
        }
    }
}

private func checkerboard(size: Int = 320, cell: Int = 8) -> CGImage {
    makeImage(width: size, height: size) { ctx in
        for y in stride(from: 0, to: size, by: cell) {
            for x in stride(from: 0, to: size, by: cell) {
                ctx.setFillColor(gray: ((x / cell + y / cell) % 2 == 0) ? 0 : 1, alpha: 1)
                ctx.fill(CGRect(x: x, y: y, width: cell, height: cell))
            }
        }
    }
}

private func jpeg(_ image: CGImage, orientation: Int? = nil) -> Data {
    let out = NSMutableData()
    let dest = CGImageDestinationCreateWithData(out, UTType.jpeg.identifier as CFString, 1, nil)!
    var props: [CFString: Any] = [:]
    if let orientation { props[kCGImagePropertyOrientation] = orientation }
    CGImageDestinationAddImage(dest, image, props as CFDictionary)
    CGImageDestinationFinalize(dest)
    return out as Data
}

private func distance(_ a: UInt64, _ b: UInt64) -> Int { (a ^ b).nonzeroBitCount }

struct ImageFeaturesTests {
    @Test func appliesOrientationAndShrinks() throws {
        // A 400x200 landscape frame tagged "rotate 90°" (EXIF 6) must come
        // out upright as portrait, as the phone displayed it.
        let data = jpeg(gradient(width: 400, height: 200), orientation: 6)
        let (out, image) = try ImageFeatures.uprightJPEG(from: data, maxEdge: 100)
        #expect(image.width < image.height)
        #expect(max(image.width, image.height) == 100)

        let src = try #require(CGImageSourceCreateWithData(out as CFData, nil))
        let props = CGImageSourceCopyPropertiesAtIndex(src, 0, nil) as? [CFString: Any] ?? [:]
        #expect(props[kCGImagePropertyGPSDictionary] == nil)
        #expect((props[kCGImagePropertyOrientation] as? Int ?? 1) == 1)
    }

    @Test func hashMatchesNearDuplicatesOnly() {
        let a = ImageFeatures.differenceHash(gradient())
        let slightlyBrighter = ImageFeatures.differenceHash(gradient(shift: 0.02))
        let other = ImageFeatures.differenceHash(checkerboard())
        #expect(distance(a, slightlyBrighter) <= 6, "the server treats ≤6 differing bits as a duplicate")
        #expect(distance(a, other) > 6)
        #expect(ImageFeatures.hex(0xF0) == "00000000000000f0")
    }

    @Test func sharpnessSeparatesDetailFromFlat() {
        let flat = makeImage(width: 320, height: 320) { $0.setFillColor(gray: 0.5, alpha: 1); $0.fill(CGRect(x: 0, y: 0, width: 320, height: 320)) }
        #expect(ImageFeatures.sharpness(checkerboard()) > 100 * max(1, ImageFeatures.sharpness(flat)))
    }

    @Test func symbologyNamesMatchTheServer() {
        #expect(OnDeviceAnalyzer.symbologyName("VNBarcodeSymbologyCode128") == "code128")
        #expect(OnDeviceAnalyzer.symbologyName("VNBarcodeSymbologyQR") == "qr")
        #expect(OnDeviceAnalyzer.symbologyName("VNBarcodeSymbologyEAN13") == "ean13")
        #expect(OnDeviceAnalyzer.symbologyName("VNBarcodeSymbologyITF14") == "itf")
        #expect(OnDeviceAnalyzer.symbologyName("VNBarcodeSymbologyCode39FullASCIIChecksum") == "code39")
        #expect(OnDeviceAnalyzer.symbologyName("VNBarcodeSymbologyGS1DataBarExpanded") == "gs1databar")
    }

    @Test func onlyDigitsOrBarcodesTriggerALookup() {
        var a = Pidex_V1_OnDeviceAnalysis()
        var t = Pidex_V1_TextObservation()
        t.text = "EXIT"
        a.text = [t]
        #expect(!OnDeviceAnalyzer.mayCarryIdentity(a))
        t.text = "MRN 12345678"
        a.text = [t]
        #expect(OnDeviceAnalyzer.mayCarryIdentity(a))
    }

    @Test func originalsKeepTheirFormatButNotWhereTheyWereTaken() throws {
        // A HEIC as the camera made it, with where it was taken.
        let image = gradient(width: 400, height: 300)
        let out = NSMutableData()
        let dest = try #require(CGImageDestinationCreateWithData(out, UTType.heic.identifier as CFString, 1, nil))
        CGImageDestinationAddImage(dest, image, [
            kCGImagePropertyGPSDictionary: [kCGImagePropertyGPSLatitude: 38.9, kCGImagePropertyGPSLatitudeRef: "N",
                                            kCGImagePropertyGPSLongitude: 121.6, kCGImagePropertyGPSLongitudeRef: "E"],
            kCGImagePropertyExifDictionary: [kCGImagePropertyExifDateTimeOriginal: "2026:10:21 09:00:00"],
        ] as CFDictionary)
        #expect(CGImageDestinationFinalize(dest))
        let heic = out as Data

        let original = try #require(ImageFeatures.original(from: heic, maxEdge: 100))
        #expect(original.contentType == "image/heic")
        let src = try #require(CGImageSourceCreateWithData(original.data as CFData, nil))
        let props = CGImageSourceCopyPropertiesAtIndex(src, 0, nil) as? [CFString: Any] ?? [:]
        #expect(props[kCGImagePropertyGPSDictionary] == nil)
        #expect((props[kCGImagePropertyExifDictionary] as? [CFString: Any])?[kCGImagePropertyExifDateTimeOriginal] as? String == "2026:10:21 09:00:00")
        #expect(props[kCGImagePropertyPixelWidth] as? Int == 400)

        // Nothing to take out: the very bytes.
        let plain = jpeg(image)
        #expect(ImageFeatures.original(from: plain, maxEdge: 100)?.data == plain)
        // A JPEG no bigger than the working copy says nothing more.
        #expect(ImageFeatures.original(from: plain, maxEdge: 400) == nil)
        #expect(ImageFeatures.original(from: Data("not an image".utf8)) == nil)
    }
}
