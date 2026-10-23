import CoreImage
import CoreImage.CIFilterBuiltins
import Foundation
import ImageIO
import Testing
import UniformTypeIdentifiers
@testable import Pidex

/// Runs against a real server when PIDEX_SERVER and PIDEX_TOKEN are
/// set (pass them to xcodebuild as TEST_RUNNER_PIDEX_SERVER and
/// TEST_RUNNER_PIDEX_TOKEN). Exercises Vision, the Connect client, the
/// signed upload and the server pipeline together.
struct LiveServerTests {
    static let env = ProcessInfo.processInfo.environment

    /// Designs a scenario with the server's real config model (about
    /// $0.03), creates a workspace with it and reads it back. Opt in with
    /// PIDEX_LIVE_DRAFT=1.
    @Test(.enabled(if: env["PIDEX_SERVER"] != nil && env["PIDEX_TOKEN"] != nil && env["PIDEX_LIVE_DRAFT"] == "1"))
    func scenarioFromDescription() async throws {
        let url = URL(string: Self.env["PIDEX_SERVER"]!)!
        let api = APIClient(baseURL: url, token: Self.env["PIDEX_TOKEN"]!)
        let draft = try await api.draftConfig(
            description: "我在家养了十几盆植物，想记录生长和病虫害。每盆插了一张写着名字和两位编号的卡片，比如“绿萝 03”。",
            base: nil, forNewTeam: true)
        let config = try ScenarioConfig(json: draft.configJson)
        #expect(!draft.summary.isEmpty)
        #expect(!draft.testTips.isEmpty)
        #expect(draft.warnings.isEmpty)
        #expect(config.tags.count >= 10)

        let (team, token) = try await api.createTeam(name: config.name, configJSON: draft.configJson)
        #expect(team.role == "owner")
        let created = APIClient(baseURL: url, token: token)
        #expect(try await created.team().id == team.id)
        let active = try ScenarioConfig(json: try await created.activeConfig().configJson)
        #expect(active.subjectTypes.map(\.key) == config.subjectTypes.map(\.key))
    }

    @Test(.enabled(if: env["PIDEX_SERVER"] != nil && env["PIDEX_TOKEN"] != nil))
    func wristbandThenPhotoAreFiledTogether() async throws {
        let api = APIClient(baseURL: URL(string: Self.env["PIDEX_SERVER"]!)!, token: Self.env["PIDEX_TOKEN"]!)
        let mrn = String(Int.random(in: 10_000_000...99_999_999))

        // 1. A real Code 128 barcode, read by Vision on the device.
        let (wristband, image) = try ImageFeatures.uprightJPEG(from: barcodeJPEG(mrn))
        let analysis = OnDeviceAnalyzer.analyze(image)
        let code = try #require(analysis.barcodes.first)
        #expect(code.payload == mrn)
        #expect(code.symbology == "code128")

        // 2. The instant banner lookup: nobody has this MRN yet.
        let anchor = try await api.resolveAnchor(analysis)
        #expect(anchor.state == .newSubject)
        #expect(anchor.extractedIdentity["mrn"] == mrn)

        // 3. Both shots go through the real queue and signed upload.
        let dir = FileManager.default.temporaryDirectory.appending(path: "live-\(UUID().uuidString)")
        let queue = try UploadQueue(directory: dir, transport: LiveUploadTransport(api: api), autoDrain: false)
        let session = UUID()
        let start = Date()
        let wristbandID = UUID().uuidString.lowercased()
        try await queue.enqueue(jpeg: wristband, sessionID: session.uuidString.lowercased(), capturedAt: start,
                                onDevice: analysis, lockedSubjectID: nil, id: wristbandID)
        let (content, contentImage) = try ImageFeatures.uprightJPEG(from: plainJPEG())
        try await queue.enqueue(jpeg: content, sessionID: session.uuidString.lowercased(), capturedAt: start.addingTimeInterval(30),
                                onDevice: OnDeviceAnalyzer.analyze(contentImage), lockedSubjectID: nil,
                                identityPhotoID: wristbandID)
        await queue.drainOnce()
        #expect(await queue.pending.isEmpty)

        // 4. The server's workers file the wristband under the new patient.
        // The plain follow-up shot is carried along by shooting order when
        // the server has no vision model, and judged junk when it has one;
        // either way it must not go anywhere else.
        var summary = try await api.sessionSummary(session)
        for _ in 0..<40 where summary.processingCount > 0 || summary.groups.isEmpty {
            try await Task.sleep(for: .milliseconds(500))
            summary = try await api.sessionSummary(session)
        }
        #expect(summary.totalCount == 2)
        #expect(summary.groups.count == 1)
        #expect(summary.groups.first?.subject.identity["mrn"] == mrn)
        #expect((summary.groups.first?.photoCount ?? 0) + summary.junkCount == 2)
        #expect(summary.needsReview.isEmpty)

        // 5. Now the banner knows them.
        #expect(try await api.resolveAnchor(analysis).state == .matched)
    }

    private func barcodeJPEG(_ payload: String) throws -> Data {
        let filter = CIFilter.code128BarcodeGenerator()
        filter.message = Data(payload.utf8)
        filter.quietSpace = 20
        let code = try #require(filter.outputImage).transformed(by: CGAffineTransform(scaleX: 6, y: 6))
        let ctx = CIContext()
        let cg = try #require(ctx.createCGImage(code, from: code.extent))
        return encode(cg)
    }

    private func plainJPEG() -> Data {
        let ctx = CGContext(data: nil, width: 640, height: 480, bitsPerComponent: 8, bytesPerRow: 0,
                            space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
        ctx.setFillColor(CGColor(red: 0.8, green: 0.6, blue: 0.5, alpha: 1))
        ctx.fill(CGRect(x: 0, y: 0, width: 640, height: 480))
        return encode(ctx.makeImage()!)
    }

    private func encode(_ image: CGImage) -> Data {
        let out = NSMutableData()
        let dest = CGImageDestinationCreateWithData(out, UTType.jpeg.identifier as CFString, 1, nil)!
        CGImageDestinationAddImage(dest, image, nil)
        CGImageDestinationFinalize(dest)
        return out as Data
    }
}
