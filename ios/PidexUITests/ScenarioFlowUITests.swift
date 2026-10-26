import XCTest

/// Taps through the app the way a person would, against a running server,
/// and keeps a screenshot of every step. Needs PIDEX_SERVER and
/// PIDEX_TOKEN (the launch hook connects a fresh install); tests that call
/// paid models also need PIDEX_LIVE_DRAFT=1.
@MainActor
final class ScenarioFlowUITests: XCTestCase {
    private let env = ProcessInfo.processInfo.environment

    override func setUp() async throws {
        continueAfterFailure = false
    }

    /// Home → new collection from a description → its card → its camera.
    func testCreateScenarioFromDescription() throws {
        guard env["PIDEX_TOKEN"] != nil, env["PIDEX_LIVE_DRAFT"] == "1" else {
            throw XCTSkip("needs PIDEX_SERVER, PIDEX_TOKEN and PIDEX_LIVE_DRAFT=1")
        }
        let app = launch()
        XCTAssertTrue(app.navigationBars["Collections"].waitForExistence(timeout: 15), "the app did not connect")
        snap(app, "1 collections")

        // From the menu: the card at the end of the grid may be off screen.
        app.buttons["homeMenu"].tap()
        app.buttons["New collection"].tap()
        // A question and examples to start from, no choices: how they are
        // told apart is worked out from the description.
        XCTAssertTrue(app.buttons["example-pets"].waitForExistence(timeout: 5))
        XCTAssertFalse(app.buttons["identifyBy-code"].exists, "no choice to make before describing")
        snap(app, "1b question and examples")
        let request = app.textFields["chatInput"]
        request.tap()
        request.typeText("We run a small bike repair shop. Each bike gets a paper repair ticket with a number like R-1042 tied to the handlebar. We photograph damage on arrival, the parts we replace and the finished bike.")
        snap(app, "2 description")
        app.buttons["chatSend"].tap()

        let create = app.buttons["commitSetup"]
        XCTAssertTrue(create.waitForExistence(timeout: 90), "no draft within 90 s")
        sleep(1)
        snap(app, "3 draft")
        for _ in 0..<12 where !create.isHittable {
            app.swipeUp()
        }
        create.tap()

        // Straight to the camera, saying how to take the first photos.
        assertCameraStarted(app)
        XCTAssertTrue(app.buttons["whoBanner"].label.contains("First photos"), app.buttons["whoBanner"].label)
        snap(app, "4 camera for the new collection")

        // A photo sent, then the batch's summary.
        importLibraryPhoto(app)
        XCTAssertTrue(app.descendants(matching: .any)["shot-0"].waitForExistence(timeout: 10))
        // Wait for the label to be read, if it has one.
        for _ in 0..<20 where app.buttons["whoBanner"].label.hasPrefix("Reading") { sleep(1) }
        if app.buttons["sendShots"].isEnabled {
            app.buttons["sendShots"].tap()
            XCTAssertTrue(app.buttons["Done"].waitForExistence(timeout: 10), "the batch's summary")
            app.buttons["Done"].tap()
        } else {
            // Nothing goes out that does not say who it shows.
            XCTAssertTrue(app.staticTexts.containing(NSPredicate(format: "label CONTAINS 'not identified'")).firstMatch.exists)
            snap(app, "4a not identified")
            app.buttons["closeCamera"].tap()
            app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'Discard'")).firstMatch.tap()
        }

        // Home, with a card for the new collection: earlier runs left
        // collections of the same name; the new one is the one the camera
        // now shoots for.
        let card = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'collection-' AND value CONTAINS 'camera shoots'")).firstMatch
        for _ in 0..<15 where !card.exists { app.swipeUp() }
        XCTAssertTrue(card.waitForExistence(timeout: 10))
        snap(app, "5 home with the new collection")
        card.tap()
        XCTAssertTrue(app.buttons["collectionCamera"].waitForExistence(timeout: 10))
        // Unrelated to bike repair, the photo is filed as not relevant
        // and hidden; deleting photos is checked in the other test.

        app.buttons["collectionMenu"].tap()
        app.buttons["Scenario"].tap()
        XCTAssertTrue(app.navigationBars["Scenario"].waitForExistence(timeout: 10))
        sleep(2)
        snap(app, "7 scenario details")
        app.navigationBars.buttons.element(boundBy: 0).tap()

        // The conversation it was set up in is still there, to carry on.
        app.buttons["collectionMenu"].tap()
        app.buttons["setupConversation"].tap()
        let earlier = app.staticTexts.containing(NSPredicate(format: "label CONTAINS 'bike repair shop'")).firstMatch
        XCTAssertTrue(earlier.waitForExistence(timeout: 10), "the setup conversation is kept with the collection")
        XCTAssertTrue(app.staticTexts.containing(NSPredicate(format: "label BEGINSWITH 'Created'")).firstMatch.exists)
        XCTAssertFalse(app.buttons["commitSetup"].exists, "the draft it was created from is already applied")
        snap(app, "8 setup conversation")
        app.navigationBars.buttons.element(boundBy: 0).tap()

        // Deleting it, for good, takes its name; this also keeps test runs
        // from piling up collections.
        app.buttons["collectionMenu"].tap()
        app.buttons["deleteCollection"].tap()
        let name = app.textFields["confirmDeleteName"]
        XCTAssertTrue(name.waitForExistence(timeout: 5))
        XCTAssertTrue(app.staticTexts["This cannot be undone"].exists, "the warning comes first")
        XCTAssertFalse(app.buttons["confirmDelete"].isEnabled, "not before the name is typed")
        snap(app, "9 delete warning")
        name.tap()
        name.typeText(name.placeholderValue ?? "")
        XCTAssertTrue(app.buttons["confirmDelete"].isEnabled)
        app.buttons["confirmDelete"].tap()
        XCTAssertTrue(app.navigationBars["Collections"].waitForExistence(timeout: 15), "back home")
        let deleted = app.buttons.matching(NSPredicate(format: "identifier == 'toast' AND label CONTAINS 'Deleted'")).firstMatch
        XCTAssertTrue(deleted.waitForExistence(timeout: 5))
        XCTAssertTrue(deleted.waitForNonExistence(timeout: 8), "the toast goes by itself")
        snap(app, "10 deleted")
    }

