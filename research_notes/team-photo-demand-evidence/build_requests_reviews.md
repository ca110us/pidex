# Demand Evidence: Build Requests, Workarounds, and Review Complaints for Team Photo Capture / Case-Based Filing / Search / Sharing

Method note: Research date 2026-10-19. Verification levels:
- **[FETCHED]**: I opened the page and the quote or detail came from the page text.
- **[SNIPPET]**: the detail came only from search-engine summaries or snippets, because the page blocked fetching. Capterra, GetApp, G2, TrustRadius and the Jobber community all returned HTTP 403. Dropbox Community pages render with JavaScript and returned no body.
- Freelancer.com shows relative dates ("posted X ago"). I converted them to approximate dates from the fetch date.
- Reddit could not be reached. Search-engine `site:reddit.com` queries returned no Reddit threads, and direct API access returned a block page. No Reddit evidence is included. This is a known gap.

## (a) Paid build requests: people paying or offering to pay for a photo-per-job/patient/case tool

### Takeaway
I found 6 verified Freelancer.com postings, dated from about 2014 to October 2026. Each one asks for a photo app that captures images into per-job, per-site, per-order or per-patient structures. Budgets are small, from about $30 to $750 USD, with one at €3,000–5,000. Bid counts are high, up to 209 proposals. Buyers want a simple, narrow tool and will not pay for an enterprise suite. Two of the requests are recent (2026), and one comes from an orthodontist handling clinical photos.

