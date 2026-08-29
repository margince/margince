# Margince will not scrape LinkedIn. We could build it in a week. We still won't.

**Our position on compliant data enrichment in the EU, Germany and Vietnam.**

| | |
|---|---|
| **Author** | Lars Jankowfsky |
| **Date** | 27 August 2026 |
| **Scope** | European Union law (with German specifics) and Vietnamese law. Other jurisdictions can be added later; nothing here covers them yet. |
| **Status** | Position paper, adversarially fact-checked. This is research to brief counsel with, not legal advice. Every load-bearing claim carries a named source in the footnotes at the end. |

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

Now read Section 8.2 of LinkedIn's User Agreement.[^1] It prohibits users from developing, supporting or using software, devices, scripts, robots or any other means or processes, explicitly including crawlers, **browser plugins** and add-ons, to scrape or copy the Services, including profiles.

Those three verbs matter: develop, support and use. They bind everyone who accepted the agreement. In practice, that reaches both sides of the plugin economy. The user is bound by definition. The vendor is bound through the accounts it holds to build and test the product. The hiQ court later held hiQ's own corporate account against it.

Another clause prohibits using LinkedIn data obtained through third parties, including search tools, **data aggregators or brokers**, without the content owner's consent. A CRM that stores purchased LinkedIn profile data sits directly inside that clause's reach.

Buying a Sales Navigator seat changes nothing. The subscription agreement calls unauthorized scraping an "Incurable Breach".[^2] There is no CSV export. The only sanctioned route from Sales Navigator into a CRM is CRM Sync on the Advanced Plus tier, and LinkedIn limits it to a short list of large incumbents: Salesforce, Microsoft Dynamics 365, HubSpot and Oracle Sales.[^3]

Then somebody always says: "But hiQ won. Scraping public profiles is legal."

That sentence may be the most expensive misreading in our industry.

The 9th Circuit said something much narrower in 2019 and reaffirmed it in April 2022: scraping publicly available pages was likely not a federal *hacking crime* under the Computer Fraud and Abuse Act.[^4]

That ruling did not bless scraping as a business model.

On remand, in November 2022, the district court held LinkedIn's anti-scraping terms enforceable and granted LinkedIn summary judgment over hiQ's use of fake "turker" accounts.[^5] Liability for the scraping itself remained open because of waiver and estoppel questions.

The court never answered those questions. hiQ folded and settled instead: a negotiated consent judgment of USD 500,000, a permanent injunction with no distinction between public and private pages, and deletion of all scraped data and code.[^6]

No precedent was set. Something more practical was. The company celebrated for "making scraping legal" ended the fight enjoined and gone.

LinkedIn has kept going. It sued Mantheos, which settled,[^7] and ProAPIs, filed in 2025.[^8]

The official route is closed as well. LinkedIn's partner documentation says it is "not currently accepting new partners" for the Sales Navigator API.[^9] The self-service API returns only the name, email address and photo of the single member who gave consent through OpenID Connect.

Now look at who carries the risk in the plugin economy.

LinkedIn sues the vendor. It bans the end user's account. That second consequence is faster and more certain, and it lands on YOUR sales rep. The rep loses a professional identity they may have built for years because a software vendor wanted to avoid applying for an API.

Then European regulators fine whoever stores the data.

That is the second layer.

## 2. "Publicly available" does not mean "free to take"

The GDPR does not ban B2B enrichment. That is important because a lot of bad legal analysis begins by pretending the answer is simply no.

In 2024, the Court of Justice confirmed that a purely commercial interest can qualify as a legitimate interest under Article 6(1)(f) (*KNLTB*, C-621/22).[^10] Recital 47 expressly names direct marketing as a possible legitimate interest. France's CNIL and the UK's ICO both accept B2B prospecting on that basis when the data relates to the person's professional capacity and the person can opt out.

Retention still needs a reason. CNIL works with a benchmark of roughly three years after the last contact. The ICO sets no fixed period and expects the controller to justify whatever period it chooses.[^11]

The part that surprised me was Article 14.

Enrichment can be lawful. Secret enrichment usually is not. When data was not collected from the person, the controller normally has to tell them individually within one month or at the first contact, whichever comes first. Publishing a privacy policy on your own website does not solve the problem. The person has never heard of you and has no reason to visit it.

