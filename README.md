# Pidex

**A work-photo camera that files who or what you shot, and finds it again from a plain sentence.**

People only take photos. Pidex works out who or what each photo shows (a patient, a flat, a vehicle, a cat), files it under that subject, tags it, and later finds it from a sentence such as "Maple Court waterproofing last week" or "the cat with the black nose".

Nothing in the engine is tied to an industry. What a subject is, how it is identified and which tags exist all come from each collection's configuration. People write that configuration by describing what they photograph in their own words. `backend/internal/teamconfig/templates/` holds examples for an orthopaedic ward, a renovation business and a household with pets.

## Why

Teams that photograph the same things over and over (wards, building sites, rentals, workshops) end up with thousands of photos in a camera roll or a shared drive, sorted by date. Finding "the second-floor bathroom before tiling" means scrolling.

Pidex is not a cloud album. It files each photo under the subject it shows, at the moment it is taken:

- **By subject, not by date.** A wristband, a serial number or a door number is read on the phone. Subjects without a code (pets, mugs, plants) are matched by their look against the ones already on file.
- **Asks a person when unsure.** A doubtful match becomes a one-tap question ("Looks like Tutu: is it?"). It never guesses silently. Look-alikes are filed automatically only after people have confirmed the model often enough, and one mistake takes that trust back.
- **Search that cites its sources.** Every answer in the chat is a stored photo, checked against what was asked.

## What it does

- **Files photos as they are taken.** A subject is identified by a code (a wristband, a serial number, a door number, read on the phone and by a vision model) or by its look (pets, mugs, plants: compared with photos of the ones already on file). The camera says who a group of shots is for before anything is uploaded, and nothing is uploaded that does not say who it shows.
- **Asks only when it must.** Unsure matches become one-tap questions ("Looks like Tutu: is it?"). Look-alikes are never filed without asking until people have confirmed the model often enough; one mistake takes that trust back.
- **Finds and answers.** A chat finds photos from a sentence, answers questions from the photos it found (citing them), and lists what has not been photographed yet. Every result is a stored photo, checked against what was asked.
- **Set up by conversation.** A new collection is described in a chat; a model drafts the configuration, the server validates it, and later changes are made the same way, with every version kept.
- **Any language, any industry.** The app is in English and Simplified Chinese; each collection's names, tags and descriptions are in the language it was described in.

## Architecture

```
iOS app (SwiftUI)                    Server (Go, one binary)                        Models
  camera, on-device OCR/barcodes       Connect API (Protobuf, also JSON)              vision: describe, read, tag
  shot tray, offline upload queue ──▶  pipeline:                                ──▶   matcher: compare looks
  local catalog, offline search          barcode → dedupe/reuse → model → review      search planner and checker
  review, chat, collections            River jobs on Postgres                         text embeddings
                                       Postgres 17 + pgvector, S3 storage             (OpenRouter or direct Gemini)
                                                │
                                                ▼
                                       Look service (Python, optional): a photo → a DINOv2 vector of its subject
```

- `proto/pidex/v1/`: the API, shared by Go and Swift (Kotlin later).
- `backend/`: the API server, the processing pipeline, search and background jobs, in one binary (`pidex serve`).
- `vision/`: the look service, which tells individuals apart by image (DINOv2); optional, the server works without it.
- `ios/`: the iPhone app, generated from `ios/project.yml` with XcodeGen.
- `design/brand/`: the icon and colours; `eval/`: evaluation sets; `reports/`, `research_notes/`: product research.

## Stack

| Layer | Technologies |
| --- | --- |
| iOS app | Swift, SwiftUI, AVFoundation, Vision (on-device OCR and barcodes), a local SQLite catalog (GRDB), English and Simplified Chinese |
| API | Protobuf and Connect (`buf` generates Go and Swift), callable as JSON too |
| Server | Go, pgx, sqlc, goose migrations, River job queue |
| Storage | Postgres 17 with pgvector, S3-compatible object storage |
| Models | OpenRouter or Gemini directly (Gemini Flash Lite by default for vision and search), Claude for drafting configurations |
| Observability | OpenTelemetry traces of every conversation turn and model call, sent to Braintrust |
| Look service | Python, FastAPI, DINOv2 |

## Status

A personal side project, built and used by one person, and still a work in progress. It runs end to end: the iPhone app, the server, the pipeline and the evaluations below. There is no hosted version. An Android app is not started.

## Quick start

Requires Go 1.26+, Docker and buf for the server; Xcode 26 and XcodeGen for the app.

```sh
make up                                   # Postgres with pgvector (55432) and the look service (8090); the first build downloads ~1.5 GB
make migrate
make seed TEAM="My team" EMAIL=you@example.com TEMPLATE=household_pets   # prints an API token (cbk_...)
export OPENROUTER_API_KEY=...             # optional: without a key, photos are filed by barcode only
make serve                                # API on :8080, background processing, a built-in dev S3
make ios-project && open ios/Pidex.xcodeproj
```

