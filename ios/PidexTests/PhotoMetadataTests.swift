import CoreLocation
import Foundation
import ImageIO
import Testing
import UIKit
import UniformTypeIdentifiers
@testable import Pidex

/// A photo added from the library keeps the day and place its camera wrote
/// into it, not the moment it was added.
struct PhotoMetadataTests {
    func jpeg(exif: [CFString: Any]? = nil, gps: [CFString: Any]? = nil) -> Data {
        let image = UIGraphicsImageRenderer(size: CGSize(width: 32, height: 32)).image { ctx in
            UIColor.gray.setFill()
            ctx.fill(CGRect(x: 0, y: 0, width: 32, height: 32))
        }
        let out = NSMutableData()
        let dest = CGImageDestinationCreateWithData(out, UTType.jpeg.identifier as CFString, 1, nil)!
        var props: [CFString: Any] = [:]
        if let exif { props[kCGImagePropertyExifDictionary] = exif }
        if let gps { props[kCGImagePropertyGPSDictionary] = gps }
        CGImageDestinationAddImage(dest, image.cgImage!, props as CFDictionary)
        CGImageDestinationFinalize(dest)
        return out as Data
    }

    @Test func readsTheDayAndPlaceTheCameraWrote() {
        let data = jpeg(
            exif: [kCGImagePropertyExifDateTimeOriginal: "2026:10:02 14:30:00", kCGImagePropertyExifOffsetTimeOriginal: "+08:00"],
            gps: [kCGImagePropertyGPSLatitude: 38.88, kCGImagePropertyGPSLatitudeRef: "N",
                  kCGImagePropertyGPSLongitude: 121.58, kCGImagePropertyGPSLongitudeRef: "E"]
        )
        let meta = PhotoMetadata.read(data)
        #expect(meta.date == ISO8601DateFormatter().date(from: "2026-10-02T06:30:00Z"))
        #expect(abs((meta.location?.coordinate.latitude ?? 0) - 38.88) < 0.0001)
        #expect(abs((meta.location?.coordinate.longitude ?? 0) - 121.58) < 0.0001)
    }

    @Test func southAndWestAreNegative() {
        let data = jpeg(gps: [kCGImagePropertyGPSLatitude: 33.86, kCGImagePropertyGPSLatitudeRef: "S",
                              kCGImagePropertyGPSLongitude: 151.21, kCGImagePropertyGPSLongitudeRef: "W"])
        let loc = PhotoMetadata.read(data).location
        #expect((loc?.coordinate.latitude ?? 0) < 0 && (loc?.coordinate.longitude ?? 0) < 0)
    }

    @Test func aPhotoWithoutMetadataSaysNothing() {
        let meta = PhotoMetadata.read(jpeg())
        #expect(meta.date == nil && meta.location == nil)
    }

    @Test func aPlaceGoesToTheServerOnlyWhenNamed() {
        var place = ShotPlace(CLLocation(latitude: 38.88, longitude: 121.58))
        #expect(place.place == nil && place.geoPoint.latitude == 38.88)
        place.city = "Dalian"
        place.name = "Xinghai Square"
        #expect(place.place?.city == "Dalian" && place.place?.name == "Xinghai Square")
    }
}
