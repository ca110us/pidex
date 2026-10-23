import Foundation
import Observation

/// Photos sent from the camera, followed until every one is uploaded and
/// filed, so leaving the camera never hides how far they got.
@MainActor
@Observable
final class UploadBatch {
    let sessionID: UUID
    let api: APIClient
    let collection: String
    /// Photos sent in this session so far.
    private(set) var expected: Int
    private(set) var summary: Pidex_V1_GetSessionSummaryResponse?
    private(set) var error: String?
    private var polling: Task<Void, Never>?

    enum Phase: Equatable {
        case uploading(done: Int)
        case filing(done: Int)
        case done
    }

    init(sessionID: UUID, api: APIClient, collection: String, expected: Int) {
        self.sessionID = sessionID
        self.api = api
        self.collection = collection
        self.expected = expected
    }

    func add(_ count: Int) {
        expected += count
        follow(pending: expected)
    }

    /// Where the photos are, given how many still wait on this phone.
    func phase(pending: Int) -> Phase {
        let uploaded = max(0, expected - pending)
        if uploaded < expected { return .uploading(done: uploaded) }
        guard let s = summary, Int(s.totalCount) >= expected else { return .filing(done: 0) }
        let filed = Int(s.totalCount - s.processingCount)
        return filed < expected ? .filing(done: filed) : .done
    }

    /// From 0 to 1: uploading is the first half, filing the second.
    func progress(pending: Int) -> Double {
        guard expected > 0 else { return 1 }
        switch phase(pending: pending) {
        case .uploading(let n): return 0.5 * Double(n) / Double(expected)
        case .filing(let n): return 0.5 + 0.5 * Double(n) / Double(expected)
        case .done: return 1
        }
    }

    var needsReview: Int { summary?.needsReview.count ?? 0 }

    /// Asks the server how filing is going until it is done.
    func follow(pending: Int) {
        guard polling == nil else { return }
        polling = Task { [weak self] in
            while !Task.isCancelled, let self {
                await self.refresh()
                if self.phase(pending: 0) == .done, self.summary.map({ Int($0.totalCount) >= self.expected }) ?? false { break }
                try? await Task.sleep(for: .seconds(2))
            }
            self?.polling = nil
        }
    }

    /// Shows a new name at once, before the next look at the server.
    func renamed(_ subject: Pidex_V1_Subject) {
        guard var s = summary else { return }
        for i in s.groups.indices where s.groups[i].subject.id == subject.id {
            s.groups[i].subject.displayName = subject.displayName
        }
        summary = s
    }

    func refresh() async {
        do {
            summary = try await api.sessionSummary(sessionID)
            error = nil
        } catch {
            self.error = error.localizedDescription
        }
    }
}
