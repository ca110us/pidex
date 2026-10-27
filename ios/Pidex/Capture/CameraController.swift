@preconcurrency import AVFoundation
import Foundation

/// Owns the capture session. All session work happens on its own queue;
/// photos are handed back as encoded data through async calls.
nonisolated final class CameraController: NSObject, @unchecked Sendable {
    enum Failure: LocalizedError {
        case denied, unavailable, captureFailed(String)

        var errorDescription: String? {
            switch self {
            case .denied: String(localized: "Camera access is off. Enable it in Settings › Pidex.")
            case .unavailable: String(localized: "No camera is available on this device.")
            case .captureFailed(let reason): String(localized: "The photo could not be taken: \(reason)")
            }
        }
    }

    let session = AVCaptureSession()
    private let output = AVCapturePhotoOutput()
    private let queue = DispatchQueue(label: "app.pidex.camera")
    private var configured = false
    // Guarded by `queue`.
    private var waiting: [Int64: CheckedContinuation<Shot, Error>] = [:]
    private var motionWanted = false
    private var movies: [Int64: URL] = [:]
    /// Motion finished before anyone asked for it (nil: none was made).
    private var motions: [Int64: URL?] = [:]
    private var motionWaiting: [Int64: CheckedContinuation<URL?, Never>] = [:]

    /// A shot as the camera made it. With Live Photos kept, the motion
    /// around it comes a second or two later, by `motion(of:)`.
    struct Shot: Sendable {
        var data: Data
        var motionID: Int64?
    }

    func start() async throws {
        switch AVCaptureDevice.authorizationStatus(for: .video) {
        case .authorized: break
        case .notDetermined:
            guard await AVCaptureDevice.requestAccess(for: .video) else { throw Failure.denied }
        default: throw Failure.denied
        }
        try await withCheckedThrowingContinuation { (cont: CheckedContinuation<Void, Error>) in
            queue.async {
                do {
                    try self.configureIfNeeded()
                    if !self.session.isRunning { self.session.startRunning() }
                    cont.resume()
                } catch {
                    cont.resume(throwing: error)
                }
            }
        }
    }

    func stop() {
        queue.async { if self.session.isRunning { self.session.stopRunning() } }
    }

    private func configureIfNeeded() throws {
        guard !configured else { return }
        guard let device = AVCaptureDevice.default(.builtInWideAngleCamera, for: .video, position: .back),
              let input = try? AVCaptureDeviceInput(device: device) else {
            throw Failure.unavailable
        }
        session.beginConfiguration()
        defer { session.commitConfiguration() }
        session.sessionPreset = .photo
        guard session.canAddInput(input), session.canAddOutput(output) else { throw Failure.unavailable }
        session.addInput(input)
        session.addOutput(output)
        output.maxPhotoQualityPrioritization = .balanced
        output.isLivePhotoCaptureEnabled = motionWanted && output.isLivePhotoCaptureSupported
        configured = true
    }

    /// Keeps the motion around each shot (a Live Photo) where the camera
    /// can. No sound is recorded: the session has no microphone.
    func keepMotion(_ on: Bool) {
        queue.async {
            self.motionWanted = on
            guard self.configured else { return }
            let enabled = on && self.output.isLivePhotoCaptureSupported
            guard self.output.isLivePhotoCaptureEnabled != enabled else { return }
            self.session.beginConfiguration()
            self.output.isLivePhotoCaptureEnabled = enabled
            self.session.commitConfiguration()
        }
    }

    /// The motion of a shot taken with it, as a QuickTime file the caller
    /// then owns; nil if none could be made.
    func motion(of id: Int64) async -> URL? {
        await withCheckedContinuation { cont in
            queue.async {
                if let done = self.motions.removeValue(forKey: id) {
                    cont.resume(returning: done)
                } else {
                    self.motionWaiting[id] = cont
                }
            }
        }
    }

    /// Takes one photo and returns its encoded bytes (HEIC where the phone
    /// can, else JPEG, with orientation metadata; ImageFeatures normalizes
    /// it), and whether its motion follows.
    func capture() async throws -> Shot {
        try await withCheckedThrowingContinuation { cont in
            queue.async {
                guard self.configured, self.session.isRunning else {
                    cont.resume(throwing: Failure.unavailable)
                    return
                }
                let settings = self.output.availablePhotoCodecTypes.contains(.hevc)
                    ? AVCapturePhotoSettings(format: [AVVideoCodecKey: AVVideoCodecType.hevc])
                    : AVCapturePhotoSettings()
                settings.photoQualityPrioritization = .balanced
                if self.output.isLivePhotoCaptureEnabled {
                    settings.livePhotoMovieFileURL = FileManager.default.temporaryDirectory
                        .appending(path: UUID().uuidString + ".mov")
                }
                if let conn = self.output.connection(with: .video), conn.isVideoRotationAngleSupported(90) {
                    conn.videoRotationAngle = 90 // portrait
                }
                self.waiting[settings.uniqueID] = cont
                self.output.capturePhoto(with: settings, delegate: self)
            }
        }
    }
}

extension CameraController: AVCapturePhotoCaptureDelegate {
    func photoOutput(_ output: AVCapturePhotoOutput, didFinishProcessingPhoto photo: AVCapturePhoto, error: Error?) {
        let id = photo.resolvedSettings.uniqueID
        let live = photo.resolvedSettings.livePhotoMovieDimensions.width > 0
        let result: Result<Shot, Error>
        if let error {
            result = .failure(Failure.captureFailed(error.localizedDescription))
        } else if let data = photo.fileDataRepresentation() {
            result = .success(Shot(data: data, motionID: live ? id : nil))
        } else {
            result = .failure(Failure.captureFailed("no image data"))
        }
        queue.async {
            self.waiting.removeValue(forKey: id)?.resume(with: result)
        }
    }

    func photoOutput(_ output: AVCapturePhotoOutput, didFinishProcessingLivePhotoToMovieFileAt outputFileURL: URL,
                     duration: CMTime, photoDisplayTime: CMTime, resolvedSettings: AVCaptureResolvedPhotoSettings, error: Error?) {
        let id = resolvedSettings.uniqueID
        guard error == nil else { return }
        queue.async { self.movies[id] = outputFileURL }
    }

    func photoOutput(_ output: AVCapturePhotoOutput, didFinishCaptureFor resolvedSettings: AVCaptureResolvedPhotoSettings, error: Error?) {
        let id = resolvedSettings.uniqueID
        guard resolvedSettings.livePhotoMovieDimensions.width > 0 else { return }
        queue.async {
            let movie = self.movies.removeValue(forKey: id)
            if let waiting = self.motionWaiting.removeValue(forKey: id) {
                waiting.resume(returning: movie)
            } else {
                self.motions.updateValue(movie, forKey: id)
            }
        }
    }
}
