# Why Margince Doesn't Scrape

**The Margince view on data enrichment: what is legal, what is not, and what we
build instead.**

| | |
|---|---|
| **Author** | Lars Jankowfsky |
| **Date** | 27 August 2026 |
| **Scope** | European Union law (with German specifics) and Vietnamese law. Other jurisdictions can be added later; nothing here covers them yet. |
| **Status** | Position paper. This is research to brief counsel with, not legal advice. Every load-bearing claim carries a named source. |

---

## The position, in one paragraph

Every CRM you have ever demoed has a plugin that reads LinkedIn and fills your
contact records. Margince does not have one, and it never will. Not because we
could not build it in a week. Because the whole model is a contract breach
stacked on a privacy violation, and the fines are no longer theoretical: three
European regulators have executed exactly this pattern since 2019, and LinkedIn
itself has won in court against the most famous scraper. What surprised me in
the research: the law does NOT force a CRM to stay dumb. There is a real set of
enrichment features that are clean, and one of them (notification automation)
nobody in the market has built. This paper walks through the law, names the
cases, and ends with what we build and what we refuse.

## 1. The industry standard is a contract breach

Start with the thing everybody does. Apollo, Kaspr, Lusha, Waalaxy: a Chrome
extension runs inside the salesperson's logged-in LinkedIn session, reads the
profile pages the person visits, and posts the parsed fields to the vendor's
backend. That is the entire "magic".

Now read LinkedIn's [User Agreement](https://www.linkedin.com/legal/user-agreement),
Section 8.2. It prohibits to "develop, support or use software, devices,
scripts, robots or any other means or processes (such as crawlers, **browser
plugins** and add-ons or any other technology) to scrape or copy the Services,
including profiles". Three verbs. The clause binds the vendor AND the user. A
separate clause prohibits using LinkedIn data obtained "through third parties
(such as search tools or **data aggregators or brokers**)". That one reaches a
CRM that merely stores bought data.