Article 14(5)(b) does contain a disproportionate-effort exception. It comes with conditions and compensating safeguards, and the leading case shows how little the sentence "notification is expensive" achieves by itself.

Three cases tell the story. The source changed each time. The missing duty did not.

### Bisnode: public registers did not make the people disappear

Between 2019 and 2023, Bisnode fought over a database containing 7.5 million sole traders and company directors, all collected from *public government registers*. It emailed the 682,000 people for whom it held an email address and placed a notice on its website for everyone else. Bisnode argued that sending postal notices would require disproportionate effort.

The Polish DPA fined it roughly EUR 220,000 and ordered individual notification.[^12]

The Supreme Administrative Court dismissed the final appeal on 19 September 2023. Transparency was the rule. Exceptions had to be read restrictively. On these facts, cost alone did not carry the exception, and the public-register origin changed nothing.

The duty follows the data.

### Kaspr: LinkedIn visibility choices mattered

On 5 December 2024, CNIL fined Kaspr EUR 240,000.[^13] Kaspr's browser extension fed a database of 160 million contacts from LinkedIn.

CNIL found four problems that matter here:

- Kaspr collected contact data from members who had *restricted their visibility*. That choice defeated the legitimate-interest balancing test.
- It retained the data for five years.
- It delivered no Article 14 notice at all until 2022.
- In access-request responses, it named only "publicly available sources", although CNIL held that Kaspr had to provide all source information it actually possessed.

CNIL later closed the injunction.[^14] Read that closure carefully before describing it as regulatory approval. Kaspr complied by deleting the offending data and stopping the LinkedIn collection. The regulator approved the exit from the model.

### Lusha: a compliance seal did not save the database

In July 2026, Italy's Garante fined Lusha EUR 2 million and ordered deletion of ALL Italian contact data within 60 days.[^15]

Lusha's US domicile did not change the result. Neither did its TrustArc compliance seal. The Garante took the certification into account when calculating the fine, and the answer was still two million euros.

Around those three enrichment cases sit the harder scraping precedents.

Clearview AI scraped public photographs and has accumulated roughly EUR 100 million in fines across five European authorities: EUR 20 million each in Italy, Greece and France, EUR 30.5 million in the Netherlands, plus the UK's GBP 7.5 million penalty, which is still in litigation.[^16] France later added a EUR 5.2 million non-compliance penalty.

The customer carries its own liability. Sweden's IMY fined the police EUR 250,000 merely for *using* Clearview.[^17] In August 2023, twelve DPAs signed a joint statement on data scraping.[^18] Buying or using unlawfully scraped data does not move the liability somewhere else.

Even the platforms get pulled in. The Irish DPC fined Meta EUR 265 million for its data-protection-by-design failures after a scraped dataset of 533 million users surfaced.[^19] That decision binds Meta, not every online platform. But the direction is uncomfortable for the plugin industry: the host platform's design duty runs against scraping.

For anyone holding enriched data, two product consequences follow.

First, Article 15(1)(g) gives the person a right to "any available information" about the source. Kaspr shows that a regulator will reject "public sources" when the company knows more. Margince therefore stores per-field provenance. That goes beyond the literal wording and makes the eventual data-subject access answer easy.

Second, erasure has to stick. The law does not literally mandate a suppression list,[^20] but the next synchronization will otherwise resurrect the deleted contact. Then you are back in the same lawful-basis analysis with worse facts. The ICO recommends a suppression list. We ship one.[^38]

I understood the product line only after reading all three enrichment decisions: scale changes the facts.

A whole-market database combines sources, tells nobody and exists before any salesperson has a reason to research a specific person. Every fine above attacked that pattern. A salesperson researching one prospect still needs an Article 6 balancing assessment, just like any other processing. But that lookup enters the test with the strongest available facts: one narrow purpose, minimal fields, and an Article 14(3)(b) notice delivered with the first outreach at zero marginal cost.

Trigger and scale separate a defensible lookup from the Bisnode pattern.

One more line matters, especially in Germany. *Storing* a business email address is a GDPR question. *Sending marketing to it* is governed separately.

