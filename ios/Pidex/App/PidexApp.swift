import SwiftUI

@main
struct PidexApp: App {
    @State private var app = AppModel()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            Group {
                if app.active == nil {
                    NavigationStack { SettingsView() }
                } else {
                    CollectionsView()
                }
            }
            // Floating, never part of a page: the photos on their way, and
            // short notes that go by themselves.
            .overlay(alignment: .bottomLeading) {
                BatchPill()
                    .padding(.leading, 16)
                    .padding(.bottom, 28)
                    .animation(.spring(duration: 0.35), value: app.batch == nil)
            }
            .overlay { ToastOverlay() }
            .fullScreenCover(isPresented: $app.showingCamera) {
                NavigationStack {
                    if let capture = app.capture {
                        // A new workspace brings a new capture model; a new
                        // view identity makes it start its camera (and the
                        // old view stop its own).
                        CaptureView(model: capture)
                            .id(ObjectIdentifier(capture))
                    }
                }
                .overlay { ToastOverlay() }
                .environment(app)
            }
            .sheet(isPresented: $app.showingBatch) {
                if let batch = app.batch {
                    NavigationStack {
                        SessionSummaryView(batch: batch) {
                            app.closeBatch()
                        }
                    }
                    .overlay { ToastOverlay() }
                    .environment(app)
                }
            }
            .environment(app)
        }
        .onChange(of: scenePhase) { _, phase in
            if phase == .active { app.becameActive() }
        }
    }
}
