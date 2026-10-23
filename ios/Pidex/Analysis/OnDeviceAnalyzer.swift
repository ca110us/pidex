import CoreGraphics
import CoreML
import Foundation
import OSLog
import Vision

private let log = Logger(subsystem: "app.pidex.ios", category: "analysis")

/// Everything the phone can tell about a photo for free, before upload:
/// barcodes, text, faces, sharpness and a perceptual hash. The server uses
/// barcodes and text to identify subjects without a model call, and as an
/// independent second read to confirm what a model read.
nonisolated enum OnDeviceAnalyzer {
    static func analyze(_ image: CGImage) -> Pidex_V1_OnDeviceAnalysis {
        let barcodes = VNDetectBarcodesRequest()
        let text = VNRecognizeTextRequest()
        text.recognitionLevel = .accurate
        // Identifiers must be read as printed, not "corrected" into words.
        text.usesLanguageCorrection = false
        // Labels are written in the team's language (a plant's Chinese name
        // and "03" on a Chinese team's pot); English-only
        // recognition turns other scripts into noise around the ID.
        text.automaticallyDetectsLanguage = true
        let faces = VNDetectFaceRectanglesRequest()
        // Where the photo's subject is, so comparisons can leave the
        // background out: a pet if one is found, else one object that
        // clearly fills the frame. A small "most salient" object in a busy
        // scene is as often a person or a door, and a wrong cut is worse
        // than none.
        let animals = VNRecognizeAnimalsRequest()
        let salient = VNGenerateObjectnessBasedSaliencyImageRequest()

        var out = Pidex_V1_OnDeviceAnalysis()
        let handler = VNImageRequestHandler(cgImage: image, orientation: .up)
        // Each request runs on its own, so one failure cannot silently cost
        // the others. Analysis is an optimisation: the photo is uploaded
        // regardless, and the server falls back to its model.
        for request in [barcodes, text, faces, animals, salient] as [VNRequest] {
            useCPUOnSimulator(request)
            do {
                try handler.perform([request])
            } catch {
                log.error("\(type(of: request), privacy: .public) failed: \(error.localizedDescription, privacy: .public)")
            }
        }

        var seen = Set<String>()
        for b in barcodes.results ?? [] {
            guard let payload = b.payloadStringValue, !payload.isEmpty else { continue }
            let name = symbologyName(b.symbology.rawValue)
            guard seen.insert(name + "|" + payload).inserted else { continue }
            var obs = Pidex_V1_BarcodeObservation()
            obs.symbology = name
            obs.payload = payload
            out.barcodes.append(obs)
        }
        for t in text.results ?? [] {
            guard let best = t.topCandidates(1).first, !best.string.isEmpty else { continue }
            var obs = Pidex_V1_TextObservation()
            obs.text = best.string
            obs.confidence = best.confidence
            // Vision's origin is bottom-left; the API's is top-left.
            let r = t.boundingBox
            var box = Pidex_V1_BoundingBox()
            box.x = Float(r.minX)
            box.y = Float(1 - r.maxY)
            box.width = Float(r.width)
            box.height = Float(r.height)
            obs.box = box
            out.text.append(obs)
        }
        out.faceCount = Int32(faces.results?.count ?? 0)
        let largest: ([CGRect]) -> CGRect? = { $0.max { $0.width * $0.height < $1.width * $1.height } }
        let objects = salient.results?.first?.salientObjects ?? []
        let clear = objects.count == 1 ? objects.filter { $0.boundingBox.width * $0.boundingBox.height >= 0.15 } : []
        if let r = largest((animals.results ?? []).map(\.boundingBox)) ?? largest(clear.map(\.boundingBox)) {
            var box = Pidex_V1_BoundingBox()
            box.x = Float(r.minX)
            box.y = Float(1 - r.maxY)
            box.width = Float(r.width)
            box.height = Float(r.height)
            out.subjectBox = box
        }
        out.sharpness = ImageFeatures.sharpness(image)
        out.perceptualHash = ImageFeatures.hex(ImageFeatures.differenceHash(image))
        return out
    }

    /// The simulator has no Neural Engine path for Vision: requests fail
    /// with "Could not create inference context" unless pinned to the CPU,
    /// and the ML barcode detectors (revision 3+) find nothing even then,
    /// so the simulator uses the classic detector. Devices keep Vision's
    /// defaults, which are more accurate.
    private static func useCPUOnSimulator(_ request: VNRequest) {
        #if targetEnvironment(simulator)
        if let barcodes = request as? VNDetectBarcodesRequest {
            barcodes.revision = VNDetectBarcodesRequestRevision2
        }
        guard let devices = try? request.supportedComputeStageDevices else { return }
        for (stage, options) in devices {
            if let cpu = options.first(where: { if case .cpu = $0 { true } else { false } }) {
                request.setComputeDevice(cpu, for: stage)
            }
        }
        #endif
    }

    /// Maps Vision's symbology names ("VNBarcodeSymbologyCode128") to the
    /// API's lowercase names ("code128"), folding variants together.
    static func symbologyName(_ raw: String) -> String {
        var s = raw.lowercased()
        if s.hasPrefix("vnbarcodesymbology") { s.removeFirst("vnbarcodesymbology".count) }
        switch s {
        case "code39checksum", "code39fullascii", "code39fullasciichecksum": return "code39"
        case "code93i": return "code93"
        case "itf14", "i2of5", "i2of5checksum": return "itf"
        case "microqr": return "qr"
        case "micropdf417": return "pdf417"
        case let x where x.hasPrefix("gs1databar"): return "gs1databar"
        default: return s
        }
    }

    /// True when the photo may show an identity carrier worth an instant
    /// lookup: a barcode, or text containing digits.
    static func mayCarryIdentity(_ a: Pidex_V1_OnDeviceAnalysis) -> Bool {
        !a.barcodes.isEmpty || a.text.contains { $0.text.contains(where: \.isNumber) }
    }
}