### Cited Findings
- **Orthodontist, about Oct 2026, $250–750 USD, 150 proposals, status "In Progress".** Title: "HIPAA compliant AI Photo Organizer App". Verbatim: "I am an orthodontist looking to develop a simple AI-powered web application" … "that automatically organizes and formats a patient's orthodontic photographs" … "The user will upload approximately 9 intraoral and extraoral patient photographs at the same time" … "Because these are patient clinical photographs, privacy and security are important." **[FETCHED]** — [Freelancer.com](https://www.freelancer.com/projects/computer-vision/hipaa-compliant-photo-organizer-app)
- **"Site Photo Organizer & Labeler", about Sep 2026, £20–250 GBP, 209 proposals, closed.** Verbatim: "Photos should be organised by site and category, saved inside the app and to the iPhone camera roll." Also: "the selected text must be permanently overlaid at the bottom of the image." It asks for photos "available for review, deletion and export." The listing does not mention search or sharing. **[FETCHED]** — [Freelancer.com (dk)](https://www.dk.freelancer.com/projects/ios-development/site-photo-organizer-labeler)
- **"AppSheet QC Media Collector", about Jun 2026, ₹12,500–37,500 INR, 26 proposals, closed.** Verbatim: "I need an AppSheet application that lets our shop-floor team capture both photos and videos." Per the page summary: staff pick an order number from a dropdown, capture geo- and time-stamped media, and the files save automatically into a structured Google Drive folder per order. The app also creates a view-only Drive link so customers can follow progress. This is a client request, not a freelancer pitch. **[FETCHED]** — [Freelancer.in](https://www.freelancer.in/projects/app-development/appsheet-media-collector)
- **"iOS Form App for Returns Inspection", about 2024, $30–250 AUD, 69 proposals (average bid about $233 AUD).** Verbatim: "I need a simple form app for iOS." … "This app will primarily be used for the inspection of returned products." According to the search summary, the scope included camera capture of up to 10 images per inspection entry. That detail is snippet-level only. **[FETCHED]** for budget and quotes — [Freelancer.com](https://www.freelancer.com/projects/iphone-app-development/ios-form-app-for-returns)
- **"PICTOCOM", Munich construction PM, about 2017–2018, €3,000–5,000, closed.** Title: "Develop iOS App/Android App/ Website and Admin Panel (Photo App)." The client wanted a photo management and archiving app. Admins invite project managers, project managers create per-project folder structures, and site managers shoot photos into those folders. The app needs a camera interface, offline storage, multi-language support and a web interface. The page shows "Posted over 8 years ago", and the client has been a member since Oct 27, 2017. **[FETCHED]** (the description came back paraphrased because of the tool's quote limit) — [Freelancer.ie](https://www.freelancer.ie/projects/php/develop-ios-app-android-app)
- **"Construction Job Photo database", San Clemente, US, about 2014 (outside the preferred window), $250–750 USD, 33 proposals, average bid about $1,265.** Verbatim: "The app is is for a construction company to document job specific photos for client closeout packages." The app needs a separate folder per job, approved photos emailed and named by job and item, and large buttons. **[FETCHED]** — [Freelancer.cl](https://www.freelancer.cl/projects/Mobile-Phone-Android/Construction-Job-Photo-database/)
- **Shop-built tools after commercial apps failed.** In March 2025, a sign shop on Signs101 wrote that "company cam does not work well for us" because it "saves all of the photos based on the client not the job." They also wrote that "Jobber compresses the photos when uploaded." Their current workflow: "we upload to a teams channel and they are manually moved to our storage and jobs folders." What they want: "just looking for a streamlined way to be able to upload photos from the field." Another shop owner replied with a self-built setup of about 8,000 photos: a synced folder, Google Photos, and an email-triggered script that creates job folders, plus weekly cleanup of misnamed albums. **[FETCHED]** — [Signs101 forum](https://www.signs101.com/threads/job-completion-photos.179718/)
- **Developer built a niche tool to fill a gap.** A Jobber community post announced FieldFrame, an iPhone app that uploads job photos to a customer-named Google Drive folder and links that folder to the Jobber job. The user manual is v1.0, April 2026. **[SNIPPET]** for the forum post (403) — [Jobber Community](https://community.getjobber.com/discussions/equipment--tools-forum/new-iphone-app-for-organizing-jobsite-photos-with-jobber/9465); manual: [FieldFrame User Manual](https://fieldframe-backend.onrender.com/FieldFrame_User_Manual.pdf)

### Inferences
- Paid demand is real but cheap. Freelance budgets cluster at $250–750 USD equivalent, while CompanyCam is quoted at about $63–99 per user per month on review sites (see below). Buyers apparently want a narrow, owned tool rather than another per-seat subscription. The very high bid counts (150–209) show plenty of developer supply for these jobs.
- Folder-per-job (or per-order, per-site, per-patient) is the universal mental model in these requests. Labeling or overlay, export, and automatic folder creation come up repeatedly. Search and AI tagging rarely appear in buyer briefs. The orthodontist's "AI-powered … organizes" request is the exception, and it is recent (2026).

### Gaps
- I found no Upwork, Guru or PeoplePerHour job postings. Upwork job pages did not surface in search, and I could not search those sites directly.
- I found no Bubble, Glide or Indie Hackers "build me an app" posts with budgets.
- Freelancer dates are approximate because the site shows only relative ages.

## (b) Low-code workaround demand (AppSheet, Power Apps, Airtable, Bubble, SharePoint)

### Takeaway
Users on AppSheet, Power Apps, Airtable and Bubble repeatedly try to build per-record photo folders: images saved into a folder named after a job or record, galleries filtered by job ID, and tags per photo. The threads show friction with slow loading, file renaming, attachment limits, and unanswered questions.

### Cited Findings
- **AppSheet, vehicle workshop (Dec 2020).** The user tracks vehicles by JobID and has thousands of in-progress repair photos. What they wanted: "Show me a gallery for images beginning with JobID 1234." Also: "I need to think outside the box and find a way for Appsheet to access the folders." Their Apps Script workaround, which writes Drive file listings into a Sheet, turned out "painfully slow on loading the image." **[FETCHED]** — [Google Developer forums / AppSheet](https://discuss.google.dev/t/i-need-a-solution-for-linking-images/78485)
- **AppSheet, nested per-row folders (Sep 2021, follow-up Nov 2023).** The user wanted images saved to paths built from row values and asked if folders could be created automatically. They solved it with a trailing "/". A 2023 reply asked for more: "solved but i want also files rename daynamic." **[FETCHED]** — [Google Developer forums / AppSheet](https://discuss.google.dev/t/saving-images-to-the-custom-folder-path/71320)
- **Bubble (May 29, 2024).** A builder wanted "a heap of images" uploaded through a multi-image uploader, with each photo saved as a record with a tag. The thread got no replies and auto-closed on Aug 7, 2024. **[FETCHED]** — [Bubble Forum](https://forum.bubble.io/t/multi-image-upload-file-manager/324348)
- **Microsoft 365, marketing department (Nov 8, 2018).** Verbatim: "the Marketing Department in my organization asked if there is an application in Office 365" … "They need to be able to add tags to the pictures" … "so it will be possible to make a search on tags instead of file names or date." The accepted answer points to SharePoint metadata columns and a Managed Metadata column. **[FETCHED]** — [Microsoft Tech Community](https://techcommunity.microsoft.com/discussions/microsoft-365/office-365-application-to-handle-pictures-in-marketing-and-communication/283906)
- **OneDrive tagging removed (Jul 2024).** A user asked "Are you still able to tag your photos via One Drive?" A Microsoft Community Support Specialist answered: "Unfortunately, the ability to tag photos directly in OneDrive has been removed by Microsoft." Another replier said tagging still worked on the web, and the user said those steps did not work for them. The status is therefore disputed. **[FETCHED]** — [Microsoft Q&A](https://learn.microsoft.com/en-us/answers/a/7666173)
- **Power Apps.** A thread notes that SharePoint library images may not display for coworkers inside an app. The suggested workaround is to embed the app on a SharePoint page. **[SNIPPET]** — [Power Users forum](https://powerusers.microsoft.com/t5/Building-Power-Apps/Save-and-retrieve-pictures-in-Share-Point/m-p/181585)
- **Airtable.** Community answers say Airtable cannot show a subset of a linked record's attachments, and moving attachments between records means downloading and re-uploading them. A 2021 feature request reports that iOS attachments silently fail to upload about half the time. **[SNIPPET]** — [Airtable Community (combine attachments)](https://community.airtable.com/t/combine-attachments/23079); [Airtable product ideas – attachments](https://community.airtable.com/t5/product-ideas/idb-p/productideas/label-name/attachments/page/5)
- **Jobber has no per-property photos.** A user with many farm properties under one client asked for photos per property. According to the summary, Jobber Support acknowledged it as "a common use case for businesses managing multiple locations under one client". The suggested workarounds were job forms, or a custom field linking to a Google Drive or Dropbox folder per property. **[SNIPPET]** (403) — [Jobber Community](https://community.getjobber.com/discussions/job-details-scheduling/how-to-manage-photos-for-clients-with-multiple-properties-in-jobber/8086)

### Inferences
- The low-code pattern is consistent: record ID → Drive or SharePoint folder → gallery filtered by ID. Pain points sit at the edges: renaming, performance at thousands of images, permissions for coworkers, and mobile upload reliability. That is evidence of workaround demand. Teams with general tools are rebuilding a case-folder photo system themselves.

### Gaps
- I found no Glide, Notion or FileMaker threads in the time available.
- No Power Apps thread text was fetched (snippet only).

## (c) "Looking for software" posts

### Takeaway
I found few verifiable "looking for software" posts. Reddit was unreachable, and HN/Indie Hackers searches returned only vendor or job posts. The verified examples below come from niche trade forums and show the same need: tagging, search across the team, and organization per job.

### Cited Findings
- **Core77 (Dec 31, 2013, older than the preferred window).** Verbatim: "My work is looking for a solution that would allow our sales team to look through a searchable database of photos" … "Do any of you know of a solution that would allow our department to upload a library of photos and easily tag them" … "so that we can securely search them on the go?" On Dropbox: "As far as I know you can't tag images in Dropbox." The team ended up using Squarespace. **[FETCHED]** — [Core77 boards](https://boards.core77.com/t/photo-database/29402)
- **Signs101 (Mar 2025).** See section (a). The shop is looking for "a streamlined way to be able to upload photos from the field" after CompanyCam and Jobber did not fit. **[FETCHED]** — [Signs101](https://www.signs101.com/threads/job-completion-photos.179718/)
- **Hacker News (Algolia API search, multiple queries).** No Ask HN request for team or job photo management turned up. Hits were CompanyCam hiring posts (2015–2020) and one "Show HN: Document jobsite images quickly and easily" (2023-07-10), which is a supply-side launch. **[FETCHED via API]** — [HN item 36673142](https://news.ycombinator.com/item?id=36673142)

### Inferences
- Many new indie entrants exist: FieldFrame, WorkPhotos, ClaimCam, RelayCam/CrewCam, ConstruktApp, Fieldpost, PictaBase and others turned up across searches. Builders evidently see the gap. The supply of small tools is crowded relative to the visible written demand.

### Gaps
- Reddit (r/Construction, r/HVAC, r/Dentistry, r/msp, r/SaaS, r/software, r/SoftwareRecommendations) could not be accessed. This is the largest gap in this note.
- No Product Hunt or Indie Hackers "looking for" threads were found.

## (d) Review complaints about existing products

### Takeaway
Across CompanyCam, Fulcrum, SiteCapture and RxPhoto, complaints group into four themes:
1. Per-seat price.
2. Clunky tagging or labeling and limited search.
3. Organization model mismatches, such as client vs. job vs. property, or no merge of property profiles.
4. Photo export and download friction.

Most review-site text is snippet-level because Capterra, GetApp, G2 and TrustRadius block fetching. Only SoftwareFinder could be fetched.

### Cited Findings
**CompanyCam**
- Price, April 2021: "The annual plan costs $192 per user which feels a bit expensive for what we get." **[FETCHED]** — [SoftwareFinder](https://softwarefinder.com/field-service/companycam/reviews)
- Labeling, December 2020, 11–50 employees: "There are some annoying glitches with the labeling feature that I find frustrating to deal with." **[FETCHED]** — [SoftwareFinder](https://softwarefinder.com/field-service/companycam/reviews)
- Sharing, November 2019: "The camera feature is a bit complicated when trying to move photos from the site to a regular business email." **[FETCHED]** — [SoftwareFinder](https://softwarefinder.com/field-service/companycam/reviews)
- Integration, November 2021: "CompanyCam doe not work with our existing ERP system." **[FETCHED]** — [SoftwareFinder](https://softwarefinder.com/field-service/companycam/reviews)
- Navigation, December 2024: "takes me all the way to the beginning instead of where I left off." **[FETCHED]** — [SoftwareFinder](https://softwarefinder.com/field-service/companycam/reviews)
- Search: a Capterra reviewer said search is "sometimes very limited" and wished for "contains" or "starts with" matching. **[SNIPPET]** — [Capterra p.2](https://www.capterra.com/p/171143/CompanyCam/reviews/?page=2)
- Tagging UX: a GetApp user found tagging unintuitive and kept editing a job's tags instead of searching by them. **[SNIPPET]** — [GetApp](https://www.getapp.com/operations-management-software/a/companycam/reviews/)
- Property profiles: a Capterra owner wanted to merge two properties, because after a house sells a new profile is created with no visible link to the old one. **[SNIPPET]** — [Capterra](https://www.capterra.com/p/171143/CompanyCam/reviews/?page=3)
- Price: Capterra reviewers wrote "too expensive for what it does" and only pay for seats "for people who really need them." Another called it expensive "for the number of field techs we have." Listed prices vary: Capterra from $99 per user per month, GetApp $63 per user per month, G2 Pro $79 for 3 users plus $29 per extra user. **[SNIPPET]** — [Capterra](https://capterra.com/p/171143/CompanyCam/); [G2 compare](https://www.g2.com/compare/companycam-vs-fieldlens-by-redteam)
- Overlap: a reviewer questioned paying for CompanyCam when their project-management software already has a photos function. **[SNIPPET]** — [Capterra](https://www.capterra.com/p/171143/CompanyCam/reviews/?page=4)
- Organization model, Signs101, Mar 2025: CompanyCam "saves all of the photos based on the client not the job." **[FETCHED]** — [Signs101](https://www.signs101.com/threads/job-completion-photos.179718/)

**SiteCapture (listed as FotoNotes on Capterra)**
- November 2024: projects often did not appear when searching by keyword, including customer names or addresses. Another reviewer was frustrated finding entries. **[SNIPPET]** — [Capterra SiteCapture](https://www.capterra.com/p/128652/FotoNotes/reviews/)

**Fulcrum**
- April 2026: a sharp price increase led a long-time user to cancel, calling it overpriced. Another reviewer warned about lock-in as prices keep rising. A 2020 reviewer complained that exporting all photos for specific records produced a single zip in one folder. Another said "downloading photos is a hassle." A December 2025 reviewer said photos load slowly in the mobile app. **[SNIPPET]** — [Capterra Fulcrum](https://www.capterra.com.au/reviews/136604/fulcrum); [Capterra Fulcrum p.2](https://www.capterra.com/p/136604/Fulcrum/reviews/?page=2)

**RxPhoto (clinical)**
- June 2026: "The interface can take time to learn, and organizing or accessing certain photos may require more steps than expected." **[SNIPPET]** — [Capterra RxPhoto](https://capterra.com/p/201853/RxPhoto/)
- July 2024: "The monthly subscription cost can be a little on the higher side but as you grow you photo library it does become reasonable." **[SNIPPET]** — [Capterra RxPhoto (CA)](https://www.capterra.ca/software/201853/rxphoto)

**WorkPhotos**
- A reviewer said sharing individual images was limited, and the developer replied that improvements were coming. **[SNIPPET]** — [GetApp UK WorkPhotos](https://www.getapp.co.uk/software/2072332/workphotos)

**Adobe Lightroom shared albums (team search)**
- Dec 3, 2021: "But, search doesn't work for shared folders." Accepted answer: "only the 'owner' of the shared album can add metadata (such as keywords…)". Jan 27, 2022: "It makes no sense not to be able to search a large shared library per keywords." Oct 27, 2023: "It is ridicukous that other people cannot search my album by tag." Jan 29, 2026: "This would be so convenient with my team and I to do photo selects at the same time on our own machines." **[FETCHED]** — [Adobe Community](https://community.adobe.com/t5/lightroom-ecosystem-cloud-based-discussions/how-to-manage-and-search-shared-albums/m-p/12710588)

### Inferences
- Per-seat pricing ($192 per user per year up to about $99 per user per month quoted) is the most frequent complaint. It pushes teams to buy fewer seats, so not every field worker gets access. That works against the "whole-team capture" value proposition.
- Search and tagging complaints are real but less frequent than price complaints. They center on weak text matching (customer name or address not found), clunky tag UI, and shared or team libraries where non-owners can't search by tag.
- The organization model is a distinct and underserved pain: client vs. job vs. property vs. multi-site client. Users cite it for CompanyCam, Jobber and Signs101 workflows.

### Gaps
- I could not extract verbatim Capterra, G2 or GetApp text (403). Snippet-level items should be re-verified manually before quoting in a final report.
- No App Store or Google Play review text was retrievable (the App Store page returned no reviews).
- I found no review complaints for Canfield, Epic Haiku, Eagle or Google Photos for business specifically.
- I found no complaints about AI tagging accuracy in the sources reached.

## (e) Feature request boards

### Takeaway
Dropbox, Google Photos, OneDrive and Adobe users have long asked for content search, people search, keyword or tag search in shared libraries, and date-range search in photo views. Vendors respond slowly or partially. Most requests are consumer-framed, but some explicitly mention team use.

### Cited Findings
- **Dropbox.** Staff said that "tagging specific people to then search for them isn't possible on the Dropbox service." They pointed to image search for generic objects like 'cat' or 'tree', available only on certain plans. One user said they would leave a paid plan for Google Drive if person search were added. **[SNIPPET]** (page is JS-rendered) — [Dropbox Community](https://dropboxforum.com/discussions/101001013/can-i-tag-people-in-photos-and-have-dropbox-recognize-them-for-search-purposes/750561)
- **Dropbox idea "Photo AI Index search & Detection"** (status Open). The user asks to select a face and find all photos of that person, and wants the same for objects. **[SNIPPET]** — [Dropbox Ideas](https://www.dropboxforum.com/idea/101002013/photo-ai-index-search--detection/852796)
- **Dropbox idea "Enhance the Photos Tab with Search and Advanced Filters".** Per the snippet: "when I switch to Photos I lose the ability to quickly locate images by keyword, filename, folder name, or a specific date range." The fetch confirmed the page title. **[SNIPPET + title FETCHED]** — [Dropbox Community](https://community.dropbox.com/en/discussion/comment/864551)
- **Dropbox, 2018-era keyword search request.** Closed for inactivity. Staff said tags were not supported because they could not sync across operating systems. **[SNIPPET]** — [Dropbox forum](https://dropboxforum.com/discussions/101001013/image-searchfacial-recognitionkeyword/314891)
- **Google Photos (Aug 2024).** The Google Photos Team introduced a "Collections view", saying it had heard users "loud and clear" that finding shared albums was difficult. The page body was not retrievable. **[SNIPPET]** — [Google Photos Help](https://support.google.com/photos/thread/289829291/introducing-the-collections-view-find-what-you-need-faster)
- **Google Photos.** A user sharing with relatives complained of "no ability to search within an album and no tags/keywords." **[SNIPPET]** — [ExifTool forum](https://exiftool.org/forum/index.php?msg=62126)
- **Adobe Lightroom shared-album keyword search.** Users have asked for this since 2021 and were still asking in Jan 2026 (see section d). **[FETCHED]** — [Adobe Community](https://community.adobe.com/t5/lightroom-ecosystem-cloud-based-discussions/how-to-manage-and-search-shared-albums/m-p/12710588)
- **Jobber per-property photo attachments.** Support acknowledged the request and passed it to development (see section b). **[SNIPPET]** — [Jobber Community](https://community.getjobber.com/discussions/job-details-scheduling/how-to-manage-photos-for-clients-with-multiple-properties-in-jobber/8086)

### Inferences
- General-purpose storage tools (Dropbox, OneDrive, Google Photos) have repeatedly declined or removed tagging, and they gate content search by plan. That leaves a durable gap for "tag plus search plus team" on business photo libraries. Vertical tools cover capture and organization per job but get criticized on price and search.

### Gaps
- I could not reach the CompanyCam feature-request board or community. I found no public CompanyCam ideas board.
- I have no vote counts for the Dropbox ideas (JS-rendered pages).
- I found nothing for Eagle, Canfield or Epic Haiku.
