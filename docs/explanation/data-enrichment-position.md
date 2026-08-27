# Margince will not scrape LinkedIn. We could build it in a week. We still won't.

**Our position on compliant data enrichment in the EU, Germany and Vietnam.**

| | |
|---|---|
| **Author** | Lars Jankowfsky |
| **Date** | 27 August 2026 |
| **Scope** | European Union law (with German specifics) and Vietnamese law. Other jurisdictions can be added later; nothing here covers them yet. |
| **Status** | Position paper, adversarially fact-checked. This is research to brief counsel with, not legal advice. Every load-bearing claim carries a named source. |

---

## The position

Almost every modern CRM demo now contains the same little magic trick. Someone opens a LinkedIn profile, clicks a browser plugin and watches the contact record fill itself. Name. Job title. Email. Phone number. Everybody claps.

Margince will never sell that feature.

There will always be third-party vendors willing to serve companies that prefer not to look too closely at the legal situation. I will not stop them. But we sell a compliant product to regulated companies and larger SMEs. Add one illegal feature and the product is no longer compliant. Punkt.

We could build the plugin in a week. Technical difficulty was never the issue. The standard model breaches LinkedIn's contract, collides with European data protection law and creates exactly the kind of employee-monitoring problem that German works councils are paid to find. In Vietnam, the position is even harder.

The fines stopped being theoretical years ago. Since 2019, three European regulators have fined enrichment databases for the same missing duty: telling the people whose data they had collected. Bisnode built its database from registers. Kaspr fed one from LinkedIn. Lusha combined multiple broker sources. LinkedIn itself took the most famous scraper in the world all the way to a permanent injunction.

But the research also surprised me. The law does not force a CRM to stay dumb. Several enrichment features are perfectly defensible, and one of the most valuable, automated Article 14 notification, appears to be absent from this market entirely.

That gap decides what we build.

## 1. The industry standard starts with a contract breach

I started with LinkedIn's contract because, frankly, it already kills the standard plugin model.

Apollo, Kaspr, Lusha and Waalaxy all sell some version of the same mechanism. A Chrome extension runs inside a salesperson's logged-in LinkedIn session. It reads each profile the person visits, parses the fields and posts them to the vendor's backend. That is the entire magic trick.

Now read Section 8.2 of LinkedIn's [User Agreement](https://www.linkedin.com/legal/user-agreement). It prohibits users from developing, supporting or using software, devices, scripts, robots or any other means or processes, explicitly including crawlers, **browser plugins** and add-ons, to scrape or copy the Services, including profiles.

Those three verbs matter: develop, support and use. They bind everyone who accepted the agreement. In practice, that reaches both sides of the plugin economy. The user is bound by definition. The vendor is bound through the accounts it holds to build and test the product. The hiQ court later held hiQ's own corporate account against it.

Another clause prohibits using LinkedIn data obtained through third parties, including search tools, **data aggregators or brokers**, without the content owner's consent. A CRM that stores purchased LinkedIn profile data sits directly inside that clause's reach.