Germany's Section 7 UWG requires prior express consent for email marketing, including B2B email.[^21] Section 7(3) contains one narrow exception for existing customers, the same-product context and an opt-out at every step.

Your CRM may lawfully hold an address that you still may not cold-email from Frankfurt.

## 3. Vietnam: consent or nothing

I live in Vietnam. We build Margince for companies here as well as in Germany. Vietnamese law is therefore not an academic footnote for us.

It is also the strictest law in this paper, and most Western vendors appear not to have noticed that it exists.

The timeline matters. Vietnam operated under Decree 13/2023/ND-CP until the end of 2025.[^22] Since 1 January 2026, the Personal Data Protection Law has applied. Law 91/2025/QH15 was passed on 26 June 2025,[^23] and implementing Decree 356/2025 entered into force on the same day as the law.

The Vietnamese model is harder than the GDPR. Processing rests on consent plus a short, closed list of exceptions. Its "legitimate interest" basis is defensive only. It covers protecting rights against an act of infringement, which means use cases such as fraud prevention and legal claims.

Every law-firm analysis I checked reaches the same conclusion: Tilleke & Gibbins,[^24] Baker McKenzie,[^25] DLA Piper[^26] and Rouse.[^27] That basis cannot carry prospecting or enrichment.

There is no separate basis for publicly available data. Publishing personal data waives nothing. Marketing requires consent, with no soft opt-in, on top of the Decree 91/2020 anti-spam regime and its do-not-call register.

Article 7(6) prohibits buying and selling personal data unless the law itself provides otherwise. The statute could hardly be clearer: "Personal data is not a commodity."

The penalty rules in Article 8 and the sanctions regime reach ten times the revenue gained from illegal data trading. Violations of the cross-border transfer rules can reach 5% of prior-year revenue.[^28]

Buying broker data about Vietnamese contacts is the highest-risk act in this entire area.

Enforcement is young. The Ministry of Public Security's A05 department ran its first compliance campaign in late 2024, and no published fine has yet hit a foreign B2B SaaS company.

I would not build a product on the assumption that this lasts.

Vietnam's data-trading black market is the stated reason the law exists. Scraping and aggregation therefore sit in the political crosshairs. They do not occupy a tolerated grey zone.

The product consequence is blunt. A Vietnamese data subject needs consent that the law recognises, and Law 91 requires that consent to be voluntary, informed, specific and verifiable.

A business card can begin a business relationship. It does not prove consent to enrichment, marketing or onward disclosure. Our verifiable path is the confirm-your-details flow described in A6 below. The contact sees the record and says yes on the record.

## 4. The Betriebsrat will read this paper too

American CRM vendors regularly forget the person who reviews software in Germany: the works council.

Germany's Federal Labour Court gave us the doctrine in one perfect image in 2024. A retail headset system required works-council co-determination even though it stored no data at all (1 ABR 16/23).[^29]

Under Section 87(1)(6) BetrVG, any technical system *capable* of monitoring employee performance or behaviour is co-determined.[^30] The employer's intent is irrelevant. There is no de-minimis threshold.

The court has applied the rule to an Excel attendance list (1 ABN 36/18),[^31] SAP (1 ABR 45/11) and Office 365 (1 ABR 20/21).[^32]

A CRM containing an audit trail and pipeline numbers per sales rep is co-determined. Period.

That part is routine. A German enterprise CRM rollout normally includes a Betriebsvereinbarung. A well-drafted agreement can itself provide the GDPR basis for employee-side processing when it meets Article 88(2) GDPR and Section 26(4) BDSG.[^33]

The feature list decides whether that negotiation takes six weeks or eighteen months.

Server-side enrichment of *external* contacts barely registers. The only employee datum involved is the audit-trail fact that user X clicked. Scraping plugins, however, combine the three things works councils actually fight:

- an extension inside the employee's browser that observes what the employee views;
- work forced through the employee's personal LinkedIn account;
- mining that turns team correspondence into per-employee conduct profiles.

The Kaspr decision gives any works council a ready-made argument that the tool itself is legally toxic.

Since 2021, an AI layer has sat on top. Section 90 BetrVG requires the employer to inform the works council during the *planning* stage of AI use. The EU AI Act classifies systems that monitor and evaluate worker performance as high-risk under Annex III 4(b).

