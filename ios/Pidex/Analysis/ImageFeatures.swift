import CoreGraphics
import Foundation
import ImageIO
import UniformTypeIdentifiers

/// Image preparation done on the phone, before anything is uploaded.
nonisolated enum ImageFeatures {
    enum Failure: Error { case unreadable, encodeFailed }

    /// Decodes a camera or library photo, applies its EXIF orientation,
    /// scales the long edge to at most `maxEdge`, and re-encodes it as JPEG
    /// with no metadata. The server then never has to deal with rotation,
    /// HEIC, or GPS hidden in EXIF (location is sent separately, if at all).
    static func uprightJPEG(from data: Data, maxEdge: Int = 2048, quality: Double = 0.85) throws -> (jpeg: Data, image: CGImage) {
        guard let source = CGImageSourceCreateWithData(data as CFData, nil) else { throw Failure.unreadable }
        let options: [CFString: Any] = [
            kCGImageSourceCreateThumbnailFromImageAlways: true,
            kCGImageSourceCreateThumbnailWithTransform: true,
            kCGImageSourceThumbnailMaxPixelSize: maxEdge,
            kCGImageSourceShouldCacheImmediately: true,
        ]
        guard let image = CGImageSourceCreateThumbnailAtIndex(source, 0, options as CFDictionary) else {
            throw Failure.unreadable
        }
        let out = NSMutableData()
        guard let dest = CGImageDestinationCreateWithData(out, UTType.jpeg.identifier as CFString, 1, nil) else {
            throw Failure.encodeFailed
        }
        CGImageDestinationAddImage(dest, image, [kCGImageDestinationLossyCompressionQuality: quality] as CFDictionary)
        guard CGImageDestinationFinalize(dest) else { throw Failure.encodeFailed }
        return (out as Data, image)
    }

    /// The file to keep as the photo's original: as the camera made it,
    /// with any location taken out (the place is sent apart, if at all).
    /// Copied, not encoded again, where the format allows. Nil when it would
    /// say no more than the working copy (a JPEG no larger), or is of a kind
    /// not kept.
    static func original(from data: Data, maxEdge: Int = 2048) -> (data: Data, contentType: String)? {
        guard let source = CGImageSourceCreateWithData(data as CFData, nil), let uti = CGImageSourceGetType(source),
              let mime = UTType(uti as String)?.preferredMIMEType, keptTypes.contains(mime) else { return nil }
        let props = CGImageSourceCopyPropertiesAtIndex(source, 0, nil) as? [CFString: Any] ?? [:]
        let w = props[kCGImagePropertyPixelWidth] as? Int ?? 0
        let h = props[kCGImagePropertyPixelHeight] as? Int ?? 0
        if mime == "image/jpeg" && max(w, h) <= maxEdge { return nil }
        // Nothing to take out: the bytes as they are.
        guard props[kCGImagePropertyGPSDictionary] != nil else { return (data, mime) }
        let out = NSMutableData()
        guard let dest = CGImageDestinationCreateWithData(out, uti, 1, nil) else { return nil }
        let copy: [CFString: Any] = [kCGImageDestinationMergeMetadata: true, kCGImageMetadataShouldExcludeGPS: true]
        if CGImageDestinationCopyImageSource(dest, source, copy as CFDictionary, nil) {
            return (out as Data, mime)
        }
        // A format that cannot be copied as is: encoded again, finely.
        let again = NSMutableData()
        guard let dest = CGImageDestinationCreateWithData(again, uti, 1, nil) else { return nil }
        CGImageDestinationAddImageFromSource(dest, source, 0, [
            kCGImagePropertyGPSDictionary: kCFNull as Any,
            kCGImageDestinationLossyCompressionQuality: 0.95,
        ] as CFDictionary)
        return CGImageDestinationFinalize(dest) ? (again as Data, mime) : nil
    }

    /// The kinds of original the server keeps.
    static let keptTypes: Set<String> = ["image/heic", "image/heif", "image/jpeg", "image/png"]

    /// 64-bit difference hash: robust to small changes, so the server can
    /// spot near-identical burst shots and skip a second model call.
    static func differenceHash(_ image: CGImage) -> UInt64 {
        guard let px = grayscale(image, width: 9, height: 8) else { return 0 }
        var hash: UInt64 = 0
        for y in 0..<8 {
            for x in 0..<8 {
                hash <<= 1
                if px[y * 9 + x] > px[y * 9 + x + 1] { hash |= 1 }
            }
        }
        return hash
    }

    static func hex(_ hash: UInt64) -> String {
        let s = String(hash, radix: 16)
        return String(repeating: "0", count: 16 - s.count) + s
    }

    /// Variance of the Laplacian on a small grayscale copy. Higher is
    /// sharper; blurred frames score low.
    static func sharpness(_ image: CGImage) -> Float {
        let w = 256
        let h = max(1, Int(Double(image.height) / Double(image.width) * Double(w)))
        guard w > 2, h > 2, let px = grayscale(image, width: w, height: h) else { return 0 }
        var sum = 0.0, sumSq = 0.0
        var n = 0.0
        for y in 1..<(h - 1) {
            for x in 1..<(w - 1) {
                let c = Double(px[y * w + x])
                let lap = Double(px[(y - 1) * w + x]) + Double(px[(y + 1) * w + x])
                    + Double(px[y * w + x - 1]) + Double(px[y * w + x + 1]) - 4 * c
                sum += lap
                sumSq += lap * lap
                n += 1
            }
        }
        let mean = sum / n
        return Float(sumSq / n - mean * mean)
    }

    private static func grayscale(_ image: CGImage, width: Int, height: Int) -> [UInt8]? {
        var px = [UInt8](repeating: 0, count: width * height)
        let ok = px.withUnsafeMutableBytes { buf -> Bool in
            guard let ctx = CGContext(
                data: buf.baseAddress, width: width, height: height, bitsPerComponent: 8,
                bytesPerRow: width, space: CGColorSpaceCreateDeviceGray(), bitmapInfo: CGImageAlphaInfo.none.rawValue
            ) else { return false }
            ctx.interpolationQuality = .medium
            ctx.draw(image, in: CGRect(x: 0, y: 0, width: width, height: height))
            return true
        }
        return ok ? px : nil
    }
}