    /// The app in Simplified Chinese: screens are translated and the
    /// dictation controls are there. No model call. Dictation itself needs a
    /// real microphone: the simulator's audio input aborts when the Mac does
    /// not grant it, so it is tested on a device.
    func testChineseInterfaceAndDictation() throws {
        guard env["PIDEX_TOKEN"] != nil else { throw XCTSkip("needs PIDEX_SERVER and PIDEX_TOKEN") }
        let app = launch(["-AppleLanguages", "(zh-Hans)", "-AppleLocale", "zh_CN"])

        XCTAssertTrue(app.navigationBars["合集"].waitForExistence(timeout: 15))
        sleep(2)
        snap(app, "zh 1 collections")

        app.buttons["homeMenu"].tap()
        app.buttons["新建合集"].tap()
        XCTAssertTrue(app.textFields["chatInput"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.textFields["描述你要拍什么"].exists, "the prompt is in Chinese")
        XCTAssertTrue(app.buttons["语音输入"].exists, "one can speak instead")
        snap(app, "zh 2 new collection")
        app.navigationBars.buttons.element(boundBy: 0).tap()

        let card = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'collection-'")).firstMatch
        card.tap()
        XCTAssertTrue(app.segmentedControls.buttons["照片"].waitForExistence(timeout: 10))
        sleep(2)
        snap(app, "zh 3 collection")
    }

    /// Adds a collection that holds the eval photos, then browses, searches
    /// and asks inside it. Needs PIDEX_SEARCH_TOKEN (a token of that team)
    /// and PIDEX_LIVE_DRAFT=1: a few cheap model calls.
    func testFindAndAskPhotos() throws {
        guard let token = env["PIDEX_SEARCH_TOKEN"], env["PIDEX_TOKEN"] != nil, env["PIDEX_LIVE_DRAFT"] == "1" else {
            throw XCTSkip("needs PIDEX_TOKEN, PIDEX_SEARCH_TOKEN and PIDEX_LIVE_DRAFT=1")
        }
        let app = launch()
        XCTAssertTrue(app.navigationBars["Collections"].waitForExistence(timeout: 15), "the app did not connect")

        let card = app.buttons.containing(NSPredicate(format: "identifier BEGINSWITH 'collection-Search eval construction'")).firstMatch
        if !card.waitForExistence(timeout: 3) {
            app.buttons["homeMenu"].tap()
            app.buttons["Settings"].tap()
            // Wait for the sheet to settle, or the tap is lost.
            XCTAssertTrue(app.buttons["Server and tokens"].waitForExistence(timeout: 5))
            sleep(1)
            app.buttons["Server and tokens"].tap()
            let field = app.secureTextFields["API token (cbk_…)"]
            XCTAssertTrue(field.waitForExistence(timeout: 5))
            field.tap()
            field.typeText(token)
            app.buttons["Add collection"].tap()
            XCTAssertTrue(app.navigationBars["Settings"].waitForExistence(timeout: 10))
            app.navigationBars["Settings"].buttons["Done"].tap()
        }
        XCTAssertTrue(card.waitForExistence(timeout: 10))
        sleep(3)
        snap(app, "lib 0 home")

        card.tap()
        XCTAssertTrue(app.buttons["collectionCamera"].waitForExistence(timeout: 10))
        sleep(2)
        snap(app, "lib 1 collection")
        // A photo and a subject open from the collection.
        app.buttons.matching(identifier: "photo-0").firstMatch.tap()
        XCTAssertTrue(app.navigationBars["Photo"].waitForExistence(timeout: 5), "a photo opens from its collection")
        app.navigationBars["Photo"].buttons.element(boundBy: 0).tap()

        // Tags combine: each picked one narrows further, and the way back is
        // in the same row.
        let tags = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'tag-'"))
        XCTAssertTrue(tags.firstMatch.waitForExistence(timeout: 10))
        let first = tags.element(boundBy: 0).identifier
        app.buttons[first].tap()
        XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS ' of '")).firstMatch.waitForExistence(timeout: 10))
        XCTAssertTrue(app.buttons[first].isSelected)
        let order = tags.allElementsBoundByIndex.map(\.identifier)
        if let second = tags.allElementsBoundByIndex.first(where: { $0.isEnabled && !$0.isSelected })?.identifier {
            app.buttons[second].tap()
            XCTAssertTrue(app.buttons[second].waitForSelected(), "two tags picked")
            XCTAssertTrue(app.buttons[first].isSelected)
            // Picking keeps every tag where it was.
            XCTAssertEqual(tags.allElementsBoundByIndex.map(\.identifier), order)
            sleep(2)
            snap(app, "lib 1a two tags")
        }
        app.buttons["Clear tags"].tap()
        XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS 'subjects'")).firstMatch.waitForExistence(timeout: 10),
                      "back to the whole collection")

        // Deleting starts by selecting; the warning comes before anything
        // goes (cancelled here: these photos are the test's own).
        app.buttons["collectionMenu"].tap()
        app.buttons["selectPhotos"].tap()
        // Each day's grid counts from photo-0; the first day's will do.
        let marked = app.buttons.matching(identifier: "photo-0").firstMatch
        XCTAssertTrue(marked.waitForExistence(timeout: 5))
        marked.tap()
        XCTAssertTrue(marked.isSelected)
        app.buttons["deleteSelected"].tap()
        XCTAssertTrue(app.buttons["Delete for good"].waitForExistence(timeout: 5))
        snap(app, "lib 1a2 delete warning")
        app.buttons["Cancel"].firstMatch.tap()
        app.buttons["Cancel"].firstMatch.tap()
        XCTAssertFalse(app.buttons["deleteSelected"].waitForExistence(timeout: 2))

        // A subject's deletion asks what becomes of its photos (cancelled:
        // these are the test's own).
        app.segmentedControls.buttons["Subjects"].tap()
        let row = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'subjectRow-'")).firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 10))
        row.tap()
        XCTAssertTrue(app.buttons["subjectMenu"].waitForExistence(timeout: 5), "a subject opens from its collection")
        app.buttons["subjectMenu"].tap()
        app.buttons["deleteSubject"].tap()
        XCTAssertTrue(app.buttons["deleteSubjectKeepPhotos"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.buttons["deleteSubjectWithPhotos"].exists)
        snap(app, "lib 1a3 delete subject")
        app.buttons["Cancel"].firstMatch.tap()
        app.navigationBars.buttons.element(boundBy: 0).tap()
        app.segmentedControls.buttons["Photos"].tap()

        // Searching the collection: a subject's name filters to its photos.
        let search = app.searchFields["Search this collection"]
        if !search.exists { app.swipeDown() }
        XCTAssertTrue(search.waitForExistence(timeout: 5))
        search.tap()
        search.typeText("203\n")
        XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS ' of '")).firstMatch.waitForExistence(timeout: 10),
                      "the photos of 203, out of all")
        sleep(1)
        snap(app, "lib 1b0 search")
        app.buttons["Cancel"].tap()
        XCTAssertTrue(app.staticTexts.matching(NSPredicate(format: "label CONTAINS 'subjects'")).firstMatch.waitForExistence(timeout: 10))

        // Shooting for this collection after shooting for another one: the
        // camera must start again, not spin.
        app.buttons["collectionCamera"].tap()
        assertCameraStarted(app)

        // Shots stay on the phone in a row until sent; any can be deleted,
        // and the banner says who they show, or asks.
        // Shots kept from an earlier run come back; start from none.
        if app.buttons["deleteShot-0"].exists {
            app.buttons["closeCamera"].tap()
            app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'Discard'")).firstMatch.tap()
            XCTAssertTrue(app.buttons["collectionCamera"].waitForExistence(timeout: 5))
            sleep(1)
            app.buttons["collectionCamera"].tap()
            assertCameraStarted(app)
        }
        for _ in 0..<2 { importLibraryPhoto(app) }
        XCTAssertTrue(app.descendants(matching: .any)["shot-1"].waitForExistence(timeout: 10), "both shots are in the tray")
        // The library photo may carry a number the phone reads as an ID.
        let said = app.buttons["whoBanner"].label
        XCTAssertTrue(["No ID photo yet", "New:", "Current:", "ID seen"].contains { said.contains($0) }, said)
        snap(app, "lib 1c tray")
        app.buttons["deleteShot-1"].tap()
        XCTAssertFalse(app.descendants(matching: .any)["shot-1"].waitForExistence(timeout: 2))
        app.buttons["whoBanner"].tap()
        XCTAssertTrue(app.navigationBars["Who is this?"].waitForExistence(timeout: 5))
        let someone = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'subject-' AND label CONTAINS '203'")).firstMatch
        XCTAssertTrue(someone.waitForExistence(timeout: 10), "existing subjects to choose from")
        snap(app, "lib 1c2 who")
        someone.tap()
        XCTAssertTrue(app.buttons["whoBanner"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.buttons["whoBanner"].label.hasPrefix("For:"), app.buttons["whoBanner"].label)
        sleep(1)
        snap(app, "lib 1d chosen")
        // Closing with shots not sent asks what to do with them.
        app.buttons["closeCamera"].tap()
        app.buttons["Discard 1 photo"].tap()
        XCTAssertTrue(app.buttons["collectionCamera"].waitForExistence(timeout: 5))
        sleep(1)
        app.buttons["collectionCamera"].tap()
        assertCameraStarted(app)

        // The camera's collection is picked on the camera, and the camera
        // starts again for it.
        let chip = app.buttons["workspaceChip"]
        XCTAssertTrue(chip.label.contains("Search eval construction"), chip.label)
        chip.tap()
        app.buttons["Demo Ortho"].tap()
        XCTAssertTrue(app.buttons["workspaceChip"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.buttons["workspaceChip"].label.contains("Demo Ortho"), app.buttons["workspaceChip"].label)
        assertCameraStarted(app)
        snap(app, "lib 1b camera switched")
        app.buttons["closeCamera"].tap()

        // A conversation, started from the collection's menu: a search, a
        // photo, a follow-up question.
        app.buttons["collectionMenu"].tap()
        app.buttons["askQuestion"].tap()
        let input = app.textFields["chatInput"]
        XCTAssertTrue(input.waitForExistence(timeout: 10))
        snap(app, "lib 2a new conversation")
        input.tap()
        input.typeText("wires inside the wall")
        app.buttons["chatSend"].tap()
        XCTAssertTrue(app.buttons["photo-0"].waitForExistence(timeout: 30), "the reply shows photos")
        sleep(2)
        snap(app, "lib 2 conversation photos")

        app.buttons.matching(identifier: "photo-0").firstMatch.tap()
        // The photo alone; its details only when asked for.
        XCTAssertTrue(app.navigationBars["Photo"].waitForExistence(timeout: 10))
        XCTAssertFalse(app.navigationBars["Details"].exists, "the viewer shows only the photo")
        sleep(2)
        snap(app, "lib 3 photo")
        app.buttons["photoMenu"].tap()
        app.buttons["Details and tags"].tap()
        XCTAssertTrue(app.navigationBars["Details"].waitForExistence(timeout: 5), "details from the menu")
        snap(app, "lib 3b details")
        app.buttons["Done"].tap()
        XCTAssertTrue(app.navigationBars["Details"].waitForNonExistence(timeout: 5))
        app.descendants(matching: .any)["photoViewer"].firstMatch.swipeUp()
        XCTAssertTrue(app.navigationBars["Details"].waitForExistence(timeout: 5), "details on swiping up")
        app.buttons["Done"].tap()
        XCTAssertTrue(app.navigationBars["Details"].waitForNonExistence(timeout: 5))
        app.navigationBars["Photo"].buttons.element(boundBy: 0).tap()

        let followUp = app.textFields["chatInput"]
        followUp.tap()
        followUp.typeText("What do these photos show?")
        app.buttons["chatSend"].tap()
        XCTAssertTrue(app.descendants(matching: .any)["answer"].waitForExistence(timeout: 40), "an answer about the photos shown")
        sleep(2)
        snap(app, "lib 4 answer")

        // The conversation is kept.
        app.navigationBars.buttons.element(boundBy: 0).tap()
        app.buttons["collectionMenu"].tap()
        app.buttons["conversations"].tap()
        XCTAssertTrue(app.staticTexts["wires inside the wall"].waitForExistence(timeout: 10), "the conversation is in the history")
        snap(app, "lib 5 history")

        // Every collection at once, from home: the replies say which
        // collection they come from.
        app.navigationBars.buttons.element(boundBy: 0).tap()
        app.navigationBars.buttons.element(boundBy: 0).tap()
        app.buttons["askButton"].tap()
        XCTAssertTrue(app.buttons["askScope"].waitForExistence(timeout: 10))
        XCTAssertTrue(app.buttons["askScope"].label.contains("All collections"), app.buttons["askScope"].label)
        let all = app.textFields["chatInput"]
        all.tap()
        all.typeText("wires inside the wall")
        app.buttons["chatSend"].tap()
        XCTAssertTrue(app.buttons["photo-0"].waitForExistence(timeout: 40), "the reply shows photos")
        let named = NSPredicate(format: "label BEGINSWITH 'Search eval construction'")
        XCTAssertTrue(app.buttons.matching(named).firstMatch.exists, "the reply names its collection")
        sleep(2)
        snap(app, "lib 6 all collections")

        // Many photos: a few in the reply, all of them in an album.
        all.tap()
        all.typeText("show me every photo")
        app.buttons["chatSend"].tap()
        let seeAll = app.buttons["seeAllPhotos"].firstMatch
        XCTAssertTrue(seeAll.waitForExistence(timeout: 40), "a reply with many photos offers them all")
        sleep(2)
        snap(app, "lib 6b many photos")
        seeAll.tap()
        XCTAssertTrue(app.navigationBars.matching(NSPredicate(format: "identifier ENDSWITH 'photos'")).firstMatch.waitForExistence(timeout: 10))
        for _ in 0..<4 { app.swipeUp() }
        sleep(2)
        snap(app, "lib 6c album")
        app.navigationBars.buttons.element(boundBy: 0).tap()

        // Another pick in the dropdown starts over with that collection.
        app.buttons["askScope"].tap()
        app.buttons.matching(named).firstMatch.tap()
        XCTAssertFalse(app.buttons["photo-0"].waitForExistence(timeout: 2))
        snap(app, "lib 7 one collection")
    }

    /// Adds the first photo of the simulator's library through the
    /// camera's import button, as a shot.
    private func importLibraryPhoto(_ app: XCUIApplication) {
        app.buttons["Import a photo"].tap()
        XCTAssertTrue(app.scrollViews.images.firstMatch.waitForExistence(timeout: 10), "the photo library opens")
        sleep(1)
        let photo = app.scrollViews.images.allElementsBoundByIndex.first { $0.isHittable }
        XCTAssertNotNil(photo, "a photo to pick")
        photo?.tap()
        sleep(1)
    }

    /// After a collection change the camera must start, not spin forever.
    /// The simulator has no camera, so a started camera says so.
    private func assertCameraStarted(_ app: XCUIApplication) {
        // A fresh install asks for the camera first.
        let allow = XCUIApplication(bundleIdentifier: "com.apple.springboard").alerts.buttons["Allow"]
        if allow.waitForExistence(timeout: 3) { allow.tap() }
        XCTAssertTrue(app.staticTexts["No camera is available on this device."].waitForExistence(timeout: 5),
                      "the capture screen is still starting its camera")
    }

    /// Launches the app; with PIDEX_SERVER and PIDEX_TOKEN set, a fresh
    /// install connects itself (the debug launch hook), so no test depends on
    /// what an earlier run left in the simulator.
    private func launch(_ extra: [String] = []) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments += extra
        if let server = env["PIDEX_SERVER"], let token = env["PIDEX_TOKEN"] {
            app.launchEnvironment["PIDEX_SERVER"] = server
            app.launchEnvironment["PIDEX_TOKEN"] = token
        }
        app.launch()
        return app
    }

    private func snap(_ app: XCUIApplication, _ name: String) {
        let a = XCTAttachment(screenshot: app.screenshot())
        a.name = name
        a.lifetime = .keepAlways
        add(a)
    }
}

private extension XCUIElement {
    /// Waits for a toggle to show as selected; searching again takes a moment.
    func waitForSelected(timeout: TimeInterval = 10) -> Bool {
        let done = XCTNSPredicateExpectation(predicate: NSPredicate(format: "isSelected == true"), object: self)
        return XCTWaiter().wait(for: [done], timeout: timeout) == .completed
    }
}
