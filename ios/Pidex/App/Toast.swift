import SwiftUI

/// A short note that floats over the screen and goes away by itself:
/// something done, or something that went wrong.
nonisolated struct Toast: Equatable, Identifiable, Sendable {
    enum Tone: Sendable {
        case done
        case problem
    }
    let id = UUID()
    let text: String
    var tone = Tone.done
}

/// The toast showing, at the top of the screen; a tap dismisses it.
struct ToastOverlay: View {
    @Environment(AppModel.self) private var app

    var body: some View {
        VStack {
            if let toast = app.toast {
                Button { app.toast = nil } label: {
                    Label {
                        Text(verbatim: toast.text).multilineTextAlignment(.leading)
                    } icon: {
                        Image(systemName: toast.tone == .done ? "checkmark.circle.fill" : "exclamationmark.triangle.fill")
                            .foregroundStyle(toast.tone == .done ? Theme.onInk : Theme.attention)
                    }
                    .font(.subheadline.weight(.medium))
                    // An ink pill: dark on a light page, light on a dark one.
                    .foregroundStyle(Theme.onInk)
                    .padding(.vertical, 12)
                    .padding(.horizontal, 18)
                    .background(Theme.ink, in: Capsule())
                    .shadow(color: .black.opacity(0.2), radius: 14, y: 6)
                }
                .buttonStyle(.plain)
                .padding(.horizontal, 20)
                .transition(.move(edge: .top).combined(with: .opacity))
                .id(toast.id)
                .accessibilityIdentifier("toast")
                .task(id: toast.id) {
                    try? await Task.sleep(for: .seconds(toast.tone == .done ? 3 : 5))
                    if app.toast?.id == toast.id { app.toast = nil }
                }
            }
            Spacer()
        }
        .animation(.spring(duration: 0.35), value: app.toast)
    }
}