The 2026 AI omnibus moved the compliance deadline for Annex III systems from 2 August 2026 to 2 December 2027 (Regulation (EU) 2026/1744).[^34] The category still exists. Only the deadline moved.

Our rule stands regardless of the date: the AI evaluates records, deals and companies. It never evaluates the work quality of a named sales rep.

Where does Margince itself sit?

Squarely inside co-determination. I will not pretend otherwise.

Margince captures connected mailboxes, including message bodies, because correspondence CRM is the product.[^39] The relationship graph uses that correspondence to calculate each colleague's warmth toward each *contact*.[^40] "Who here can open this door?" is a legitimate account question for a sales team.

Both features are designed to be governed:

- no browser extension;
- everything runs server-side;
- no rep leaderboards and no performance scores;
- warmth bands are deliberately non-comparable across people and cannot be merged into a ranking;
- exclusion lists keep mailboxes and domains that must never be captured outside the system;
- access to audit data is restricted.

The Betriebsvereinbarung then fixes the purpose in writing: account coverage yes, performance evaluation never, backed by a Verwertungsverbot.

That negotiation is winnable because the product gives the works council something concrete to hold. A scraping plugin gives it something to veto.

## 5. What we build, what we are building, and what we refuse

The product policy now becomes very simple.

Each item below is rated on four axes: GDPR posture, Vietnam compatibility, platform-contract exposure and works-council temperature.

### Tier A: Building or already built

**A1. Company enrichment from official sources.** Company data is the easy case. Data about the legal entity itself sits outside the GDPR under Recital 14: company name, legal form, VAT number and registered address.

We will build these adapters on demand once the need arises. No worries, we will be quick.

| Source | Data | Access | Limit |
|---|---|---|---|
| VIES[^35] | VAT validity, registered name and address in many member states | Free API; we store the consultation number as proof | Availability varies by state |
| GLEIF LEI[^36] | Legal entity plus ownership tree | Free API and bulk files, CC0 | Weak outside the finance sector |
| Swiss Zefix | Commercial register, daily change feed | Free REST API | None worth noting |
| UK Companies House | Register, officers, filings | Free API and streaming | Rate limits only |
| German Handelsregister | Register data, free since the 2022 DiRUG reform | Web, per-lookup | Portal caps queries and bans bulk |
| Licensed aggregators (e.g. North Data) | Structured DACH register and financial data | Paid API | The clean bulk route |


**A2. The counterparty's own website, read deeper.** Built.

Margince already reads company websites.[^41] The deeper read handles people named on those sites in exactly the shape this paper argues for. A stranger found on a team page is staged as a lead and remains staged until a human accepts. The system never auto-creates a new person.

When the published person unmistakably matches someone already recorded at that company, through an exact email match or a single high-confidence name match, Margince fills empty fields such as role, title and profile link. Every value is evidence-backed with the source stored on the row. The system never overwrites information entered by a human.

Margince also parses the Impressum, whose publication German law *mandates* under Section 5 DDG, and consumes newsroom RSS feeds for company events. We store the signal and a link, never a cached copy of the full text.

Crawling stays polite and respects robots.txt. The existing rule also remains: a human click can write directly; an automatic read stages the suggestion. That rule is about PEOPLE, who are staged because a wrong person on a record is a privacy problem. Company-level facts write directly in both lanes — they are attributed, evidence-backed and reversible, and a company's mail provider is not personal data.

**A3. Technical enrichment.** Reading a company's website also tells you what that company runs.[^46]

Every company with a website and email leaves public technical traces, because the internet does not work otherwise. Which system receives their mail: Google Workspace, Microsoft 365, or their own server. What their website is built with: shop system, CMS, marketing tools. Which services they operate, visible through their subdomains: a webshop, a customer portal, a careers page. And where they host.

Margince reads those traces and writes them onto the company record. The reading rides the site read rather than sitting behind a button of its own, and a scheduled pass refreshes it, because a company's mail provider changes at the company and no write on our side announces it. Your rep sees the company's size class, how modern their IT is and what they operate before the first call. Filtering a segment on these fields — every account with a webshop, every account on Microsoft 365 — is not built yet.

