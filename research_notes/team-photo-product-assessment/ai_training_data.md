# Expert-Labeled Domain Image Data for AI Training: Market, Tools, Auto-Tagging, and the "Labels as a Byproduct of Work" Angle

Research date: 2026-10-19. All prices and figures carry the date of their source where known. Many company revenue figures come from secondary or estimate sources (Sacra, blogs that cite Reuters, Forbes, or the FT). Treat them as approximate.

---

## 1. Size and growth of the data labeling / AI training data market, and the shift to expert data

### Takeaway
Market-size estimates vary 10x depending on definition: about $2–4B (2025–26) for "AI data labeling / annotation" narrowly, and about $20–25B for broad "solutions and services". Growth is consistently ~20–30% CAGR. Company revenues show where the money actually goes: a handful of expert-network vendors (Surge, Scale, Mercor, Handshake, Turing, Micro1, Invisible) each make hundreds of millions to more than $1B a year, mostly from frontier labs buying RLHF/evaluation work from credentialed experts. That is mainly text and reasoning work, not image annotation. Physicians are paid about $130–400/hr.

### Cited Findings
**Market size (definitions vary widely)**
- The Business Research Company: data labeling solution & services market $20.41B (2025) → $25.39B (2026), 24.4% CAGR; $53.64B by 2030 — [GII/TBRC](https://www.giikorea.co.kr/report/tbrc1980865-data-labeling-solution-services-global-market.html)
- Mordor Intelligence: AI data labeling market $2.32B (2026) → $6.53B (2031). Grand View Research: data collection & labeling $3.77B (2024) → $17.10B (2030). Both cited in a May 2026 roundup — [blog.mean.ceo](https://blog.mean.ceo/?p=4579)
- 360iResearch: data annotation & labeling $2.37B (2025) → $2.97B (2026), 27.11% CAGR to 2032 — [GII/360iResearch](https://www.giiresearch.com/report/ires2006337-data-annotation-labeling-market-by-offering-data.html)
- GlobalData: data collection & labeling revenue $3,999.7M in 2025, 25% CAGR 2020–2025 — [GlobalData](https://www.globaldata.com/store/?p=3982535)
- MarketsandMarkets also publishes a data labeling report. I did not retrieve the figures — [MarketsandMarkets](https://www.marketsandmarkets.com/Market-Reports/data-labeling-market-88605245.html)
- Note: the company revenues below (Surge plus Scale plus Mercor alone add up to more than $3B in 2025–26) already exceed several of the "narrow" market estimates. The narrow reports likely leave out RLHF and expert-evaluation spend (see Inferences).

**Scale AI and the Meta deal (June 2025)**
- Meta invested $14.3B for a 49% non-voting stake (implied $29B valuation). Alexandr Wang left to lead Meta's superintelligence effort, and Jason Droege became interim CEO. Meta pledged no board seat and no privileged access to Scale's customer data — [Nasdaq/Reuters syndication](https://www.nasdaq.com/articles/meta-invests-143-bln-scale-ai-taps-ceo-alexandr-wang-lead-superintelligence-push); [NBC Chicago/CNBC](https://www.nbcchicago.com/news/business/money-report/scale-ais-alexandr-wang-confirms-departure-for-meta-as-part-of-14-3-billion-deal/3767336/). One outlet reported $14.8B, citing The Information — [Daily Star](https://www.thedailystar.net/tech-startup/news/meta-invests-148-bln-scale-ai-hires-its-28-year-old-ceo-3916596)
- Reuters reported that Google (Scale's largest customer) planned to cut ties, and that Microsoft, OpenAI and xAI planned to step back. Rivals (Snorkel, Labelbox, Turing) reported a surge in interest. I saw this only via secondary coverage, not the Reuters original — [texxr aggregator](https://texxr.com/entity/scale)
- Scale reportedly told Forbes that its 2025 revenue was just under $1B (2024: $870M) and that it expects to top $1B in 2026 — [valueaddvc (secondary)](https://valueaddvc.com/blog/how-does-scale-ai-make-money-data-labeling-rlhf-and-the-meta-14-3b-stake)

**Surge AI**
- Reuters (July 2025): Surge took in more than $1B in revenue in 2024, more than Scale's ~$870M. It was bootstrapped and profitable, and was seeking up to $1B in its first raise at a valuation above $15B — [Reuters via TradingView](https://www.tradingview.com/news/reuters.com%2C2025%3Anewsml_L6N3SX0O9%3A0-scale-ai-s-bigger-rival-surge-ai-seeks-up-to-1-billion-capital-raise-sources-say/). Sacra estimates $1.2B in 2024 revenue — [Sacra](https://sacra.com/research/surge-ai). Later reports put valuation talks at $25B+. No closed round was confirmed as of mid-2026 per [aiwiki (secondary, low confidence)](https://www.aiwiki.ai/wiki/surge_ai)

**Mercor**
- Reported annualized revenue: ~$500M (Sep 2025), >$1B (Feb 2026), ~$2B (Jun 2026). Last priced valuation $10B (Oct 2025) — [valueaddvc (secondary)](https://valueaddvc.com/blog/how-does-mercor-make-money-2b-arr-20b-valuation-and-the-expert-data-marketplace-explained). Sources conflict on the 2026 run-rate.
- An FT profile (via aggregator) says Mercor pays ~$2M/day to ~30K experts at a $95/hr average, with radiologists earning up to $375/hr — [Ben's Bites summary of FT](https://news.bensbites.com/posts/56252-a-profile-of-mercor-which-pays-2m-per-day-to-30k-experts-training-ai-models-at-95-per-hour-on-average-with-roles-like-radiologists-earning-up-to-375hour). Mercor's own claim is that experts earned more than $1M/day in 2025. These two figures conflict.
- Mercor reportedly disclosed a March 2026 supply-chain security incident, and a proposed lawsuit (Bloomberg Law) makes unproven allegations — [mozibox (secondary)](https://mozibox.com/articles/1769)

**Expert hourly rates (doctors)**
- Mercor job listing, Emergency Medicine Expert: $130–$180/hr, ~20 hrs/wk default, paid weekly — [Mercor listing](https://work.mercor.com/jobs/list_AAABndgW-_AC7W-RiLNN4qDm)
- Mercor Hematology/Oncology Expert: $130–$180/hr — [4dayweek mirror of Mercor listing](https://4dayweek.io/job/hematology-oncology-expert-at-mercor-1a9ee6dd)
- A Mercor healthcare listing gives $180–$400/hr by seniority tier. A physician-focused site says specialty-agnostic roles start at $130–175/hr and radiology pays up to $400/hr. Its self-reported median is $170/hr (range $100–420, small sample) — [mozibox](https://mozibox.com/articles/1769); [mozibox salaries](https://mozibox.com/salaries/ai_evaluation_results)
- A vendor blog estimates radiologist image annotation at about $5–$40 per study, and 10K–30K studies with multi-reader consensus at about $80K–$600K — [Fora Soft (vendor marketing)](https://www.forasoft.com/blog/article/ai-medical-imaging-software-guide)

**Other expert-data players**
- Handshake: Sacra estimates ~$1.1B annualized gross revenue (Apr 2026), up from ~$245M a year earlier, with ~$450M net after contractor payouts. ME News/KuCoin reports ~$1B gross and ~$300M net — [Sacra](https://sacra.com/research/handshake-indeed-for-data-labelers/); [KuCoin/ME News](https://kucoin.com/news/flash/handshake-ai-training-business-nears-10b-annual-revenue-in-2026). Sacra links part of Handshake's growth to the Meta–Scale deal and a Mercor data breach.
- Micro1: ~$4M (2024) → ~$200M (early 2026) per [Implicator](https://www.implicator.ai/the-200-million-middleman-between-humans-and-machines-2.md). Later reporting (citing Forbes): >$500M annualized, raised >$100M at a $4B valuation — [aiweekly](https://aiweekly.co/alerts/micro1-raises-100m-at-4b-valuation-8x-jump-from-500m-mark-in-september-2025). Sacra lists $300M (Apr 2026). Sources conflict.
- Turing: ~$300M in 2024 revenue at a $2.2B valuation, per Sacra comparison. I found no 2025 figure — [Sacra](https://sacra.com/research/handshake-indeed-for-data-labelers/)
- Invisible Technologies: $134M in 2024 revenue (company-stated). I found no 2025 figure — [Sacra/Invisible](https://sacra.com/research/invisible-revenue-growth-valuation)
- Overview of concentration among Scale, Surge, Mercor and Handshake — [Pebblous blog](https://blog.pebblous.ai/blog/ai-data-supply-oligopoly/en/)

### Inferences
- The money in "expert data" in 2025–26 is mostly frontier-lab spend on RLHF, evals and reasoning traces. That work is text-heavy and done by contractors paid per hour. It is not image datasets sold as assets. A photo-workflow product would be selling into a different, smaller sub-market: vertical CV and medical-imaging developers, plus multimodal eval sets for labs.
- Physician time costs about $130–400/hr. Any process that captures expert judgment *without* paying for dedicated labeling hours has a large structural cost advantage, provided the label quality holds up (see Q5).
- The Meta–Scale deal showed that buyers care about vendor neutrality and data confidentiality. A neutral, provenance-rich supplier has a selling point.

### Gaps
- I could not access the primary Grand View or MarketsandMarkets report pages for exact 2025/2026 figures.
- I found no breakdown of expert-data spend by modality (image vs text).
- Turing, Invisible and Micro1 2025 revenues are unverified.

---

## 2. Medical image data licensing: deals, prices, controversies, legal basis

### Takeaway
A broker market exists (Segmed, Gradient Health, Protege, Avandra, plus Truveta for EHR/genomics). Brokers buy de-identified data from hospitals and resell it to AI developers. Per-image or per-study prices are almost never published. The public figures available are whole-dataset quotes ($40K–$300K+), equity-linked deals, and Mayo's under-$5M across 16 deals as of 2020. Controversies show that de-identification under HIPAA, or under Australian law as in I-MED, usually makes these deals *legal*. The reputational risk comes from patients not being told. EU GDPR (Art. 9) and the AI Act (Art. 10) add stricter requirements on purpose, provenance and representativeness. High-risk AI Act obligations are now delayed to Dec 2027 / Aug 2028.

### Cited Findings
**Brokers and deals**
- STAT (Jan 16, 2025): Avandra, Gradient Health, Segmed and Protege offer de-identified patient images. These companies buy patient data from hospitals and sell it to AI developers and researchers. STAT calls the practice legal. Bioethicist Marielle Gross (Johns Hopkins) compared it to "Soylent Green". Prices and revenue shares are behind the STAT+ paywall and I could not see them — [STAT](https://www.statnews.com/2025/01/16/health-ai-electronic-health-records-hipaa-deidentified-data-market/)
- Truveta: Regeneron invested $119.5M in Truveta's Series C (Jan 2025) and launched the Truveta Genome Project, which will sequence up to 10M de-identified volunteers with linked EHRs. Regeneron has exclusive research-sequencing rights. Illumina invested $20M. Microsoft Azure provides the infrastructure. The payment is equity, not a disclosed licensing fee — [Regeneron IR](https://investor.regeneron.com/news-releases/news-release-details/regeneron-collaborates-truveta-and-leading-american-health); [Nasdaq](https://www.nasdaq.com/articles/regeneron-and-truveta-announce-strategic-collaboration-truveta-genome-project-aiming); [Pulse2](https://pulse2.com/truveta-ehr-data-company-raises-320-million-at-over-1-billion-valuation) ($320M raised at a valuation above $1B)
- Mayo Clinic (STAT investigation, June 2020): Mayo gave 16 companies access to de-identified data over two years, bringing in less than $5M in total (Mayo's annual revenue is ~$13B). Patients were not notified or asked to consent. Mayo says it does not sell to brokers and instead co-creates products through investments. John Halamka: "It would be us bringing them in, and we would control their access." — [STAT 2020](https://statnews.com/2020/06/03/mayo-clinic-patient-data-fuels-artificial-intelligence-consent-concerns); [Becker's](https://www.beckershospitalreview.com/data-analytics/how-mayo-clinic-manages-patient-data-privacy-consent-in-licensing-deals-with-tech-companies.html)
- Memorial Sloan Kettering gave Paige.AI access to 25M pathology slides. MSK and several clinicians and board members held equity — [Becker's (citing STAT)](https://www.beckershospitalreview.com/data-analytics/how-mayo-clinic-manages-patient-data-privacy-consent-in-licensing-deals-with-tech-companies.html)
- Dataset pricing data points (vendor marketing): custom medical imaging datasets from ~$40K for a focused single-modality set to $300K+ for multi-center, multi-modality collections — [LXT](https://www.lxt.ai/datasets/medical-imaging/). Some public archives, such as TCIA, allow commercial use with attribution, which caps the price of commodity data — [iMerit](https://imerit.ai/resources/blog/top-10-medical-ai-training-datasets-for-2026/); [medDARE](https://meddare.ai/medical-imaging-datasets-for-ai-public-vs-private-vs-custom/)
- For comparison, AI *inference* on radiology studies costs about $0.45–$7.50 per study — [fractify](https://aiscan.fractify.net/blog/radiology-ai-software-pricing-comparison-what-7-platforms-actually-cost-in-2025-en)
- Clinical-photography analog: True-See, a medical photography platform that integrates with EHRs, partnered with UC Irvine to use its proprietary database of more than 2M color-accurate medical photographs to build gold-standard training datasets and AI tools for wound care and dermatology — [MobiHealthNews](https://www.mobihealthnews.com/news/true-see-teams-uc-irvine-ai-medical-photographs)
- Dermatology datasets: Google's SCIN (Mar 2024, with Stanford Medicine) has more than 10,000 images contributed by individuals. Contributors were recruited via Google Search ads, gave informed consent, and the study had IRB approval. Up to 3 dermatologists labeled each case. Google says the labels are retrospective and "not equivalent to a clinical diagnosis". The dataset includes self-reported Fitzpatrick type and demographics. Paper: JAMA Netw Open 2024, DOI 10.1001/jamanetworkopen.2024.46615 — [Google Research blog](https://www.research.google/blog/scin-a-new-resource-for-representative-dermatology-images/); [GitHub](https://github.com/google-research-datasets/scin)
- DermAI (Brazil): a research app for standardized lesion capture during routine consultations at a public university hospital, with a structured form per case (age, Fitzpatrick type, sex, location, lesion mask). This is a direct example of workflow-time capture — [arXiv 2511.10367](https://arxiv.org/html/2511.10367v1)

**Controversies**
- I-MED / Harrison.ai (Australia): in Sept 2024, Crikey reported that Australia's largest radiology chain shared patient scans with Harrison.ai/Annalise.ai to train AI, with no public evidence of patient consent. The data was later disclosed as fewer than 30M chest X-rays, CTs and reports. The OAIC opened preliminary inquiries. On 31 July 2025 it closed them with no regulatory action, concluding the data had been de-identified enough to no longer be personal information. It noted multiple de-identification techniques and contractual protections on Annalise.ai — [Crikey](https://www.crikey.com.au/?p=1175720); [ACS IA 2024](https://ia.acs.org.au/article/2024/patient-data-used-to-train-aussie-startup-s-ai.html); [RSM analysis](https://www.rsm.global/australia/insights/what-health-organisations-can-learn-i-med-privacy-case); [OAIC investigation reports index](https://www.oaic.gov.au/privacy/privacy-decisions/investigation-reports). RSM warns that models can still leak sensitive data even from anonymized training data.
- DeepMind / Royal Free (UK): in 2017 the ICO found that the Royal Free breached the Data Protection Act 1998 by giving DeepMind data on 1.6M patients for the Streams acute-kidney-injury app without telling patients. The ICO required an undertaking (legal basis plus a privacy impact assessment) and issued no fine — [SCL](https://www.scl.org/6953-ico-ruling-on-royal-free-and-google-deepmind-trial/); [Inside Privacy](https://www.insideprivacy.com/health-privacy/uk-hospital-deepmind-trial-failed-to-comply-with-uk-data-protection-law/)
- Google / Ascension "Project Nightingale" (2019): Google accessed records of up to ~50M patients across 21 states without patient notification. Google called it a standard business-associate arrangement. HHS OCR opened an inquiry. 50M is an upper bound — [TechTarget](https://www.techtarget.com/searchhealthit/definition/Project-Nightingale); [POZ](https://poz.com/article/google-collects-health-data-50-million-americans-legal)
- Mayo consent concerns (STAT 2020) are covered above.

**Legal basis**
- HIPAA de-identification (45 CFR 164.514(b)) has two routes: (1) Expert Determination, where a qualified expert certifies very small re-identification risk, and (2) Safe Harbor, which removes 18 identifiers, including "(Q) full-face photographic images and any comparable images", and requires no actual knowledge of re-identifiability — [HHS de-identification guidance](https://www.hhs.gov/hipaa/for-professionals/special-topics/de-identification/index.html); [eCFR 164.514](https://www.ecfr.gov/current/title-45/subtitle-A/subchapter-C/part-164/subpart-E/section-164.514). (The HHS page returned 403 in this session. This content is the long-standing regulation text.) For a photo-capture product: clinical photos of faces, tattoos or other distinctive features may not meet Safe Harbor, so Expert Determination or consent is more likely to be needed.
- GDPR: health data (and biometric data used for identification) is "special category" data under Art. 9. Processing needs an Art. 9(2) condition such as explicit consent, or scientific research with Art. 89 safeguards — [GDPR Art. 9](https://gdpr-info.eu/art-9-gdpr/). Pseudonymized data is still personal data. Only truly anonymized data falls outside GDPR.
- EU AI Act Art. 10 (high-risk systems; most medical-device AI qualifies) requires data governance covering design choices, *data collection processes and origin*, and for personal data *the original purpose of collection*. It also covers annotation/labelling and cleaning steps, assumptions, bias examination, and datasets that are "relevant, sufficiently representative, and to the best extent possible free of errors and complete". Art. 10(5) allows limited processing of special-category data for bias detection — [AI Act Art. 10](https://artificialintelligenceact.eu/article/10/)
- AI Act timing: the Digital Omnibus agreement (reported reached 7 May 2026) postpones high-risk obligations to 2 Dec 2027 (Annex III standalone) and 2 Aug 2028 (product-embedded, e.g. medical devices under Annex I). This is secondary-source reporting, and I did not confirm publication in the Official Journal — [CASRAI](https://casrai.org/news/digital-omnibus-ai-act); [OneTrust on the Nov 2025 proposal](https://www.onetrust.com/blog/eu-digital-omnibus-proposes-delay-of-ai-compliance-deadlines/)
- Consent signals are fragmented across metadata, copyright notices and watermarks, and there is no universally recognized framework for expressing data consent — [arXiv 2511.08637](https://arxiv.org/pdf/2511.08637). Buyers increasingly ask for provenance records: source, rights and consent basis — [DEV Community](https://dev.to/aidata_broker/what-is-training-data-provenance-and-why-buyers-now-ask-for-it-3aib); [7wData](https://7wdata.be/data-management/data-lineage-provenance-ai-training-sets/)

### Inferences
- Hospital-held imaging is plentiful (I-MED alone had ~30M studies). Clean provenance, consent, structured context and expert labels are scarce. A workflow app that records consent, purpose, capturer credentials and clinical context *at capture* lines up directly with AI Act Art. 10's "origin / original purpose / annotation process" requirements.
- Precedents (Mayo, I-MED, Royal Free) suggest that "legal but not disclosed" deals generate backlash and regulators then accept the de-identification. A product that builds patient or customer notice into the workflow lowers this risk but adds friction.
- Price discovery is poor. Mayo's under-$5M across 16 deals suggests raw data *access* has historically been cheap relative to its strategic value. Value tends to accrue to whoever adds labels and curation (Paige, Harrison) and takes equity.

### Gaps
- I found no published per-image or per-study licensing prices from Segmed, Gradient Health, Protege or hospitals (paywalled or undisclosed).
- I found no dermatology image *commercial* licensing deal terms.
- I did not obtain the OAIC report PDF text itself, only summaries.
- I did not confirm whether AI Act Art. 10 text itself was amended by the Omnibus.

---

## 3. Labeling/curation tools: pricing, users, limitations, and point-of-capture alternatives

### Takeaway
Labeling and curation tools are mature and cheap at the low end. CVAT, Label Studio and FiftyOne are open source, and Roboflow starts at about $79/mo. Enterprise platforms (Labelbox, Encord, V7, Scale) sell on custom quotes. They all start *after* data exists: you import images, then a workforce (in-house or outsourced) labels them. Capture context, capturer identity and consent are not part of the core data model. I found no established product that combines "capture in the field by domain experts" with "training-ready dataset with provenance". The closest are generic field-data apps (ODK, KoboToolbox), consumer dataset-capture apps, and clinical-photography vendors such as True-See.

### Cited Findings
- Labelbox: free plan (1 workspace, 10 users, 3 integrated models, 100-item limit). Enterprise pricing on request. Reviewers describe it as costly — [G2 Labelbox pricing](https://www.g2.com/products/labelbox/pricing)
- Encord: "pricing scales with your organization", custom pricing with trial — [G2 Encord vs V7](https://g2.com/compare/encord-vs-v7); [Lightly alternatives guide](https://lightly.ai/blog/the-10-best-voxel51-alternatives-in-2026-a-practical-guide-for-ml-teams)
- V7 Darwin: Capterra lists from £150/user/month (third-party estimate, not official) — [Capterra](https://www.capterra.com/p/206720/V7-Darwin/)
- Roboflow: free tier, paid from ~$79/mo. CVAT: free open source, hosted from ~$23/mo. Label Studio: open-source Community edition, Starter ~$50/mo, Enterprise custom — [Dupple comparison](https://dupple.com/learn/best-ai-data-labeling-tools); [Labellerr](https://labellerr.com/blog/best-data-annotation-software)
- Voxel51 FiftyOne: open source. FiftyOne Teams/Enterprise is paid with no published price, and one guide says enterprise pricing can be prohibitive. Lightly: free tier and paid options, no published price found — [Lightly blog](https://lightly.ai/blog/the-10-best-voxel51-alternatives-in-2026-a-practical-guide-for-ml-teams); [Lightly CV tools](https://www.lightly.ai/blog/best-computer-vision-tools)
- Multimodal tool comparison (Encord, Label Studio, Labelbox, SuperAnnotate) — [Pixeltable](https://pixeltable.com/blog/multimodal-annotation-tools-comparison-2025)
- Field-capture tooling exists mostly as generic apps: Open Data Kit and KoboToolbox (offline forms, versioned sync), and the consumer app "AI Capture: Create ML Datasets" (capture, label, export) — [search summary incl. App Store listing](https://apps.apple.com/us/app/ai-capture-create-ml-datasets/id6444910890); [aiacquisition list](https://www.aiacquisition.com/blog/ai-data-collection-tools)
- Clinical photo capture: a Stanford study found that most clinical photo-taking apps, including Stanford Healthcare's, rely on the patient's judgment for photo quality — [arXiv 2209.09105](https://arxiv.org/pdf/2209.09105). True-See (EHR-integrated medical photography, more than 2M photos) is moving toward AI training datasets — [MobiHealthNews](https://www.mobihealthnews.com/news/true-see-teams-uc-irvine-ai-medical-photographs). Innowise describes a physician photo-collection app whose clinic uploads feed ongoing model training — [Innowise case](https://innowise.com/case/ai-skin-scanner-app/)

### Inferences
- Limitations of incumbent tools for this use case (my synthesis, not vendor-stated):
  1. They begin at import, after collection. Capture-time metadata such as who took the photo, their role and credential, the task or job context, location, device, and the decision taken is lost unless a custom pipeline adds it.
  2. Labels come from paid annotators or experts doing separate labeling sessions. Cost scales with hours (see the $130–400/hr physician rates).
  3. Consent and rights management, data provenance and lineage, and purpose limitation are not part of the core data model. Buyers and regulators (AI Act Art. 10) increasingly ask for them.
- Positioning opportunity: a "team photo management" app that already sits in the expert's workflow could export to these tools (COCO/YOLO formats, Labelbox/Encord APIs) rather than compete with them. It would be the "data engine at the point of capture" upstream of curation tools.
- The "data engine" concept (fleet capture → triage → label → retrain) is common in autonomous driving, but I found no horizontal product that offers it for expert field photo workflows.

### Gaps
- I did not find official 2026 pricing pages for Labelbox, Encord, V7, Lightly or FiftyOne Enterprise.
- I found no named startup that explicitly sells "expert workflow photos → licensed training data" outside medicine (construction, insurance, agriculture). This is worth a targeted follow-up, e.g. CompanyCam or Hover-style vertical photo apps and their AI data policies.

---

## 4. Auto-tagging tech: VLM and embedding costs, medical caveats, and TypeSafe "Jev"

### Takeaway
Auto-tagging with cheap VLMs costs roughly $0.10–$1 per 1,000 images on Flash-Lite, Haiku or nano-class models. It rises to several dollars per 1,000 on Sonnet, Opus or Pro-class models, and batch APIs halve the cost. Image embeddings for search and clustering cost about $0.06–$0.12 per 1,000 images (Gemini Embedding 2). Open CLIP/SigLIP can be self-hosted for compute cost only. General-purpose models are weak on specialist medical images: medical-specific CLIP variants beat general CLIP, and Anthropic says Claude is not designed for complex diagnostic scans. TypeSafe's Jev (Sept 15, 2026) is a text/JSON-only "decision model". It cannot look at images. It could cheaply route or score records *after* a VLM or the expert has produced tags and metadata.

### Cited Findings
**Gemini (Google AI pricing page, retrieved 2026-10-19; paid tier, per 1M tokens)**
- gemini-3.8-flash / 3.6-flash: $0.75 in / $3.75 out (batch $0.375/$1.875). gemini-3-flash-preview: $0.50/$3.00 (batch $0.25/$1.50). gemini-2.5-flash: $0.30/$2.50. gemini-3.5-flash-lite: $0.30/$2.50. gemini-3.1-flash-lite: $0.25/$1.50 (batch $0.125/$0.75). gemini-2.5-flash-lite: $0.10/$0.40 (batch $0.05/$0.20). gemini-3.1-pro-preview: $2/$12 (≤200K). Several Flash/Pro prices rise 1 Jan 2027 — [Gemini API pricing](https://ai.google.dev/gemini-api/docs/pricing)
- gemini-embedding-2: text $0.20/M. Image $0.45/M tokens = **$0.00012 per image** (batch $0.00006) — [Gemini API pricing](https://ai.google.dev/gemini-api/docs/pricing)
- Image tokens: 258 tokens if both dimensions ≤384px. Larger images are tiled at 768×768 with 258 tokens per tile (e.g. 960×540 → 6 tiles ≈ 1,548 tokens). Gemini 3 has a `media_resolution` parameter to cap tokens per image — [Gemini image understanding](https://ai.google.dev/gemini-api/docs/image-understanding)

**Claude (Anthropic, cached 2026-10-06 and docs)**
- Per 1M tokens in/out: Haiku 5.5 $0.10/$0.50 (≤100K prompt), Haiku 4.5 $1/$5, Sonnet 5.5 $2/$10, Opus 5.5 $4/$20, Fable 5.1 $10/$50. Batch API is 50% off — [Claude pricing](https://claude.com/pricing) (via Anthropic's claude-api reference)
- Image tokens = ⌈w/28⌉×⌈h/28⌉. Claude 4.7+ models allow up to 2576px long edge / 4,784 tokens, older models 1568px / 1,568 tokens. A 1000×1000 image is 1,296 tokens. Anthropic's examples: Haiku 4.5 ≈ $1.30 per 1,000 1MP images. Opus 5.5 ≈ $5.18 per 1,000 1MP images, or $19.14 per 1,000 4K images (input only). Anthropic states Claude "is not designed to interpret complex diagnostic scans such as CTs or MRIs". It does not read image metadata (EXIF) — [Claude vision docs](https://platform.claude.com/docs/en/build-with-claude/vision)

**OpenAI (pricing page retrieved 2026-10-19, per 1M tokens in/out)**
- gpt-6-astra $10/$50, gpt-6.1-sol / gpt-6-sol $2/$10, gpt-5.4-mini $0.75/$4.50, gpt-5-mini $0.25/$2.00, gpt-5.4-nano $0.20/$1.25, gpt-5-nano $0.05/$0.40, gpt-6-luna $0.10/$0.50. Batch is ~50% off — [OpenAI API pricing](https://developers.openai.com/api/docs/pricing). I did not verify OpenAI's tokens-per-image rules for these models.

**Cohere / others**
- Cohere Embed v4 is listed at ~$0.12/M input tokens with vision support on Bedrock, Azure and OCI (aggregator, verified May 2026). Per-image billing not confirmed — [Maxim/Bifrost](https://www.getmaxim.ai/bifrost/llm-cost-calculator/provider/bedrock/model/cohere.embed-v4-0.md)
- Vertex AI `multimodalembedding` per-image price: not retrieved.

**Zero-shot accuracy caveats in medical images**
- BiomedCLIP (trained on 15M biomedical image-text pairs) had the highest mean zero-shot accuracy across 5 biomedical datasets. A radiology-pretrained model beat general CLIP on RSNA, and the pathology model PLIP did much worse on RSNA. Domain fit matters — [BiomedCLIP arXiv 2303.00915](https://arxiv.org/pdf/2303.00915)
- RegionMed-CLIP reports an average AUC of 77.09%. MedMoE reports 30–35% gains over BiomedCLIP on MRI tasks. Absolute zero-shot performance is moderate, not clinical-grade — [RegionMed-CLIP](https://arxiv.org/pdf/2508.05244); [MedMoE](https://arxiv.org/pdf/2506.08356)
- A fine-tuned CLIP for chest X-ray roughly matched radiologists on 5 CheXpert diseases (F1 0.619 vs 0.625). That required domain fine-tuning — [arXiv 2212.07050](https://ar5iv.labs.arxiv.org/html/2212.07050)

**TypeSafe AI "Jev"**
- Announced in early access on Sept 15, 2026 by TypeSafe, founded by ex-OpenAI researcher Diogo Almeida. It is a "System One" decision model that does not generate text. It answers developer-defined, typed questions: yes/no with probability, choice among options, or a score on a scale, each with a confidence measure. Example uses are routing requests, scoring records and screening outputs for jailbreaks. Pricing is $0.042 per 1M input tokens with free output. Latency is 70–500ms, with claims of 40–200x faster and up to ~444x cheaper than LLMs on narrow decision tasks. Caveats: "can't hallucinate" only means it can't produce an answer outside the allowed set, not that the answer is right. Accuracy was measured against reference answers averaged from GPT-6 Astra and Claude Fable 5.1. Load behavior is unproven — [The Rundown AI](https://www.therundown.ai/news/typesafe-jev-ai-decisions-software); [TrueFoundry explainer](https://www.truefoundry.com/blog/typesafe-ai-jev)
- Input is text or JSON. Vercel AI Gateway, Cloudflare, LangChain and Langfuse integrated it within ~3 days, and Vercel called it the fastest-adopted launch on its gateway — [Rezolve explainer](https://www.rezolve.ai/blog/what-is-jev-typesafe-ai-s-decision-model); [Startup Fortune](https://startupfortune.com/typesafe-ais-decision-model-jev-becomes-vercels-fastest-adopted-launch/)
- Funding: launched alongside a $40M seed led by DCVC per Rezolve and Startup Fortune. A later claim of an $870M raise at a $7.5B valuation (Oct 9, 2026) appears only on X and minor sites and is unconfirmed — [AGTP on X](https://x.com/AGTPinsights/status/2110299314312663045); [openaimaster](https://openaimaster.com/typesafe-ai-870m-jev-decision-model/)

### Inferences
- Worked per-image cost estimates (my arithmetic). These assume a ~1MP photo (≈1,000–1,300 input tokens) plus ~100 output tokens of tags:
  - gemini-2.5-flash-lite: ≈ $0.00014/image ≈ **$0.14 per 1,000** (≈ $0.07 batch)
  - gemini-3.1-flash-lite: ≈ $0.0004/image ≈ $0.40 per 1,000
  - Claude Haiku 5.5: 1,296×$0.10/M + 100×$0.50/M ≈ $0.00018 ≈ **$0.18 per 1,000**
  - gemini-3-flash-preview: ≈ $0.0008 ≈ $0.80 per 1,000
  - Claude Sonnet 5.5: ≈ $0.0036 ≈ $3.60 per 1,000. Opus 5.5 ≈ $7 per 1,000.
  - Embeddings (gemini-embedding-2): $0.12 per 1,000 images ($0.06 batch)
  - Jev on a ~500-token JSON record (VLM tags + capture metadata + expert notes): ≈ $0.00002/decision ≈ $0.02 per 1,000
  - So for a team photo app, model cost for auto-tagging is negligible compared with storage or human review. The cost that matters is human verification.
- Suggested pipeline: cheap VLM proposes tags → embeddings for dedup, search and clustering → Jev or a small classifier routes records (e.g. "needs expert confirmation?", "eligible for training export?", "consent complete?") based on JSON metadata → the expert confirms with one tap during normal work. Jev is useful only on text/JSON and cannot replace vision models.
- For medical or specialist imagery, general VLM tags should be treated as weak hints. Expert confirmation captured in the workflow is the asset that sets the data apart.

### Gaps
- Vertex `multimodalembedding` per-image price and Cohere Embed v4 per-image billing are unverified.
- OpenAI image-token rules for GPT-6/5.4 models were not retrieved.
- Gemini 3 `media_resolution` token counts were not stated on the page.
- I found no independent benchmark of Jev, and no head-to-head GPT/Gemini/Claude zero-shot evaluation on clinical photos in 2026.

---

## 5. Are workflow-generated labels (clinical tags/metadata) good enough for training?

### Takeaway
Evidence is mixed but encouraging at scale. Weak labels taken from routine clinical workflow can produce clinical-grade models: Campanella 2019 trained on pathology report diagnoses only, with 44,732 slides and AUC above 0.98. But labels mined from reports or metadata (e.g. NLP-derived chest X-ray labels) can be 10–30 points less accurate than claimed, and they hide confounders such as chest drains, so models can look good overall while failing on clinically important subgroups. Workflow labels work best when they record the expert's actual decision, the dataset is large, and a small expert-verified test set exists.

### Cited Findings
- Campanella et al., Nature Medicine 2019: multiple-instance learning using *only the reported diagnoses* from pathology reports as labels, with no pixel annotation. 44,732 whole-slide images from 15,187 patients with no data curation. AUC above 0.98 for prostate cancer, basal cell carcinoma, and breast metastases to lymph nodes. Pathologists could exclude 65–75% of slides at 100% sensitivity. Includes external slides from US and 44 other countries. Caveat: limited cancer types — [PMC7418463](https://pmc.ncbi.nlm.nih.gov/articles/PMC7418463/); [Yearbook review](https://www.thieme-connect.com/products/ejournals/html/10.1055/s-0040-1701995)
- Oakden-Rayner's review of ~700 ChestX-ray14 images (labels NLP-mined from reports) found positive predictive values mostly 10–30% below the documented values, and labels that did not reflect visual content. Errors tracked the weaknesses of the labeling method. MURA fared better — [AuntMinnie](https://www.auntminnie.com/imaging-informatics/artificial-intelligence/article/15624523/study-public-image-datasets-may-have-qc-issues)
- Hidden stratification: most pneumothorax-positive X-rays contain chest drains, so models perform much worse on the clinically important subset without drains — [Oakden-Rayner et al., arXiv 1909.12475](https://arxiv.org/pdf/1909.12475). A follow-up tested a commercial CXR model on 2,557 radiographs with robust ground truth, stratified by drain presence — [PMC10378683](https://www.ncbi.nlm.nih.gov/pmc/articles/PMC10378683/); [BMJ Open 2021](https://doaj.org/article/24f002e2d0d34a5aad4957cd0e6dfae7). (A secondary blog cites AUROC 0.940 with drains vs 0.770 without. I did not verify this against the primary source — [Pebblous](https://blog.pebblous.ai/blog/chest-xray-shortcut-layer-probes/en/))
- Karimi et al. (2019/2020) review of label noise in medical image analysis: tested strategies on 3 medical datasets with different noise types, gave recommendations, and argued the medical imaging field had largely ignored noise-robust deep learning methods — [arXiv 1912.02911](https://arxiv.org/abs/1912.02911v2)
- EHR-label noise: deep models overfit to noisy EHR labels. Adapting computer-vision noise-robust methods substantially improved performance on real EHR data for COVID-19 diagnosis — [BMC Med Inform Decis Mak 2024](https://link.springer.com/article/10.1186/s12911-024-02581-5); [Oxford Eng PDF](https://eng.ox.ac.uk/media/1txlv3ny/2024-bmc-midm-jy.pdf)
- SCIN's dermatologist labels are explicitly "not equivalent to a clinical diagnosis". Even expert labels without the full clinical workup are proxies — [Google Research](https://www.research.google/blog/scin-a-new-resource-for-representative-dermatology-images/)
- Point-of-care capture quality: clinical photo apps generally leave photo quality to the user's judgment, which motivates AI quality checks at capture time — [arXiv 2209.09105](https://arxiv.org/pdf/2209.09105)
- AI Act Art. 10 requires datasets to be "to the best extent possible free of errors", and labeling processes must be documented — [AI Act Art. 10](https://artificialintelligenceact.eu/article/10/)

### Inferences
- "Labels as a byproduct of work" sits in between. A tag the expert enters *because the job requires it* (e.g. diagnosis selected, defect type logged, action taken) is closer to Campanella's report-diagnosis labels: noisy but tied to a real decision and plentiful. Labels inferred later from free text or folder names are closer to the ChestX-ray14 NLP labels: systematically noisy.
- Design implications: (1) record structured choices at capture, not free text; (2) log context such as device, site and whether a treatment or device was present, to detect shortcuts and confounders; (3) record capturer credential and confidence; (4) sample a small slice for independent expert adjudication to measure the noise rate. This turns a noisy byproduct into a sellable dataset with a documented quality level.
- Compared with paid expert labeling, workflow labels are far cheaper per image but noisier and less consistent. Annotators follow one guideline, while practitioners tag to get the job done. A hybrid works best: workflow labels for scale, plus paid expert review of a stratified subset (at $130–400/hr, or ~$5–40/study) for a gold test set.

### Gaps
- I found no study that directly compares models trained on *photo-workflow* tags (non-radiology, e.g. dermatology clinic photos or field inspection photos) with curated expert labels.
- I did not retrieve the primary Oakden-Rayner 2020 paper (figures come from AuntMinnie's summary).
- I found no quantitative estimates of label-noise rates for practitioner-entered structured tags outside medicine.
