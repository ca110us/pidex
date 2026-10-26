import CoreLocation
import Foundation
import ImageIO

/// Where a photo was taken: its coordinates, and what the phone named them
/// (country down to a landmark). Photos are found and grouped by place and
/// day, not only by who they show: a trip to Dalian, a site, a client's home.
nonisolated struct ShotPlace: Codable, Sendable, Equatable {
    var latitude: Double
    var longitude: Double
    var accuracy: Double
    var country: String?
    var region: String?
    var city: String?
    var district: String?
    /// A landmark, park, building or street.
    var name: String?

    init(_ location: CLLocation) {
        latitude = location.coordinate.latitude
        longitude = location.coordinate.longitude
        accuracy = max(0, location.horizontalAccuracy)
    }

    var location: CLLocation { CLLocation(latitude: latitude, longitude: longitude) }
    var named: Bool { [country, region, city, district, name].contains { !($0 ?? "").isEmpty } }

    var geoPoint: Pidex_V1_GeoPoint {
        var p = Pidex_V1_GeoPoint()
        p.latitude = latitude
        p.longitude = longitude
        p.accuracyMeters = accuracy
        return p
    }

    var place: Pidex_V1_Place? {
        guard named else { return nil }
        var p = Pidex_V1_Place()
        p.country = country ?? ""
        p.region = region ?? ""
        p.city = city ?? ""
        p.district = district ?? ""
        p.name = name ?? ""
        return p
    }
}

/// When and where a photo says it was taken, from the metadata the camera
/// wrote into it: a photo added from the library keeps its own day and
/// place, not the moment it was added.
nonisolated enum PhotoMetadata {
    static func read(_ data: Data) -> (date: Date?, location: CLLocation?) {
        guard let source = CGImageSourceCreateWithData(data as CFData, nil),
              let props = CGImageSourceCopyPropertiesAtIndex(source, 0, nil) as? [CFString: Any] else { return (nil, nil) }
        return (date(props), location(props))
    }

    private static func date(_ props: [CFString: Any]) -> Date? {
        let exif = props[kCGImagePropertyExifDictionary] as? [CFString: Any]
        let tiff = props[kCGImagePropertyTIFFDictionary] as? [CFString: Any]
        guard let text = exif?[kCGImagePropertyExifDateTimeOriginal] as? String ?? tiff?[kCGImagePropertyTIFFDateTime] as? String else { return nil }
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        // With the offset the camera noted, the moment is exact; without
        // it, the phone's own time zone is the best guess.
        if let offset = exif?[kCGImagePropertyExifOffsetTimeOriginal] as? String {
            f.dateFormat = "yyyy:MM:dd HH:mm:ssxxx"
            if let d = f.date(from: text + offset) { return d }
        }
        f.dateFormat = "yyyy:MM:dd HH:mm:ss"
        f.timeZone = .current
        return f.date(from: text)
    }

    private static func location(_ props: [CFString: Any]) -> CLLocation? {
        guard let gps = props[kCGImagePropertyGPSDictionary] as? [CFString: Any],
              var lat = gps[kCGImagePropertyGPSLatitude] as? Double,
              var lon = gps[kCGImagePropertyGPSLongitude] as? Double else { return nil }
        if (gps[kCGImagePropertyGPSLatitudeRef] as? String) == "S" { lat = -lat }
        if (gps[kCGImagePropertyGPSLongitudeRef] as? String) == "W" { lon = -lon }
        guard CLLocationCoordinate2DIsValid(CLLocationCoordinate2D(latitude: lat, longitude: lon)), lat != 0 || lon != 0 else { return nil }
        let accuracy = gps[kCGImagePropertyGPSHPositioningError] as? Double ?? 30
        return CLLocation(coordinate: CLLocationCoordinate2D(latitude: lat, longitude: lon), altitude: 0,
                          horizontalAccuracy: accuracy, verticalAccuracy: -1, timestamp: Date())
    }
}

/// Where the phone is while the camera is open, with permission; each shot
/// takes the latest fix if it is recent.
@MainActor
final class LocationTracker: NSObject, CLLocationManagerDelegate {
    private let manager = CLLocationManager()
    private var latest: CLLocation?

    override init() {
        super.init()
        manager.delegate = self
        // A street is enough to name a place, and spares the battery.
        manager.desiredAccuracy = kCLLocationAccuracyHundredMeters
    }

    func start() {
        if manager.authorizationStatus == .notDetermined {
            manager.requestWhenInUseAuthorization()
        }
        manager.startUpdatingLocation()
    }

    func stop() { manager.stopUpdatingLocation() }

    /// The latest fix, if taken in the last two minutes.
    var current: CLLocation? {
        guard let l = latest, -l.timestamp.timeIntervalSinceNow < 120 else { return nil }
        return l
    }

    nonisolated func locationManager(_ manager: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        guard let last = locations.last, last.horizontalAccuracy >= 0 else { return }
        Task { @MainActor in self.latest = last }
    }

