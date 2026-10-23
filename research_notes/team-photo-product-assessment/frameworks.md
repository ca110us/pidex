# Product Evaluation Frameworks Applied to a Team Photo Case-Management App

Scope: frameworks for judging (a) whether a B2B SaaS need is worth building and (b) whether target users will "instantly" see it as worth paying for, applied to: a horizontal, English-first (US/EU) team photo case-management app (in-app camera, file-at-capture under a case, AI tag suggestions, filter + semantic + image-similarity search, shared team space, timeline / before-after), wedge = hospital departments (orthopedics, wound care, dermatology), ambition = other verticals + expert-labeled AI training data.

Source-quality note: framework definitions marked "(canonical, not re-fetched)" are standard author sources I did not open this session; everything else was retrieved during research (Oct 2026). Many benchmark numbers come from vendor blogs; this is flagged inline.

---

## 1. What do the leading frameworks say? (one-paragraph summary + the key test for each)

### Takeaway
Nearly all credible frameworks agree that the strongest early signal is evidence of an existing, frequent, painful job that people already spend time/money working around, held by a specific, narrowly defined customer, before building. Post-launch, the de facto quantitative test is the Sean Ellis / Superhuman "very disappointed" ≥40% score, measured on a segmented, actively-using cohort.

### Cited Findings