A Sales Navigator seat changes nothing. The
[subscription agreement](https://www.linkedin.com/legal/l/lsa) calls
unauthorized scraping an "Incurable Breach", there is no CSV export, and the
only sanctioned exit into a CRM is CRM Sync on the top tier, into Salesforce or
Microsoft Dynamics only.

"But hiQ won, scraping public profiles is legal." I hear this a lot, and it is
the most expensive misreading in the industry. What the 9th Circuit actually
said (2019, reaffirmed
[April 2022](https://cdn.ca9.uscourts.gov/datastore/opinions/2022/04/18/17-16783.pdf))
is that scraping public pages is likely not a federal *hacking crime* under the
CFAA. That is all. On remand, in
[November 2022](https://caselaw.findlaw.com/court/us-dis-crt-n-d-cal/2182242.html),
LinkedIn won summary judgment: hiQ breached the User Agreement. The case ended
in a
[consent judgment](https://www.privacyworld.blog/2022/12/linkedins-data-scraping-battle-with-hiq-labs-ends-with-proposed-judgment/):
USD 500,000, a permanent injunction with no public/private distinction, and
deletion of all scraped data and code. The company that "made scraping legal"
went broke doing it. LinkedIn has since sued
[Mantheos](https://news.linkedin.com/2022/february/taking-legal-action-to-protect-members-against-scraping)
(settled) and
[ProAPIs](https://securityaffairs.com/183001/security/linkedin-sues-proapis-for-15k-month-linkedin-data-scraping-scheme.html)
(filed 2025).

And the official door? Closed. LinkedIn's
[partner documentation](https://learn.microsoft.com/en-us/linkedin/sales/)
states it is "not currently accepting new partners" for the Sales Navigator
API. The self-serve API returns the name, email and photo of the one member who
consented via OpenID Connect. Nothing else.

Look at who carries the risk in the plugin economy. LinkedIn sues the vendor.
LinkedIn bans the end user's account, which is the fastest and most certain
consequence: your sales rep loses their professional identity, not the plugin
company. And European regulators fine whoever stores the data. Which brings us
to the second layer.

## 2. "Publicly available" does not mean "free to take"

The GDPR does not ban B2B enrichment. The Court of Justice confirmed in 2024
that a purely commercial interest can be a legitimate interest under
Art. 6(1)(f)
([*KNLTB*, C-621/22](https://privacymatters.dlapiper.com/2024/10/eu-cjeu-confirms-that-legitimate-interests-can-cover-purely-commercial-interests/)),
and Recital 47 names direct marketing as a candidate. The French CNIL and the
UK ICO both accept B2B prospecting on legitimate interest with opt-out,
professional-capacity data only, and roughly a three-year retention benchmark.

What kills enrichment products is Article 14. When you did not collect the data
from the person, you must tell them individually: within one month, or at the
first contact, whichever comes first. A privacy policy on your own website does
not count, because the person has never heard of you. And "notifying millions
is too expensive" does not count as disproportionate effort. Three cases prove
each part of that sentence:

**Bisnode (Poland, 2019 to 2023).** Bisnode built a database of 7.5 million
sole traders and directors from *public government registers*. It emailed the
682,000 people whose address it had and posted a website notice for the rest.
The Polish DPA [fined it](https://uodo.gov.pl/en/553/1572) roughly EUR 220,000
and ordered individual notification; the Supreme Administrative Court dismissed
the final appeal on 19 September 2023. Public-register origin did not help.
The duty follows the data.

**Kaspr (France, 5 December 2024, EUR 240,000).**
[CNIL fined Kaspr](https://www.cnil.fr/en/data-scraping-kaspr-fined-eu240000),
whose extension fed a 160-million-contact database from LinkedIn. The findings:
collecting contacts of members who had *restricted their visibility* (that
choice defeats the balancing test), five-year retention, no Art. 14 notice at
all until 2022, and answering access requests with "publicly available sources"
instead of the actual source. CNIL later
[closed the injunction](https://www.cnil.fr/en/closure-order-issued-against-kaspr)
after remediation. Read that closure carefully, because it is the closest thing
to a regulator-reviewed acceptable model: public-by-choice data only, real
individual notification, bounded retention.

**Lusha (Italy, July 2026, EUR 2 million).** The Garante
[fined Lusha](https://www.garanteprivacy.it/home/docweb/-/docweb-display/docweb/10275230)
and ordered erasure of ALL Italian contact data within 60 days. Lusha is a US
company with no EU establishment. Did not matter. It carried a TrustArc
compliance seal. Did not matter either.

Around these sit the harder precedents. Clearview AI has collected over
EUR 110 million in fines across five European authorities for scraping public
photos, and Sweden's IMY
[fined the police](https://www.imy.se/en/about-us/arkiv/nyhetsarkiv/police-unlawfully-used-facial-recognition-app/)
EUR 250,000 merely for *using* Clearview. Customers of unlawful scraping carry
their own liability. Twelve DPAs signed a
[joint statement on data scraping](https://ico.org.uk/media2/migrated/4026232/joint-statement-data-scraping-202308.pdf)
in August 2023. And the Irish DPC fined Meta
[EUR 265 million](https://www.dataprotection.ie/en/news-media/press-releases/data-protection-commission-announces-decision-in-facebook-data-scraping-inquiry)
for failing to engineer *against* scraping. Platforms are now legally required
to fight the exact tools the CRM industry sells.

Two consequences follow for anyone holding enriched data. First, an access
request must name the actual source, per field ("public sources" was one of
Kaspr's violations). Second, erasure must propagate: a re-sync that resurrects
a deleted contact is a fresh violation, so a suppression list is mandatory
infrastructure, not a nice-to-have. Margince ships one; see
[privacy-and-consent.md](privacy-and-consent.md).

Here is the distinction that decides product design, and it took me the whole
research to see it sharply. A salesperson looking up ONE prospect they are
about to contact is proportionate, and Art. 14(3)(b) lets the notice ride
along with the first outreach at zero marginal cost. A silent background
database of a whole market is the Bisnode pattern. Same data. Opposite legal
outcomes. The trigger and the scale are the dividing line, and regulators have
said so consistently: scale, source-combination and profiling are the
aggravating factors in every decision above.

One more split to respect: *storing* a business email address is a GDPR
question. *Emailing* it is a separate marketing-law question, and Germany's
§7 UWG requires consent for email marketing even B2B. Your CRM can lawfully
hold what you may not lawfully cold-email from Frankfurt.

## 3. Vietnam: consent or nothing

I live and build in Vietnam, so this jurisdiction is not academic for us. It is
also the strictest one in this paper, and most Western vendors have not
noticed.

Vietnam ran on
[Decree 13/2023/ND-CP](https://thuvienphapluat.vn/van-ban/EN/Cong-nghe-thong-tin/Decree-No-13-2023-ND-CP-dated-April-17-2023-on-protection-of-personal-data/564343/tieng-anh.aspx)
until the end of 2025. Since 1 January 2026 the Personal Data Protection Law
applies
([Law 91/2025/QH15](https://english.luatvietnam.vn/dan-su/law-on-personal-data-protection-law-no-91-2025-qh15-405135-d1.html),
passed 26 June 2025, implementing Decree 356/2025). The model is
consent-centric with a short closed list of exceptions. The law's "legitimate
interest" basis is defensive-only: protecting rights against an act of
infringement, meaning fraud prevention and legal claims. Every law firm
commentary I checked
([Tilleke & Gibbins](https://www.tilleke.com/insights/vietnams-new-personal-data-protection-law-a-closer-look/),
[Baker McKenzie](https://connectontech.bakermckenzie.com/vietnam-decoding-vietnams-pdp-law-gdpr-inspired-rules-with-local-twists/),
[DLA Piper](https://www.dlapiperdataprotection.com/?t=law&c=VN),
[Rouse](https://rouse.com/insights/news/2025/vietnam-s-new-personal-data-protection-law-what-businesses-need-to-know))
concurs: it cannot carry prospecting or enrichment. There is no
publicly-available-data basis at all. Making data public waives nothing.
Marketing use requires consent, with no soft opt-in, on top of the
Decree 91/2020 anti-spam regime with its do-not-call register.

And Article 8 bans buying and selling personal data outright. "Personal data
is not a commodity." Fines run up to ten times the revenue gained, minimum
around USD 115,000, and cross-border violations reach 5% of prior-year
revenue. Buying broker data about Vietnamese contacts is the single
highest-risk act in this entire space.

Enforcement is young. The Ministry of Public Security's A05 department ran its
first compliance campaign in late 2024, and no published fine has hit a
foreign B2B SaaS yet. I would not bet a product on that lasting. Vietnam's
data-trading black market is the stated reason this law exists; scraping and
aggregation sit in the political crosshairs, not in a tolerated gray zone.

The product consequence is blunt: a Vietnamese data subject needs a
consent-backed path. The contact confirms their own details, connects their
own profile, or hands over their card. Nothing else.

## 4. The Betriebsrat will read this paper too

German buyers bring a fourth reviewer to the table that US vendors keep
forgetting: the works council. Under
[§87(1)(6) BetrVG](https://www.gesetze-im-internet.de/betrvg/__87.html), any
technical system *capable* of monitoring employee performance or behaviour is
co-determined. Intent is irrelevant and there is no de-minimis threshold. The
Federal Labour Court has held this for an Excel attendance list
([1 ABN 36/18](https://www.bundesarbeitsgericht.de/entscheidung/1-abn-36-18/)),
for SAP (1 ABR 45/11), for Office 365
([1 ABR 20/21](https://www.hensche.de/bag-beschluss-vom-08.03.2022-1-abr-20-21-zustaendigkeit-des-gesamtbetriebsrats-bei-unternehmenseinheitlicher-nutzung-von-microsoft-office-365.html)),
and in 2024 for a retail headset system that stored nothing at all
([1 ABR 16/23](https://www.bundesarbeitsgericht.de/wp-content/uploads/2024/11/1-ABR-16-23.pdf)).
A CRM with an audit trail and per-rep pipeline numbers is co-determined,
period. That part is routine; every German Salesforce rollout has a
Betriebsvereinbarung, and the works agreement even doubles as the GDPR legal
basis for the employee-side processing (Art. 88 GDPR, §26(4) BDSG).

What decides whether that negotiation takes six weeks or eighteen months is
the feature list. Server-side enrichment of *external* contacts barely
registers; the only employee datum it touches is the audit-trail fact that
user X clicked. The scraping plugins fail on all three points works councils
actually fight: an extension in the employee's browser that watches what they
view, work forced through the employee's personal LinkedIn account, and
correspondence mining that builds per-employee communication profiles. The
Kaspr fine hands any works council a ready-made argument that the tool itself
is legally toxic.

There is an AI layer on top since 2021: §90 BetrVG obliges the employer to
inform the works council at the *planning* stage of AI use, and the EU AI Act
classifies systems that monitor and evaluate worker performance as high-risk
(Annex III 4(b), obligations binding since 2 August 2026). Our rule is simple:
the AI evaluates records, deals and companies. Never a named rep's work
quality.

Margince's architecture already satisfies the properties works councils
negotiate for: no browser extension, everything server-side, no per-employee
analytics by default, signature and header fields instead of mailbox content
mining, access-restricted audit data. In a DACH sales cycle that list shortens
the negotiation by months, and the competitor's plugin is the thing the
Betriebsrat vetoes.

## 5. What we build, what we gate, what we refuse

Everything above compresses into three tiers. Each item is rated on four axes:
GDPR posture, Vietnam compatibility, platform-contract exposure, works-council
temperature.

### Tier A: build freely

**A1. Company enrichment from official sources.** Legal-entity data sits
outside the GDPR entirely (Recital 14). [VIES](https://ec.europa.eu/taxation_customs/vies/)
VAT validation, free, returns name and address in 16 member states; we store
the consultation number as proof. [GLEIF LEI](https://www.gleif.org/en/about/open-data),
CC0-licensed, free API and bulk files, including the ownership tree. Swiss
Zefix with its free REST API and daily change feed. UK Companies House, free
API and streaming. German Handelsregister lookups, free since the 2022 DiRUG
reform, but strictly per-lookup: the portal caps queries and bans bulk
retrieval, so bulk always goes through a licensed aggregator such as North
Data. Named directors inside register data are personal data, but published
under statutory duty, which is the strongest legitimate-interest fact pattern
that exists. Identifier spine: EUID, LEI, VAT ID, and the Vietnamese MST.

**A2. The counterparty's own website, read deeper.** Margince already reads
company sites (see [company-context.md](company-context.md)), and a deep read
already handles the people a site names, in exactly the shape this paper
argues for: a stranger on the team page stages as a lead and stays staged
until a human accepts, and a person the workspace already records at that
company gets only empty fields filled, evidence-backed, with the source on the
row. Nothing person-shaped is auto-written behind the user's back. The clean
extensions from here: parse the Impressum, whose publication German law
*mandates* (§5 DDG), and consume newsroom RSS for company events, storing the
signal plus a link and never the cached full text. Polite crawling, robots.txt
respected, and the existing rule stands: a human click writes directly, an
automatic read stages.

**A3. Technical enrichment.** MX and DNS records, TLS certificate transparency
logs, tech-stack fingerprints from the company's public pages. Company-level,
no personal data, works identically in Hamburg and in Da Nang.

**A4. First-party person capture.** The contact sent you their details. Email
signature parsing (stated fields only, never inferred scores, per-mailbox
opt-in with exclusion lists), calendar attendees of your own meetings, vCard
import. This is the only person-data channel that scales cleanly, and it is
already how Margince capture works: see
[capture-connectors.md](capture-connectors.md) and
[ingress-gate-and-auto-capture.md](ingress-gate-and-auto-capture.md).

**A5. LinkedIn as a link, plus subject-side connect.** We store the profile
URL as a clickable link and never fetch it server-side. The user reads the
profile in their own browser and types what they learned into a structured
form. The one automated path LinkedIn explicitly permits is the member
exporting their own data: that is exactly what
[import-your-linkedin-network.md](../how-to/import-your-linkedin-network.md)
implements with the user's own `Connections.csv`. What we will not build is a
parser for pasted profile text; the moment software copies the page for you,
you are back inside the scraping clause.

**A6. The "confirm your details" flow.** Send the contact a link to view and
correct their own card. One feature, three legal jobs: it is the Art. 14
transparency notice, the Art. 16 accuracy mechanism, and, because it collects
consent, the one enrichment channel that works for Vietnamese contacts. It
actively de-risks everything else on this page.

### Tier B: build with controls, counsel signs off first

**B1. Widening person lookup beyond the counterparty's own site.** The
shipped shape described in A2 (staged leads, fill-only-empty, evidence on
every field, per-field provenance through the write shape; see
[write-backbone.md](write-backbone.md)) is already the proportionate pattern:
one company at a time, sources the person published for business contact, a
human between the suggestion and the record. What stays gated on counsel is
widening it. New sources beyond the company's own site (speaker bios,
conference pages, Impressum contact lines of third parties) enter only under
the same staging discipline. And before we widen anything, the missing
controls land: the Art. 14 notice generated and delivered with the first
outreach or within one month of a staged lead being accepted, roughly
three-year retention with refresh-or-delete, the erasure suppression list
consulted before a re-read can re-stage a deleted person, visibility
restrictions honored wherever a platform offers them (that is precisely where
Kaspr lost), and Vietnamese subjects excluded and routed to A6.

**B2. Art. 14 notice automation as a customer feature.** The CRM generates,
sends and logs the notification our customer owes for any contact not
collected from the subject, with objection handling wired into the suppression
list. No competitor's plugin does this, because their model cannot survive
transparency. Ours is built on it. Needs per-market legal review of the notice
texts before shipping.

**B3. Licensed notified-database providers.** Providers like
[Cognism](https://www.cognism.com/compliance) and Dealfront run a "notified
database": they claim Art. 6(1)(f) with a documented balancing test and
discharge Art. 14 by emailing the data subjects. That model is
provider-asserted, not regulator-certified, and the customer remains an
independent controller with their own duties. Margince's side of the bargain
is already built the right way around: what a licensed provider asserts about
a person lands as a claim row with the purchasing run attached
(`person_provider_claim`), separate from the record itself, so
purge-on-termination and downstream erasure signals are executable per
provider. Adding a specific provider stays a per-contract counsel question,
and never for Vietnamese subjects; Article 8 forbids the trade itself.

### Tier C: refuse

- **Bulk background enrichment of a market.** The Bisnode and Lusha pattern:
  a standing shadow database of everyone, built before any user asked. No
  Art. 14 story, scale defeats the balancing test, and Italy has shown a
  regulator will order erasure of the entire national dataset. Reading one
  company's own site for a human working that company (A2) is the opposite
  end of the proportionality scale.
- **LinkedIn or Xing ingestion in any automated form.** Contract breach for
  vendor and user, the Kaspr fine replayed, and the sanctioned API path is
  closed. "The user's browser does it" defends nothing. LinkedIn sues over
  exactly this model.
- **Any browser extension.** Disqualified twice over: LinkedIn's contract
  names plugins, and a tool observing what an employee views is textbook
  §87(1)(6) surveillance.
- **Purchased lists and broker data of unclear provenance.** The recipient
  becomes controller of data with no lawful-basis chain. For Vietnamese
  subjects, add the trading ban and its ten-times-revenue fine.
- **Correspondence content mining and relationship scoring per employee.**
  Reading mailbox content beyond signature and header fields fails the §26
  BDSG necessity test, and per-employee "who knows whom, how well" scoring is
  the works-council red line.

## 6. Why this is a position and why it wins deals

I did not write this paper to justify a gap. The scraping vendors have made a
bet: that LinkedIn's enforcement stays slow, that DPAs stay busy elsewhere,
and that no works council reads the CNIL's press releases. Every year that bet
gets worse. Since 2022 the count reads: one permanent injunction, two LinkedIn
lawsuits, three DPA fines executing the same pattern, one nationwide erasure
order.

Margince took the other side of the bet. Verified registers, the counterparty's
own site, first-party signals, consent flows, and notification automation give
real data quality with an audit trail behind every field. In the market we
sell into, DACH companies with works councils and DPOs on the buying
committee, the compliance IS the differentiator. And for Vietnamese contacts
we are, as far as I can tell, the only CRM with a considered answer at all.

The sales pitch writes itself: ask your current vendor who indemnifies you
when your reps' LinkedIn accounts get banned. Then ask who sends the Art. 14
notices for the ten thousand contacts their plugin just imported. We have
answers to both questions. They have a Chrome extension.

## Sources and verification status

Primary decisions and rulings, verified against the issuing body where linked
above: CJEU C-621/22 (*KNLTB*); UODO v. Bisnode with the NSA ruling of
19 Sep 2023; CNIL v. Kaspr, EUR 240,000, 5 Dec 2024, plus the closure notice;
Garante v. Lusha, EUR 2M, Jul 2026; the Clearview AI fine series (IT, GR, FR,
NL, UK); IMY v. the Swedish police; Irish DPC v. Meta, EUR 265M; hiQ v.
LinkedIn (9th Cir. 2022, N.D. Cal. Nov 2022, consent judgment Dec 2022);
LinkedIn v. Mantheos and v. ProAPIs; BAG 1 ABN 36/18, 1 ABR 45/11,
1 ABR 20/21, 1 ABR 16/23. Statutes: GDPR Arts. 5, 6(1)(f), 14, 15(1)(g), 17,
21, 88; Recitals 14 and 47; §87(1)(6), §90, §80(3) BetrVG; §26 BDSG; §5 DDG;
§7 UWG; EU AI Act Annex III 4(b); Vietnam Decree 13/2023/ND-CP, PDPL Law
91/2025/QH15, Decree 356/2025, Decree 91/2020. Vietnamese-law readings rest on
concurring commentary from Tilleke & Gibbins, DLA Piper, Baker McKenzie, Rouse
and DFDL, since English primary text exists only for Decree 13.

Open flags a lawyer should close before Tier B ships: the final adoption
status of the EDPB's Guidelines 1/2024 on legitimate interest (still draft as
of this writing); Vietnam's sanctions decree (reported as Decree 330/2026,
single-source); the exact section numbering of LinkedIn's subscription
agreement; the §7 UWG detail for each outreach channel. None of these flags
changes a tier placement.
