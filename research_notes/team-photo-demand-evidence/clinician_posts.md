# First-person demand evidence: clinicians and clinic operators wanting to capture, organize, search and team-share patient photos

**Researcher note on method and coverage (read first):** Reddit could not be reached by any route. The web search tool refuses `reddit.com` ("domains are not accessible to our user agent"), direct fetches of reddit.com and old.reddit.com return a block page or a redirect, and the Pullpush archive API returned "This website does not provide free scraping resources for agents", so I stopped using it. Spiceworks is also refused by the search tool, and its own site search returned nothing relevant. allnurses.com returns HTTP 403 to every fetch method, so allnurses items below are **search-snippet only**. Student Doctor Network (SDN), Apple Community and Dentaltown pages were fetched and their quotes checked word for word. **I did not reach the 15–30 item target with strong, recent posts:** there are 14 first-person items, and most of the verified ones are from 2009–2021. Reddit, where most recent demand probably sits, is the main hole. The "Gaps" sections list Reddit searches a human should run by hand.

Verification levels: **[FETCHED]** means the quote was checked against the fetched page HTML. **[SNIPPET]** means the quote is from search-engine summary text only, so it may be paraphrased, and the date is unknown.

---

## Q1. Catalog of verifiable posts and threads (URL, platform, date, poster, verbatim quote, current tools, replies, willingness to pay)

### Takeaway
Of everything reachable, the clearest operator-side demand is a 2024 Apple Community post from a clinic that photographs **every patient at every visit** and needs those photos **"immediately available"** to the treating doctors on iPads. It found no compliant answer: the only reply says iCloud Photos "would not be HIPPA compliant." The other items show the same pattern: personal phones are banned or discouraged, and departments fall back on dedicated cameras, EHR-tethered iPads, DSLRs, or print-and-delete. No poster in any item named a price or said they would pay.

### Cited Findings