**Jobs-to-be-Done (Christensen) and Outcome-Driven Innovation (Ulwick / Strategyn)**
- Christensen's JTBD: customers "hire" a product to make progress in a particular circumstance; understanding the job (functional, social, emotional dimensions) and its circumstances predicts adoption better than customer demographics. Key test: what job, in what circumstance, and what is it currently "hired" to do it (incumbent / workaround)? — [HBR, "Know Your Customers' Jobs to Be Done" (canonical, not re-fetched)](https://hbr.org/2016/09/know-your-customers-jobs-to-be-done)
- Ulwick's ODI quantifies the job as "desired outcome statements" and scores each: Opportunity = Importance + max(Importance − Satisfaction, 0), which weights importance twice as heavily as the satisfaction gap — [ProductPlan glossary](https://www.productplan.com/glossary/opportunity-scoring); [Wikipedia: Outcome-Driven Innovation](https://en.wikipedia.org/wiki/Outcome-Driven_Innovation)
- Strategyn defines inputs as % of respondents rating an outcome very/extremely important and % rating it very/extremely satisfied; other sources use 1–10 averages (input conventions vary). Outcomes that are highly important and poorly satisfied are the value-creation opportunities — [Strategyn](https://strategyn.com/?p=17500); [ProductPlan](https://www.productplan.com/glossary/opportunity-scoring)
- Commonly quoted bands (>10 underserved, 6–10 appropriately served, <6 overserved) appear in secondary sources; I could not confirm them in Ulwick's own material — [Koji (secondary)](https://www.koji.so/docs/outcome-driven-innovation-odi)
- Critique: PDMA review argues subtracting satisfaction from importance mixes unlike quantities and that Ulwick "casually dismisses" the criticism — [PDMA review](https://www.pdma.org/review_what_customer)

**Painkiller vs. vitamin / "hair on fire"**
- The painkiller-vs-vitamin heuristic (painkillers solve an urgent, recognized pain and get budget; vitamins are "nice to have" and get cut) is folk wisdom in VC/startup writing; I did not retrieve a canonical primary source this session.
- "Hair on fire": I could not find a primary-source quote from Michael Seibel using this phrase; it appears in third-party "evaluate startup ideas" checklists. Do not attribute it to Seibel without a primary source — [third-party agent skill listing](https://llmbase.ai/skills/refoundai/evaluating-startup-ideas/)
- What Seibel verifiably argues (via YC): start from a problem you care about, not an idea; "Hold the problem you're solving tightly. Hold the customer tightly. Hold the solution you're building loosely." — [Antoine Buteau summary of Seibel](https://www.antoinebuteau.com/lessons-from-michael-seibel/); [YC blog interview with Seibel](https://www.ycombinator.com/blog/michael-seibel-on-starting-a-startup-finding-product-market-fit-and-fundraising)
- Jared Friedman (YC) "How to Get and Evaluate Startup Ideas": biggest mistake is "solution in search of a problem" (working backward from technology); avoid "tar pit" ideas (widespread problem, plausible solution, hidden structural problem); score ideas on opportunity size, founder-market fit, confidence the problem is real, and genuine insight; don't reject ideas just because they're "boring," "hard," or "competitors exist" — [MIT notes on Friedman talk](https://www.mit.edu/~edchen93/posts/how-to-get-and-evaluate-startup-ideas/); [Xsolla summary](https://accelerator.xsolla.com/blog/how-to-get-and-evaluate-startup-ideas); [onecerebral summary](https://onecerebral.beehiiv.com/p/what-makes-good-and-bad-startups-ideas)

**Pain frequency × intensity × budget**
- No single canonical author; it's a practitioner heuristic combining JTBD frequency, ODI importance/satisfaction, and "who owns budget." I found no primary source defining a formal score; treat as a synthesis lens (see Inferences).

**Sean Ellis 40% PMF test**
- Ask active users "How would you feel if you could no longer use this product?" (Very disappointed / Somewhat disappointed / Not disappointed). Ellis benchmarked ~100 startups and found "the magic number was 40%"; companies below that generally struggled to grow — [First Round Review, Vohra](https://review.firstround.com/how-superhuman-built-an-engine-to-find-product-market-fit/)
- Survey only users who used the product at least twice in the last two weeks; ~40 respondents is directionally useful ("much less than most people think"); survey each user only once — [First Round Review](https://review.firstround.com/how-superhuman-built-an-engine-to-find-product-market-fit/)

**Superhuman PMF engine (Rahul Vohra)**
- Four steps: (1) segment to find supporters and define the high-expectation customer (HXC, from Julie Supan); (2) analyze feedback to convert fence-sitters; (3) build the roadmap 50/50: half doubling down on what lovers love, half addressing what holds back "somewhat disappointed" users whose main benefit matches the lovers'; (4) repeat and make PMF score the top metric. Follow-up questions: who would most benefit; main benefit; how can we improve — [First Round Review](https://review.firstround.com/how-superhuman-built-an-engine-to-find-product-market-fit/)
- Trajectory: 22% → 33% just by segmenting to the right persona → 58% after ~three quarters of work — [First Round Review](https://review.firstround.com/how-superhuman-built-an-engine-to-find-product-market-fit/)
- Score moves when expanding into new segments; PMF isn't one-time — [Superhuman blog version](https://blog.superhuman.com/how-superhuman-built-an-engine-to-find-product-market-fit/); [underscore.vc summary](https://underscore.vc/?p=1560)

**The Mom Test (Rob Fitzpatrick)**
- Talk about their life, not your idea; ask about specifics in the past, not hypotheticals. Compliments are bad data; "fluff" (generic "I always," future "I would," hypothetical "I might") is unreliable — "I would definitely buy that" is the most dangerous. Redirect to the last time the problem happened, what they tried, what it cost — [mtlynch book report](https://mtlynch.io/book-reports/the-mom-test/); [Unusual VC](https://www.unusual.vc/rob-fitzpatricks-mom-test/); [Readingraphics](https://readingraphics.com/book-summary-the-mom-test/)
- A meeting succeeds only if it ends in commitment/advancement: the customer gives up something of value (time, reputation, money), e.g., intro to the decision-maker, a pilot, a letter of intent, pre-payment — [Yevgeniy Brikman review](https://www.ybrikman.com/blog/2023/03/29/the-mom-test/); [mtlynch](https://mtlynch.io/book-reports/the-mom-test/)

**Van Westendorp Price Sensitivity Meter**
- Four questions: at what price is it too expensive to consider; expensive but still worth considering; a bargain; so cheap you'd doubt its quality. Cumulative curves intersect to give point of marginal cheapness, point of marginal expensiveness, optimal price point, and indifference price point; method from 1976 — [Wikipedia](https://en.wikipedia.org/wiki/Van_Westendorp%27s_Price_Sensitivity_Meter); [Metricgate](https://metricgate.com/calculator/van-westendorp-price-sensitivity-meter)
- Limitation: measures perceived price acceptability, not actual willingness to pay; Newton/Miller/Smith extension adds purchase-probability questions; Verint reports up to 20% of paper responses discarded for failing validation — [Wikipedia](https://en.wikipedia.org/wiki/Van_Westendorp%27s_Price_Sensitivity_Meter); [Verint](https://verint.com/blog/the-van-westendorp-price-sensitivity-meter)

**Value Proposition Canvas (Strategyzer)**
- Customer profile (jobs, pains, gains) mapped to value map (products/services, pain relievers, gain creators); "fit" when value map addresses the most important jobs, extreme pains, and essential gains — [Strategyzer (canonical, not re-fetched)](https://www.strategyzer.com/library/the-value-proposition-canvas)

**Marty Cagan's four big risks (SVPG)**
- Value risk ("whether customers will buy it or users will choose to use it," owner: PM); usability risk (designer); feasibility risk (lead engineer); business viability risk ("whether this solution also works for the various aspects of our business," owner: PM). Strong teams tackle the big risks early, especially value and business risk; value risk is usually the toughest — [SVPG, The Four Big Risks](https://www.svpg.com/four-big-risks/)

**Kano model**
- Must-be (absence causes dissatisfaction; presence doesn't delight), performance/one-dimensional (more is better, linear), attractive/delighters (delight when present, not missed when absent); plus indifferent/reverse/questionable. Delighters decay into performance and then must-be over time — [Learning Loop](https://learningloop.io/plays/kano-analysis); [Kano model substack](https://kanomodel.substack.com/p/the-kano-model-in-a-nutshell)

### Inferences
- The frameworks split into two families: pre-build evidence tests (JTBD/ODI, Mom Test, VPC, Cagan value risk, Friedman/Seibel problem-first) and post-build measurement (Ellis 40%, Superhuman engine, Kano, Van Westendorp). For a pre-build idea, the first family should be the gate.
- A practical composite "worth building" score: Frequency (how many photo events per clinician/week) × Intensity (consequences of a lost/misfiled/non-compliant photo) × Budget (is there a line item or compliance mandate that pays). This is a synthesis, not a published formula.

### Gaps
- No primary source confirmed for "hair on fire" attributed to Seibel or for the painkiller/vitamin origin.
- ODI score bands not confirmed in Ulwick's primary text.
- JTBD, VPC definitions taken from canonical author sources without re-fetching.

---

## 2. Which signals predict "instant willingness to pay" in B2B, and what are the benchmarks?

### Takeaway
"Instant" B2B willingness to pay is predicted by (1) money/time already spent on a workaround or incumbent, (2) a single budget owner who is also close to the pain, (3) compliance/risk pressure that makes the status quo indefensible, and (4) sub-session time-to-value. Benchmarks show self-serve B2B free trials convert at only ~4–10% median (no card), ~25–35% (card required), with a bimodal distribution, so "instant" is the exception rather than the rule.

### Cited Findings

**Trial-to-paid benchmarks (2025–2026)**
- Kyle Poyar / ChartMogul / Growth Unhinged ProductLed SaaS Conversion Report (Jan 2026, 200 B2B products): median free-trial conversion 8%; bimodal—20% of trial products convert <2.5%, 23% convert >25%; ~10x gap between top and bottom 20%; conversion defined as signup → paid within 6 months — [Userpilot summarizing the report (updated Aug 2026)](https://userpilot.com/blog/free-trial-conversion-rate/)
- ChartMogul 2026 "good/great" (50th/75th pct): free trial no card 4–6% / 10–15%; free trial card required 25–35% / 50–60%; freemium 3–5% / 8–12%; reverse trial 4–6% / 8–12%; B2B-focused products 6–10% / 15–20%; AI-native 6–8% / 15–20% — [Userpilot](https://userpilot.com/blog/free-trial-conversion-rate/)
- Per 1,000 visitors: card-required trial → 35 signups → 10.5 paying; standard free trial → 45 → 3.6; freemium → 90 → ~5 — [Userpilot](https://userpilot.com/blog/free-trial-conversion-rate/)
- First Page Sage (agency client data, 50–86 SaaS cos, Q1 2022–Q3 2025): opt-in (no card) 18.2%, opt-out (card) 48.8%; Healthcare/Medtech trial-to-paid 21.5%. Userpilot notes these run well above ChartMogul, possibly due to client-only data — [Userpilot](https://userpilot.com/blog/free-trial-conversion-rate/); [Shno summary](https://www.shno.co/marketing-statistics/free-trial-conversion-statistics)
- ProductLed survey (600+ B2B, self-reported): average free-to-paid 9%; PQL-driven ~25%; PQL-driven at $1k–5k ACV 30–39%; only 24% use PQLs — [Userpilot](https://userpilot.com/blog/free-trial-conversion-rate/)
- ICONIQ 2025: at $100M+ ARR, AI-native companies convert trials at 56% vs 32% for traditional SaaS (different scale/definition than above) — [SaaStr on ICONIQ 2025](https://www.saastr.com/the-age-of-ai-the-top-10-gtm-learnings-from-iconiqs-2025-b2b-saas-report)
- Flint claims median B2B trial-to-paid 18.5%, top 35–45% (vendor blog, sourcing unclear) — [Flint](https://www.flint.com/blog/b2b-saas-free-trial-conversion-rate-statistics)
- Conflict: medians range from 8% (ChartMogul/Poyar) to 18–19% (First Page Sage, Flint). Differences stem from definition (6-month window, card vs no card) and sample (agency clients vs broad dataset).

**Activation / time-to-value benchmarks**
- OpenView 2020 Product Benchmarks: median activation at top PLG companies ~20% (freemium) to ~40% (free trial) — [OpenView](https://openviewpartners.com/blog/activation-product-qualified-leads/)
- OpenView/Pendo 2023 survey (1,000+ products): typical self-serve initial spend $10–$100/month; 30–60 days to convert free signups; organic + virality >50% of leads in PLG businesses — [Growth Unhinged](https://www.growthunhinged.com/p/your-guide-to-saas-product-metrics)
- Userpilot 2025 (547 companies, first-party): average activation 37.5%; average time-to-value 1 day 12 hours (vendor data) — [Userpilot metrics](https://userpilot.com/blog/vanity-metrics-vs-actionable-metrics-saas/)
- Lenny's Newsletter data (via Tandem): average SaaS activation 36%, median 30% — [Tandem](https://usetandem.ai/blog/activation-rate-vs-time-to-value)
- Several widely circulated "Pendo/OpenView 2024 time-to-first-value 10–14 days" figures appear only on low-quality generated pages and could not be verified — [sparkco (unreliable)](https://sparkco.ai/blog/design-user-activation-milestone-tracking)

**Comparable "field photo documentation" product (proof that SMB pays for this job)**
- CompanyCam (contractor photo documentation): 40,000+ contractors/teams (vendor claim); 14-day trial, no card; Capterra listings show per-user plans from ~$63 to $99/user/month and tiers up to $249 (sources conflict on current pricing); features include time/GPS-stamped photos, shareable PDF photo reports, AI summaries; ratings ~4.6–4.7/5 — [Capterra](https://capterra.com/p/171143/CompanyCam/); [Capterra UK](https://www.capterra.co.uk/software/171143/companycam); [GetApp](https://www.getapp.co.nz/software/126007/companycam)

### Inferences
- Signals to look for, ranked by predictive power for "instant" WTP:
  1. Existing spend or incumbent being paid for (e.g., wound-imaging tools, DSLR + medical photographer, CompanyCam in construction). If nobody pays for anything today, the job is probably a vitamin.
  2. Ugly workarounds with measurable time cost (personal phone + WhatsApp/email + manual EHR upload).
  3. Compliance trigger (HIPAA/GDPR audit finding, consent rules, insurance claim disputes) that makes the status quo a liability.
  4. Buyer = user (or one hop away) with a card/discretionary budget.
  5. Time-to-value under one session: first photo filed to a case and found again via search in <5 minutes.
- For this product, an "aha moment" candidate: "Teammate's photo of the same patient/job appears in my case timeline automatically, and I find any photo in seconds." Activation metric candidate: ≥N photos filed into ≥2 cases by ≥2 team members within 7 days (the shared-space network effect is the value, not single-user capture).
- Given the bimodal distribution, the product should aim to be in the >25% conversion cohort via card-required or sales-assisted trials to a narrow ICP, rather than broad free trials.

### Gaps
- No primary OpenView/Pendo 2023 or 2024 time-to-value number retrieved (chart was an image).
- No reliable conversion benchmark specifically for healthcare-department or clinical-tool self-serve products (First Page Sage's 21.5% "Healthcare/Medtech" is agency data).
- CompanyCam ARR/revenue not found.

---

## 3. User vs. buyer in hospitals vs SMB verticals; effect on "instant" purchase

### Takeaway
In hospitals the user (clinician/nurse) is rarely the buyer; purchase runs through IT security, compliance/privacy, procurement, and often a Value Analysis Committee, with 12–18+ month cycles and EHR (Epic) integration expectations. That structurally prevents "instant" purchase. In SMB verticals (contractors, restoration, auto body, small clinics), the owner is often both buyer and user-adjacent, making self-serve, card-in-hand purchase realistic (as CompanyCam shows).

### Cited Findings
- Hospital buying committees typically include clinical stakeholders, IT security, compliance, procurement and C-suite, all needing sign-off; vendors face security questionnaires, HIPAA reviews and months-long pilots before a decision is tabled; compliance/security are baseline, not differentiators — [MarketBetter (vendor blog)](https://marketbetter.ai/blog/healthcare-technology-buyer-intent-signals-long-sales-cycles/); [Elevation B2B (vendor blog, Mar 2026)](https://elevationb2b.com/blog/healthcare-software-marketing-playbook-for-hospitals/)
- Hospital sales cycles routinely 12–18+ months vs 3–6 months typical in other B2B; annual budget cycles; GPO contracts can lock out vendors for years — [MarketBetter](https://marketbetter.ai/blog/healthcare-technology-buyer-intent-signals-long-sales-cycles/); [Prospeo](https://prospeo.io/s/healthcare-sales-cycle) (vendor sources; directional)
- Value Analysis Committee: supply chain, nursing leadership, physician champion, finance, operations/risk; finance focuses on total cost of ownership, physicians on clinical benefit and ease of use — [Elevation B2B](https://elevationb2b.com/blog/healthcare-software-marketing-playbook-for-hospitals/)
- The pain is real at the user level: 89.1% of surveyed Canadian plastic surgeons used personal smartphones for clinical photos (2014); 65% of respondents at a Melbourne hospital captured sensitive images on smartphones, with no consent in 24% of cases; 79% of Saudi physicians used personal phones and only 28.6% stored photos in a separate password-protected folder (2022); non-compliance with written-consent policy described as "endemic" at an Australian tertiary hospital — [HCPLive](https://hcplive.com/view/clinical-photos-often-captured-on-smartphones-without-consent); [PMC4806754](https://pmc.ncbi.nlm.nih.gov/articles/PMC4806754); [DOAJ, J Nat Sci Med 2022](https://doaj.org/article/8510973db10c4034a76947dc3a0bd4f4); [CDU / Clinicians and their cameras](https://researchers.cdu.edu.au/en/publications/clinicians-and-their-cameras-policy-ethics-and-practice-in-an-aus/)
- Incumbent/free alternative: Epic Haiku/Canto already lets clinicians select a patient, "Capture Clinical Media," pick a document type, add description and save into the chart (Media Manager / Chart Review) — [Houston Methodist Epic training PDF](https://it.houstonmethodist.org/wp-content/uploads/2022/02/Epic-Mobile_020222_Capture-and-Review-Patient-Photos-and-Media.pdf); [HMH Maestro IT alert 2023](https://hmhmaestro.org/en/action-center/it-alerts/2023/03/15/clinicians-can-now-take-photos-in-haikucanto)
- Remaining friction with Haiku-type tools: patient positioning, capture technique, patient identification; in a 16-nurse wound-care pilot, a Google Glass prototype was preferred for sterile capture, with split views on quality/ease — [PMC4406552](https://pmc.ncbi.nlm.nih.gov/articles/PMC4406552/)
- Large health systems have built their own: Mayo's PhotoExam (EHR-integrated iOS app; 49% of providers responded to a survey, generally satisfied) and CliniCam (images viewable by all credentialed EHR users; users wanted photos embedded/linked in notes rather than separate PDFs). Mayo authors recommend secure EHR-integrated apps "whenever possible" — [PMC7251478 / JMIR 2020](https://pmc.ncbi.nlm.nih.gov/articles/PMC7251478); [JMIR mHealth 2015 CliniCam](https://mhealth.jmir.org/2015/1/e1/)
- Vertical specialist exists in wound care: Swift Medical captures calibrated wound photos on smartphones and computes measurements and longitudinal wound metrics — [MTEC directory](https://mtec-sc.org/life-sciences/swift-medical)
- SMB contrast: CompanyCam sells per-user self-serve with 14-day no-card trial to contractors — [Capterra](https://capterra.com/p/171143/CompanyCam/)

### Inferences
- Hospitals: "instant" WTP by the user does not translate to "instant" purchase. Expect: clinician champion → department head → IT security/privacy review (BAA, SOC 2/HITRUST, data residency for EU/GDPR) → EHR integration question ("does it write back to Epic/Cerner?") → procurement/VAC. Price-sensitivity and the "it's free in Haiku" objection will dominate in Epic shops.
- The most realistic hospital wedge is departments where Haiku-style capture is clearly insufficient (multi-visit longitudinal comparison, standardized positioning, research/teaching libraries, cross-staff case collections, or non-Epic/ambulatory clinics, private derm/plastic practices) — and private practices behave more like SMB buyers.
- SMB verticals (restoration, roofing, auto damage/claims, property management) look closer to "instant purchase," but CompanyCam and similar tools already occupy contractors, so differentiation must be the cross-vertical search/AI layer or an underserved vertical.

### Gaps
- No independent (non-vendor) study of hospital software sales-cycle length found; figures are directional.
- No data on what % of target hospitals use Epic Haiku media capture or how satisfied derm/ortho/wound departments are with it.
- Did not verify Swift Medical pricing, Epic integration, or other clinical photo vendors (e.g., Canfield, ModMed, Imitio).

---

## 4. Applying each framework to the idea: likely high/low scores and what must be validated

### Takeaway
The idea scores well on problem reality (well-documented insecure personal-phone photo practices) and on "existing workaround" signals, but scores poorly on buyer clarity, incumbent substitution (Epic Haiku, homegrown apps, Swift Medical, CompanyCam), and focus (horizontal + hospital + data-business ambitions at once). The biggest risks per Cagan are value (vs. "free in EHR") and viability (hospital sales cycle, security/compliance cost), not feasibility.

### Cited Findings
(Evidence supporting the scoring below; see sections 1–3 for full citations.)
- Problem reality: widespread personal-phone capture and weak consent/storage practices — [HCPLive](https://hcplive.com/view/clinical-photos-often-captured-on-smartphones-without-consent); [DOAJ 2022](https://doaj.org/article/8510973db10c4034a76947dc3a0bd4f4)
- Native camera apps lack security and EHR integration; SMS/email doesn't preserve images in the record — [PMC7251478](https://pmc.ncbi.nlm.nih.gov/articles/PMC7251478)
- EHR-native capture exists (Haiku/Canto) — [Houston Methodist PDF](https://it.houstonmethodist.org/wp-content/uploads/2022/02/Epic-Mobile_020222_Capture-and-Review-Patient-Photos-and-Media.pdf)
- Users want images inside/linked to clinical notes, not siloed — [JMIR mHealth 2015](https://mhealth.jmir.org/2015/1/e1/)
- Value risk is usually the toughest; tackle value and business risk early — [SVPG](https://www.svpg.com/four-big-risks/)
- Segmenting to the right persona alone moved Superhuman 22% → 33% — [First Round Review](https://review.firstround.com/how-superhuman-built-an-engine-to-find-product-market-fit/)
- Friedman: avoid "solution in search of a problem" (technology-first) — [MIT notes](https://www.mit.edu/~edchen93/posts/how-to-get-and-evaluate-startup-ideas/)

### Inferences (framework-by-framework scorecard)

| Framework | Likely HIGH | Likely LOW / risk | Must validate |
|---|---|---|---|
| JTBD (Christensen) | Clear functional job: "capture, file and later retrieve/compare clinical photos for a patient across visits and staff." Strong emotional/social job: avoid compliance exposure from photos on personal phones. | Job may already be "hired" to Epic Haiku/Canto, PACS/VNA, Swift Medical (wound), or departmental DSLR + photographer. Job may differ sharply by vertical (claims vs. derm). | What is currently hired, in which circumstance it fails (longitudinal compare? cross-staff? search?), and whether the failure is frequent. |
| ODI opportunity score | Outcomes like "minimize time to find all prior photos of this patient's lesion," "minimize risk of photo stored on personal device," "minimize time to compare before/after" likely high importance, low satisfaction. | "Capture a photo into the chart" may already be satisfied (overserved) in Epic sites. | Run a 100–200 respondent ODI survey per department; target outcomes with opportunity >10 (secondary-source threshold). |
| Painkiller vs vitamin / hair-on-fire | Painkiller for compliance officers after an audit/breach; painkiller in wound care (staging, documentation for reimbursement/litigation). | For many clinicians, better tagging/search is a vitamin; AI tag suggestions and semantic search are Kano delighters, not must-haves. | Has anyone lost a case, failed an audit, been sued, or lost reimbursement due to missing/misfiled photos? Ask for the last instance (Mom Test). |
| Frequency × intensity × budget | Frequency high in wound care (every dressing change) and derm (every visit); intensity high where photos are legal/reimbursement evidence. | Budget unclear: department-level discretionary spend is small; enterprise IT budget is gated by committees; ortho photo frequency may be lower. | Map per department: photos/week, consequence of a lost photo, who holds budget and its size. |
| Sean Ellis 40% | Plausible among a tight HXC (e.g., wound care nurse teams in non-Epic or outpatient settings; derm practices doing longitudinal mole tracking). | Unlikely to reach 40% across a horizontal user base; mixing verticals dilutes the score (Superhuman: segmenting raised score). | Pilot with ≥40 active users (used ≥2x in 2 weeks) per segment; compute score by segment. |
| Superhuman PMF engine | Good fit for iterating once a pilot exists; HXC definition forces narrowing. | Engine assumes self-serve iteration speed; hospital releases are slowed by IT change control. | Identify the HXC from "very disappointed" respondents; 50/50 roadmap. |
| The Mom Test | — | Risk of collecting compliments ("that's cool, AI tagging!") from clinicians who aren't buyers. | Success = commitments: intro to department head/IT security, a paid pilot, data-processing agreement signed, LOI. Ask about last photo they couldn't find, what they did, what it cost. |
| Van Westendorp | Useful to anchor per-seat vs. per-department pricing; CompanyCam's ~$63–$99/user/month gives an SMB reference point. | Hospitals buy via procurement and contracts; stated-price surveys of clinicians who don't hold budget mislead. | Run with budget holders (department managers, practice owners), not end users. Pair with purchase-probability questions. |
| Value Proposition Canvas | Pain relievers: compliant capture, no personal-device storage, team-shared case. Gain creators: instant retrieval, before/after, research/teaching libraries. | Must map separately for each customer segment (clinician, nurse manager, CISO, compliance), each with different jobs/pains. | Fit for the buyer's profile (CISO/compliance), not only the user's. |
| Cagan: value | Real pain documented. | Free/"good enough" EHR capture; homegrown apps at major systems. | Will departments choose this over Haiku/Canto and pay? |
| Cagan: usability | In-app camera + file-at-capture can be fast. | Clinical workflow constraints: sterile fields, gloves, patient ID (wristband/barcode), consent capture. | Time-in-motion tests in clinic: capture-to-filed in <10 s, with correct patient match. |
| Cagan: feasibility | Mobile capture, tagging, CLIP-style semantic + similarity search are technically mature. | EHR integration (Epic/FHIR media write-back), on-prem/data-residency for EU, de-identification for training data. | Prototype EHR write-back; legal review of data reuse for AI training (HIPAA/GDPR consent, BAA terms). |
| Cagan: viability | Data asset (expert-labeled images) could be strategically valuable. | Long hospital sales cycles; security certifications cost; data-training ambition may conflict with hospital BAAs and patient consent, scaring buyers. | Can CAC/sales cycle be supported at achievable ACV? Will customers contractually allow data reuse? |
| Kano | Must-be: security, access control, audit log, consent, patient-ID match, EHR link (in hospitals). Performance: capture speed, search speed. Delighters: AI tag suggestions, image-similarity, auto before/after. | Delighters can't compensate for missing must-bes; in hospitals the must-be list is long and costly. | Kano survey to separate must-be vs. delighter per segment. |

- Overall: the idea currently reads as "solution-first" (AI tagging + semantic search + data business) with a plausible but unproven painkiller underneath (compliant shared longitudinal photo records). The validation priority should be: (1) buyer and budget identification in a single department type; (2) proof that Haiku-type capture fails specific longitudinal/team jobs; (3) a paid pilot commitment.

### Gaps
- No direct primary interviews/data on orthopedics photo frequency or satisfaction.
- No verified data on how hospitals contractually treat vendor reuse of clinical images for AI training (likely covered by another researcher on regulation).

---

## 5. Guidance on horizontal products ("a product for everyone is a product for no one")

### Takeaway
Credible startup guidance (Superhuman's segment-first PMF, Seibel's "hold the customer tightly," Friedman's warnings on technology-first ideas) favors starting narrow; evidence on vertical vs horizontal SaaS performance is mostly from vendor blogs and should be treated as directional. Horizontal can produce bigger outcomes but needs a narrow wedge first.

### Cited Findings
- Superhuman: score rose from 22% to 33% just by narrowing to the right persona (HXC); the score changes as you expand to new segments — [First Round Review](https://review.firstround.com/how-superhuman-built-an-engine-to-find-product-market-fit/); [underscore.vc](https://underscore.vc/?p=1560)
- Seibel: "Hold the customer tightly … hold the solution loosely" — [Antoine Buteau](https://www.antoinebuteau.com/lessons-from-michael-seibel/)
- Friedman: working backward from technology ("solution in search of a problem") is the most common mistake — [MIT notes](https://www.mit.edu/~edchen93/posts/how-to-get-and-evaluate-startup-ideas/)
- Vendor-blog claims: vertical SaaS has higher conversion and spends ~22% less on S&M (no dataset cited); horizontal has larger TAM and possibly shorter cycles but stiffer competition and lower retention; horizontal can produce $100B+ outcomes but requires massive capital — [Bitstudios](https://www.bitstudios.com/blog/vertical-vs-horizontal-saas/); [Stax](https://staxpayments.com/blog/vertical-saas-vs-horizontal-saas/); [Brights](https://brights.io/blog/vertical-vs-horizontal-saas) (all unsourced; sources conflict on which grows faster)
- Kano: delighters decay into must-bes; horizontal AI features (auto-tagging, semantic search) are likely to commoditize — [Learning Loop](https://learningloop.io/plays/kano-analysis)
- Existence of vertical specialists that own the same job in specific verticals: CompanyCam (contractors), Swift Medical (wound care) — [Capterra](https://capterra.com/p/171143/CompanyCam/); [MTEC](https://mtec-sc.org/life-sciences/swift-medical)

### Inferences
- The horizontal framing increases competitive surface: in each vertical the product meets a vertical incumbent with deeper workflow (measurement, reports, billing codes, claim integrations). Horizontal "photo + case + search" is a feature of those products.
- A defensible path: win one vertical with vertical-specific must-bes (e.g., wound measurements + EHR write-back, or derm lesion tracking), then extract the horizontal core. The data-business ambition is likely downstream of vertical depth, since expert labels are only valuable with consistent vertical taxonomies.
- Hospitals as the first wedge are the slowest-to-buy segment; frameworks suggest either picking a faster-buying medical segment (private derm/plastics/wound clinics, home health) or a non-medical SMB vertical for early PMF measurement, while running hospital pilots in parallel.

### Gaps
- Could not retrieve Bessemer or other VC primary research quantifying vertical vs horizontal SaaS outcomes; only vendor blogs found.
- No primary source located for the exact phrase "a product for everyone is a product for no one."
