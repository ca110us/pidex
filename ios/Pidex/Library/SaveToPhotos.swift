import CoreLocation
import Foundation
import ImageIO
import Photos

/// Saves photos to the phone's Photos library: as the camera made them,
/// Live Photos with their motion, where the collection keeps those; else
/// the working copy. It only ever adds: the app never reads the library.
nonisolated enum SaveToPhotos {
    enum Failure: LocalizedError {
        case denied

        var errorDescription: String? {
            String(localized: "Pidex may not add to Photos. Allow it in Settings › Pidex › Photos.")
        }
    }

    /// How many were saved, a few fetched at a time.
    static func save(_ photos: [Pidex_V1_Photo], from api: APIClient) async throws -> Int {
        let status = await PHPhotoLibrary.requestAuthorization(for: .addOnly)
        guard status == .authorized || status == .limited else { throw Failure.denied }
        await PhotoLinks.shared.know(photos.map(\.id), in: api)
        return await withTaskGroup(of: Bool.self) { group in
            var saved = 0
            for (i, photo) in photos.enumerated() {
                if i >= 3, await group.next() == true { saved += 1 }
                group.addTask { (try? await save(photo)) != nil }
            }
            for await ok in group where ok { saved += 1 }
            return saved
        }
    }

    private static func save(_ photo: Pidex_V1_Photo) async throws {
        var still: Data?
        if photo.originalKept, let url = await PhotoLinks.shared.link(for: photo.id, .original) {
            still = try? await fetch(url)
        }
        // Motion pairs only with the still it was made with.
        let paired = still != nil
        if still == nil {
            still = DiskImages.originals.data(photo.id)
        }
        if still == nil, let url = await PhotoLinks.shared.link(for: photo.id, .view) {
            still = try await fetch(url)
        }
        guard let still, let source = CGImageSourceCreateWithData(still as CFData, nil),
              let type = CGImageSourceGetType(source) as String? else { throw CocoaError(.fileReadCorruptFile) }
        var movie: URL?
        if paired, photo.live, let url = await PhotoLinks.shared.link(for: photo.id, .live),
           let (file, response) = try? await URLSession.shared.download(from: url),
           (response as? HTTPURLResponse)?.statusCode == 200 {
            let named = file.deletingPathExtension().appendingPathExtension("mov")
            movie = (try? FileManager.default.moveItem(at: file, to: named)).map { named } ?? file
        }
        defer { if let movie { try? FileManager.default.removeItem(at: movie) } }
        let taken = photo.hasCapturedAt ? photo.capturedAt.date : nil
        let location = photo.hasLocation
            ? CLLocation(latitude: photo.location.latitude, longitude: photo.location.longitude) : nil
        try await PHPhotoLibrary.shared().performChanges {
            let request = PHAssetCreationRequest.forAsset()
            let options = PHAssetResourceCreationOptions()
            options.uniformTypeIdentifier = type
            request.addResource(with: .photo, data: still, options: options)
            if let movie {
                let moved = PHAssetResourceCreationOptions()
                moved.shouldMoveFile = true
                request.addResource(with: .pairedVideo, fileURL: movie, options: moved)
            }
            request.creationDate = taken
            request.location = location
        }
    }

    private static func fetch(_ url: URL) async throws -> Data {
        let (data, response) = try await URLSession.shared.data(from: url)
        guard (response as? HTTPURLResponse)?.statusCode == 200 else { throw URLError(.badServerResponse) }
        return data
    }
}
