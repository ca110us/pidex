import Foundation
import Observation

/// App-wide state: the server, the saved workspaces (shown as collections),
/// the one the camera shoots for, and the single upload queue that outlives
/// settings changes.
@MainActor
@Observable
final class AppModel {
    private(set) var serverURL: String
    private(set) var workspaces: WorkspaceList
    private(set) var api: APIClient?
    private(set) var capture: CaptureModel?
    private(set) var uploadStatus = UploadStatus()
    private(set) var startupError: String?
    /// The camera is presented above everything for the active workspace.
    var showingCamera = false
    /// The photos last sent from the camera, followed until filed.
    private(set) var batch: UploadBatch?
    /// The batch's details are shown above everything.
    var showingBatch = false
    /// The note floating over the screen, if any; it goes by itself.
    var toast: Toast?

    func show(_ text: String, tone: Toast.Tone = .done) {
        toast = Toast(text: text, tone: tone)
    }
    private var uploads: UploadQueue?
    private var clients: [String: APIClient] = [:]

    private static let serverKey = "serverURL"
    private static let workspacesAccount = "workspaces"
    private static let legacyTokenAccount = "apiToken"

    init() {
        serverURL = UserDefaults.standard.string(forKey: Self.serverKey) ?? ""
        workspaces = Self.loadWorkspaces()
        do {
            let dir = try FileManager.default
                .url(for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: true)
                .appending(path: "PendingUploads", directoryHint: .isDirectory)
            uploads = try UploadQueue(directory: dir, transport: nil, keep: { DiskImages.originals.adopt($0, id: $1) }, onChange: { [weak self] status in
                Task { @MainActor in self?.uploadStatus = status }
            })
        } catch {
            startupError = String(localized: "Photo storage is unavailable: \(error.localizedDescription)")
        }
        rebuild()
        #if DEBUG
        bootstrapFromLaunchEnvironment()
        #endif
    }

    #if DEBUG
    /// Development only: `devicectl ... --environment-variables` or
    /// `simctl launch` with SIMCTL_CHILD_ can hand a fresh install its server
    /// and token, so nobody types a token on a phone. Ignored once the app
    /// has a workspace; compiled out of release builds.
    private func bootstrapFromLaunchEnvironment() {
        let env = ProcessInfo.processInfo.environment
        guard active == nil, let server = env["PIDEX_SERVER"], let token = env["PIDEX_TOKEN"], !token.isEmpty else { return }
        Task {
            do {
                try await connect(serverURL: server, token: token)
            } catch {
                startupError = "Launch settings did not connect: \(error.localizedDescription)"
            }
        }
    }
    #endif

    var isConfigured: Bool { api != nil }
    var active: Workspace? { workspaces.active }

    /// Queued photos belong to the active workspace and server, so neither
    /// may change until they are uploaded.
    var canSwitch: Bool { uploadStatus.unfinished == 0 }

    /// Checks a server and token, then saves them as the active workspace.
    func connect(serverURL: String, token: String) async throws {
        guard serverURL == self.serverURL || canSwitch else { throw SwitchBlocked(pending: uploadStatus.unfinished) }
        guard let url = URL(string: serverURL) else { throw URLError(.badURL) }
        let team = try await APIClient(baseURL: url, token: token).team()
        self.serverURL = serverURL
        UserDefaults.standard.set(serverURL, forKey: Self.serverKey)
        workspaces.upsert(Workspace(id: team.id, name: team.name, token: token), activate: canSwitch || active == nil)
        saveAndRebuild()
    }

    /// Saves a workspace created on this server, switching to it if no
    /// uploads are waiting. Returns whether it is now active.
    @discardableResult
    func add(team: Pidex_V1_Team, token: String) -> Bool {
        let switching = canSwitch
        workspaces.upsert(Workspace(id: team.id, name: team.name, token: token), activate: switching)
        saveAndRebuild()
        return switching
    }

    func switchTo(_ id: String) throws {
        guard id != active?.id else { return }
        guard canSwitch else { throw SwitchBlocked(pending: uploadStatus.unfinished) }
        workspaces.activate(id)
        saveAndRebuild()
    }

    /// Browsing any collection needs no switch: each has its own client.
    func api(for workspace: Workspace) -> APIClient? {
        guard let url = URL(string: serverURL), url.host() != nil else { return nil }
        if let c = clients[workspace.token], c.baseURL == url { return c }
        let c = APIClient(baseURL: url, token: workspace.token)
        clients[workspace.token] = c
        return c
    }

    /// Opens the camera for a collection. Shooting for another collection
    /// waits until queued photos are uploaded, since they belong to this one.
    /// firstShoot: the collection was just made; the camera says how to
    /// take its first photos.
    func openCamera(for id: String, firstShoot: Bool = false) throws {
        try switchTo(id)
        capture?.firstShoot = firstShoot
        showingCamera = true
    }

