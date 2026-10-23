import Foundation

nonisolated struct CreatedUpload: Sendable {
    var photoID: String
    var uploadURL: URL?
    var headers: [String: String]
    var alreadyUploaded: Bool
}

/// How an upload attempt failed decides what the queue does next.
nonisolated enum UploadFailure: Error, Equatable, Sendable {
    /// Network or server trouble: keep the photo, retry with backoff.
    case transient(String)
    /// The server has no bytes for this photo: upload them again.
    case notReceived
    /// Bad token or no permission: pause until settings change.
    case unauthorized(String)
    /// The server refuses this photo outright; keep it and show the reason.
    case rejected(String)
}

/// The network side of the upload queue, separated so the queue's
/// bookkeeping can be tested without a server.
nonisolated protocol UploadTransport: Sendable {
    func createUpload(clientPhotoID: String, sizeBytes: Int64, sha256: String) async throws(UploadFailure) -> CreatedUpload
    func put(file: URL, to url: URL, headers: [String: String]) async throws(UploadFailure)
    func complete(_ request: Pidex_V1_CompleteUploadRequest) async throws(UploadFailure)
}

nonisolated struct LiveUploadTransport: UploadTransport {
    let api: APIClient
    var session: URLSession = .shared

    func createUpload(clientPhotoID: String, sizeBytes: Int64, sha256: String) async throws(UploadFailure) -> CreatedUpload {
        var r = Pidex_V1_CreateUploadRequest()
        r.clientPhotoID = clientPhotoID
        r.contentType = "image/jpeg"
        r.sizeBytes = sizeBytes
        r.sha256Hex = sha256
        do {
            let resp = try await api.createUpload(r)
            return CreatedUpload(
                photoID: resp.photoID,
                uploadURL: URL(string: resp.uploadURL),
                headers: resp.uploadHeaders,
                alreadyUploaded: resp.alreadyUploaded
            )
        } catch {
            throw Self.classify(error)
        }
    }

    func put(file: URL, to url: URL, headers: [String: String]) async throws(UploadFailure) {
        var req = URLRequest(url: url)
        req.httpMethod = "PUT"
        for (k, v) in headers where k.caseInsensitiveCompare("Content-Length") != .orderedSame {
            req.setValue(v, forHTTPHeaderField: k)
        }
        let response: URLResponse
        do {
            (_, response) = try await session.upload(for: req, fromFile: file)
        } catch {
            throw .transient(error.localizedDescription)
        }
        let status = (response as? HTTPURLResponse)?.statusCode ?? 0
        // A 403 is usually an expired signed URL; the next attempt gets a
        // fresh one from CreateUpload.
        guard (200..<300).contains(status) else { throw .transient("upload returned HTTP \(status)") }
    }

    func complete(_ request: Pidex_V1_CompleteUploadRequest) async throws(UploadFailure) {
        do {
            _ = try await api.completeUpload(request)
        } catch {
            throw Self.classify(error)
        }
    }

    static func classify(_ error: Error) -> UploadFailure {
        guard let e = error as? APIError else { return .transient(error.localizedDescription) }
        switch e.code {
        case .unauthenticated, .permissionDenied: return .unauthorized(e.message)
        case .failedPrecondition where e.message.contains("upload not received"): return .notReceived
        case .invalidArgument, .notFound, .failedPrecondition: return .rejected(e.message)
        default: return .transient(e.message)
        }
    }
}