    nonisolated func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        let status = manager.authorizationStatus
        Task { @MainActor in
            if status == .authorizedWhenInUse || status == .authorizedAlways { self.manager.startUpdatingLocation() }
        }
    }

    nonisolated func locationManager(_ manager: CLLocationManager, didFailWithError error: Error) {}
}

/// Names coordinates on the phone (Apple's geocoder: free, and nothing is
/// sent to our server), one request at a time, and remembers each spot so a
/// hundred photos from one square ask once.
actor PlaceNamer {
    static let shared = PlaceNamer()

    private let geocoder = CLGeocoder()
    private var cache: [String: ShotPlace] = [:]

    /// The place with its names filled in, or as it was when the phone
    /// cannot name it (offline, or asked too often).
    func name(_ place: ShotPlace) async -> ShotPlace {
        // About 200 m: one square, one street.
        let key = "\((place.latitude * 500).rounded()),\((place.longitude * 500).rounded())"
        if let known = cache[key] { return merged(place, known) }
        guard let mark = try? await geocoder.reverseGeocodeLocation(place.location).first else { return place }
        var named = place
        named.country = mark.country
        named.region = mark.administrativeArea
        named.city = mark.locality ?? mark.subAdministrativeArea
        named.district = mark.subLocality
        // A landmark when there is one; otherwise the street.
        named.name = mark.areasOfInterest?.first ?? mark.thoroughfare
        cache[key] = named
        return named
    }

    private func merged(_ place: ShotPlace, _ known: ShotPlace) -> ShotPlace {
        var p = place
        (p.country, p.region, p.city, p.district, p.name) = (known.country, known.region, known.city, known.district, known.name)
        return p
    }
}

/// Place names as shown: the phone names a Chinese city "大连市" and often
/// starts a landmark with its city ("杭州市北京航空航天大学…"). Search keeps the
/// full names, so either form still finds the photos.
nonisolated enum PlaceNames {
    /// A city without the "市" Chinese names end in, unless that leaves one
    /// character ("沙市" stays).
    static func city(_ name: String) -> String {
        guard name.count > 2, name.hasSuffix("市"), let before = name.dropLast().last, before.isCJK else { return name }
        return String(name.dropLast())
    }

    /// A landmark without the region, city or district it begins with,
    /// in either form; kept whole when that would leave nothing.
    static func area(_ name: String, in place: Pidex_V1_Place) -> String {
        area(name, within: [place.region, place.city, place.district])
    }

    private static func area(_ name: String, within places: [String]) -> String {
        var out = name
        for place in places where !place.isEmpty {
            // The full name if it begins with it ("杭州市…"), else the short one.
            let prefix = out.hasPrefix(place) ? place : city(place)
            if out.hasPrefix(prefix), out.count > prefix.count { out.removeFirst(prefix.count) }
        }
        let trimmed = out.trimmingCharacters(in: .whitespaces.union(.punctuationCharacters))
        return trimmed.isEmpty ? name : trimmed
    }

    /// The name a place count is shown with on a chip, cut short if long.
    static func shown(_ c: Pidex_V1_PlaceCount) -> String {
        switch c.level {
        case "city": city(c.name)
        case "area": short(area(c.name, within: [c.parent]))
        default: c.name
        }
    }

    /// At most `limit` characters, ending in "…" when cut.
    static func short(_ name: String, limit: Int = 14) -> String {
        name.count > limit ? String(name.prefix(limit - 1)) + "…" : name
    }
}

/// Which places a collection's photos are grouped by: the level that tells
/// them apart. Photos from several countries are grouped by country, and a
/// picked country shows its cities; from one country, by city; from one
/// city, by landmark or district.
nonisolated struct PlaceLevels {
    /// The chips to show, most photos first.
    var shown: [Pidex_V1_PlaceCount]
    /// The country picked to look inside, shown as a way back to all.
    var inside: Pidex_V1_PlaceCount?

    init(_ counts: [Pidex_V1_PlaceCount], picked: String?) {
        func at(_ level: String) -> [Pidex_V1_PlaceCount] {
            counts.filter { $0.level == level }.sorted { $0.count > $1.count }
        }
        let countries = at("country"), cities = at("city"), areas = at("area")
        if countries.count > 1 {
            // Inside a country when it, or one of its cities, is picked.
            let country = countries.first { $0.name == picked }
                ?? cities.first { $0.name == picked }.flatMap { c in countries.first { $0.name == c.parent } }
            if let country {
                inside = country
                shown = cities.filter { $0.parent == country.name }
            } else {
                shown = countries
            }
        } else if cities.count > 1 || areas.isEmpty {
            shown = cities
        } else {
            shown = areas
        }
    }

    /// How many places the photos were taken in, as the header counts them.
    static func count(_ counts: [Pidex_V1_PlaceCount]) -> Int {
        let cities = counts.filter { $0.level == "city" }.count
        return cities > 1 ? cities : max(cities, counts.filter { $0.level == "area" }.count)
    }
}

private extension Character {
    nonisolated var isCJK: Bool {
        unicodeScalars.allSatisfy { (0x4E00...0x9FFF).contains($0.value) || (0x3400...0x4DBF).contains($0.value) }
    }
}