    /// Forgets a workspace on this device, with the copy of it kept here.
    /// The team and its photos stay on the server; adding its token again
    /// brings it back.
    func remove(_ id: String) throws {
        if id == active?.id && !canSwitch {
            throw SwitchBlocked(pending: uploadStatus.unfinished)
        }
        let wasActive = id == active?.id
        if let w = workspaces.items.first(where: { $0.id == id }) {
            LocalLibrary.erase(token: w.token)
        }
        workspaces.remove(id)
        if wasActive {
            saveAndRebuild()
        } else {
            save()
        }
    }

    /// Forgets a collection deleted on the server, and the shots kept for
    /// it on this phone.
    func forgetDeleted(_ workspace: Workspace) {
        if let dir = Self.stagingDirectory(for: workspace) {
            try? FileManager.default.removeItem(at: dir)
        }
        LocalLibrary.erase(token: workspace.token)
        if batch?.collection == workspace.name { batch = nil }
        let wasActive = workspace.id == active?.id
        workspaces.remove(workspace.id)
        if wasActive {
            saveAndRebuild()
        } else {
            save()
        }
    }

    /// Follows photos just sent: added to the batch of their session, or a
    /// new batch for a new session.
    func track(sent count: Int, session: UUID, api: APIClient) {
        if let b = batch, b.sessionID == session {
            b.add(count)
        } else {
            let b = UploadBatch(sessionID: session, api: api, collection: active?.name ?? "", expected: count)
            batch = b
            b.follow(pending: count)
        }
    }

    /// Closes the batch's details. Once all of it is filed, it was seen:
    /// it goes, with any photos still needing someone left to the
    /// collection's unfiled.
    func closeBatch() {
        guard let batch else { return showingBatch = false }
        let pending = uploadStatus.pendingBySession[batch.sessionID.uuidString.lowercased()] ?? 0
        if batch.phase(pending: pending) == .done {
            dismissBatch()
        } else {
            showingBatch = false
        }
    }

    /// Hides a finished batch.
    func dismissBatch() {
        batch = nil
        showingBatch = false
    }

    /// Called when the app comes to the foreground: uploads that were
    /// waiting for a connection get another chance.
    func becameActive() {
        guard let uploads else { return }
        Task { await uploads.resume() }
    }

    private func saveAndRebuild() {
        save()
        rebuild()
    }

    private func save() {
        if let data = try? JSONEncoder().encode(workspaces) {
            Keychain.write(String(decoding: data, as: UTF8.self), account: Self.workspacesAccount)
        }
    }

    private func rebuild() {
        // Only one capture model may hold the camera.
        capture?.stopCamera()
        guard let url = URL(string: serverURL), url.host() != nil, let workspace = active, let uploads else {
            api = nil
            capture = nil
            return
        }
        let api = APIClient(baseURL: url, token: workspace.token)
        self.api = api
        capture = CaptureModel(api: api, uploads: uploads, staging: Self.stagingDirectory(for: workspace))
        Task { await uploads.setTransport(LiveUploadTransport(api: api)) }
        // Keep the name current, and learn the team ID of a migrated token.
        // The token is unchanged, so the capture session carries on.
        Task { [weak self] in
            guard let team = try? await api.team() else { return }
            guard let self, team.id != workspace.id || team.name != workspace.name else { return }
            self.workspaces.update(token: workspace.token, id: team.id, name: team.name)
            self.save()
        }
    }

    /// Shots kept on the phone before sending, one directory per collection
    /// so switching back finds them again.
    private static func stagingDirectory(for workspace: Workspace) -> URL? {
        try? FileManager.default
            .url(for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: true)
            .appending(path: "StagedShots", directoryHint: .isDirectory)
            .appending(path: workspace.id.isEmpty ? "default" : workspace.id, directoryHint: .isDirectory)
    }

    private static func loadWorkspaces() -> WorkspaceList {
        if let json = Keychain.read(workspacesAccount), let list = try? JSONDecoder().decode(WorkspaceList.self, from: Data(json.utf8)) {
            return list
        }
        // Before workspaces, the app kept one token.
        var list = WorkspaceList()
        if let token = Keychain.read(legacyTokenAccount), !token.isEmpty {
            list.upsert(Workspace(id: "", name: String(localized: "Workspace"), token: token), activate: true)
            if let data = try? JSONEncoder().encode(list) {
                Keychain.write(String(decoding: data, as: UTF8.self), account: workspacesAccount)
                Keychain.write("", account: legacyTokenAccount)
            }
        }
        return list
    }
}

nonisolated struct SwitchBlocked: LocalizedError {
    let pending: Int
    var errorDescription: String? {
        String(localized: "\(pending) photos are still uploading to the collection the camera shoots for. Shoot for another one when they are done.")
    }
}