Two honest limits. You only see what faces the public internet; the ERP behind the firewall stays invisible unless a subdomain betrays it. And the picture goes stale, so it refreshes like every other enrichment.

Legally this is the cleanest item on the page. All of it describes the company, never a person, so neither the GDPR nor Vietnam's consent rules are engaged, and no platform terms are involved: DNS and certificate logs exist to be queried. It works the same way in Hamburg and Da Nang. One guardrail: a personal name occasionally appears inside a certificate or domain record, so Margince stores signals at company level only.

**A4. First-party person capture.** Built.[^44]

When a contact sends you their details, Margince uses them. An email arrives with a signature: we read name, title and phone number out of it and enrich the person's record automatically. Someone sends a vCard: we import it into the record the same way. Only what the contact actually wrote, nothing guessed, nothing inferred.

The legal basis could not be simpler: the contact sent this data to you themselves.

Two controls sit on top. Exclusion lists keep chosen addresses and whole domains out of capture entirely, and every mailbox has its own switch for the signature pass, so an organisation can turn the granularity all the way down.

Data the contact hands you is the only person-data channel that scales cleanly.[^39]

**A5. LinkedIn as a link, plus the member's own export.** We store the LinkedIn profile URL as a clickable link. Margince never fetches the page server-side.

The user reads the profile in their own browser and enters what they learned into a quick-capture form built for exactly that moment. LinkedIn also allows every member to download their own data, and Margince imports the member's `Connections.csv` in a deliberately limited form.[^42]

The imported connections become private graph substrate, "ghosts", used only for reach and warmth. They are visible only to the importer. They never become contact records and never create people.

I do not claim that LinkedIn's terms permit anything beyond this private graph use. Margince therefore stops there. It does not create contacts and provides no parser for pasted profile text. Once software copies the page on the user's behalf, the scraping clause is back in play. We draw the line on the safe side.

**A6. The "confirm your details" flow.** Bundled with the marketing-consent ask.[^45]

Margince sends the contact a link through which they can view and correct their own card. The same flow delivers the Article 14 transparency notice and gives the contact an Article 16 accuracy mechanism. Because the consent is verifiable, it also meets the standard in Vietnam's Law 91. And the same email asks the contact to grant or decline marketing consent, with the answer landing on the consent proof log and the send itself logged in the record's history.

This flow actively reduces the risk of everything else on this page.

### Tier B: Could be built... theoretically....

Three ideas. All three are legal in principle. None of them is planned yet. We only touch this section if a customer need forces one item, and then a lawyer reviews that one item before we write code. These are a bit more tricky.

**B1. Reading about a person on other websites.** Today Margince reads only the counterparty's own company website. B1 would go further: also read conference pages, speaker bios and contact lines on other companies' sites where the same person shows up.

The machinery for it exists already. Found people land as suggestions, never directly in the records, a human accepts or rejects them, and every field says where it came from.[^43] One company at a time, sources where the person published the details for business contact.

But the moment we collect person data from pages beyond the person's own employer site, GDPR asks for more. Five things we would have to build first:

- Tell the person. When someone we found this way gets stored, they must get a short notice, with the first email or within a month at the latest.
- An expiry date on the data. Roughly three years, then re-check or delete.
- A memory of deletions. A person who asked to be deleted must not come back through the next read.
- Respect "visible to members only" and similar settings on the source. Kaspr got fined exactly for ignoring those.
- Leave Vietnamese people out. For them only the confirm-your-details flow in A6 is clean.

And for every new source someone would have to write down, before switching it on, why our interest in the data outweighs the person's interest in being left alone. That is the "balancing test" GDPR demands. "Single lookups are fine" as a blanket sentence is not one.

**B2. Margince sends the legal notice for our customer.** GDPR says: if you store someone whose data did not come from that person directly, you must tell them. Almost no company does it. B2 would make Margince do it for the customer: write the message, send it, log it, and if the person objects, put them on the do-not-contact list automatically.

I have not found this feature at any vendor. I think I know why. Their model cannot survive the person finding out. Ours is built around exactly that.

The only reason this sits in B: the wording of that message must be checked by a lawyer for each market before it ever goes out.

**B3. Buying contact data from providers like Cognism[^37] or Dealfront.** These vendors sell person data and say they are GDPR-compliant because they informed the people in their database. That claim is theirs. No regulator has confirmed it, and our customer stays legally responsible either way.

