# GTM Lessons: Horizontal vs Vertical SaaS for a "Team Photo Case Management" App with a Wedge Vertical

Research date: 2026-10-19. Most sources found are secondary (VC blogs, Substack analyses, vendor content). Source quality is noted inline. Figures from vendor or aggregator blogs are directional only.

## Vertical SaaS economics and advantages (retention, fintech, CAC, NRR/CAC data)

### Takeaway
VC literature (a16z, Tidemark, Bessemer) agrees that vertical SaaS gets winner-take-most market structure, purpose-built stickiness, and 2-5x revenue per customer from embedded fintech. The hard numbers comparing vertical and horizontal NRR and CAC mostly come from weak aggregator sources, so treat them as directional: vertical NRR roughly 5-15 points higher, and CAC payback shorter.

### Cited Findings
- a16z: "Vertical software markets tend to have winner-take-most dynamics". Customers "prefer purpose-built software for their specific industry and use cases". Example: "Veeva, a CRM for pharma, has over 50% market share." — [a16z, Fintech Scales Vertical SaaS](https://a16z.com/fintech-scales-vertical-saas/)
- a16z: embedded fintech can "increase revenue per customer by 2-5x" over software alone. The footnote says this is based on "market research and conversations with vertical SaaS companies", not audited data. — [a16z](https://a16z.com/fintech-scales-vertical-saas/)
- Mindbody earned about $250 per customer per month: about $150 from software and about $100 from payments. ServiceTitan launched payments only after reaching tens of millions in ARR. Toast Capital offers $5K-$250K loans underwritten with Toast transaction data. — [a16z](https://a16z.com/fintech-scales-vertical-saas/)
- a16z conditions for embedded fintech: payments needs scale, usually over $50M GMV. Lending and insurance need the software company to own the underwriting data. At over $5,000 average revenue per customer, an outbound inside sales team becomes affordable. — [a16z](https://a16z.com/fintech-scales-vertical-saas/)
- Some vertical SaaS companies give the software away free and monetize through fintech to win markets that resist digitizing (example: Silo, wholesale food distribution). — [a16z](https://a16z.com/fintech-scales-vertical-saas/)
- a16z's 2024 "AI inside" thesis: vertical software can take share from labor budgets. In 2022, total US software spend was about $313B against about $10.5T in US labor spend. a16z claims this could lift revenue per customer by up to 10x (speculative). — [a16z, Vertical SaaS: Now with AI Inside](https://a16z.com/vertical-saas-now-with-ai-inside/); [a16z, VSaaS: AI Opens New Markets](https://a16z.com/vsaas-vertical-saas-ai-opens-new-markets/)
- Tidemark/Stripe 2025 Vertical & SMB SaaS Benchmark (200+ vertical SaaS companies):
  - Multiproduct platforms have about 10x the addressable market and grow 21% faster. Median addressable market went from $250M to $513M.
  - The median payments attach rate doubled in one year.
  - 87% of companies offering fintech also offer payments, up from 30%.
  - Fintech is the #1 second-product expansion play for 45% of companies launching a second product.
  - Companies offering AI products grow 8 points faster.
  - The landing page publishes no NRR or CAC figures. — [Stripe/Tidemark benchmark](https://stripe.com/lp/vertical-saas-benchmark-2025)
- NRR comparisons (low-quality aggregators, methodology unclear):
  - One cites vertical at 92% GRR and 112% NRR, with a 46% valuation premium.
  - Another cites a SaaS Capital-attributed 125% vertical vs 110% horizontal NRR, and CAC payback of 11 vs 18 months.
  - A third says vertical NRR is 5-10 points higher.
  - I could not verify these against the original SaaS Capital report. — [saasvaluationmultiple.com](https://saasvaluationmultiple.com/vertical-saas-multiples); [saasmag.com](https://www.saasmag.com/vertical-saas-outperforming-horizontal-2026/); [revenuemap.app](https://revenuemap.app/blog/vertical-saas-financial-model)
- Baseline: the median private B2B SaaS NRR is about 106% (SaaS Capital, cited secondhand). A 2026 aggregator gives medians of about 108% NRR and 88% GRR. — [fungies.io citing SaaS Capital](https://fungies.io/saas-retention-strategies/); [GrowthSpree](https://www.growthspreeofficial.com/blogs/b2b-saas-nrr-grr-net-gross-revenue-retention-benchmarks-2026-by-acv-stage-vertical)
- Payments drag blended gross margin for vertical SaaS down to about 45-60%, because interchange sits in COGS (vendor blog). — [revenuemap.app](https://revenuemap.app/blog/vertical-saas-financial-model)
- Tidemark's market criteria for SMB verticals:
  - The market should be "gigantic", measured by number of locations, initial ARPU, and expansion ARPU.
  - It should be highly fragmented: avoid markets where SMBs "fight over the leftovers" of large enterprises.
  - It should have a fragmented value chain, which enables a marketplace and a payments profit pool.
  - Restaurants (500K+ locations) are the archetype (Toast, Olo).
  - Tidemark measures fragmentation as the share of a 6-digit NAICS industry held by everyone outside the top 50. — [Tidemark VSKP, Picking Your Market](https://www.tidemarkcap.com/vskp-chapter/picking-your-market); [Tidemark 2025 update](https://www.tidemarkcap.com/tools/where-are-the-opportunities-in-vertical-saas-picking-your-market-2025); [Tidemark census analysis](https://www.tidemarkcap.com/vskp-chapter/what-census-data-tells-us-about-growth-fragmentation-and-the-next-great-saas-markets-maybe)
- Tidemark: "market structure drives strategy". Merchant fragmentation and location count shape the fundraising needs and the roadmap. — [Tidemark, Market Structure Drives Strategy](https://tidemarkcap.com/post/market-structure-drives-strategy)

### Inferences
- A photo and case documentation app has no natural payments flow in most verticals, so the 2-5x fintech uplift probably isn't available. In verticals with a transaction attached, such as contractor invoicing or insurance claims, a second-product "monetization layer" might partly substitute. The more plausible "vertical uplift" here is workflow depth: compliance reports, integrations with the industry system of record, and AI that replaces labor (auto-captioning, report generation). That fits the a16z labor-budget thesis.
- Winner-take-most within a vertical means a horizontal tool competes against a vertical leader that will out-feature it. In contractors, that leader is already CompanyCam.

### Gaps
- No primary, methodologically sound dataset comparing vertical vs horizontal NRR and CAC turned up. The SaaS Capital and Benchmarkit originals were not retrieved.
- No CAC-via-community data (for example, the cost of trade-association channels vs paid ads).
- No Bessemer vertical-specific report was retrieved. Euclid and Fractal sources were not found in searches.

## Horizontal tools that won via templates and use cases (Airtable, Notion, Monday, Fulcrum, etc.)

### Takeaway
The winning horizontal tools did not launch "for everyone". They started with a narrow tinkerer persona, watched which pockets got organic adoption, then narrowed their landing focus. They mass-produced use-case templates, and later template and integration SEO pages, so each visitor sees a specific job. The positioning stayed broad, but acquisition was always use-case specific.

### Cited Findings
- **Airtable, initial users:** early adopters were "programmers or database people", a "tinker persona who's very structured in their thinking". An early pocket was a video producer tracking cast and crew. — [First Round Review, Airtable's Path to PMF](https://review.firstround.com/airtables-path-to-product-market-fit-lessons-for-building-horizontal-products/)
- **Airtable, alpha and focus:**
  - The invite-only private alpha lasted about two years: "We had to build so many features to get to something useful."
  - Ofstad cautions against casting a wide net too early: "We got more and more narrow with our focus on landing customers over time."
  - "You can build a horizontal product with a deep customer understanding of a specific use case". The key is "not going *too* deep on it". Avoid hyper-specific features that don't fit the platform. — [First Round](https://review.firstround.com/airtables-path-to-product-market-fit-lessons-for-building-horizontal-products/)
- **Airtable, feature-unlocked use cases:** a calendar view led to marketing use cases, and kanban led to pipeline use cases. Growth spread through functional and industry word-of-mouth. A media production head carried it from Netflix to a new company. Airtable now lands mainly through marketing and product organizations. — [First Round](https://review.firstround.com/airtables-path-to-product-market-fit-lessons-for-building-horizontal-products/)
- **Airtable, templates:** after organic adoption revealed use cases, the team "would create hundreds of templates". Enterprise inflection came around 2018, with CRO and CMO hires. Pricing was benchmarked against "the Salesforces and ServiceNows". — [First Round](https://review.firstround.com/airtables-path-to-product-market-fit-lessons-for-building-horizontal-products/)
- **Airtable, PLG:** the growth team used use-case templates for activation because the product is open-ended. Marketing found it hard to pick ad keywords for a horizontal product, so it promoted the most popular templates instead. Lesson: "If your product is horizontal, activate users with the use case templates and guidance that matter to them." — [Decibel, PLG and PQLs at Airtable](https://www.decibel.vc/articles/plg-pqls-and-shortening-the-time-to-a-ha-at-airtable)
- Airtable reportedly did not market itself as "a tool for everyone". It built specific templates and case studies for education, film production, and VC (secondary analysis). — [Holy Shift case study](https://www.holyshift.ai/insights/academy/airtable-product-market-fit)
- **Notion:** analysts attribute growth to community-led templates and a fan base. They warn that horizontal products can overwhelm users and risk not being best-in-class at any single job (secondary). — [Medium, Sherry Sun](https://medium.com/@sherrysun/notion-from-near-collapse-to-a-10b-all-in-one-workspace-unicorn-6113f7e1dc4c); [Lever Digital](https://www.leverdigital.co.uk/post/the-step-by-step-marketing-strategy-that-made-notion-a-household-name)
- **monday.com:**
  - Programmatic SEO produced one landing page per template ("CRM template", "content calendar template") plus integration pages (for example, Slack), combined with heavy brand advertising and invite-driven virality.
  - Traffic estimates conflict wildly: 14K vs 525K+ monthly organic visits.
  - Sources are third-party marketing blogs. — [Gracker case study](https://gracker.ai/case-studies/monday); [Lilach Bullock](https://www.lilachbullock.com/monday-com-marketing-strategy/)
- **Fulcrum (horizontal field data):**
  - Spatial Networks (founded 2000) first tried Geodexy. Commercialization attempts failed for about 3 years because it could not "identify a single commercial problem" it could solve.
  - It then built a vertical product, Allinspections, for residential home inspection (2008-2010). It shut down in 2012: "The market was not ready", and it was "too disruptive to the inspector community".
  - Fulcrum, a general "data collection platform", launched in October 2011 at the USGIF GEOINT Symposium: "It's not all things to all people, nor will it ever be, by design." — [Fulcrum, Origins of Fulcrum](https://www.fulcrumapp.com/blog/origins-of-fulcrum/)
- **Fulcrum scale:** by 2018, the company reported 110,000 users across 47,000 organizations in 180 countries (self-reported). Use cases ranged from utility pole inspection and vegetation management to municipal asset management and landmine clearance. A third-party directory claims Fulcrum was acquired in April 2024, which I could not verify. — [Fulcrum 2018 in Review](https://www.fulcrumapp.com/blog/2018-in-review/); [TrustRadius](https://www.trustradius.com/vendors/fulcrum); [motherbase profile](https://my.motherbase.ai/startup/40396-fulcrum)

### Inferences
- The Fulcrum story is the closest analog to this product: a vertical inspection app failed and a horizontal field-capture platform survived. But Fulcrum's horizontal success came from serving technical GIS and enterprise buyers who configure their own forms, not from SMB self-serve. A horizontal photo app aimed at non-technical teams needs Airtable-style templates to supply the structure users won't build themselves.
- For practical messaging, keep one brand promise (for example, "photo evidence for every case") with use-case and industry landing pages and templates underneath. Run acquisition (ads, SEO) on the use-case keywords, never on the generic category.
- An Airtable-style long alpha is risky for a small team. The safer variant is a wedge vertical with real paid usage, plus deliberate tracking of which off-wedge pockets show up organically.

### Gaps
- No primary sources on Typeform, Zapier, or Smartsheet first-user selection were retrieved. Zapier's integration-page SEO is well known, but it was not sourced here.
- No hard data on conversion lift from use-case landing pages.

## Photo and field documentation case studies (CompanyCam, Fulcrum, SiteCapture, OpenSpace, Procore) and failures

### Takeaway
The photo documentation category's biggest winners are vertical. CompanyCam serves residential and specialty contractors. OpenSpace serves general contractors with 360° capture. Procore is the construction system of record. Horizontal field-capture tools (Fulcrum, SiteCapture) stayed smaller and drifted toward specific verticals such as solar, utilities, and inspection. I found no documented shutdown of a horizontal photo-management startup. The closest cautionary tale is Fulcrum's own failed earlier attempts (Geodexy, Allinspections).

### Cited Findings
- **CompanyCam scale and positioning:**
  - It is a "visual communication platform for contractors". Growth was driven by referrals and product quality.
  - Founder Luke Hansen cites about 30,000 companies using it regularly and an obtainable market of about $1-1.5B (a podcast transcript, partially truncated).
  - Frontlines' write-up is titled "scaling CompanyCam to $50M ARR". — [Category Visionaries podcast](https://categoryvisionaries.podbean.com/e/luke-hansen-ceo-and-founder-of-companycam-38-million-raised-to-build-the-future-of-visual-communication-for-contractors/); [Frontlines.io write-up](https://www.frontlines.io/photos-projects-and-progress-luke-hansen-on-scaling-companycam-to-50m-arr/)
- **CompanyCam channels:** early marketing ran on Facebook, Instagram, and TikTok, showing contractors the product solving their problems. Google Ads and other intent-based channels "never had much luck", because contractors weren't searching for the category. — [Frontlines, Construction Tech GTM Channels](https://www.frontlines.io/construction-tech-gtm-channels/)
- **CompanyCam community and content:** CompanyCam founded in 2015. It hosts the Good Contractor Podcast, which interviews contractors across trades, and runs contractor marketing webinars. More than 5 million projects have been documented. — [Commercial Landscaper podcast](https://thecommerciallandscaper.libsyn.com/interview-with-luke-hansen-founder-ceo-of-companycam); [CompanyCam blog](https://companycam.com/resources/blog/35-lessons-from-the-good-contractor-podcast-what-separates-great-contractors-from-the-rest)
- **CompanyCam financials (conflicting):**
  - Latka reports $20.2M in 2023 revenue, up 41% YoY.
  - An August 2025 Series C was led by B Capital. One source claims a $415M round at a $2B valuation. CB Insights lists only about $39.6M in total funding.
  - The $415M/$2B figure is unconfirmed. — [Latka](https://getlatka.com/companies/companycam); [Preqin](https://www.preqin.com/data/profile/asset/companycam--inc-/381155); [Altaroc](https://www.altaroc.pe/en/resources/news/companycam-desormais-valorisee-2mds); [CB Insights](https://www.cbinsights.com/company/companycam/financials)
- **CompanyCam pricing:** an early Pro tier at $19/user/month plus a free tier for small companies (Series A era). — [Roofing Contractor](https://www.roofingcontractor.com/articles/94672-companycam-secures-6-million-investment?v=preview)
- **SiteCapture:** formerly FotoNotes. It is a mobile and cloud field tool for inspections, photo capture, and reporting, used heavily in solar, roofing, and battery site surveys and lender funding packages. It is listed from $30/month. Users praise high-resolution photos, timestamps, and bulk download. They complain that "creation of templates is a little confusing". — [Software Advice](https://www.softwareadvice.com/field-service/fotonotes-profile/); [Fluix, SiteCapture alternatives](https://fluix.io/blog/sitecapture-alternatives); [Capterra](https://capterra.com/p/128652/FotoNotes/)
- **OpenSpace:** 360° jobsite capture for GCs and owners (progress tracking, remote site walks). It raised a $55M Series C in 2021. Competitors include Multivista, StructionSite, and HoloBuilder. — [Bisnow](https://www.bisnow.com/national/news/technology/ai-construction-tech-firm-open-space-garners-55m-in-series-c-funding-108890); [IronPros](https://www.ironpros.com/product-categories/technology-and-software/site-data-capture/augmented-or-virtual-reality-systems/product/22903788/openspace)
- **Procore:**
  - Founded in 2002-03 after Tooey Courtemanche struggled to manage construction of his own home. Early clients were a few local home builders. The founders installed jobsite WiFi to drive adoption.
  - Its early dual focus was big contractors on clunky client-server software and small builders with no software.
  - The 2008 recession "decimated" its core builder customers, and it struggled to make payroll. Revenue was $4.8M in 2012.
  - Bessemer led a $15M round in 2014. — [Cloud Substack on Courtemanche](https://cloud.substack.com/p/how-tooey-courtemanche-built-a-1b); [BVP, Procore IPO story](https://www.bvp.com/atlas/procore-ipo)
- Procore's founders cite "focus" as key to their success. — [BuiltWorlds C-Suite Corner](https://builtworlds.com/?p=804)

### Inferences
- In construction, CompanyCam (SMB contractors), OpenSpace (GC 360 capture), and Procore (system of record) already contest photo documentation. Entering contractors as a wedge means competing against a well-funded, community-led leader. A wedge where photo case management is underserved is more promising.
- CompanyCam's finding that intent search didn't work is a warning sign for horizontal products. If buyers don't search for "team photo management", demand must be created through demonstration on social channels and in vertical communities. That is far easier when the content speaks to one trade.
- Template complexity complaints about SiteCapture echo the core horizontal risk: configurability that SMB users find confusing. Pre-built vertical templates reduce it.

### Gaps
- I found no documented failed horizontal photo-management startup. A search returned none, which does not prove none exist. Searching Crunchbase and CB Insights "dead startups" lists would be needed.
- No primary CompanyCam data for 2025-26 on ARR, NRR, or trade mix. No confirmation of whether CompanyCam expanded beyond contractors (for example, into property management or insurance).
- No SiteCapture revenue, funding, or ownership data.

## Wedge strategy: start narrow, expand (Toast, ServiceTitan, Veeva) and wedge-selection criteria

### Takeaway
The canonical vertical winners started with one painful workflow for one customer type, refused off-ICP revenue, then layered adjacent modules and fintech once they owned the segment. Veeva is the key "horizontal engine plus vertical skin" precedent: it ran pharma CRM on top of Salesforce, then later built its own platform. Wedge criteria from the literature: a large, fragmented market; acute workflow pain; data capture that compounds; a reachable community; and a short sales cycle.

### Cited Findings
- **ServiceTitan:**
  - It started as a call tracker for plumbing and HVAC firms and became a cloud platform in 2012. After about five years with local contractors, it expanded into dispatch, scheduling, CRM, estimates, mobile, reporting, and payments.
  - The co-founder's playbook: pick an industry and customer type, build for their workflows, then earn trust to build the other software they need. — [Alson Substack](https://alson.substack.com/p/fundamentally-servicetitan); [Cloud Substack, ServiceTitan](https://cloud.substack.com/p/from-30m-to-11b-the-servicetitan)
- ServiceTitan's leadership "refused to chase dollars outside their ICP, even when it meant turning down revenue" (former head of sales, via SaaStr, cited secondhand). It later landed a $4M ACV deal with a large plumbing franchise. — [Allison Pickens Substack](https://allisonpickens.substack.com/p/what-made-servicetitan-a-rocket-ship); [BVP, ServiceTitan path to IPO](https://bvp.com/atlas/servicetitan-a-values-driven-companys-path-to-ipo)
- ServiceTitan expanded into fintech by bundling payment processing and consumer financing (S-1 breakdown). One profile cites about 11,800 customers, $961M revenue, and $82.1B GTV for FY2026 (secondary, unverified). — [Alexandre Substack, S-1 breakdown](https://alexandre.substack.com/p/servicetitans-s-1-breakdown)
- **Toast:** the founders knocked on Boston restaurant doors and signed their first 50 customers by 2013. They expanded into inventory, KDS, payroll, online ordering, payments, and lending. Hardware in the building creates switching costs (secondary). — [Deal Lift](https://dealift.beehiiv.com/p/the-20b-pivot-how-three-friends-rebuilt-the-restaurant-industry-from-a-basement); [Colossus, Schreiber on Toast](https://colossus.com/episode/schreiber-toast-the-restaurant-operating-system/)
- **Veeva:**
  - Founder thesis: "The next wave after horizontal solutions like Accounting and Payroll would be something specific to an industry."
  - It targeted the largest pharma companies first.
  - It built on Salesforce so it could focus on industry-specific features rather than infrastructure. — [European Tech Substack, Veeva](https://europeantech.substack.com/p/veeva-systems-a-vertical-saas-champion); [Stacksync, origin of Veeva](https://www.stacksync.com/blog/one-industry-owned-completely-the-origin-story-of-veeva.md)
- **Veeva and Salesforce:** the contract reportedly included non-compete clauses that barred Veeva from some CRM segments (for example, MedTech). Veeva later moved CRM onto its own Vault platform, reportedly removing Salesforce fees of about 12% of revenue (secondary, unverified). Veeva is now building a horizontal CRM for large enterprises as a separate team, with product-market fit "in development". — [IntuitionLabs, Veeva-Salesforce split](https://intuitionlabs.ai/articles/veeva-salesforce-split-pharma-crm-shake-up); [Sergey CYW earnings analysis](https://sergeycyw.substack.com/p/veeva-systems-q4-2024-earnings-analysis)
- a16z (Joe Schmidt, quoted secondhand) defines a wedge as a product that "helps start a relationship with a customer and (hopefully!) is able to capture valuable downstream data". Wedges historically focus on capturing data. — [Venture Chronicles roundup](https://alexandre.substack.com/p/venture-chronicles-february-2025)
- Sacra argues vertical SaaS faces AI-driven compression, where owning a real operational wedge matters more than feature depth. — [Sacra](https://sacra.com/chat/h/2718e48d-a5d1-4429-8cc2-d8d352f00c50/)
- Tidemark criteria (large, fragmented, fragmented value chain) appear in Section 1. OMERS Ventures adds market efficiency and IT spend as parameters. — [OMERS Ventures](https://omersventures.com/news/finding-the-right-fit-in-vertical-saas)
- Bessemer frames its Procore, Toast, and Shopify bets as niche markets that scale through "expansion into adjacent fields". — [BVP, ServiceTitan](https://bvp.com/atlas/servicetitan-a-values-driven-companys-path-to-ipo)

### Inferences
- A wedge-selection scorecard synthesized from the sources:
  1. Acute, frequent photo-evidence pain tied to money or liability (disputes, claims, compliance).
  2. A large and fragmented customer base (Tidemark).
  3. Dense word-of-mouth or community channels (CompanyCam's social and podcast approach).
  4. Budget held by an owner-operator who can buy in one meeting.
  5. A short sales cycle with low regulatory burden (Section 5: healthcare fails this for a first wedge).
  6. Captured data that compounds into downstream products (a16z wedge definition).
  7. No entrenched, well-funded vertical leader (fails for contractors because of CompanyCam).
- Veeva shows "horizontal engine plus vertical skin" works and is defensible when the vertical layer carries domain depth: regulatory workflows, data, and integrations. It also shows the platform dependency risk. Here the analog is owning the core engine, so verticals become skins on your own platform, not on someone else's.
- The expansion pattern in every case: own the wedge (ServiceTitan's refusal of off-ICP revenue), then add adjacent modules for the same customer before adding new verticals. Moving into adjacent verticals is a later step (Veeva into MedTech and horizontal CRM, 15+ years in).

### Gaps
- No quantitative studies linking wedge-criteria scores to outcomes; the criteria are qualitative frameworks.
- The original a16z wedge essay was not retrieved, only quoted secondhand.

## PLG vs sales-led in healthcare vs SMB; healthcare procurement (SOC 2, HIPAA, BAA) costs and timelines

### Takeaway
Bottom-up, clinician-led PLG is real (Doximity, AI scribes), but it hits a ceiling once a clinic needs EHR integration, central security review, or a BAA. At that point it becomes a 12-18 month enterprise sale. Compliance costs are modest in dollars (roughly $35-80K for a first SOC 2 Type II, $15-40K for a lean HIPAA program) but heavy in time (6-14 months). That makes healthcare a poor first wedge for a small team, and a better expansion vertical once the engine and trust artifacts exist.

### Cited Findings
- **Healthcare sales cycles:**
  - Hospital sales cycles "routinely stretch 12 to 18 months and often longer". Healthcare software deals take 50-100% longer than comparable B2B sales (vendor marketing playbook). — [Elevation B2B](https://elevationb2b.com/blog/healthcare-software-marketing-playbook-for-hospitals/)
  - Another vendor blog cites an average of about 14.7 months. It gives about 12 months and about nine decision-makers for hospital software, about two years for enterprise EHRs, and 60-90 days for small physician-office software (unverified). — [Prospeo, Healthcare Sales Cycle](https://prospeo.io/s/healthcare-sales-cycle)
- A Black Book flash survey (October 2025, 107 US health IT vendors, ±9.5 points) found 63% reported longer sales cycles, most often by 30-60 days and 12% by 90+ days, tied to the federal shutdown. — [Black Book via AccessNewswire](https://www.accessnewswire.com/newsroom/en/healthcare-and-pharmaceutical/7-in-10-health-it-vendors-report-paused-procurements-as-shutdown-leng-1093544)
- **Security questionnaires without SOC 2:** each runs 100+ questions and takes 20-40 hours of sales and security time. They delay sales cycles by 4-8 weeks (vendor guide). — [Topflight, SOC 2 for Healthcare Startups](https://topflightapps.com/ideas/soc-2-healthcare-startups/)
- **SOC 2 cost and timing:** a first Type II audit costs about $35K-80K all-in, or $39K-70K in another source. Type I takes about 3-5 months and Type II about 9-14 months (compliance vendors). — [ComplyJet](https://www.complyjet.com/blog/soc-2-for-startups); [Promise Legal](https://promise.legal/guides/soc2-roadmap)
- **Fast track:** one healthtech company completed SOC 2 Type II in 90 days with a mature HIPAA program, fewer than 50 employees, and a single cloud. Companies with basic HIPAA compliance should plan for 4-5 months, and those with no baseline for 6-9 months. — [Agency case study](https://blog.getagency.com/articles/healthtech-soc-2-90-days-case-study)
- **HIPAA program costs:** a lean greenfield HIPAA build costs $15K-40K. An existing product with gaps costs $60K-120K. Enterprise-grade (SOC 2 Type II plus HITRUST) costs $150K-300K+. A credible Year 1 program takes 4-6 months. — [Topflight, HIPAA Year 1](https://topflightapps.com/ideas/hipaa-compliance-year-one/)
- **BAA and HITRUST:** a signed BAA plus a documented risk assessment is the legal baseline for covered-entity buyers and can be done in days to weeks. HITRUST costs about $30K-200K+ and is usually pursued only for a specific deal (hospital systems, payers). — [Topflight, HITRUST vs SOC 2 vs HIPAA](https://topflightapps.com/ideas/hitrust-vs-soc2-vs-hipaa/)
- **Healthcare PLG:** clinicians and scientists increasingly adopt consumer-grade tools bottom-up, and healthcare companies borrow cloud PLG playbooks. — [BVP Atlas, Direct-to-clinician](https://www.bvp.com/atlas/direct-to-clinician-how-product-led-growth-is-changing-healthcare-and-life-sciences); [Morgan Cheatham Substack](https://morgancheatham.substack.com/p/direct-to-clinician-how-product-led?r=1nwou0)
- **PLG ceiling:** for AI scribes, once deeper EHR integration or health-system sales is needed, "HIPAA paperwork, business associate agreements, and IT review" turn PLG back into a slow enterprise sale (Sacra, secondary). — [Sacra, EHR integration limits PLG](https://sacra.com/chat/h/01b09ab5-89f0-49e8-9302-c021003f5c4a/); [The Paths to Healthcare PLG](https://quiteafewclaims.substack.com/p/the-paths-to-healthcare-plg-part?open=false)
- **Doximity:** a HIPAA-compliant clinician network (secure messaging, eFax, telehealth). Clinicians use it free, and pharma and health systems pay. This is a bottom-up distribution model where the user is not the payer. — [GuruFocus](https://www.gurufocus.com/news/8696904/doximity-clinician-network-leader)

### Inferences
- Photos of patients (wound care, dermatology, clinical cases) are PHI. Any healthcare wedge requires a BAA from day one, HIPAA-grade storage, and soon SOC 2. The compliance time cost (6-14 months), on top of 12-18 month hospital cycles, fits poorly with a startup's first 18 months. Small private practices (60-90 day cycles) are the only PLG-compatible healthcare segment.
- In SMB verticals such as trades, property management, and field services, self-serve PLG with a free tier works (CompanyCam's free tier and $19/user Pro). The recommended sequence: wedge in an SMB vertical with PLG, build SOC 2 while scaling, then enter healthcare or enterprise verticals as a sales-assisted expansion with trust artifacts in hand.
- SOC 2 Type I (3-5 months) can start early, since it also unlocks mid-market buyers in non-healthcare verticals.

### Gaps
- No primary or independent data on healthcare startup sales cycles; all figures come from vendor blogs.
- Nothing found on Figure1 or other clinical photo-sharing apps' GTM.
- No data on what fraction of health-system deals require HITRUST vs accept SOC 2.

## Platform architecture: one core engine with vertical "skins" and templates

### Takeaway
Precedent supports a single core engine with vertical layers (templates, terminology, report formats, integrations) on top. Airtable (templates over one engine), Fulcrum (configurable forms over one capture platform), and Veeva (pharma-specific layer over a generic CRM engine) all built this way. Tidemark data shows multiproduct platforms reach about 10x the addressable market. The risk is building a skin too thin to beat vertical leaders, or so many hyper-specific features that the core fragments.

### Cited Findings
- Veeva built pharma-specific features on the Salesforce engine, so it could focus on "industry-specific customisations rather than developing infrastructure from scratch". Over about 15 years it invested in its own Vault platform for optionality. — [European Tech Substack](https://europeantech.substack.com/p/veeva-systems-a-vertical-saas-champion); [IntuitionLabs](https://intuitionlabs.ai/articles/veeva-api)
- Airtable avoided hyper-specific apps that don't pattern-match to the platform vision, and used hundreds of templates as the vertical layer over one engine. — [First Round](https://review.firstround.com/airtables-path-to-product-market-fit-lessons-for-building-horizontal-products/)
- Fulcrum is positioned as "a data collection platform... not all things to all people... by design". It serves utilities, municipalities, and humanitarian demining through configuration rather than separate products. — [Fulcrum Origins](https://www.fulcrumapp.com/blog/origins-of-fulcrum/); [Fulcrum 2018 in Review](https://www.fulcrumapp.com/blog/2018-in-review/)
- Multiproduct vertical platforms have about 10x the addressable market and grow 21% faster than single-product ones. — [Stripe/Tidemark](https://stripe.com/lp/vertical-saas-benchmark-2025)
- Horizontal products risk being overwhelming and "not best-in-class at any single function". Templates, examples, and education are the remedy. — [Medium, Notion analysis](https://medium.com/@sherrysun/notion-from-near-collapse-to-a-10b-all-in-one-workspace-unicorn-6113f7e1dc4c)
- SiteCapture users find template creation confusing, which shows the cost of exposing raw configurability to SMB users. — [Software Advice](https://www.softwareadvice.com/field-service/fotonotes-profile/)

### Inferences
- Suggested layering for a team photo case management app:
  - **Core engine:** capture with metadata (GPS, time, user), cases and projects, tagging, annotation, timeline, permissions, sharing links, report and PDF generation, API and webhooks, AI captioning and search.
  - **Vertical skin:** vocabulary ("job", "claim", "patient case", "unit"), default case templates and checklists, report formats (insurance claim packet, before/after, compliance log), integrations with that vertical's system of record (for example, ServiceTitan or JobTread for trades, Yardi for property management, an EHR for healthcare), and compliance mode (HIPAA/BAA, retention).
  - **GTM skin:** a vertical landing page, onboarding flow, template gallery, and community content.
- Build the engine so a new vertical is mostly configuration (templates, terms, report specs) plus one or two integrations. Then expansion cost per vertical is mainly GTM, the Airtable and monday template-page approach.
- Launch recommendation from the evidence: brand and architect horizontally, but go to market vertically. Pick one wedge using the Section 4 scorecard. Avoid contractors (CompanyCam) and healthcare as the first wedge (compliance and cycle length). Track organic off-wedge adoption pockets the way Airtable did to choose vertical #2.

### Gaps
- No published engineering case study on "vertical skins" architecture for SaaS. The claims are inferred from GTM narratives, not technical writeups.
- No data on the incremental cost of adding a vertical skin (engineering or GTM) for comparable companies.
