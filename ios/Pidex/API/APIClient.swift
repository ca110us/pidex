import Connect
import Foundation
import SwiftProtobuf

/// A failed call, with the Connect code so callers can tell "retry later"
/// from "fix your settings".
nonisolated struct APIError: Error, LocalizedError, Sendable {
    let code: Code
    let message: String

    var errorDescription: String? { message }
    var isAuthProblem: Bool { code == .unauthenticated || code == .permissionDenied }
}

/// Typed access to the Pidex API. Calls throw `APIError` instead of
/// returning Connect result wrappers.
nonisolated final class APIClient: Sendable {
    let baseURL: URL
    /// Names what this phone keeps of the collection (LocalLibrary).
    let storeKey: String
    private let capture: Pidex_V1_CaptureServiceClient
    private let library: Pidex_V1_LibraryServiceClient
    private let config: Pidex_V1_ConfigServiceClient
    private let teams: Pidex_V1_TeamServiceClient
    private let search: Pidex_V1_SearchServiceClient
    private let chat: Pidex_V1_ChatServiceClient
    private let headers: Headers

    init(baseURL: URL, token: String) {
        self.baseURL = baseURL
        storeKey = Self.storeKey(for: token)
        var host = baseURL.absoluteString
        while host.hasSuffix("/") { host.removeLast() }
        func client(timeout: TimeInterval) -> ProtocolClient {
            ProtocolClient(
                httpClient: URLSessionHTTPClient(),
                config: ProtocolClientConfig(host: host, networkProtocol: .connect, codec: ProtoCodec(), timeout: timeout)
            )
        }
        let quick = client(timeout: 30)
        capture = Pidex_V1_CaptureServiceClient(client: quick)
        library = Pidex_V1_LibraryServiceClient(client: quick)
        teams = Pidex_V1_TeamServiceClient(client: quick)
        search = Pidex_V1_SearchServiceClient(client: quick)
        // A reply may run a search and an answer: give it the long timeout.
        chat = Pidex_V1_ChatServiceClient(client: client(timeout: 90))
        // Drafting a config runs a strong model for up to a minute.
        config = Pidex_V1_ConfigServiceClient(client: client(timeout: 90))
        headers = ["Authorization": ["Bearer \(token)"]]
    }

    func createUpload(_ r: Pidex_V1_CreateUploadRequest) async throws -> Pidex_V1_CreateUploadResponse {
        try Self.unwrap(await capture.createUpload(request: r, headers: headers))
    }

    func completeUpload(_ r: Pidex_V1_CompleteUploadRequest) async throws -> Pidex_V1_CompleteUploadResponse {
        try Self.unwrap(await capture.completeUpload(request: r, headers: headers))
    }

    func createFileUpload(_ r: Pidex_V1_CreateFileUploadRequest) async throws -> Pidex_V1_CreateFileUploadResponse {
        try Self.unwrap(await capture.createFileUpload(request: r, headers: headers))
    }

    func completeFileUpload(_ r: Pidex_V1_CompleteFileUploadRequest) async throws {
        _ = try Self.unwrap(await capture.completeFileUpload(request: r, headers: headers))
    }

    func resolveAnchor(_ onDevice: Pidex_V1_OnDeviceAnalysis) async throws -> Pidex_V1_ResolveAnchorResponse {
        var r = Pidex_V1_ResolveAnchorRequest()
        r.onDevice = onDevice
        return try Self.unwrap(await capture.resolveAnchor(request: r, headers: headers))
    }

    /// Has the server's vision model read a shot still on the phone.
    /// `clientPhotoID` is the id the shot will be uploaded with: the answer
    /// a person gives on the camera checks what the server said of it.
    func identifyShot(jpeg: Data, onDevice: Pidex_V1_OnDeviceAnalysis, clientPhotoID: String) async throws -> Pidex_V1_ResolveAnchorResponse {
        var r = Pidex_V1_IdentifyShotRequest()
        r.jpeg = jpeg
        r.onDevice = onDevice
        r.clientPhotoID = clientPhotoID
        return try Self.unwrap(await capture.identifyShot(request: r, headers: headers)).anchor
    }

    func sessionSummary(_ sessionID: UUID) async throws -> Pidex_V1_GetSessionSummaryResponse {
        var r = Pidex_V1_GetSessionSummaryRequest()
        r.sessionID = sessionID.uuidString.lowercased()
        return try Self.unwrap(await capture.getSessionSummary(request: r, headers: headers))
    }

    func assign(photoIDs: [String], to subjectID: String) async throws -> [Pidex_V1_Photo] {
        var r = Pidex_V1_AssignPhotosRequest()
        r.photoIds = photoIDs
        r.subjectID = subjectID
        return try Self.unwrap(await library.assignPhotos(request: r, headers: headers)).photos
    }

    /// Plain-language search; the response says how it was understood.
    func searchPhotos(query: String) async throws -> Pidex_V1_SearchPhotosResponse {
        var r = Pidex_V1_SearchPhotosRequest()
        r.query = query
        r.timeZone = TimeZone.current.identifier
        return try Self.unwrap(await search.searchPhotos(request: r, headers: headers))
    }

    /// Search with filters the user edited: no model call. `offset` pages
    /// through more matches than one response holds.
    func searchPhotos(filters: Pidex_V1_SearchFilters, offset: Int = 0, pageSize: Int = 0,
                      tagCounts: Bool = false) async throws -> Pidex_V1_SearchPhotosResponse {
        var r = Pidex_V1_SearchPhotosRequest()
        r.filters = filters
        r.offset = Int32(offset)
        r.pageSize = Int32(pageSize)
        r.tagCounts = tagCounts
        r.timeZone = TimeZone.current.identifier
        return try Self.unwrap(await search.searchPhotos(request: r, headers: headers))
    }

    func askPhotos(question: String) async throws -> Pidex_V1_AskPhotosResponse {
        var r = Pidex_V1_AskPhotosRequest()
        r.question = question
        r.timeZone = TimeZone.current.identifier
        return try Self.unwrap(await search.askPhotos(request: r, headers: headers))
    }

    func updateTags(photoID: String, add: [String] = [], remove: [String] = []) async throws -> Pidex_V1_Photo {
        var r = Pidex_V1_UpdatePhotoTagsRequest()
        r.photoID = photoID
        r.add = add
        r.remove = remove
        return try Self.unwrap(await library.updatePhotoTags(request: r, headers: headers)).photo
    }

    func conversations() async throws -> [Pidex_V1_Conversation] {
        try Self.unwrap(await chat.listConversations(request: .init(), headers: headers)).conversations
    }

    func conversation(_ id: String) async throws -> Pidex_V1_GetConversationResponse {
        var r = Pidex_V1_GetConversationRequest()
        r.id = id
        return try Self.unwrap(await chat.getConversation(request: r, headers: headers))
    }

    /// Sends a message; nil starts a new conversation.
    func send(_ text: String, in conversationID: String?) async throws -> Pidex_V1_SendMessageResponse {
        var r = Pidex_V1_SendMessageRequest()
        r.conversationID = conversationID ?? ""
        r.text = text
        r.timeZone = TimeZone.current.identifier
        return try Self.unwrap(await chat.sendMessage(request: r, headers: headers))
    }

    func deleteConversation(_ id: String) async throws {
        var r = Pidex_V1_DeleteConversationRequest()
        r.id = id
        _ = try Self.unwrap(await chat.deleteConversation(request: r, headers: headers))
    }

    /// What a collection card shows: counts, newest photos, common tags.
    func overview(recent: Int32 = 4) async throws -> Pidex_V1_GetOverviewResponse {
        var r = Pidex_V1_GetOverviewRequest()
        r.recentCount = recent
        return try Self.unwrap(await teams.getOverview(request: r, headers: headers))
    }

    /// Deletes photos for good. Returns how many photos, and how many
    /// subjects made from them alone, went.
    func deletePhotos(_ ids: [String]) async throws -> (photos: Int, subjects: Int) {
        var r = Pidex_V1_DeletePhotosRequest()
        r.photoIds = ids
        let out = try Self.unwrap(await library.deletePhotos(request: r, headers: headers))
        return (Int(out.deletedPhotos), Int(out.deletedSubjects))
    }

    /// Deletes a subject, and its photos too when asked; otherwise they are
    /// left unfiled.
    func deleteSubject(_ id: String, withPhotos: Bool) async throws -> (deleted: Int, unfiled: Int) {
        var r = Pidex_V1_DeleteSubjectRequest()
        r.subjectID = id
        r.withPhotos = withPhotos
        let out = try Self.unwrap(await library.deleteSubject(request: r, headers: headers))
        return (Int(out.deletedPhotos), Int(out.unfiledPhotos))
    }

    /// A subject known by its look, which has no identity to read.
    func createSubject(typeKey: String, name: String) async throws -> Pidex_V1_Subject {
        var r = Pidex_V1_CreateSubjectRequest()
        r.typeKey = typeKey
        r.displayName = name
        return try Self.unwrap(await library.createSubject(request: r, headers: headers)).subject
    }

    func renameSubject(_ id: String, to name: String) async throws -> Pidex_V1_Subject {
        var r = Pidex_V1_RenameSubjectRequest()
        r.subjectID = id
        r.displayName = name
        return try Self.unwrap(await library.renameSubject(request: r, headers: headers)).subject
    }

    /// What a person says a subject looks like; empty hands it back to be
    /// written from its photos.
    func setSubjectProfile(_ id: String, to profile: String) async throws -> Pidex_V1_Subject {
        var r = Pidex_V1_SetSubjectProfileRequest()
        r.subjectID = id
        r.profile = profile
        return try Self.unwrap(await library.setSubjectProfile(request: r, headers: headers)).subject
    }

    /// Joins subjects that are one; their photos move to `into`.
    func mergeSubjects(_ ids: [String], into: String) async throws -> Pidex_V1_Subject {
        var r = Pidex_V1_MergeSubjectsRequest()
        r.subjectIds = ids
        r.intoID = into
        return try Self.unwrap(await library.mergeSubjects(request: r, headers: headers)).subject
    }

    /// What changed in the collection since the cursor (empty: everything).
    func syncLibrary(cursor: String) async throws -> Pidex_V1_SyncLibraryResponse {
        var r = Pidex_V1_SyncLibraryRequest()
        r.cursor = cursor
        return try Self.unwrap(await library.syncLibrary(request: r, headers: headers))
    }

    /// Fresh links to photos' images, by photo id; at most 200 at a time.
    func photoLinks(_ ids: [String]) async throws -> [String: Pidex_V1_PhotoLinks] {
        var r = Pidex_V1_GetPhotoLinksRequest()
        r.ids = ids
        return try Self.unwrap(await library.getPhotoLinks(request: r, headers: headers)).links
    }

    func subjects(limit: Int32 = 200) async throws -> [Pidex_V1_Subject] {
        var r = Pidex_V1_ListSubjectsRequest()
        r.pageSize = limit
        return try Self.unwrap(await library.listSubjects(request: r, headers: headers)).subjects
    }

    /// Deletes the team and everything in it, for good. Returns how many
    /// photos went with it.
    func deleteTeam(confirmName: String) async throws -> Int {
        var r = Pidex_V1_DeleteTeamRequest()
        r.confirmName = confirmName
        return Int(try Self.unwrap(await teams.deleteTeam(request: r, headers: headers)).deletedPhotos)
    }

    /// The team this client's token belongs to; also checks the settings.
    func team() async throws -> Pidex_V1_Team {
        try Self.unwrap(await teams.getTeam(request: .init(), headers: headers)).team
    }

    /// Creates a team owned by the caller and returns it with its token.
    func createTeam(name: String, configJSON: String) async throws -> (Pidex_V1_Team, token: String) {
        var r = Pidex_V1_CreateTeamRequest()
        r.name = name
        r.configJson = configJSON
        let out = try Self.unwrap(await teams.createTeam(request: r, headers: headers))
        return (out.team, out.token)
    }

    func activeConfig() async throws -> Pidex_V1_GetActiveConfigResponse {
        try Self.unwrap(await config.getActiveConfig(request: .init(), headers: headers))
    }

    /// Asks the server's model for a config. `base` is the config to change;
    /// nil means the team's active config, or a new one when `forNewTeam`.
    /// `identifyBy` is "code", "appearance", or empty to let the description
    /// decide. `sessionID` names the conversation the draft is part of.
    func draftConfig(description: String, base: String?, forNewTeam: Bool, identifyBy: String = "", sessionID: String = "") async throws -> Pidex_V1_DraftConfigResponse {
        var r = Pidex_V1_DraftConfigRequest()
        r.description_p = description
        r.baseConfigJson = base ?? ""
        r.forNewTeam = forNewTeam
        r.identifyBy = identifyBy
        r.sessionID = sessionID
        return try Self.unwrap(await config.draftConfig(request: r, headers: headers))
    }

    /// The conversation the collection was set up in, oldest first.
    func setupConversation() async throws -> [Pidex_V1_SetupMessage] {
        try Self.unwrap(await config.getSetupConversation(request: .init(), headers: headers)).messages
    }

    func addSetupMessages(_ messages: [Pidex_V1_SetupMessage]) async throws -> [Pidex_V1_SetupMessage] {
        var r = Pidex_V1_AddSetupMessagesRequest()
        r.messages = messages
        return try Self.unwrap(await config.addSetupMessages(request: r, headers: headers)).messages
    }

    func publishConfig(json: String) async throws -> Int32 {
        var r = Pidex_V1_PublishConfigRequest()
        r.configJson = json
        return try Self.unwrap(await config.publishConfig(request: r, headers: headers)).version
    }

    func restoreConfig(version: Int32) async throws -> Int32 {
        var r = Pidex_V1_PublishConfigRequest()
        r.restoreVersion = version
        return try Self.unwrap(await config.publishConfig(request: r, headers: headers)).version
    }

    func configVersions() async throws -> [Pidex_V1_ConfigVersion] {
        try Self.unwrap(await config.listConfigVersions(request: .init(), headers: headers)).versions
    }

    private static func unwrap<T: ProtobufMessage>(_ response: ResponseMessage<T>) throws -> T {
        switch response.result {
        case .success(let message):
            return message
        case .failure(let error):
            throw APIError(code: error.code, message: error.message ?? String(describing: error.code))
        }
    }
}
