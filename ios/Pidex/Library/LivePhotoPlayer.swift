import ImageIO
import PhotosUI
import SwiftUI
import UniformTypeIdentifiers

/// A Live Photo's motion, played once over the photo when its badge is
/// tapped. Its two files are fetched only then: they are large.
struct LiveBadge: View {
    let photo: Pidex_V1_Photo
    let placeholder: UIImage?
    @State private var live: PHLivePhoto?
    @State private var loading = false
    @State private var playing = false

    var body: some View {
        ZStack(alignment: .bottomLeading) {
            if playing, let live {
                LivePhotoPlayer(live: live) { playing = false }
                    .background(.black)
                    .ignoresSafeArea()
                    .allowsHitTesting(false)
            }
            Button {
                Task { await play() }
            } label: {
                HStack(spacing: 4) {
                    if loading {
                        ProgressView().controlSize(.mini)
                    } else {
                        Image(systemName: "livephoto")
                    }
                    Text("LIVE")
                }
                .font(.caption.weight(.semibold))
                .padding(.horizontal, 10)
                .padding(.vertical, 6)
                .glassSurface(Capsule())
            }
            .buttonStyle(.plain)
            .disabled(loading || playing)
            .padding(.leading, 16)
            .padding(.bottom, 40)
            .accessibilityLabel(Text("Play Live Photo"))
            .accessibilityIdentifier("playLive")
        }
    }

    private func play() async {
        if live == nil {
            loading = true
            live = await LiveFiles.livePhoto(photo, placeholder: placeholder)
            loading = false
        }
        if live != nil { playing = true }
    }
}

private struct LivePhotoPlayer: UIViewRepresentable {
    let live: PHLivePhoto
    let ended: () -> Void

    func makeCoordinator() -> Coordinator { Coordinator(ended: ended) }

    func makeUIView(context: Context) -> PHLivePhotoView {
        let view = PHLivePhotoView()
        view.contentMode = .scaleAspectFit
        view.isMuted = true
        view.delegate = context.coordinator
        view.livePhoto = live
        view.startPlayback(with: .full)
        return view
    }

    func updateUIView(_ view: PHLivePhotoView, context: Context) {
        context.coordinator.ended = ended
    }

    final class Coordinator: NSObject, PHLivePhotoViewDelegate {
        var ended: () -> Void
        init(ended: @escaping () -> Void) { self.ended = ended }

        func livePhotoView(_ view: PHLivePhotoView, didEndPlaybackWith style: PHLivePhotoViewPlaybackStyle) {
            ended()
        }
    }
}

/// A Live Photo's still and motion, fetched to temporary files.
private enum LiveFiles {
    static func livePhoto(_ photo: Pidex_V1_Photo, placeholder: UIImage?) async -> PHLivePhoto? {
        guard let stillURL = await PhotoLinks.shared.link(for: photo.id, .original),
              let movieURL = await PhotoLinks.shared.link(for: photo.id, .live),
              let (still, _) = try? await URLSession.shared.data(from: stillURL),
              let source = CGImageSourceCreateWithData(still as CFData, nil),
              let type = CGImageSourceGetType(source).flatMap({ UTType($0 as String) }),
              let (movie, _) = try? await URLSession.shared.download(from: movieURL) else { return nil }
        let dir = FileManager.default.temporaryDirectory.appending(path: "Live", directoryHint: .isDirectory)
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let stillFile = dir.appending(path: photo.id).appendingPathExtension(type.preferredFilenameExtension ?? "heic")
        let movieFile = dir.appending(path: photo.id).appendingPathExtension("mov")
        try? FileManager.default.removeItem(at: movieFile)
        guard (try? still.write(to: stillFile)) != nil,
              (try? FileManager.default.moveItem(at: movie, to: movieFile)) != nil else { return nil }
        return await withCheckedContinuation { cont in
            var done = false
            PHLivePhoto.request(withResourceFileURLs: [stillFile, movieFile], placeholderImage: placeholder,
                                targetSize: .zero, contentMode: .aspectFit) { live, info in
                // Called again as it gets better; the first full one, once.
                let degraded = info[PHLivePhotoInfoIsDegradedKey] as? Bool ?? false
                let failed = info[PHLivePhotoInfoErrorKey] != nil || info[PHLivePhotoInfoCancelledKey] as? Bool == true
                guard !done, !degraded || failed else { return }
                done = true
                cont.resume(returning: failed ? nil : live)
            }
        }
    }
}