Run the app, enter the server address and the token, and take photos. To run on a phone, see [iOS App](#ios-app).

## License

[MIT](LICENSE)

---

The rest of this file is the developer manual: layout, local setup, model configuration and evaluation, and how each part works.

## Layout

```
proto/pidex/v1/         API definitions (Protobuf + Connect), shared by Go / Swift / Kotlin
backend/
  cmd/pidex/            Executable: serve / migrate / seed / reindex / name-descriptions
  cmd/eval*/            Evaluation tools (vision, appearance matching, embeddings, search)
  internal/
    teamconfig/         Team configuration format, validation, industry templates
    configgen/          Drafts a configuration from a description, with validation and repair
    analysis/           Builds model instructions and JSON Schema from the configuration, and validates model answers
    identity/           Identity field normalization, OCR reading, matching against existing objects
    segment/            Splits the photos of one capture session into segments by identity photo and files them
    pipeline/           Tiered processing pipeline (barcode → dedupe/reuse → cheap model → strong model → human), appearance matching, subject profiles
    search/             Search planning, ranking, result checking, indexing
    ai/                 Model provider abstraction, routing and fallback, cost recording
    api/                Connect API implementation
    store/              Migrations (goose), queries (sqlc generates into dbq/)
    jobs/               Postgres-backed job queue (River)
    e2e/                End-to-end tests
ios/
  Pidex/                The app: Capture, Collections, Chat, Library, Review, Scenario, Upload, App (theme, brand)
  PidexTests/, PidexUITests/
vision/                 Look service (Python): a photo of a subject → a vector of what it looks like
deploy/postgres/        Postgres 17 + pgvector image
design/brand/           Icon originals and render script (`swift design/brand/render.swift`), colour notes
eval/                   Evaluation labels and sources (photos are downloaded, not committed)
reports/, research_notes/  Product research reports
```

## Running locally

Requires Go 1.26+, Docker and buf.

```sh
make up                      # start Postgres (port 55432)
make migrate                 # apply database migrations
make seed TEAM="My team" EMAIL=you@example.com TEMPLATE=<template>   # create a team, print an API token; without TEMPLATE it lists the available templates
make serve                   # start the API (:8080) and background processing
```

If `S3_ENDPOINT` is not set, the server starts a built-in S3-compatible store on the local machine (development only, `127.0.0.1:59000`, data in `backend/.data/s3`). When testing on a physical device, set `DEV_S3_ADDR` and `S3_PUBLIC_ENDPOINT` to the computer's LAN address.

The API can be called directly with JSON, for example:

```sh
curl -X POST http://127.0.0.1:8080/pidex.v1.ConfigService/ListTemplates \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d '{}'
```

### Connecting a vision model

OpenRouter is recommended during development. One key gives access to Gemini, Claude, GPT and other models. Switching models is only a configuration change.

```sh
export OPENROUTER_API_KEY=...      # put this in ~/.zshrc; do not commit it to the repository
```

| Variable | Description |
|---|---|
| `AI_PROVIDER` | `openrouter` or `gemini`. If unset, it is chosen automatically from the keys present, preferring OpenRouter |
| `OPENROUTER_API_KEY` / `GEMINI_API_KEY` | If neither is set, photos are filed by barcode only and no tags are added. The system still runs normally |
| `VISION_MODEL` | Default on OpenRouter is `google/gemini-3.1-flash-lite`; default for direct Gemini is `gemini-3.1-flash-lite` |
| `VISION_FALLBACK_MODELS` | Comma-separated fallback models, tried in order when the primary model fails (rate limit, refusal). Default on OpenRouter is `anthropic/claude-haiku-5.5`; set to an empty string to disable |
| `STRONG_VISION_MODEL` | Optional. In strict mode, used to re-check when OCR cannot confirm the identity |
| `CONFIG_MODEL` | Model that generates a scenario configuration from a description. Default on OpenRouter is `anthropic/claude-sonnet-5.5`; default for direct Gemini is `gemini-3.5-flash`. Rarely called, about $0.03 per call |
| `MATCH_MODEL` | For identification by appearance, the model that compares photos to decide which object it is. Default on OpenRouter is `openai/gpt-6-luna`; for zero misfiling set `anthropic/claude-sonnet-5.5` (about 20x more expensive). See [Identification by appearance](#identification-by-appearance) |
| `SEARCH_MODEL` | Model that turns a sentence into search conditions. Default is `google/gemini-3.1-flash-lite`; if it does not respond within 6 seconds, search falls back to text search |
| `TEXT_EMBEDDING_MODEL` | Text embedding model for photo search (via OpenRouter). Default is `voyageai/voyage-4`, fixed at 1024 dimensions; after changing models, run `pidex reindex` |
| `LOOK_SERVICE_URL` | The look service (`vision/`). `make serve` uses `http://127.0.0.1:8090`; empty runs without it. See [Identification by appearance](#identification-by-appearance) |
| `LOOK_AUTO_SIMILARITY` / `LOOK_MARGIN` | When a photo is filed by its look alone: the closest subject at least this similar (default 0.85) and this much closer than the next (default 0.08), measured for `dinov2_b@336` |
| `EMBEDDING_MODEL` | Optional. Image embeddings needed for search by image. Currently only supported with direct Gemini (requires `GEMINI_API_KEY`) |
| `VISION_PRICE_IN` / `VISION_PRICE_OUT` | Optional. Price per million tokens. OpenRouter returns the actual charge, so these are usually not needed |

> Compliance note: real patient photos must go through a channel covered by a BAA or similar agreement. OpenRouter is a reseller. It is suitable for development, testing and non-medical scenarios. For medical customers in production, connect directly to a contracted vendor (set `AI_PROVIDER=gemini`, or another direct provider added later).

Every model call is recorded in the `ai_calls` table (token counts, cost, latency, success or failure).

### Tracing model calls (Braintrust)

With a Braintrust key set, the server sends a trace of each piece of work that calls a model, over OpenTelemetry (`backend/internal/tracing`). Each trace holds every model call made for that work.

| Trace | Started by | Steps inside |
| --- | --- | --- |
| `chat turn` | each message in a conversation | `search_plan` (reading the message), `search photos` (the database search), `search_check` (checking what was found), `ask` (the answer) |
| `search` | a library search in words (searches by chips or paging call no model and are not traced) | `search_plan`, `search photos`, `search_check` |
| `ask` | a question about photos | `search_plan`, `search photos`, `ask` |
| `config draft` | each draft in a setup conversation | `config_draft`, with retries |
| `identify shot` | the camera asking who a shot shows | `identify_shot`, `appearance_match` |
| `process photo`, `name description`, `index photo` | the background jobs for each uploaded photo | `analyze`, `identity_escalation`, `appearance_match`, embeddings |
| `subject profile` | each subject profiled by look | `subject_profile`, `profile_embed` |

- Each model call is an `llm` span. It records the provider and model, the instructions and prompt as chat messages, the JSON the model returned, token counts, cost (`cost_usd`), latency, and the error if the call failed. When a fallback model is used, each attempt is a separate span.
- Metadata holds `team_id`, `user_id`, `photo_id` and `purpose`. A conversation turn also has `conversation_id` and `turn`, and a config draft has `session_id`. Filter on these in Braintrust to read a whole conversation turn by turn.
- Images are never sent, only their type and size (`[image 1: image/jpeg, 182 KB]`).
- What people wrote and what models answered is sent only for teams whose privacy is not `high`. For teams with high privacy, and whenever `BRAINTRUST_LOG_CONTENT=false`, traces keep only their structure, timings, tokens, cost and ids.

| Variable | Description |
| --- | --- |
| `BRAINTRUST_API_KEY` | Turns tracing on. Without it nothing is sent |
| `BRAINTRUST_PROJECT` | The project traces are logged to, created on first use. Default `pidex` |
| `BRAINTRUST_PROJECT_ID` | Optional. A project id, used instead of the name |
| `BRAINTRUST_API_URL` | Default `https://api.braintrust.dev`. Use `https://api-eu.braintrust.dev` for the EU region, or a self-hosted data plane |
| `BRAINTRUST_LOG_CONTENT` | Default `true`. `false` sends no prompts or answers for any team |

> Compliance note: Braintrust is a separate processor. Before sending traces with content for medical teams, have a BAA with Braintrust, or set `BRAINTRUST_LOG_CONTENT=false`.

### Model evaluation

Before changing prompts or switching models, compare them on the evaluation set first. The evaluation set is public images from Wikimedia Commons (orthopedic clinical photos, wristbands, renovation sites, unrelated photos). Each image is labeled with the correct role, tags and identity fields:

```sh
make eval-fetch                                  # download the evaluation images (about 70, not committed)
make eval                                        # run with the default model
make eval MODEL=anthropic/claude-haiku-5.5       # compare against another model
```

The output gives role, tag and identity accuracy per team configuration, cost per thousand photos, latency, and every photo that was answered wrongly. Labels are in `eval/commons/labels.tsv`; see `backend/cmd/evalvision` for the format.

Comparison results from 2026-10 (87 judgments; latency is the full request round trip):

| Model | Role | Tags | Identity | Per 1,000 | Latency p50 / p90 | Errors |
|---|---|---|---|---|---|---|
| `google/gemini-3.1-flash-lite` (default) | 86/87 | 25/25 | 86/87 | $0.73 | 2.1s / 2.5s | 0 |
| `google/gemini-3.5-flash-lite` | 84/87 | 25/25 | 86/87 | $0.95 | 2.6s / 9.0s | 0 |
| `anthropic/claude-haiku-5.5` (fallback) | 84/87 | 25/25 | 85/87 | $0.46 | 4.4s / 6.8s | 0 |
| `openai/gpt-6-luna` | 83/87 | 25/25 | 85/87 | $0.35 | 5.3s / 8.7s | 0 (another run refused one wound photo) |
| `google/gemini-2.5-flash-lite` (old default) | 47/66 | 19/19 | 65/66 | $0.09 | 5.1s / 8.6s | 21 (rate limits, refused wound photos) |

## Collections and their configuration

Each collection is one team on the server, with its own configuration (its scenario), subjects and photos. Nobody writes the configuration by hand:

1. On the home screen, tap **New collection**. A conversation opens, tied to the collection; it is not among the photo chats. It asks one question, "What do you photograph?", with a few examples to tap (pets at home, wounds at a clinic, units on a renovation site, equipment with serial numbers).
2. Describe, in any language, what you will photograph and what you will want to find later. There is nothing to choose first: how subjects are told apart (by a label or code, or by how they look) is worked out from the description, shown on the draft, and changed there with one tap. While the draft is made, the app says what step it is at.
3. The server has `CONFIG_MODEL` generate the configuration and runs the same full validation as for a hand-written one. If validation fails, the errors are sent back to the model to fix (at most two rounds). Each identity field must also include an example of "how it looks printed on the label". The server checks it with the same rules as the pipeline, to prevent formats that could never match.
4. The draft is one card: its name (rename in place), how each kind of subject is told apart (change it from a menu), its tags, and **Create and shoot**. To change anything else, say it in one more sentence. Creating it makes a new team with you as owner and opens the camera straight away, which says how to take the first photos (a label first, or each one close up and face on).

Later, reopen the same conversation from the collection's page and describe what to change.

**Collections kept by place and time.** A collection may have no subject types at all: a trip, a holiday, an event, a diary of places ("photos from my trip to Dalian"). Photos then need not say who they show and nothing waits for a person; each is kept by where and when it was taken. Nothing the person chose to photograph is hidden as unrelated: only shots showing nothing at all (black, completely blurred, a pocket) are, and the batch says how many were hidden. A collection is named for what it holds in general ("Travels", not one city), and its tags are kinds of content, never a place: places come from the phone. The collection's page heads with how many photos and places it holds (it has no subjects to switch to) and shows a row of places with their counts, at the level that tells the photos apart, chosen without asking: countries when they span several (tap one to see its cities, with a way back), cities when they are all in one country, landmarks or districts when all in one city. Tap a place to see its photos. Names are shown short: a Chinese city without its "市" ("Dalian" for "大连市"; search finds either), a landmark without the city it begins with, long ones cut. Each day's heading names where most of its photos were taken ("Wed 1 Oct · Dalian · Xinghai Square"), and a run of days in one city apart from the collection's usual one is headed as a trip ("Dalian, 1–4 Oct"). The `trips` template is an example. The model edits the current version and existing keys stay the same. If a deletion or change would affect the content of photos already filed, warnings are listed before publishing. Every version is kept and can be restored from History. Each photo records the version used to process it.

The app holds several collections, each with its own token. A collection can be deleted by its owner, after typing its name; this cannot be undone.

Next to the description box there is **voice input**. When the device has an on-device language model, recognition runs locally and audio never leaves the phone. The recognition language follows the App by default and can also be chosen separately. Voice cannot be tested in the simulator (its audio input depends on the Mac's microphone permission); try it on a physical device.

## Capture

- **Every photo keeps where and when it was taken.** With permission, the camera notes the phone's position for each shot; a photo added from the library keeps the day and GPS its camera wrote into it (not the moment it was added). The phone names the coordinates itself (Apple's geocoder: free, nothing sent to our server), country down to a landmark, and sends the names with the photo. Up to 200 photos can be added from the library at once. High-privacy teams keep coordinates but no place names.
- **Shots stay on the phone until sent.** They collect in a tray at the bottom of the camera, like a story being made; any can be deleted. Nothing reaches the server until **Upload**.
- **Every shot must say who it shows**, in collections that file by subject. A shot of a label or code, or (for collections told apart by look) the first shot of a group, is identified at once, and its frame turns green; the shots after it follow it. A person can also choose the subject, name a new one, or clear the choice. Upload stays disabled while any shot is unidentified, and the server refuses photos without an identity as well.
- **Uploads survive the app closing.** A queue on the phone retries with backoff and keeps each photo's ID, so nothing is sent twice. A small floating pill shows progress; tapping it opens the batch.
- **Review asks once per subject.** After a batch is filed, the review page lists what went where and asks about the few that need a person: "Are these 5 Tutu?", one tap for the lot.

## Identification by appearance

There are two ways to identify objects. They are set per object type in the scenario configuration (`identify_by`), and the model chooses one itself when generating the configuration:

- **By code (code)**: patient wristbands, serial numbers, house numbers, license plates, asset tags. Take an identity photo first; the photos after it are filed under it. Precise. Medical, legal and financial users always use this method.
- **By appearance (appearance)**: things nobody would put a label on, such as pets, mugs, house plants, personal belongings. No number needs to be photographed; just take the photo.

The appearance identification flow (`backend/internal/pipeline/appearance.go`, `look.go`):

1. While analyzing a photo, the vision model also writes a **short name** for the subject ("orange tabby cat") and its **distinguishing features** (patterns, markings, chips, accessories).
2. **By look first.** With the look service running, the subject (cut out by the phone's box) becomes a vector, which is kept with the photo. It is compared with the photos already filed; each subject counts by its closest photo. If the closest subject is at least 0.85 similar and at least 0.08 closer than the next, the photo is filed without asking the model. Words cannot tell two black cats apart; the vector can. Otherwise those subjects become the first candidates below, in that order. Without the service, or when it fails, this step is skipped.
3. Using the embedding of the feature description, fill up to 6 candidates with the most similar existing objects of the same kind, and take 3 reference photos of each: first those where the subject is clearly visible; among those, first ones a human has confirmed, then the photo from its first appearance, then the newest.
4. `MATCH_MODEL` looks at the new photo and these photos together. It answers which one it is, that it is new, or "can't tell" (too small, too dark, occluded, not there at all), and gives a confidence. A confidence above 0.9 is allowed only when it sees a feature unique to one individual. Features that other individuals of the same kind may also have, such as color or breed, allow at most 0.7. A "can't tell" photo is neither filed nor used to create a new object.
5. Only confidence ≥ 0.97 (0.99 in strict mode) files the photo directly. Moderate confidence becomes a suggestion, confirmed with one tap on the review page after shooting ("Is this Mimi? Yes / No, choose another"). If it is clearly new, a new object is created automatically and named after how it looks; it can be renamed later.
6. Each photo a human confirms or corrects becomes a reference photo for that object, so recognition improves over time. If one object has been split into two, merge them on the object page with "Same as another...".

Within one continuous shooting group, photos that do not show the subject (for example the cat bowl) still follow the object recognized in the previous photo.

**Descriptions use the name**: when the vision model writes a description, it also writes a version with the subject replaced by `{subject}` ("{subject} is crouching by the kitchen sink."). Once a photo is filed under an object (automatically or confirmed by a human; suggestions do not count), display and Q&A use the object's name: "Tutu is crouching by the kitchen sink." It follows renames. Photos that do not show the subject (the cat bowl) keep the original description. High-privacy teams do not generate this sentence. Backfill old photos with `pidex name-descriptions` (the server re-describes them one by one in the background, about $0.001 per photo).

A few rules that make it more reliable:

- **One batch is one object**: photos in the same upload that are recognized as the same object all take the score of the most confident photo among them. When a human confirms one of them, the rest are filed together. The review page asks once, "Are all N of these Tiger?", instead of asking for every photo.
- **Recognize while shooting**: for collections identified by appearance, the camera sends the first photo of each group to the server for matching. A few seconds later it shows "Current: Tiger" or "New: golden shaded cat". Later photos are assumed to be the same object and are not recognized again. If it is wrong, tap "Change".
- **Look only at the subject**: the phone uses Vision to find cats and dogs in the photo. If there is no cat or dog, a single prominent object in the frame is used only when there is exactly one. Matching looks only at that region, so people, doors and sofas in the background no longer interfere. Small, cluttered frames are not cropped; a wrong crop is worse than no crop.
- **3 reference photos per object**, first those where the subject is clearly visible, then those a human has confirmed. An unclear photo filed manually by a human (or a photo that only shows the cat bowl and was filed by following the previous photo) does not become a main reference; otherwise later matches would have nothing to compare against.
- **Auto-file only when very confident**: appearance matching needs confidence ≥ 0.97 to skip asking a human. In testing, wrong matches scored at most 0.96, and everything ≥ 0.97 was correct. Below that, the camera asks directly, "Looks like Tiger, right?", and one tap on "Yes" is enough.
- **Look-alikes are not auto-filed**: when two existing objects have very close feature descriptions (embedding similarity ≥ 0.85, for example two tuxedo cats), a match to either one is only a suggestion and asks a human to confirm. When matching an object that has a look-alike, each candidate gets 5 reference photos (3 normally), taken from different shoots where possible.
- **Gets more accurate with use (earning trust)**: every time a human taps "Yes" or picks another option on the camera, or confirms/corrects on the review page, the system records "who the model said, who the human said". Camera verdicts are held by the photo's client id (`shot_verdicts`) and saved with the photo after it uploads. For an object and its look-alikes: if, in the last 30 checks, the model has been right at least 8 times in a row **at a confidence no lower than the current one** (one shoot counts once, scored by its worst photo), and the confidence is ≥ 0.8, it stops asking a human and files directly. One wrong answer revokes the trust, and it has to be earned again. This does not apply to teams in strict mode.
- When the matching model errors or is rate-limited, the fallback models are used (the same as `VISION_FALLBACK_MODELS`).

**Limitations**: identical mass-produced items (two mugs of the same model) cannot be told apart by appearance. The system says it is not sure and asks a human to confirm, rather than guessing. Faces are not supported here: for scenarios involving people, use codes.

### Appearance identification evaluation

`backend/cmd/evalappearance` "shoots" a set of photos in random order into an empty collection, files them exactly as the pipeline does, and records the result for each photo. Correct suggestions simulate a one-tap confirmation, wrong ones simulate a human correction, and auto-filed photos are not checked (as in real use).

```sh
make eval-fetch                                          # download the evaluation images (not committed)
make eval-appearance                                     # default matching model
make eval-appearance MATCH=anthropic/claude-sonnet-5.5   # compare against another model
```

Photo descriptions are cached per model in `eval/appearance/descriptions.jsonl`, so switching the matching model does not require re-describing. Labels are in `eval/appearance/labels.tsv`.

Results from 2026-10 on 57 public Commons photos of 5 cats (Palmerston and Socks are two very similar black-and-white "tuxedo" cats; the photos are mostly in crowds, old photos or long shots, harder than photos taken at home):

| Matching model | Auto-filed wrongly | Auto-filed | One-tap confirm | Per match |
|---|---|---|---|---|
| `google/gemini-3.1-flash-lite` | 7 photos (12%) | 67% | 6% | $0.0032 |
| `openai/gpt-6-luna` (default) | 1 photo (2%) | 35% | 54% | $0.0005 |
| `anthropic/claude-haiku-5.5` | 0 | 5% | 64% | $0.0006 |
| `anthropic/claude-sonnet-5.5` | 0 | 32% | 59% | $0.0103 |

Flash Lite is overconfident: if something looks similar, it gives high confidence. Luna honestly asks a human to confirm when unsure, and is also the cheapest (with many images, Gemini's per-image billing costs more). The one remaining error was Palmerston recognized as Socks.

After adding the 0.97 threshold, "can't tell" and reference photo selection (2026-10-23, `openai/gpt-6-luna`, each set run once in each of 3 different shooting orders):

| Evaluation set | Auto-filed | Of which correct | Filed under another existing object | One-tap confirm | Per match |
|---|---|---|---|---|---|
| Cats (57 photos, including the two tuxedo cats) | 33 photos (19%) | 31 (94%) | 0 | About 70%, of which 12–33% need "Other" to pick another | $0.0006, about 7 seconds |
| Mugs (13 mugs, 36 photos) | 105 photos (97%) | 105 (100%) | 0 | 3% | $0.0006, about 4–5 seconds |

- In total there were 138 automatic decisions, 136 correct (98.6%). **Not once was a photo filed under another existing object.** Both errors were the same photo: a night shot of Downing Street where Larry is only a tiny blob on the windowsill. The model answered "can't tell". The evaluation's simulated human manually created Larry from it. Later, clear photos of Larry could not match against it, so another Larry was created and needed merging. In real use, the camera would show "Not recognized, move closer and retake" for this photo.
- The two tuxedo cats are never auto-filed, at the cost of a few more taps: Socks is mostly in long-shot old photos from the 90s, and is often suggested as Palmerston, so "Other" is needed to pick another.
- Mugs (`eval/mugs`): 4 white mugs, plus two from the same maker (Denby) with different glazes. Mugs photographed many times use real photos. The rest use one photo to generate two "retakes" (closer, tilted, darker or blurrier); see `eval/mugs/make_shots.py`. `make eval-fetch` downloads and generates them; `make eval-mugs` runs them. The mug results in the table above used the phone's subject boxes (`-boxes`); `make eval-mugs` does not use them by default.
- Limitations: the sample is small (5 cats, 13 mugs). 98% is the result on these photo sets, not a guarantee. Photos taken at home are usually clearer than these public photos.

**Filing by look, measured** (2026-10-26, `dinov2_b@336`, `openai/gpt-6-luna` for the rest, the same three shooting orders as above; `evalappearance -look`):

| Set | Filed automatically | Filed by look alone | "Other" taps to fix a suggestion | Model comparisons |
|---|---|---|---|---|
| Cats, without the look service | 38, 36 right | | 30 | 156 |
| Cats, with it | 49, 47 right | 22, all right | 16 | 134 |
| Mugs, without it | 35, all right | | 0 | 35 |
| Mugs, with it | 36, all right | 23, all right | 0 | 12 |

The two cat errors in both rows are the night shot of Downing Street described above. `eval/visual/` measures the look models themselves (DINOv2 against MegaDescriptor, keypoint matching for rigid things) and how the thresholds were chosen. Photos filed before the service ran get their vectors with `pidex embed-looks`; after changing `LOOK_MODEL`, `pidex embed-looks -all`. Strict teams never file by look alone.

**Profiles.** A subject told apart by look also gets a profile: what it looks like in full (coat, nose, eyes, ears, paws, build, marks, collar), written by the vision model from up to four clear photos of it from different shoots, and written again as its photos double (1, 2, 4, 8…). A periodic job does this every two minutes, about $0.001 to $0.002 a subject each time. Features no photo shows clearly are left out. On the subject page a person can add to or correct it ("black nose"); what they write is kept and no longer rewritten from photos. Search matches subjects by profile too, so "the cat with the black nose" finds that cat's photos even where its nose is not visible, and the result check reads each subject's profile.

## Search

Search and questions live in a chat, opened with the Pidex mark at the top of the home screen (all collections) or of a collection's page (that collection). A collection's page also has a search field over its photos. Type or speak, describing in your own words what you are looking for; follow-ups build on the previous search ("only the back ones").

1. **Understand the request**: `SEARCH_MODEL` turns a sentence into search conditions: objects ("bed 12", "203"), a place ("Dalian", "Xinghai Square", matched against any part of where each photo was taken; a named project or site stays an object), tags, a time range ("last week" is interpreted in the user's time zone), the remaining semantic text, and whether the user is "finding photos" or "finding what is missing". Tags and object types can only be values that exist in the team configuration. The conditions are shown as small chips above the results. Tapping ✕ to remove a condition re-runs the query directly, without calling the model again.
2. **Find**: objects and time are filtered exactly in the database (a project automatically includes its units). Ranking fuses three signals: how well the tags match, how semantically close the whole sentence is (embeddings), and keywords. AI tags often miss things, so tags only affect ranking and are not a hard condition.
   **Verify**: meaning-based matching always ranks "the closest few", even if none is actually right (ask for "Tiger on the sofa" and you get Tiger on the floor). So for photos found from a sentence, whenever the question is about content (tags or text), each photo's description and tags are passed to `SEARCH_MODEL` to check, and only the ones that truly match are kept (text only, about $0.0003 per query). Searches only by object or time, or by tags the user selected themselves, are not verified. If verification fails, results are shown as they are. When the model requires "all tags present" but finds no photos at all, tags switch to ranking only and the results go to verification.
3. **"What is still missing"**: for example "which units have not had waterproofing photographed yet". The database computes the set difference directly, without the model.
4. **Ask a question**: first find real photos as above, then pass the thumbnails and descriptions to the vision model to answer. Each sentence cites the photo numbers it is based on. High-privacy teams (medical) have questions disabled and can only search.

The model never "recalls" photos: results are always photos that really exist in the database. Requests unrelated to photos ("what model are you", "write me a poem") are neither searched nor answered; the reply only explains what it can help with. When searching all collections at once, only collections with results are shown. If nothing is found anywhere, it just says "No photos found".

**Thresholds follow the embedding model**: which semantic matches count is decided by three cosine distances. The defaults were measured on `voyage-4`. When changing `TEXT_EMBEDDING_MODEL`, re-measure with `evalembed -gaps` and `evalsearch`, then override with `SEARCH_MAX_DISTANCE`, `SEARCH_RELATIVE_DISTANCE` and `SEARCH_MISSING_DISTANCE`.

**Indexing**: after a photo is processed, a background job generates thumbnails, assembles the search text (description + tags + file fields, excluding identity information such as names and medical record numbers) and computes embeddings. Changing tags re-indexes automatically. `pidex reindex` backfills old photos or rebuilds after changing the embedding model. `pidex reprocess -team NAME [-role junk]` analyses and files a collection's photos again after its rules changed (people's own filings are kept). If the original image is lost, the text is still indexed.

**Evaluation**:

```sh
# compare embedding models (documents come from the make eval results)
go run ./cmd/evalembed -answers ../eval/commons/last-run.jsonl -queries ../eval/search/embedding_queries.tsv \
  -own ortho=hospital_orthopedics -own construction=construction_renovation -own junk=construction_renovation \
  voyageai/voyage-4:1024 qwen/qwen3-embedding-8b:1024      # add -gaps to print the similarity distribution per query, used to set search thresholds
# end-to-end search: first upload the evaluation photos into two teams, then run natural-language queries in Chinese and English
go run ./cmd/evalsearch -queries ../eval/search/queries.tsv -team construction_renovation=TOKEN,uploaded.tsv -team hospital_orthopedics=TOKEN,uploaded.tsv
```

Results from 2026-10: the `voyage-4` embedding model scores MRR 1.00 on both Chinese and English queries. End to end, 18 of 21 queries were fully correct, average recall was 0.94, about 60% of returned results were relevant, and latency p50 was 1.7 seconds.

## Language

The two kinds of language are handled separately:

- **UI language** follows each person: the App follows the iPhone system language by default, and it can also be chosen separately in iOS "Settings › Pidex › Language" (the app's Settings page has a shortcut). English and Simplified Chinese are currently available. Strings are in `ios/Pidex/Resources/Localizable.xcstrings`.
- **Content language** follows the team: when a scenario is generated, names, tags and descriptions use the language the description was written in (a Chinese team's scenario is written in Chinese), and this is recorded in the configuration's `language` field (for example `zh-Hans`). The descriptions the vision model writes for each photo also use this language. Editing the scenario keeps the original language.

The server does not compose sentences for people to read: scenario change warnings are returned as structured `ConfigChange` values, and the App builds the text in the UI language (an English `message` is also included).

**Adding a UI language**: open the String Catalog in Xcode, add the language and translate; or export XLIFF with `xcodebuild -exportLocalizations -exportLanguage <language>` for a translator, then import it back with `-importLocalizations`. Entries without a translation in the export are the ones that were missed.

APIs: `ConfigService.DraftConfig`, `ListConfigVersions`, `PublishConfig(restore_version)`, `TeamService.CreateTeam`, `GetTeam`.

## iOS App

```sh
make ios-project            # generate ios/Pidex.xcodeproj with XcodeGen (the project file is not committed)
open ios/Pidex.xcodeproj
```

- After opening it for the first time, select your Team under Signing for the Pidex target (a free Apple ID is enough for debugging on a device).
- On first launch the App opens the connect page. Enter the server address and the token printed by `make seed`. The token is stored only in the local Keychain. More collections are added with their own tokens from Settings.
- Personal signing values (your team and a bundle identifier registered to it) go in `ios/Local.xcconfig`, which is not committed; see `ios/Signing.xcconfig`.
- The look follows `ios/Pidex/App/Theme.swift`: black and white, after Grok, with colour kept for state (see `design/brand/README.md`).
- The simulator has no camera. Tap the bottom-left corner to import photos from the photo library; this goes through exactly the same flow as taking a photo.

**A copy on the phone**: each collection's photos and subjects are kept on the phone (SQLite through GRDB, `ios/Pidex/Library/LocalLibrary.swift`), one file per collection named from a hash of its token. A collection opens from the copy at once and then asks the server what changed (`SyncLibrary`); with no connection it is browsed from the copy, with its places, tags and subjects as last seen. Images are kept by photo id in Application Support, which the system does not clear for space (`DiskImages.swift`): thumbnails up to 1 GB, and on Wi-Fi the newest 1,000 of a collection's are fetched ahead; originals opened in full, and photos taken on this phone (the uploaded file is kept rather than fetched again), up to what Settings allows (2 GB by default, or none). The least recently seen go first. High-privacy collections keep no originals. Photos deleted on the server are deleted from the phone at the next sync. The copy carries no links: images not yet on the phone get short-lived links in batches of up to 200 (`GetPhotoLinks`), as do expired ones. Whatever the phone can find exactly as the server would is found on the phone, at once and offline: all photos, or those with picked tags, a place, days or subjects. A search by meaning (a sentence, or text) runs on the server; offline, the phone reads the names it knows in the words (subjects, tags, places: "大连睡觉的Tutu") into filters, else matches the words against each photo's description, fields read, tags and place, and says the results were matched offline. Files are out of backups and readable only while the phone is unlocked (or they are open); removing a collection from the phone erases its copy.

**Originals and Live Photos**: every model and thumbnail reads the working copy, an upright JPEG of at most 2048 px with no metadata. Beside it the server keeps the file as the camera made it (HEIC, full size; a library photo's location is taken out without encoding it again; a JPEG no larger than the working copy is not sent twice), and, for collections whose scenario sets `live_photos` (off until an owner or admin turns it on from the camera's Live button, or asks for it in the setup conversation), the motion around each shot, recorded without sound: taken in the app with Live on (each person's camera remembers it, as the Camera app does), or picked from the library as a Live Photo, its motion's location taken out. Grids, the camera's tray and the viewer mark Live Photos. They are sent after the photos (`CreateFileUpload`, `CompleteFileUpload`), so filing never waits for them; a phone hears of them through sync (`original_kept`, `live`) and fetches them by link (`GetPhotoLinks`). They go with the photo. This costs roughly 3 to 4 times the storage of the working copy alone, about 8 times with Live Photos. **Save to Photos** (the photo's menu, or several selected) adds the original, with its motion as a Live Photo where kept, else the working copy, with the time and place it was taken; Pidex asks only to add to the library, never to read it. High-privacy collections cannot save to Photos, since a personal library may sync to a personal cloud.

On the server, every write to a photo or subject stamps it with the transaction that made it, by trigger, so no code path can forget to; a change to a photo's tags, or to the name of the subject it is filed under, stamps the photo too, and deletions leave tombstones (migration `00012_sync.sql`). A sync reads, a page at a time, everything stamped since its cursor, and the next starts at the oldest transaction still running when this one began, so a write that commits late is not missed. Tombstones are kept 30 days; a phone away longer starts again from nothing.

**Connecting a physical device to a local server**: the phone and the computer must be on the same Wi-Fi, and the server must listen on the LAN address:

```sh
HTTP_ADDR=0.0.0.0:8080 DEV_S3_ADDR=0.0.0.0:59000 \
S3_PUBLIC_ENDPOINT=http://<your-computer-LAN-IP>:59000 make serve
```

In the App, set the server address to `http://<your-computer-LAN-IP>:8080`. The built-in development S3 has no authentication; only use it this way on a trusted network.

**Testing**: `make ios-test` runs the unit tests. With the server running, setting `PIDEX_SERVER` and `PIDEX_TOKEN` also runs the integration tests: the simulator reads a barcode with Vision, uploads through the upload queue to the server, then checks that the server filed the photos under the same patient. Adding `PIDEX_LIVE_DRAFT=1` calls a real model (about $0.06): one test calls the API directly to generate a scenario and create a collection, and one UI test taps through the whole "New collection" flow like a person would, saving a screenshot of each step in the test results.

**Simulator vs. physical device**: in the simulator, Vision can only use the CPU, and barcode recognition uses the older recognizer. On a device, the system default, more accurate version is used.

## Database changes

- Migration files are in `backend/internal/store/migrations/`, numbered in increasing order. Each must have a `-- +goose Down`.
- Make changes in an "expand then contract" way: both the old and the new version of the program must be able to run on the same schema, so rolling back the program does not require rolling back the schema.
- `go run ./cmd/pidex migrate down` rolls back only one step. It is for development and for verifying reversibility. Production relies on the database's point-in-time recovery.
- Customer-defined fields are stored in JSONB and in `team_configs`. Changing a team configuration does not need a migration.

## API changes

After changing `proto/`, run `make generate`. Use `buf breaking --against '.git#branch=main'` to check whether compatibility with older App versions is broken.

## Testing

```sh
make test   # unit tests + end-to-end tests (requires make up)
make lint
```

The end-to-end tests create a temporary database for each test case and run through the real API, pipeline and storage. Only the vision model is scripted.