**1. Apple Community, "Is it possible to save photos directly to iCloud using…" — 5 Feb 2024 [FETCHED]**
- Poster: "Jonathanvdb", operator or staff at a clinic (size not given). Setup: "The doctors use iPads."
- Pain: "we have a clinic where each patient needs to be photographed during every visit." / "It is crucial that these photos are immediately available for the doctors responsible for the patient's treatment." / Asks "whether there is an external digital camera available that can directly save photos to iCloud" "via a wifi connection."
- Current tool: iPads, plus an iCloud workaround they were considering.
- Only reply (muguy, Level 10): "Just the camera in an iPhone or iPad." … "If the photos that you are taking are protected data, using iCloud Photos would not be HIPPA compliant." The thread was then closed and the problem was never solved.
- Willingness to pay: not stated. — [Apple Community 255459485](https://discussions.apple.com/thread/255459485)

**2. Apple Community, "iCloud Conditions require no PHI to use iCloud" — 8 Nov 2021 [FETCHED]**
- Poster: "Sunshine_777" (iPhone 12), role not stated. Asks: "How can iPhone users who are covered entities prevent PHI from using iCloud?" and "How do doctors use iPhones to receive patient texts, voicemails, etc. if the Terms And Conditions require the doctor to NOT use the cloud for PHI?"
- Reply (Michael Black): "The doctors I know who use iPhones or iPads in their practices do NOT use iCloud on those devices for anything related to their patients or their work." Reply (Halliday): doctors "would, likely, be best served by using an email service provided as a part of their practice/organization."
- Relevance: the default consumer sync path (iCloud/Photos) is off-limits for PHI, which pushes clinicians toward something else. — [Apple Community 253343858](https://discussions.apple.com/thread/253343858)

**3. SDN, "Should ORs have hidden cameras?" — 17 May 2011 — "Winged Scapula" (attending; specialty not given) [FETCHED]**
- "I'm not even allowed to take a photo of a patient with my cell phone (they even tried to prohibit photos at all until I pointed out that their own consent form allowed for it)."
- Relevance: hospital policy bans phone capture outright, and the clinician still wants to take photos. — [SDN thread 822048](https://forums.studentdoctor.net/threads/should-ors-have-hidden-cameras.822048/)

**4. SDN, "Video recording of procedures" — 30 Aug 2013 — "cpants" (surgeon or proceduralist; does laparoscopic/endoscopic work) [FETCHED]**
- "The recording must be kept as part of the official medical record, and it must be kept secured. Recording it on your iphone--not cool. Using the OR light camera and putting it on a DVD kept in a locked file cabinet--ok. We take pictures all the time especially during laparascopic/endoscopic procedures. Very useful to show patients their pathology and for the medical record."
- Current tool: OR light camera, then DVD, then a locked file cabinet, which is a very manual storage path. — [SDN thread 1029364](https://forums.studentdoctor.net/threads/video-recording-of-procedures.1029364/)

**5. SDN, "app for keeping patient info organized" — 1–2 Jun 2014 — "SouthernSurgeon" [FETCHED]**
- "I know a number of hospitals (mine included) that have an explicit policy that patient information cannot be stored on your mobile device (phone or iPad). We can access our EMR from our phones through a secure network, but we aren't allowed to save anything on the phone." / "HIPAA compliance is a minimum benchmark; for a lot of hospitals their privacy guidelines go well above and beyond that."
- Context: the original poster (student, 16 May 2014) wanted an iPhone app to organize patient info. Replies recommended pen and paper or Evernote. This is about patient data in general, not photos specifically. — [SDN thread 1071771](https://forums.studentdoctor.net/threads/app-for-keeping-patient-info-organized.1071771/)

**6. SDN, "Dermatoscope phone adaptor" — 27–28 Apr 2020 — dermatology residents [FETCHED]**
- "dermie1985": "Use mine everyday! … Only annoying part is you have to buy a new one whenever you upgrade your phone."
- "Dral": "it cost about $35 or $40 bucks (super overpriced for a glorified phone cover imo). However, I did find it useful."
- "asmallchild": "My program actually purchased individual dermlites for us for clinic use. However, they also required that we use the clinic's fancy DSLR with dermoscopy setup when it came to actually taking photos for conference / publications / etc"
- Relevance: dermatology residents capture on their **personal phones** every day. The only price signals are for capture hardware (about $35–40, which one resident called "overpriced"), not for software. — [SDN thread 1407957](https://forums.studentdoctor.net/threads/dermatoscope-phone-adaptor.1407957/)

**7. SDN, "Could teledermatology help with clinical efficiency?" — 30 Jan–2 Feb 2019 — practising dermatologists [FETCHED]**
- "asmallchild" (dermatologist in a group practice): "the platform we use utilizes a mobile app which in turn uses the smartphone camera. Lots of variation in terms of image quality, focus quality, and background lighting quality."
- "doctalaughs" (telederm experience): telederm works "assuming the PCP is sending the photos with some sort of quality assurance program. Patients sending photos is trash and doesn't work due to lack of QA." Also: "The main problem is still 1) reimbursement and 2) setting up the large QA process … Bigger problem in a fractured system like private practice".
- Another poster ("HDelVe") named FotoFinder, FirstDerm and DermEngine as cloud-storage telederm tools they were looking into.
- Relevance: the job here is consistent capture and quality-controlled sharing between PCP and dermatologist, not storage alone. — [SDN thread 1363391](https://forums.studentdoctor.net/threads/could-teledermatology-help-with-clinical-efficiency.1363391/)

**8. SDN, "The Best Camera for Photographing Skin lesions is?????" — Feb 2009 — derm applicant, residents, a dermatologist ("sensitive skin | derm") [FETCHED, old]**
- "sensitive skin": "You also need a standard background for all photos -- that is, dark cloth for contrast. Use the same setup for each photo." / "On derm rotations, I noticed that almost all residents carry small digital cams ("point-and-clicks") in their pockets for clinic and consult."
- "DermMatch": "clinic is hella busy and whipping out your digital fancy pantsy SLR is not that easy if it doesn't fit into your pocket."
- Original poster (IzzyMD09): "this is a camera for the department for taking pictures of our patients enrolled in clinical trials".
- Relevance: the need for standardized, repeatable capture (for before/after comparison) predates smartphones. — [SDN thread 602322](https://forums.studentdoctor.net/threads/the-best-camera-for-photographing-skin-lesions-is.602322/)

**9. SDN, "Technology - Dermatology apps and devices" — 12 Aug 2015 — "NSight8" [FETCHED]**
- "I like the MoleScope, which is an iPhone attachable dermatoscope that's actually affordable. It also comes with a pretty cool app that lets you track your or your patient's moles and monitor them for change."
- Relevance: tracking lesions over time (longitudinal comparison) is the job this poster values. — [SDN thread 1155485](https://forums.studentdoctor.net/threads/dermatology-apps-and-devices.1155485/)

**10. SDN, "Ultrasound Pictures" — 29–30 Dec 2012 — EM physicians and a medical student [FETCHED]**
- "DoctorTinyCat" (medical student): "How do you guys feel about letting a patient use their cell phone to snag a shot of the image? … as a medical student, I have seen both attendings and residents allow it."
- "Hamhock" (EM): "all ED ultrasound images are electronically transferred to PACS -- haven't been 'printable' for years."
- Relevance: this one is only loosely related (it is about patients' own access to images). — [SDN thread 973539](https://forums.studentdoctor.net/threads/ultrasound-pictures.973539/)

**11. Dentaltown, "Managing Dental Images On Your Computer" — blog post dated 6/2/2016, quoting a Dentaltown message-board reply from a dentist [FETCHED]**
- The quoted dentist, replying to "What is keeping you from implementing intra-oral photography in your practice?": "my issue is getting them [dental images] on the computer and into some kind of database quickly, ideally your regular imaging or practice management software. Last time I tried to load extraoral images into Eaglesoft imaging it took forever and the pics were so slow to load it wasn't practical. It should be as easy as taking a bitewing. Does open dental or sidexis 4 make this process any easier?"
- The blogger, a dental-photography consultant, describes the typical manual workflow: import from the camera, "(Hopefully) placing them in a folder where you can find them", "(Hopefully) renaming them so you can tell which patient the image is associated with", then importing into practice software.
- Current tools: DSLR or intraoral camera, desktop folders, Eaglesoft, Open Dental, Sidexis. — [Dentaltown blog post 4873](https://www.dentaltown.com/blog/post/4873/managing-dental-images-on-your-computer)

**12. allnurses, "Picture of wound?" (General Nursing) — date unknown [SNIPPET only; page returns 403]**
- From search summaries: a home-health commenter describes an iPad linked to the EHR that "will automatically upload the pic into that persons chart", and says a colleague was fired for taking wound photos on a personal phone because management said it "violated facility policy." Another commenter: "We have an app at our facility (I personally don't need it) that can take pictures on encrypted phones that can be inserted into progress notes." Another describes consent being obtained first and identifying patients by "only initials and the medical record number." — [allnurses t673081](https://allnurses.com/picture-wound-t673081/)

**13. allnurses, "Sending MD pictures of wounds on cell phone?" (Wound, Ostomy, Continence forum) — date unknown (search tool said the threads it found were mostly 2007–2014) [SNIPPET only]**
- From search summaries: a WOC nurse says their department "has two cameras, takes photos, prints them off, and deletes them." Other replies: a photo should not be texted to a provider with patient identifiers, and "texts are typically not secure forms of communication." One reply says that because a work photograph "is owned by your employer, and a part of the medical record, it must be taken on agency-owned equipment."
- Relevance: the need is for a nurse to send a wound image to a physician quickly. — [allnurses t456526](https://allnurses.com/sending-md-pictures-wounds-cell-t456526/)

**14. allnurses, "Taking Photos on Your Mobile Phone in a Aged Care Facility" and "Is taking this photo a HIPAA violation? (Ask Nurse Beth)" — dates unknown [SNIPPET title only; content not verified]**
- These were found only as titles. They show recurring nurse questions about phone photos in long-term or aged care, but I have no verifiable quotes from them. — [allnurses t189737](https://allnurses.com/taking-photos-your-mobile-phone-t189737/); [allnurses t762361](https://allnurses.com/is-taking-photo-hipaa-violation-t762361/)

**Context sources (not first-person posts; use as framing only):**
- An NYU Langone photography policy says clinical photos "should only be taken using department cameras, Medical Center Information Technology (MCIT) issued or encrypted devices, secure MCIT approved applications, such as Epic Haiku or Canto." This is an example of the institutional rule that creates the pain. [SNIPPET] — [NYU Langone policy PDF](https://nyulangone.org/files/photography-policy.pdf)
- A Stanford/PLOS ONE pilot compared a wound-photography workflow in Epic Haiku with Google Glass, noting that Haiku photos "are not stored on the phone's memory." [SNIPPET] — [PLOS ONE 2015](https://journals.plos.org/plosone/article?id=10.1371%2Fjournal.pone.0121179)
- A 2022 survey of Saudi physicians: 147 (70%) took clinical photographs, 79% of them on personal smartphones, and only 28.6% stored images in a separate password-protected folder. [SNIPPET; article not fetched] — [DOAJ, J Nature & Science of Medicine 2022](https://doaj.org/article/8510973db10c4034a76947dc3a0bd4f4)
- A survey of medical photography use among doctors at a Portuguese hospital (MDPI IJERPH 2022). [Title only; not fetched] — [MDPI 19/12/7304](https://www.mdpi.com/1660-4601/19/12/7304/html)
- A vendor blog cites "a 2016 survey where 57% stored clinical photos on their phone (Chan et al., Plastic Surgery, 2016)". This is second-hand through a vendor and was not verified. — [Baylor/BCM blog search result context](https://blogs.bcm.edu/2022/12/09/in-focus-patient-photography-and-the-medico-legal-considerations/)

### Inferences
- The strongest single demand signal is the 2024 clinic post (#1). It names the exact job: capture at every visit, available right away to the whole care team on iPads, and compliant. The consumer default (iCloud) is ruled out, and nobody in the thread pointed to a product. That gap matches a team photo-library product closely.
- The forums show two coping patterns. In hospitals, IT-sanctioned EHR capture apps (Haiku/Canto, iPads tied to the EHR) are in use and personal phones are banned, sometimes with firings. Outside them (small practices, residents, dermatology clinics), clinicians use personal phones, point-and-shoots, DSLRs and desktop folders. The second group looks like the more open market for a standalone product.
- The lack of any price or willingness-to-pay statement in these sources is itself a finding. Clinicians on these forums talk about policy and compliance, not about buying software. Buying-intent evidence would more likely come from Reddit's r/medicalpractice, r/HealthIT and r/msp, or from G2/Capterra reviews, none of which I could reach.

### Gaps
- **Reddit (the most important source) was completely inaccessible.** Searches a human should run on Reddit: `"clinical photos" app` in r/medicine, r/Residency, r/emergencymedicine, r/physicianassistant; `wound photos phone` in r/nursing and r/woundcare; `photo` in r/HealthIT, r/sysadmin, r/msp (IT staff asked to support clinical photos); `before and after photos software` in r/MedSpa, r/Esthetics, r/PlasticSurgery; `intraoral photos` in r/Dentistry; `photos` in r/VetTech and r/veterinary; `Haiku photos` in r/Residency. Fetching each thread at old.reddit.com from a normal browser would give full verification.
- The allnurses quotes could not be checked against the page (403), and their dates are unknown.
- Spiceworks: search refused, and its site search found no healthcare-IT thread asking for a clinical photo solution.
- LinkedIn, X, VIN (behind a login) and KevinMD: no first-person posts on this topic turned up in search.
- HN: searching comments through the Algolia API for "clinical photos", "patient photos", "wound photos", "HIPAA photos" and similar found no first-person clinician pain posts. The only loosely related hit was a 2026 hiring post from Deya, an eye-care startup capturing ocular images on smartphones ([HN 49528637](https://news.ycombinator.com/item?id=49528637)). That is a vendor, not demand evidence.
- No willingness-to-pay or price statements for photo software were found in any first-person post.

---

## Q2. Which specialties and settings complain most, and which jobs come up most

### Takeaway
In what I could reach, **dermatology** (capture consistency, tracking lesions over time, telederm quality control) and **nursing wound care** (sending wound photos to MDs, keeping them off personal phones, getting them into the EHR) show up most, followed by **surgery** (secure storage of intraoperative images) and **dentistry** (slow import into imaging or practice-management software). The jobs that recur most are: keeping photos off personal phones and consumer clouds; getting images into the chart or a shared place quickly; consistent or standardized capture for comparison; and team sharing (nurse to MD, PCP to dermatologist, clinic staff to doctors on iPads). "Can't find old photos" shows up only indirectly, as the dentist's folder-and-rename workflow.

### Cited Findings
- **Keeping photos off personal phones / policy bans:** a hospital barred phone photos and tried to ban photos entirely — [SDN 822048](https://forums.studentdoctor.net/threads/should-ors-have-hidden-cameras.822048/). "Recording it on your iphone--not cool" — [SDN 1029364](https://forums.studentdoctor.net/threads/video-recording-of-procedures.1029364/). A hospital policy says nothing may be saved on phones — [SDN 1071771](https://forums.studentdoctor.net/threads/app-for-keeping-patient-info-organized.1071771/). A nurse was reportedly fired over personal-phone wound photos [SNIPPET] — [allnurses t673081](https://allnurses.com/picture-wound-t673081/).
- **Consumer cloud ruled out:** iCloud Photos "would not be HIPPA compliant" — [Apple 255459485](https://discussions.apple.com/thread/255459485). Doctors "do NOT use iCloud on those devices for anything related to their patients" — [Apple 253343858](https://discussions.apple.com/thread/253343858).
- **Immediate team access and sharing:** "immediately available for the doctors responsible for the patient's treatment" — [Apple 255459485](https://discussions.apple.com/thread/255459485). Nurses sending wound photos to MDs [SNIPPET] — [allnurses t456526](https://allnurses.com/sending-md-pictures-wounds-cell-t456526/). PCP-to-dermatologist photo QA — [SDN 1363391](https://forums.studentdoctor.net/threads/could-teledermatology-help-with-clinical-efficiency.1363391/).
- **EHR / practice-software upload pain:** "getting them … into some kind of database quickly … Eaglesoft imaging it took forever … It should be as easy as taking a bitewing" — [Dentaltown 4873](https://www.dentaltown.com/blog/post/4873/managing-dental-images-on-your-computer). An iPad tied to the EHR that auto-uploads is described as the good outcome [SNIPPET] — [allnurses t673081](https://allnurses.com/picture-wound-t673081/).
- **Standardized capture for before/after and longitudinal comparison:** "standard background for all photos … Use the same setup for each photo" — [SDN 602322](https://forums.studentdoctor.net/threads/the-best-camera-for-photographing-skin-lesions-is.602322/). An app to "track … moles and monitor them for change" — [SDN 1155485](https://forums.studentdoctor.net/threads/dermatology-apps-and-devices.1155485/). "Lots of variation in terms of image quality, focus quality, and background lighting quality" — [SDN 1363391](https://forums.studentdoctor.net/threads/could-teledermatology-help-with-clinical-efficiency.1363391/).
- **Consent:** one surgeon used the hospital's own consent form to overturn a photo ban — [SDN 822048](https://forums.studentdoctor.net/threads/should-ors-have-hidden-cameras.822048/). Consent obtained first, and only initials plus the medical record number used [SNIPPET] — [allnurses t673081](https://allnurses.com/picture-wound-t673081/).
- **Tools in use today:** personal phones with DermLite adapters, point-and-shoots, clinic DSLRs ([SDN 1407957](https://forums.studentdoctor.net/threads/dermatoscope-phone-adaptor.1407957/), [SDN 602322](https://forums.studentdoctor.net/threads/the-best-camera-for-photographing-skin-lesions-is.602322/)); the OR light camera with DVDs ([SDN 1029364](https://forums.studentdoctor.net/threads/video-recording-of-procedures.1029364/)); department cameras with print-and-delete [SNIPPET] ([allnurses t456526](https://allnurses.com/sending-md-pictures-wounds-cell-t456526/)); iPads ([Apple 255459485](https://discussions.apple.com/thread/255459485)); Epic Haiku/Canto as the sanctioned hospital route ([NYU policy](https://nyulangone.org/files/photography-policy.pdf)). WhatsApp, Google Photos and Dropbox were **not** mentioned in any first-person item I could verify.

### Inferences
- Combining "can't use my phone or iCloud" with "need it in front of the team now" points to a product that captures in-app (nothing in the camera roll), stores in a shared, BAA-backed library, and lets a whole team find and view the photos on iPads and phones. Hospital users may already be covered by Haiku/Canto. Independent clinics, dermatology and aesthetic practices, wound-care and home-health teams, and dental practices seem underserved, judging by the workarounds they describe.
- Before/after and longitudinal comparison needs standardized capture (same background, lighting and angle), which suggests capture guides and overlays are part of the job, not an extra.

### Gaps
- I could not measure how often each specialty complains, because the sample is small and Reddit is missing. ED, orthopedics, PA and family medicine produced no verifiable first-person items.
- "Can't find old wound photos" (search and retrieval) as a stated pain was not found verbatim in any reachable source.
- WhatsApp use, common outside the US, was not evidenced by any first-person post I could reach.

---

## Q3. Veterinary and dental equivalents

### Takeaway
The dental evidence is real but thin and dated. A dentist names the import-into-practice-software bottleneck in specific terms (Eaglesoft "took forever"), and the standard workflow is manual: folders, renaming files by patient name, then import. I found no verifiable first-person veterinary posts.

### Cited Findings
- Dentist (via the Dentaltown message board, republished 6/2/2016): "my issue is getting them [dental images] on the computer and into some kind of database quickly … Eaglesoft imaging it took forever and the pics were so slow to load it wasn't practical. It should be as easy as taking a bitewing. Does open dental or sidexis 4 make this process any easier?" [FETCHED] — [Dentaltown 4873](https://www.dentaltown.com/blog/post/4873/managing-dental-images-on-your-computer)
- A 2002 Dentaltown magazine reprint of a message-board thread ("What software do you suggest for dental image manipulation?"): one dentist saved images "in their individual folders with the patients' names", and others pointed to Dentrix's imaging module so as to "simply manage one database." [SNIPPET; old] — [Dentaltown Dec 2002 PDF](https://www.dentaltown.com/images/dentaltown/magimages/dec02/dec02dtpg58.pdf)
- A Dentaltown CE course on clinical photography had a reviewer asking for more on "tips for efficient file transfer and organization of image library." [SNIPPET] — [Dentaltown CE 287](https://www.dentaltown.com/onlinece/details/287/picture-perfect-clinical-photography-made-easy)
- Dentaltown's terms bar members from posting PHI, photos included, on its boards, which limits how much case-photo sharing happens there. [SNIPPET] — [Dentaltown Terms](https://www.dentaltown.com/terms.aspx)
- Veterinary: the only item found was a third-party subreddit roundup claiming r/vettech members "frequently share photos of interesting cases". That is not first-person and not verified. [SNIPPET] — [painonsocial.com roundup](https://painonsocial.com/subreddits/veterinary-technicians)

### Inferences
- In dentistry, the pain is mainly getting DSLR or intraoral images into the practice-management or imaging system quickly, plus organizing them by patient and procedure. That matches "capture, organize, search" closely, but dental imaging bridges (Dentrix, Eaglesoft, Sidexis, Dolphin) are entrenched competitors.
- In veterinary practice, HIPAA does not apply, so the "keep it off my phone" compliance driver is likely weaker. Any vet demand would more likely be about organization and team sharing. This is unverified.

### Gaps
- No verifiable first-person veterinary posts: VIN is behind a login, and r/veterinary and r/VetTech could not be reached.
- No recent (2020+) Dentaltown message-board threads could be surfaced. Dentaltown forum search needs a login, and the search engine mainly returned magazine, blog and CE pages. r/Dentistry could not be reached.
- No dental or veterinary willingness-to-pay statements were found.
