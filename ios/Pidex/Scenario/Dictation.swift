import AVFoundation
import Observation
import Speech
import SwiftUI

/// Speech to text for the scenario description. Recognition runs on the
/// device when its language model is installed, so the voice stays on the
/// phone; otherwise Apple's speech service is used, as with the keyboard's
/// dictation.
@MainActor
@Observable
final class Dictation {
    enum State: Equatable {
        case idle
        case listening
        case unavailable(String)
    }

    private(set) var state: State = .idle
    private var engine: AVAudioEngine?
    private var request: SFSpeechAudioBufferRecognitionRequest?
    private var task: SFSpeechRecognitionTask?

    /// Starts listening. `update` gets the whole transcript so far each
    /// time it changes.
    func start(locale: Locale, update: @escaping @MainActor (String) -> Void) async {
        guard state != .listening else { return }
        guard await Self.authorize() else {
            state = .unavailable(String(localized: "Allow the microphone and speech recognition for Pidex in Settings to dictate."))
            return
        }
        guard let recognizer = SFSpeechRecognizer(locale: locale), recognizer.isAvailable else {
            state = .unavailable(String(localized: "Speech recognition is not available for this language right now."))
            return
        }
        let request = SFSpeechAudioBufferRecognitionRequest()
        request.shouldReportPartialResults = true
        request.addsPunctuation = true
        request.requiresOnDeviceRecognition = recognizer.supportsOnDeviceRecognition
        do {
            let session = AVAudioSession.sharedInstance()
            // Touching the engine's input without a microphone aborts the
            // app inside Core Audio, so ask the session first.
            guard session.isInputAvailable else { throw DictationError.noMicrophone }
            try session.setCategory(.record, mode: .measurement, options: .duckOthers)
            try session.setActive(true, options: .notifyOthersOnDeactivation)
            let engine = AVAudioEngine()
            try Self.feed(engine, into: request)
            engine.prepare()
            try engine.start()
            self.engine = engine
        } catch {
            state = .unavailable(String(localized: "The microphone could not start: \(error.localizedDescription)"))
            return
        }
        self.request = request
        task = Self.recognize(recognizer, request) { [weak self] text, done in
            if let text { update(text) }
            if done { self?.stop() }
        }
        state = .listening
    }

    func stop() {
        guard state == .listening else { return }
        engine?.stop()
        engine?.inputNode.removeTap(onBus: 0)
        request?.endAudio()
        task?.finish()
        engine = nil
        request = nil
        task = nil
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
        state = .idle
    }

    // The callbacks below run on audio and recognition threads, so they are
    // made outside the main actor; a main-actor closure would trap there.

    private nonisolated static func feed(_ engine: AVAudioEngine, into request: SFSpeechAudioBufferRecognitionRequest) throws {
        let input = engine.inputNode
        let format = input.outputFormat(forBus: 0)
        guard format.sampleRate > 0 else { throw DictationError.noMicrophone }
        nonisolated(unsafe) let request = request
        input.installTap(onBus: 0, bufferSize: 1024, format: format) { buffer, _ in
            request.append(buffer)
        }
    }

    private nonisolated static func recognize(
        _ recognizer: SFSpeechRecognizer, _ request: SFSpeechRecognitionRequest,
        handler: @escaping @MainActor (String?, Bool) -> Void
    ) -> SFSpeechRecognitionTask {
        recognizer.recognitionTask(with: request) { result, error in
            let text = result?.bestTranscription.formattedString
            let done = error != nil || (result?.isFinal ?? false)
            Task { @MainActor in handler(text, done) }
        }
    }

    private nonisolated static func authorize() async -> Bool {
        let speech = await withCheckedContinuation { c in
            SFSpeechRecognizer.requestAuthorization { c.resume(returning: $0) }
        }
        guard speech == .authorized else { return false }
        return await AVAudioApplication.requestRecordPermission()
    }

    /// The recognizer locale for an app or dictation language: the
    /// matching script first (zh-Hant is not zh-CN), then the user's own
    /// region ("en" in Britain is en-GB), then the language's home region
    /// ("en" is en-US, "zh-Hant" zh-TW).
    nonisolated static func speechLocale(for language: String, region: String? = Locale.current.region?.identifier,
                                         supported: Set<Locale> = SFSpeechRecognizer.supportedLocales()) -> Locale {
        if let exact = supported.first(where: { $0.identifier == language }) {
            return exact
        }
        let wanted = Locale.Language(identifier: language)
        var candidates = supported
            .filter { $0.language.languageCode == wanted.languageCode }
            .sorted { $0.identifier < $1.identifier }
        if let script = wanted.script {
            let sameScript = candidates.filter { Locale.Language(identifier: $0.identifier).maximalIdentifier.contains("-\(script.identifier)") }
            if !sameScript.isEmpty { candidates = sameScript }
        }
        let home = Locale.Language(identifier: wanted.maximalIdentifier).region?.identifier
        for r in [region, home].compactMap({ $0 }) {
            if let m = candidates.first(where: { $0.region?.identifier == r }) { return m }
        }
        return candidates.first ?? Locale(identifier: "en-US")
    }
}

private enum DictationError: LocalizedError {
    case noMicrophone
    var errorDescription: String? { String(localized: "No microphone is available.") }
}

/// The language dictation listens for, remembered on this device: the
/// languages this iPhone is set up for, then every language speech
/// recognition supports. Shown on a long press of the microphone.
struct DictationLanguagePicker: View {
    /// A speech locale identifier; empty means the app's own language.
    @AppStorage("dictationLanguage") private var language = ""

    struct Choice: Identifiable, Hashable {
        let id: String
        let name: String
    }

    /// The user's own languages that speech recognition supports, then all
    /// supported languages, each named in its own language.
    nonisolated static func choices(preferred: [String] = Locale.preferredLanguages,
                        supported: Set<Locale> = SFSpeechRecognizer.supportedLocales()) -> (mine: [Choice], all: [Choice]) {
        func name(_ id: String) -> String {
            Locale(identifier: id).localizedString(forIdentifier: id) ?? id
        }
        var mine: [Choice] = []
        for p in preferred {
            let l = Dictation.speechLocale(for: p, region: Locale(identifier: p).region?.identifier, supported: supported)
            guard supported.contains(l), !mine.contains(where: { $0.id == l.identifier }) else { continue }
            mine.append(Choice(id: l.identifier, name: name(l.identifier)))
        }
        let all = supported.map { Choice(id: $0.identifier, name: name($0.identifier)) }
            .sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
        return (mine, all)
    }

    var body: some View {
        let choices = Self.choices()
        Picker("Dictation language", selection: $language) {
            Text("Same as the app").tag("")
            ForEach(choices.mine) { c in
                Text(verbatim: c.name).tag(c.id)
            }
        }
        Menu("More languages") {
            Picker("Dictation language", selection: $language) {
                ForEach(choices.all) { c in
                    Text(verbatim: c.name).tag(c.id)
                }
            }
        }
    }
}
