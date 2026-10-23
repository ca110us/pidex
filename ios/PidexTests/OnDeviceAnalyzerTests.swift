import CoreImage
import CoreImage.CIFilterBuiltins
import Testing
@testable import Pidex

struct OnDeviceAnalyzerTests {
    @Test func readsAWristbandBarcode() throws {
        let filter = CIFilter.code128BarcodeGenerator()
        filter.message = Data("MRN 24681357".utf8)
        filter.quietSpace = 20
        let code = try #require(filter.outputImage).transformed(by: CGAffineTransform(scaleX: 6, y: 6))
        let image = try #require(CIContext().createCGImage(code, from: code.extent))

        let a = OnDeviceAnalyzer.analyze(image)
        #expect(a.barcodes.map(\.payload) == ["MRN 24681357"])
        #expect(a.barcodes.first?.symbology == "code128")
        #expect(a.perceptualHash.count == 16)
        #expect(a.sharpness > 0)
    }
}