If we ever connected such a provider, their data would not mix into the contact records. It lands in a separate claim area next to the record, marked as bought, with the purchase logged, deletable in one action. That part exists.

Two parts do not exist yet:

- when the provider is disconnected, their data deletes itself;
- when the provider tells us a person asked for deletion, that deletion reaches our customer's records.

Both go on the table together with the provider contract, if that day ever comes. And never for Vietnamese people: Vietnamese law flatly forbids trading personal data.

### Tier C: Nah, we won't implement these...

**A database of a whole market, built in advance.** This is what Bisnode and Lusha did: collect everyone in a market before any user ever asked for a specific person. The problem is simple. You must tell every one of those people that you stored them, and at millions of records nobody does. The bigger the database, the weaker the justification. And Italy has shown that a regulator will order an entire national dataset deleted.

The opposite of that: one user reads one company's own website because they are working on that company. That is A2, and it is fine.

**Pulling data out of LinkedIn or Xing automatically, in any form.** It breaks the contract every LinkedIn user signed, it is exactly what Kaspr was fined for, and there is no official API we could apply for. Running the whole thing inside the user's browser instead of on our servers changes nothing. That is precisely the model LinkedIn takes to court.

For Xing I quote no contract clause. The data-protection and works-council problems alone are enough to refuse it.

**Any browser extension.** It fails twice. LinkedIn's contract bans plugins by name. And software that watches which pages an employee opens in their browser is exactly the kind of employee surveillance a works council exists to stop.

Maybe some very narrow extension would survive a lawyer's reading. We have no reason to build our product on top of that argument.

**Buying contact lists of unclear origin.** The moment the list is in your CRM, the legal responsibility for it is yours, and you cannot prove where the data came from. If Vietnamese people are on the list, Vietnam's ban on trading personal data sits on top, with fines of up to ten times the revenue made.

**Grading salespeople.** We refuse this, and the wording matters, because Margince does read correspondence, as Section 4 explains.

Margince builds no profiles of its users. It guesses no character traits, computes no performance scores and shows no ranking of one rep against another. The relationship graph answers exactly one question: "Who on our side knows this contact well enough to open the door?" It answers per contact, and the answer is deliberately built so it cannot be added up into a leaderboard.

Whether reading mailboxes is allowed under German employee-data law depends on what you do it for. "Which of us knows this customer" is a purpose we can defend in writing. "Which of our reps is the worst" is not. So it stays refused.

## 6. Why this wins deals

This paper says what Margince can sell with a straight face.

The scraping vendors made a bet. They bet that LinkedIn would not bother suing, that the data-protection authorities would stay busy elsewhere, and that no works council would ever read the regulators' press releases.

That bet gets worse every year.

Since 2022: LinkedIn shut down the best-known scraper for good and filed two more lawsuits. Three European authorities fined enrichment vendors. Italy ordered one of them to delete every Italian person in its database.

We bet the other way.

Our data comes from places nobody can attack: official registers, the customer's own website, what contacts send us themselves, and what people confirm with their own click. Every field says where it came from. Our buyers are companies in Germany, Austria and Switzerland where the works council, the data-protection officer and sometimes a regulator sit at the table when software gets bought.

For those buyers, being clean is not a checkbox on our product. It is the product. And for Vietnamese contacts I have not found a single other CRM that has even thought about the question.

If you use one of the other CRMs today, ask your vendor two questions:

Who pays when LinkedIn bans your sales reps' accounts?

Who tells the ten thousand people your plugin just imported that they are now in your database, as the law requires?

We have answers to both.

They have a Chrome extension.

## 7. Sources and verification status

The primary decisions and rulings cited in the footnotes were verified against the issuing body where available: CJEU C-621/22 (*KNLTB*); UODO v. Bisnode including the NSA ruling of 19 September 2023; CNIL v. Kaspr plus the closure notice; Garante v. Lusha; the Clearview AI fine series in Italy, Greece, France, the Netherlands and the UK, with the UK litigation continuing; IMY v. the Swedish police; Irish DPC v. Meta; hiQ v. LinkedIn across the 9th Circuit ruling, the N.D. California ruling of November 2022 and the consent judgment of December 2022; LinkedIn v. Mantheos and LinkedIn v. ProAPIs; and BAG 1 ABN 36/18, 1 ABR 45/11, 1 ABR 20/21 and 1 ABR 16/23.

