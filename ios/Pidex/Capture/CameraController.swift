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
    private var waiting: [Int64: CheckedContinuation<Data, Error>] = [:]

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
        configured = true
    }

    /// Takes one photo and returns its encoded bytes (HEIC or JPEG, with
    /// orientation metadata; ImageFeatures normalizes it).
    func capture() async throws -> Data {
        try await withCheckedThrowingContinuation { cont in
            queue.async {
                guard self.configured, self.session.isRunning else {
                    cont.resume(throwing: Failure.unavailable)
                    return
                }
                let settings = AVCapturePhotoSettings()
                settings.photoQualityPrioritization = .balanced
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
        let result: Result<Data, Error>
        if let error {
            result = .failure(Failure.captureFailed(error.localizedDescription))
        } else if let data = photo.fileDataRepresentation() {
            result = .success(data)
        } else {
            result = .failure(Failure.captureFailed("no image data"))
        }
        queue.async {
            self.waiting.removeValue(forKey: id)?.resume(with: result)
        }
    }
}