Buying a Sales Navigator seat changes nothing. The [subscription agreement](https://www.linkedin.com/legal/l/lsa) calls unauthorized scraping an "Incurable Breach". There is no CSV export. The only sanctioned route from Sales Navigator into a CRM is CRM Sync on the Advanced Plus tier, and LinkedIn limits it to a short list of large incumbents: Salesforce, Microsoft Dynamics 365, HubSpot and Oracle Sales. LinkedIn publishes the list in its [CRM matrix](https://www.linkedin.com/help/linkedin/answer/a106005/?lang=en).

Then somebody always says: "But hiQ won. Scraping public profiles is legal."

That sentence may be the most expensive misreading in our industry.

The 9th Circuit said something much narrower in 2019 and reaffirmed it in [April 2022](https://cdn.ca9.uscourts.gov/datastore/opinions/2022/04/18/17-16783.pdf): scraping publicly available pages was likely not a federal *hacking crime* under the Computer Fraud and Abuse Act.

That ruling did not bless scraping as a business model.

On remand, in [November 2022](https://caselaw.findlaw.com/court/us-dis-crt-n-d-cal/2182242.html), the district court held LinkedIn's anti-scraping terms enforceable and granted LinkedIn summary judgment over hiQ's use of fake "turker" accounts. Liability for the scraping itself remained open because of waiver and estoppel questions.

The court never answered those questions. hiQ folded and [settled](https://www.zwillgen.com/alternative-data/hiq-v-linkedin-wrapped-up-web-scraping-lessons-learned/) instead: a negotiated consent judgment of USD 500,000, a permanent injunction with no distinction between public and private pages, and deletion of all scraped data and code.

No precedent was set. Something more practical was. The company celebrated for "making scraping legal" ended the fight enjoined and gone.

LinkedIn has kept going. It sued [Mantheos](https://news.linkedin.com/2022/february/taking-legal-action-to-protect-members-against-scraping), which settled, and [ProAPIs](https://securityaffairs.com/183001/security/linkedin-sues-proapis-for-15k-month-linkedin-data-scraping-scheme.html), filed in 2025.

The official route is closed as well. LinkedIn's [partner documentation](https://learn.microsoft.com/en-us/linkedin/sales/) says it is "not currently accepting new partners" for the Sales Navigator API. The self-service API returns only the name, email address and photo of the single member who gave consent through OpenID Connect.

Now look at who carries the risk in the plugin economy.

LinkedIn sues the vendor. It bans the end user's account. That second consequence is faster and more certain, and it lands on YOUR sales rep. The rep loses a professional identity they may have built for years because a software vendor wanted to avoid applying for an API.

Then European regulators fine whoever stores the data.

That is the second layer.

## 2. "Publicly available" does not mean "free to take"

The GDPR does not ban B2B enrichment. That is important because a lot of bad legal analysis begins by pretending the answer is simply no.

In 2024, the Court of Justice confirmed that a purely commercial interest can qualify as a legitimate interest under Article 6(1)(f) ([*KNLTB*, C-621/22](https://privacymatters.dlapiper.com/2024/10/eu-cjeu-confirms-that-legitimate-interests-can-cover-purely-commercial-interests/)). Recital 47 expressly names direct marketing as a possible legitimate interest. France's CNIL and the UK's ICO both accept B2B prospecting on that basis when the data relates to the person's professional capacity and the person can opt out.

Retention still needs a reason. CNIL works with a benchmark of roughly three years after the last contact. The ICO [sets no fixed period](https://ico.org.uk/for-organisations/direct-marketing-and-privacy-and-electronic-communications/direct-marketing-guidance/plan-direct-marketing/) and expects the controller to justify whatever period it chooses.

The part that surprised me was Article 14.

Enrichment can be lawful. Secret enrichment usually is not. When data was not collected from the person, the controller normally has to tell them individually within one month or at the first contact, whichever comes first. Publishing a privacy policy on your own website does not solve the problem. The person has never heard of you and has no reason to visit it.

Article 14(5)(b) does contain a disproportionate-effort exception. It comes with conditions and compensating safeguards, and the leading case shows how little the sentence "notification is expensive" achieves by itself.

Three cases tell the story. The source changed each time. The missing duty did not.

### Bisnode: public registers did not make the people disappear

Between 2019 and 2023, Bisnode fought over a database containing 7.5 million sole traders and company directors, all collected from *public government registers*. It emailed the 682,000 people for whom it held an email address and placed a notice on its website for everyone else. Bisnode argued that sending postal notices would require disproportionate effort.

The Polish DPA [fined it](https://uodo.gov.pl/en/553/1572) roughly EUR 220,000 and ordered individual notification.

The Supreme Administrative Court dismissed the final appeal on 19 September 2023. Transparency was the rule. Exceptions had to be read restrictively. On these facts, cost alone did not carry the exception, and the public-register origin changed nothing.

The duty follows the data.

### Kaspr: LinkedIn visibility choices mattered

On 5 December 2024, [CNIL fined Kaspr](https://www.cnil.fr/en/data-scraping-kaspr-fined-eu240000) EUR 240,000. Kaspr's browser extension fed a database of 160 million contacts from LinkedIn.

CNIL found four problems that matter here:

- Kaspr collected contact data from members who had *restricted their visibility*. That choice defeated the legitimate-interest balancing test.
- It retained the data for five years.
- It delivered no Article 14 notice at all until 2022.
- In access-request responses, it named only "publicly available sources", although CNIL held that Kaspr had to provide all source information it actually possessed.

CNIL later [closed the injunction](https://www.cnil.fr/en/closure-order-issued-against-kaspr). Read that closure carefully before describing it as regulatory approval. Kaspr complied by deleting the offending data and stopping the LinkedIn collection. The regulator approved the exit from the model.

### Lusha: a compliance seal did not save the database

In July 2026, Italy's Garante [fined Lusha](https://www.garanteprivacy.it/home/docweb/-/docweb-display/docweb/10275230) EUR 2 million and ordered deletion of ALL Italian contact data within 60 days.

Lusha's US domicile did not change the result. Neither did its TrustArc compliance seal. The Garante took the certification into account when calculating the fine, and the answer was still two million euros.

Around those three enrichment cases sit the harder scraping precedents.

Clearview AI scraped public photographs and has accumulated roughly EUR 100 million in fines across five European authorities: EUR 20 million each in Italy, Greece and France, EUR 30.5 million in the Netherlands, plus the UK's GBP 7.5 million penalty, which is [still in litigation](https://ico.org.uk/about-the-ico/media-centre/news-and-blogs/2025/10/uk-upper-tribunal-hands-down-judgment-on-clearview-ai-inc/). France later added a EUR 5.2 million non-compliance penalty.

The customer carries its own liability. Sweden's IMY [fined the police](https://www.imy.se/en/about-us/arkiv/nyhetsarkiv/police-unlawfully-used-facial-recognition-app/) EUR 250,000 merely for *using* Clearview. In August 2023, twelve DPAs signed a [joint statement on data scraping](https://ico.org.uk/media2/migrated/4026232/joint-statement-data-scraping-202308.pdf). Buying or using unlawfully scraped data does not move the liability somewhere else.

Even the platforms get pulled in. The Irish DPC fined Meta [EUR 265 million](https://www.dataprotection.ie/en/news-media/press-releases/data-protection-commission-announces-decision-in-facebook-data-scraping-inquiry) for its data-protection-by-design failures after a scraped dataset of 533 million users surfaced. That decision binds Meta, not every online platform. But the direction is uncomfortable for the plugin industry: the host platform's design duty runs against scraping.

For anyone holding enriched data, two product consequences follow.

First, Article 15(1)(g) gives the person a right to "any available information" about the source. Kaspr shows that a regulator will reject "public sources" when the company knows more. Margince therefore stores per-field provenance. That goes beyond the literal wording and makes the eventual data-subject access answer easy.

Second, erasure has to stick. The law does not [literally mandate a suppression list](https://ico.org.uk/for-organisations/direct-marketing-and-privacy-and-electronic-communications/direct-marketing-guidance/respect-peoples-preferences/), but the next synchronization will otherwise resurrect the deleted contact. Then you are back in the same lawful-basis analysis with worse facts. The ICO recommends a suppression list. We ship one. The design is documented in [privacy-and-consent.md](privacy-and-consent.md).

I understood the product line only after reading all three enrichment decisions: scale changes the facts.

A whole-market database combines sources, tells nobody and exists before any salesperson has a reason to research a specific person. Every fine above attacked that pattern. A salesperson researching one prospect still needs an Article 6 balancing assessment, just like any other processing. But that lookup enters the test with the strongest available facts: one narrow purpose, minimal fields, and an Article 14(3)(b) notice delivered with the first outreach at zero marginal cost.

Trigger and scale separate a defensible lookup from the Bisnode pattern.

One more line matters, especially in Germany. *Storing* a business email address is a GDPR question. *Sending marketing to it* is governed separately.

Germany's [Section 7 UWG](https://www.gesetze-im-internet.de/englisch_uwg/englisch_uwg.pdf) requires prior express consent for email marketing, including B2B email. Section 7(3) contains one narrow exception for existing customers, the same-product context and an opt-out at every step.

Your CRM may lawfully hold an address that you still may not cold-email from Frankfurt.

## 3. Vietnam: consent or nothing

I live in Vietnam. We build Margince for companies here as well as in Germany. Vietnamese law is therefore not an academic footnote for us.

It is also the strictest law in this paper, and most Western vendors appear not to have noticed that it exists.

The timeline matters. Vietnam operated under [Decree 13/2023/ND-CP](https://thuvienphapluat.vn/van-ban/EN/Cong-nghe-thong-tin/Decree-No-13-2023-ND-CP-dated-April-17-2023-on-protection-of-personal-data/564343/tieng-anh.aspx) until the end of 2025. Since 1 January 2026, the Personal Data Protection Law has applied. [Law 91/2025/QH15](https://english.luatvietnam.vn/dan-su/law-on-personal-data-protection-law-no-91-2025-qh15-405135-d1.html) was passed on 26 June 2025, and implementing Decree 356/2025 entered into force on the same day as the law.

The Vietnamese model is harder than the GDPR. Processing rests on consent plus a short, closed list of exceptions. Its "legitimate interest" basis is defensive only. It covers protecting rights against an act of infringement, which means use cases such as fraud prevention and legal claims.

Every law-firm analysis I checked reaches the same conclusion: [Tilleke & Gibbins](https://www.tilleke.com/insights/vietnams-new-personal-data-protection-law-a-closer-look/), [Baker McKenzie](https://connectontech.bakermckenzie.com/vietnam-decoding-vietnams-pdp-law-gdpr-inspired-rules-with-local-twists/), [DLA Piper](https://www.dlapiperdataprotection.com/?t=law&c=VN) and [Rouse](https://rouse.com/insights/news/2025/vietnam-s-new-personal-data-protection-law-what-businesses-need-to-know). That basis cannot carry prospecting or enrichment.

There is no separate basis for publicly available data. Publishing personal data waives nothing. Marketing requires consent, with no soft opt-in, on top of the Decree 91/2020 anti-spam regime and its do-not-call register.

Article 7(6) prohibits buying and selling personal data unless the law itself provides otherwise. The statute could hardly be clearer: "Personal data is not a commodity."

The penalty rules in Article 8 and the sanctions regime reach ten times the revenue gained from illegal data trading. Violations of the cross-border transfer rules can reach 5% of prior-year revenue. The [official text is available through the Ministry of Public Security](https://bocongan.gov.vn/media/bca-media/photo-library/20250728100402_a4c3955c-ea1d-48a0-9998-c262b14864af-Lu%E1%BA%ADt-B%E1%BA%A3o-v%E1%BB%87-d%E1%BB%AF-li%E1%BB%87u-c%C3%A1-nh%C3%A2n.pdf).

Buying broker data about Vietnamese contacts is the highest-risk act in this entire area.

Enforcement is young. The Ministry of Public Security's A05 department ran its first compliance campaign in late 2024, and no published fine has yet hit a foreign B2B SaaS company.

I would not build a product on the assumption that this lasts.

Vietnam's data-trading black market is the stated reason the law exists. Scraping and aggregation therefore sit in the political crosshairs. They do not occupy a tolerated grey zone.

The product consequence is blunt. A Vietnamese data subject needs consent that the law recognises, and Law 91 requires that consent to be voluntary, informed, specific and verifiable.

A business card can begin a business relationship. It does not prove consent to enrichment, marketing or onward disclosure. Our verifiable path is the confirm-your-details flow described in A6 below. The contact sees the record and says yes on the record.

## 4. The Betriebsrat will read this paper too

American CRM vendors regularly forget the person who reviews software in Germany: the works council.

Germany's Federal Labour Court gave us the doctrine in one perfect image in 2024. A retail headset system required works-council co-determination even though it stored no data at all ([1 ABR 16/23](https://www.bundesarbeitsgericht.de/wp-content/uploads/2024/11/1-ABR-16-23.pdf)).

Under [Section 87(1)(6) BetrVG](https://www.gesetze-im-internet.de/betrvg/__87.html), any technical system *capable* of monitoring employee performance or behaviour is co-determined. The employer's intent is irrelevant. There is no de-minimis threshold.

The court has applied the rule to an Excel attendance list ([1 ABN 36/18](https://www.bundesarbeitsgericht.de/entscheidung/1-abn-36-18/)), SAP (1 ABR 45/11) and Office 365 ([1 ABR 20/21](https://www.hensche.de/bag-beschluss-vom-08.03.2022-1-abr-20-21-zustaendigkeit-des-gesamtbetriebsrats-bei-unternehmenseinheitlicher-nutzung-von-microsoft-office-365.html)).

A CRM containing an audit trail and pipeline numbers per sales rep is co-determined. Period.

That part is routine. A German enterprise CRM rollout normally includes a Betriebsvereinbarung. A well-drafted agreement can itself provide the GDPR basis for employee-side processing when it meets Article 88(2) GDPR and [Section 26(4) BDSG](https://www.gesetze-im-internet.de/englisch_bdsg/englisch_bdsg.html).

The feature list decides whether that negotiation takes six weeks or eighteen months.

Server-side enrichment of *external* contacts barely registers. The only employee datum involved is the audit-trail fact that user X clicked. Scraping plugins, however, combine the three things works councils actually fight:

- an extension inside the employee's browser that observes what the employee views;
- work forced through the employee's personal LinkedIn account;
- mining that turns team correspondence into per-employee conduct profiles.

The Kaspr decision gives any works council a ready-made argument that the tool itself is legally toxic.

Since 2021, an AI layer has sat on top. Section 90 BetrVG requires the employer to inform the works council during the *planning* stage of AI use. The EU AI Act classifies systems that monitor and evaluate worker performance as high-risk under Annex III 4(b).

The 2026 AI omnibus moved the compliance deadline for Annex III systems from 2 August 2026 to 2 December 2027 ([Regulation (EU) 2026/1744](https://www.steptoe.com/en/news-publications/steptechtoe-blog/eu-ai-act-amendments-enter-into-force.html)). The category still exists. Only the deadline moved.

Our rule stands regardless of the date: the AI evaluates records, deals and companies. It never evaluates the work quality of a named sales rep.

Where does Margince itself sit?

Squarely inside co-determination. I will not pretend otherwise.

Margince captures connected mailboxes, including message bodies, because correspondence CRM is the product. The architecture is documented in [capture-connectors.md](capture-connectors.md) and [ingress-gate-and-auto-capture.md](ingress-gate-and-auto-capture.md). The [relationship graph](relationship-graph.md) uses that correspondence to calculate each colleague's warmth toward each *contact*. "Who here can open this door?" is a legitimate account question for a sales team.

Both features are designed to be governed:

- no browser extension;
- everything runs server-side;
- no rep leaderboards and no performance scores;
- warmth bands are deliberately non-comparable across people and cannot be merged into a ranking;
- exclusion lists keep mailboxes and domains that must never be captured outside the system;
- access to audit data is restricted.

The Betriebsvereinbarung then fixes the purpose in writing: account coverage yes, performance evaluation never, backed by a Verwertungsverbot.

That negotiation is winnable because the product gives the works council something concrete to hold. A scraping plugin gives it something to veto.

## 5. What we build, what we will build, and what we refuse

The product policy now becomes very simple.

We will build A1 to A6 once there is demand. No worries, we release quickly; you won't have to wait months for this. It might be built already if you read this much later than August 2026. We refuse all five Tier C models.

Each item below is rated on four axes: GDPR posture, Vietnam compatibility, platform-contract exposure and works-council temperature.

### Tier A: build freely

**A1. Company enrichment from official sources.** A1 remains simple until a natural person appears. Data about the legal entity itself sits outside the GDPR under Recital 14: company name, legal form, VAT number and registered address.

| Source | Data | Access | Limit |
|---|---|---|---|
| [VIES](https://ec.europa.eu/taxation_customs/vies/) | VAT validity, registered name and address in many member states | Free API; we store the consultation number as proof | Availability varies by state |
| [GLEIF LEI](https://www.gleif.org/en/about/open-data) | Legal entity plus ownership tree | Free API and bulk files, CC0 | Coverage thins outside finance |
| Swiss Zefix | Commercial register, daily change feed | Free REST API | None that bites |
| UK Companies House | Register, officers, filings | Free API and streaming | Rate limits only |
| German Handelsregister | Register data, free since the 2022 DiRUG reform | Web, per-lookup | Portal caps queries and bans bulk |
| Licensed aggregators (e.g. North Data) | Structured DACH register and financial data | Paid API | The clean bulk route |

The line must stay sharp. The moment a natural person appears in the source, perhaps a director, sole trader or named representative, the information becomes personal data again. Publication under a statutory duty creates the strongest legitimate-interest fact pattern available. It does not remove the duties attached to personal data.

The identifier spine is EUID, LEI, VAT ID and the Vietnamese MST.

**A2. The counterparty's own website, read deeper.** Built already, and it needs to be enhanced.

Margince already reads company websites, as described in [company-context.md](company-context.md). The deeper read handles people named on those sites in exactly the shape this paper argues for. A stranger found on a team page is staged as a lead and remains staged until a human accepts. The system never auto-creates a new person.

When the published person unmistakably matches someone already recorded at that company, through an exact email match or a single high-confidence name match, Margince fills empty fields such as role, title and profile link. Every value is evidence-backed with the source stored on the row. The system never overwrites information entered by a human.

The clean extensions are straightforward: parse the Impressum, whose publication German law *mandates* under Section 5 DDG, and consume newsroom RSS feeds for company events. We store the signal and a link, never a cached copy of the full text.

Crawling stays polite and respects robots.txt. The existing rule also remains: a human click can write directly; an automatic read stages the suggestion.

**A3. Technical enrichment.** Not built yet: none of this is in the tree today.

MX and DNS records, TLS certificate-transparency logs, and technology-stack fingerprints from the company's public pages are company-level signals. They work the same way in Hamburg and Da Nang.

The A1 caveat still applies. A personal name inside a certificate or domain record is personal data, so Margince keeps the stored signal at company level.

**A4. First-party person capture.** Half built.

The contact sent you the details. Margince's capture works from that fact. Connected mailboxes are processed automatically. A nightly pass extracts only stated fields from email signatures: name, title and phone number, with nothing inferred. Workspace exclusion lists keep entire addresses and domains outside capture.

Next, this tier extends cleanly to vCard import and a per-mailbox switch for the signature pass. An organisation can turn the granularity all the way down.

First-party collection is the only person-data channel that scales cleanly. See [capture-connectors.md](capture-connectors.md) and [ingress-gate-and-auto-capture.md](ingress-gate-and-auto-capture.md).

**A5. LinkedIn as a link, plus the member's own export.** We store the LinkedIn profile URL as a clickable link. Margince never fetches the page server-side.

The user reads the profile in their own browser and manually enters what they learned into a structured form. LinkedIn also allows every member to download their own data. The flow in [import-your-linkedin-network.md](../how-to/import-your-linkedin-network.md) imports the member's `Connections.csv` in a deliberately limited form.

The imported connections become private graph substrate, "ghosts", used only for reach and warmth. They are visible only to the importer. They never become contact records and never create people.

I do not claim that LinkedIn's terms permit anything beyond this private graph use. Margince therefore stops there. It does not create contacts and provides no parser for pasted profile text. Once software copies the page on the user's behalf, the scraping clause is back in play. We draw the line on the safe side.

**A6. The "confirm your details" flow.** Build. Nothing of this ships yet; it is the first Tier A item on the roadmap.

The design: Margince sends the contact a link through which they can view and correct their own card. The same flow delivers the Article 14 transparency notice and gives the contact an Article 16 accuracy mechanism. Because the consent is verifiable, it also meets the standard in Vietnam's Law 91.

Once built, this flow actively reduces the risk of everything else on this page.

### Tier B: build with controls, counsel signs off first

**B1. Widening person lookup beyond the counterparty's own site.** B1 stays gated.

The shipped A2 shape provides the correct chassis: staged leads, fill-only-empty behaviour, evidence on every field and per-field provenance through the write shape documented in [write-backbone.md](write-backbone.md). The lookup covers one company at a time. It uses sources from which the person published details for business contact. A human remains between the suggestion and the record.

New sources beyond the company's own website could include speaker biographies, conference pages and Impressum contact lines belonging to third parties. Each one enters only under the same staging discipline, and every expansion needs its own written Article 6 balancing assessment. A generic statement that "single lookups are fine" is not enough.

Before Margince widens anything, these missing controls must ship:

- an Article 14 notice generated and delivered with the first outreach, or within one month after a staged lead is accepted;
- a justified retention rule with refresh-or-delete, beginning from CNIL's three-year benchmark;
- an erasure suppression list checked before a new read can re-stage a deleted person;
- respect for visibility restrictions wherever a platform provides them, because that is precisely where Kaspr lost;
- exclusion of Vietnamese subjects, who are routed to A6.

**B2. Article 14 notice automation as a customer feature.** The CRM generates, sends and logs the notification that our customer owes whenever contact data was not collected from the subject. Objection handling connects directly to the suppression list.

I have not found this feature at any vendor. I think I know why. Their model cannot survive transparency. Ours is built around it.

The notice text does not ship until counsel has reviewed it market by market.

**B3. Licensed notified-database providers.** Providers such as [Cognism](https://www.cognism.com/compliance) and Dealfront operate what they call a "notified database". They claim Article 6(1)(f) based on a documented balancing test and say they discharge Article 14 by emailing the data subjects.

Those are provider assertions. No regulator has certified the model. The customer remains an independent controller with duties of its own.

In Margince, anything a licensed provider asserts about a person lands as a claim row (`person_provider_claim`) separate from the contact record, with the purchasing run attached. A delete-data action removes the local claims and scrubs the run metadata.

Two controls still do not exist:

- automatic purge when a provider is disconnected;
- downstream plumbing for erasure signals received from the provider.

Both belong on the same counsel checklist as the provider contract. Until they exist, B3 remains a gated integration. It is never available for Vietnamese subjects because Article 7(6) prohibits the trade itself.

### Tier C: refuse

**Bulk background enrichment of a market.** This is the Bisnode and Lusha pattern: a permanent shadow database of everyone, created before any user asks for a specific person. It has no realistic Article 14 story. Scale destroys the balancing argument, and Italy has demonstrated that a regulator will order deletion of an entire national dataset.

Reading one company's own website for a human researching that company, as in A2, sits at the opposite end of the proportionality scale.

**LinkedIn or Xing ingestion in any automated form.** LinkedIn ingestion breaches the quoted contract for everyone who accepted it, repeats the Kaspr fact pattern and has no open sanctioned API route. Moving the action into the user's browser solves nothing. LinkedIn sues this exact model.

I cite no Xing contract clause here. The GDPR and works-council analyses independently put Xing ingestion in Tier C. That is enough.

**Any browser extension.** It fails twice. LinkedIn's clause expressly names plugins, and software that observes what an employee views in a browser is textbook Section 87(1)(6) surveillance.

Some narrow extension might survive a lawyer's reading. This product line has no reason to live inside that argument.

**Purchased lists and broker data of unclear provenance.** The recipient becomes controller of data without a lawful-basis chain. If the list contains Vietnamese subjects, the trading ban and its ten-times-revenue penalty sit on top.

**Rep scoring.** We refuse it, and the precision matters because Margince DOES compute from correspondence, as explained in Section 4.

Margince creates no inferred employee traits, conduct profiles or performance scores. It exposes no surface that ranks reps against one another. The relationship graph answers one account question: "Who can open this door for this contact?" It answers per contact, using bands designed not to be summed into a scoreboard.

Whether mailbox processing satisfies Section 26 BDSG must be assessed purpose by purpose. Account coverage with these controls is a purpose we can defend in writing. Rep scoring is not. It remains refused.

## 6. Sources and verification status

The primary decisions and rulings linked above were verified against the issuing body where available:

- CJEU C-621/22 (*KNLTB*);
- UODO v. Bisnode, including the NSA ruling of 19 September 2023;
- CNIL v. Kaspr, EUR 240,000 on 5 December 2024, plus the closure notice;
- Garante v. Lusha, EUR 2 million in July 2026;
- the Clearview AI fine series in Italy, Greece, France, the Netherlands and the UK, with the UK litigation continuing;
- IMY v. the Swedish police;
- Irish DPC v. Meta, EUR 265 million;
- hiQ v. LinkedIn, including the 9th Circuit ruling in 2022, the N.D. California ruling in November 2022 and the consent judgment in December 2022;
- LinkedIn v. Mantheos and LinkedIn v. ProAPIs;
- BAG 1 ABN 36/18, 1 ABR 45/11, 1 ABR 20/21 and 1 ABR 16/23.

The statutory set is:

- GDPR Articles 5, 6(1)(f), 14, 15(1)(g), 17, 21 and 88, plus Recitals 14 and 47;
- Sections 87(1)(6), 90 and 80(3) BetrVG;
- Section 26 BDSG;
- Section 5 DDG;
- Section 7 UWG;
- EU AI Act Annex III 4(b), as amended by Regulation (EU) 2026/1744;
- Vietnam Decree 13/2023/ND-CP, Personal Data Protection Law 91/2025/QH15 Articles 7(6), 8 and 9, Decree 356/2025 and Decree 91/2020.

The Vietnamese-law analysis rests on the official texts plus concurring commentary from Tilleke & Gibbins, DLA Piper, Baker McKenzie, Rouse and DFDL.

On 27 August 2026, I reconciled all 26 findings from an adversarial fact-check into this version.

Counsel still needs to close four Tier B questions:

- the final adoption status of the EDPB's Guidelines 1/2024 on legitimate interest, still in draft as of this writing;
- Vietnam's administrative sanctions decree, which remains in flux through 2026;
- the exact section numbering in LinkedIn's subscription agreement;
- the Section 7 UWG detail for each outreach channel.

None of those open questions changes a tier placement.

## 7. Why this wins deals

This paper defines what Margince can sell with a straight face.

The scraping vendors made a bet. They assumed LinkedIn enforcement would stay slow, DPAs would remain busy elsewhere and no works council would read CNIL's press releases.

That bet gets worse every year.

Since 2022, the count is one permanent injunction, consented and no less permanent for it; two additional LinkedIn lawsuits; three DPA fines covering untold numbers of data subjects; and one nationwide erasure order.

We took the other side.

Verified registers, the counterparty's own website, first-party signals, consent flows and notification automation produce real data quality with an audit trail behind every field. Our market is DACH companies whose buying committees include works councils, DPOs and regulators.

For those customers, compliance IS the product. For Vietnamese contacts, I have yet to find another CRM with a considered answer at all.

Ask your current vendor two questions:

Who indemnifies you when your sales reps' LinkedIn accounts are banned?

Who sends the Article 14 notices for the ten thousand contacts that its plugin just imported?

We have answers to both.

They have a Chrome extension.