The statutory set: GDPR Articles 5, 6(1)(f), 14, 15(1)(g), 17, 21 and 88 plus Recitals 14 and 47; Sections 87(1)(6), 90 and 80(3) BetrVG; Section 26 BDSG; Section 5 DDG; Section 7 UWG; EU AI Act Annex III 4(b) as amended by Regulation (EU) 2026/1744; Vietnam Decree 13/2023/ND-CP, Personal Data Protection Law 91/2025/QH15 Articles 7(6), 8 and 9, Decree 356/2025 and Decree 91/2020. The Vietnamese-law analysis rests on the official texts plus concurring commentary from Tilleke & Gibbins, DLA Piper, Baker McKenzie, Rouse and DFDL.

On 27 August 2026, I reconciled all 26 findings from an adversarial fact-check into this version.

Counsel still needs to close four Tier B questions: the final adoption status of the EDPB's Guidelines 1/2024 on legitimate interest, still in draft as of this writing; Vietnam's administrative sanctions decree, which remains in flux through 2026; the exact section numbering in LinkedIn's subscription agreement; and the Section 7 UWG detail for each outreach channel. None of those open questions changes a tier placement.

[^1]: LinkedIn User Agreement, Section 8.2: https://www.linkedin.com/legal/user-agreement
[^2]: LinkedIn Subscription Agreement: https://www.linkedin.com/legal/l/lsa
[^3]: LinkedIn CRM integration matrix: https://www.linkedin.com/help/linkedin/answer/a106005/?lang=en
[^4]: hiQ Labs v. LinkedIn, 9th Cir., opinion of 18 April 2022: https://cdn.ca9.uscourts.gov/datastore/opinions/2022/04/18/17-16783.pdf
[^5]: hiQ Labs v. LinkedIn, N.D. Cal., November 2022: https://caselaw.findlaw.com/court/us-dis-crt-n-d-cal/2182242.html
[^6]: ZwillGen, "hiQ v. LinkedIn Wrapped Up": https://www.zwillgen.com/alternative-data/hiq-v-linkedin-wrapped-up-web-scraping-lessons-learned/
[^7]: LinkedIn on the Mantheos suit: https://news.linkedin.com/2022/february/taking-legal-action-to-protect-members-against-scraping
[^8]: Coverage of LinkedIn v. ProAPIs: https://securityaffairs.com/183001/security/linkedin-sues-proapis-for-15k-month-linkedin-data-scraping-scheme.html
[^9]: LinkedIn Sales Navigator partner documentation: https://learn.microsoft.com/en-us/linkedin/sales/
[^10]: DLA Piper on CJEU C-621/22: https://privacymatters.dlapiper.com/2024/10/eu-cjeu-confirms-that-legitimate-interests-can-cover-purely-commercial-interests/
[^11]: ICO direct-marketing retention guidance: https://ico.org.uk/for-organisations/direct-marketing-and-privacy-and-electronic-communications/direct-marketing-guidance/plan-direct-marketing/
[^12]: UODO on the Bisnode case and the NSA ruling: https://uodo.gov.pl/en/553/1572
[^13]: CNIL, Kaspr fine of 5 December 2024: https://www.cnil.fr/en/data-scraping-kaspr-fined-eu240000
[^14]: CNIL, closure of the Kaspr injunction: https://www.cnil.fr/en/closure-order-issued-against-kaspr
[^15]: Garante, Lusha decision: https://www.garanteprivacy.it/home/docweb/-/docweb-display/docweb/10275230
[^16]: ICO on the Clearview Upper Tribunal judgment: https://ico.org.uk/about-the-ico/media-centre/news-and-blogs/2025/10/uk-upper-tribunal-hands-down-judgment-on-clearview-ai-inc/
[^17]: IMY, fine against the Swedish police: https://www.imy.se/en/about-us/arkiv/nyhetsarkiv/police-unlawfully-used-facial-recognition-app/
[^18]: Joint DPA statement on data scraping, August 2023: https://ico.org.uk/media2/migrated/4026232/joint-statement-data-scraping-202308.pdf
[^19]: Irish DPC, Meta data-scraping decision: https://www.dataprotection.ie/en/news-media/press-releases/data-protection-commission-announces-decision-in-facebook-data-scraping-inquiry
[^20]: ICO guidance on suppression: https://ico.org.uk/for-organisations/direct-marketing-and-privacy-and-electronic-communications/direct-marketing-guidance/respect-peoples-preferences/
[^21]: UWG Section 7, official English translation: https://www.gesetze-im-internet.de/englisch_uwg/englisch_uwg.pdf
[^22]: Decree 13/2023/ND-CP, English text: https://thuvienphapluat.vn/van-ban/EN/Cong-nghe-thong-tin/Decree-No-13-2023-ND-CP-dated-April-17-2023-on-protection-of-personal-data/564343/tieng-anh.aspx
[^23]: Law 91/2025/QH15, English translation: https://english.luatvietnam.vn/dan-su/law-on-personal-data-protection-law-no-91-2025-qh15-405135-d1.html
[^24]: Tilleke & Gibbins on the PDPL: https://www.tilleke.com/insights/vietnams-new-personal-data-protection-law-a-closer-look/
[^25]: Baker McKenzie on the PDPL: https://connectontech.bakermckenzie.com/vietnam-decoding-vietnams-pdp-law-gdpr-inspired-rules-with-local-twists/
[^26]: DLA Piper data protection handbook, Vietnam: https://www.dlapiperdataprotection.com/?t=law&c=VN
[^27]: Rouse on the PDPL: https://rouse.com/insights/news/2025/vietnam-s-new-personal-data-protection-law-what-businesses-need-to-know
[^28]: PDPL official text via the Ministry of Public Security: https://bocongan.gov.vn/media/bca-media/photo-library/20250728100402_a4c3955c-ea1d-48a0-9998-c262b14864af-Lu%E1%BA%ADt-B%E1%BA%A3o-v%E1%BB%87-d%E1%BB%AF-li%E1%BB%87u-c%C3%A1-nh%C3%A2n.pdf
[^29]: BAG 1 ABR 16/23, decision text: https://www.bundesarbeitsgericht.de/wp-content/uploads/2024/11/1-ABR-16-23.pdf
[^30]: BetrVG Section 87: https://www.gesetze-im-internet.de/betrvg/__87.html
[^31]: BAG 1 ABN 36/18: https://www.bundesarbeitsgericht.de/entscheidung/1-abn-36-18/
[^32]: Hensche on BAG 1 ABR 20/21: https://www.hensche.de/bag-beschluss-vom-08.03.2022-1-abr-20-21-zustaendigkeit-des-gesamtbetriebsrats-bei-unternehmenseinheitlicher-nutzung-von-microsoft-office-365.html
[^33]: BDSG, official English translation: https://www.gesetze-im-internet.de/englisch_bdsg/englisch_bdsg.html
[^34]: Steptoe on Regulation (EU) 2026/1744: https://www.steptoe.com/en/news-publications/steptechtoe-blog/eu-ai-act-amendments-enter-into-force.html
[^35]: VIES VAT validation: https://ec.europa.eu/taxation_customs/vies/
[^36]: GLEIF open data: https://www.gleif.org/en/about/open-data
[^37]: Cognism compliance statements: https://www.cognism.com/compliance
[^38]: The suppression-list and erasure design: docs/explanation/privacy-and-consent.md in this repository.
[^39]: The capture architecture: docs/explanation/capture-connectors.md and docs/explanation/ingress-gate-and-auto-capture.md in this repository.
[^40]: docs/explanation/relationship-graph.md in this repository.
[^41]: docs/explanation/company-context.md in this repository. The A2 enhancements are ticket 2855 in the repository issue tracker.
[^42]: docs/how-to/import-your-linkedin-network.md in this repository. The A5 quick-capture form is ticket 2857.
[^43]: The write shape: docs/explanation/write-backbone.md in this repository.
[^44]: Ticket 2856 in the repository issue tracker.
[^45]: Ticket 2858 in the repository issue tracker.
[^46]: Ticket 2888 in the repository issue tracker.
