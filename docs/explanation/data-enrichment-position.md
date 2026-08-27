# Margince will not scrape LinkedIn. What we build instead.

**Our position on compliant data enrichment in the EU, Germany and Vietnam.**

| | |
|---|---|
| **Author** | Lars Jankowfsky |
| **Date** | 27 August 2026 |
| **Scope** | European Union law (with German specifics) and Vietnamese law. Other jurisdictions can be added later; nothing here covers them yet. |
| **Status** | Position paper, adversarially fact-checked. This is research to brief counsel with, not legal advice. Every load-bearing claim carries a named source. |

---

## The position

Most CRM you have ever demoed offers LinkedIn enrichment. A plugin reads the
profile, the record fills itself, everybody claps. Margince will never sell
this. There will always be third-party vendors happy to serve people who want
to ignore the legal situation, and I won't stop them. But we sell a compliant
product to regulated companies and larger SMEs. A compliant product with an
illegal feature is not a compliant product anymore. Punkt.

We could build it in a week; that was never the question. The whole plugin
model breaches LinkedIn's contract and collides with European data protection
law, and the fines stopped being theoretical years ago. Since 2019 three
European regulators have fined enrichment databases over the same missing
duty, telling the people in them: Bisnode's register-built database, Kaspr's
LinkedIn-fed one, Lusha's multi-source broker one. LinkedIn itself walked the
most famous scraper into a permanent injunction.

The research did surprise me: the law does not force a CRM to stay dumb.
Several enrichment features are clean, and one of them, notification
automation, I have not found at any vendor in this market. That gap decides
what we build.

## 1. The industry standard is a contract breach

I started with the contract, because it already kills the standard plugin
model. The thing everybody does, Apollo, Kaspr, Lusha, Waalaxy: a Chrome
extension runs inside the salesperson's logged-in LinkedIn session, reads the
profile pages the person visits, and posts the parsed fields to the vendor's
backend. That is the entire "magic".

Now read LinkedIn's [User Agreement](https://www.linkedin.com/legal/user-agreement),
Section 8.2. It prohibits to "develop, support or use software, devices,
scripts, robots or any other means or processes (such as crawlers, **browser
plugins** and add-ons or any other technology) to scrape or copy the Services,
including profiles". Those three verbs, develop, support and use, bind
everyone who agreed to the agreement, and in practice that is both sides of
the plugin economy: the user by definition, and the vendor through the
accounts it holds to build and test (the hiQ court held hiQ's own corporate
account against it). A separate clause prohibits using LinkedIn data obtained
"through third parties (such as search tools or **data aggregators or
brokers**)" without the content owner's consent. A CRM that stores bought
profile data sits inside that clause's reach.

A Sales Navigator seat changes nothing. The
[subscription agreement](https://www.linkedin.com/legal/l/lsa) calls
unauthorized scraping an "Incurable Breach". There is no CSV export. The only
sanctioned exit into a CRM is CRM Sync on the Advanced Plus tier, into a short
list of big incumbents: Salesforce, Microsoft Dynamics 365, HubSpot, Oracle
Sales ([LinkedIn's CRM matrix](https://www.linkedin.com/help/linkedin/answer/a106005/?lang=en)).

"But hiQ won. Scraping public profiles is legal." I hear this a lot, and it
is the most expensive misreading in our industry. What the 9th Circuit
actually said (2019, reaffirmed
[April 2022](https://cdn.ca9.uscourts.gov/datastore/opinions/2022/04/18/17-16783.pdf))
is only that scraping public pages is likely not a federal *hacking crime*
under the CFAA.

On remand, in
[November 2022](https://caselaw.findlaw.com/court/us-dis-crt-n-d-cal/2182242.html),
the district court held LinkedIn's anti-scraping terms enforceable and granted
LinkedIn summary judgment on hiQ's fake-account "turker" conduct. Liability
for the scraping itself stayed open on waiver and estoppel questions.

Those questions never got answered, because hiQ folded and
[settled](https://www.zwillgen.com/alternative-data/hiq-v-linkedin-wrapped-up-web-scraping-lessons-learned/):
a negotiated consent judgment of USD 500,000, a permanent injunction with no
public/private distinction, deletion of all scraped data and code. No
precedent was set. Something better than precedent was set: the company that
"made scraping legal" ended the fight enjoined and gone. LinkedIn has since
sued
[Mantheos](https://news.linkedin.com/2022/february/taking-legal-action-to-protect-members-against-scraping)
(settled) and
[ProAPIs](https://securityaffairs.com/183001/security/linkedin-sues-proapis-for-15k-month-linkedin-data-scraping-scheme.html)
(filed 2025).

The official route is closed too: LinkedIn's
[partner documentation](https://learn.microsoft.com/en-us/linkedin/sales/)
says it is "not currently accepting new partners" for the Sales Navigator
API, and the self-serve API returns only the name, email and photo of the one
member who consented via OpenID Connect.

Look at who carries the risk in the plugin economy. LinkedIn sues the vendor.
LinkedIn bans the end user's account. That second one is the fastest and most
certain consequence, and it hits YOUR sales rep, not the plugin company. Your
rep loses their professional identity so a vendor could skip an API
application. And European regulators fine whoever stores the data. Which
brings us to the second layer.

## 2. "Publicly available" does not mean "free to take"

The GDPR does not ban B2B enrichment. The Court of Justice confirmed in 2024
that a purely commercial interest can be a legitimate interest under
Art. 6(1)(f)
([*KNLTB*, C-621/22](https://privacymatters.dlapiper.com/2024/10/eu-cjeu-confirms-that-legitimate-interests-can-cover-purely-commercial-interests/)).
Recital 47 names direct marketing as a candidate. The French CNIL and the UK
ICO both accept B2B prospecting on legitimate interest with opt-out and
professional-capacity data only. On retention, CNIL works with a benchmark of
roughly three years after last contact; the ICO
[sets no fixed period](https://ico.org.uk/for-organisations/direct-marketing-and-privacy-and-electronic-communications/direct-marketing-guidance/plan-direct-marketing/)
and demands you justify whatever you keep.

The surprise for me was Article 14: enrichment is possible, but secrecy is
not. If you did not collect the data from the person, you normally have to
tell them individually, within one month or at the first contact, whichever
comes first, and a privacy policy on your own website does not count, because
the person has never heard of you. The disproportionate-effort exception in
Art. 14(5)(b) exists, but it comes with conditions and compensating
safeguards, and the leading case shows how little "it's expensive" buys on
its own. Three cases, three flavours of the same missing duty:

**Bisnode (Poland, 2019 to 2023).** Bisnode built a database of 7.5 million
sole traders and directors from *public government registers*. It emailed the
682,000 people whose address it had and posted a website notice for the rest,
arguing postal notice was disproportionate. The Polish DPA
[fined it](https://uodo.gov.pl/en/553/1572) roughly EUR 220,000 and ordered
individual notification. The Supreme Administrative Court dismissed the final
appeal on 19 September 2023: transparency is the rule, the exceptions are
read restrictively, and on these facts cost alone did not carry the
exemption, with the public-register origin helping nothing. The duty follows
the data.

**Kaspr (France, 5 December 2024, EUR 240,000).**
[CNIL fined Kaspr](https://www.cnil.fr/en/data-scraping-kaspr-fined-eu240000),
whose extension fed a 160-million-contact database from LinkedIn. The
findings: collecting contacts of members who had *restricted their visibility*
(that choice defeats the balancing test), five-year retention, no Art. 14
notice at all until 2022, and access-request answers naming only "publicly
available sources" when CNIL held Kaspr had to hand over all the source
information it actually possessed. CNIL later
[closed the injunction](https://www.cnil.fr/en/closure-order-issued-against-kaspr).
Read that closure carefully before anyone calls it a blessing: Kaspr complied
by deleting the offending data and ceasing the LinkedIn collection. The
regulator approved the exit, not the model.

**Lusha (Italy, July 2026, EUR 2 million).** The Garante
[fined Lusha](https://www.garanteprivacy.it/home/docweb/-/docweb-display/docweb/10275230)
and ordered erasure of ALL Italian contact data within 60 days. Its US
domicile and its TrustArc compliance seal did not change the outcome: the
Garante weighed the certification when setting the amount, and the answer was
still two million.

Around these sit the harder precedents. Clearview AI has been fined roughly
EUR 100 million in total across five European authorities for scraping public
photos: Italy, Greece and France EUR 20M each, the Netherlands EUR 30.5M,
plus the UK's GBP 7.5M,
[still in litigation](https://ico.org.uk/about-the-ico/media-centre/news-and-blogs/2025/10/uk-upper-tribunal-hands-down-judgment-on-clearview-ai-inc/),
and a later French EUR 5.2M non-compliance penalty.

The customer can be fined too. Sweden's IMY
[fined the police](https://www.imy.se/en/about-us/arkiv/nyhetsarkiv/police-unlawfully-used-facial-recognition-app/)
EUR 250,000 merely for *using* Clearview, and twelve DPAs signed a
[joint statement on data scraping](https://ico.org.uk/media2/migrated/4026232/joint-statement-data-scraping-202308.pdf)
in August 2023. Whoever buys or uses unlawfully scraped data carries their
own liability.

Even the platforms are pulled in. The Irish DPC fined Meta
[EUR 265 million](https://www.dataprotection.ie/en/news-media/press-releases/data-protection-commission-announces-decision-in-facebook-data-scraping-inquiry)
over its own data-protection-by-design failures after the 533M-user scraped
dataset surfaced. That decision binds Meta, not every platform, but the
direction it points is uncomfortable for the plugin industry: the host
platform's design duty runs *against* scraping, not with it.

Two consequences follow for anyone holding enriched data. First: Art. 15(1)(g)
gives the person a right to "any available information" about the source, and
Kaspr shows a regulator will not accept "public sources" from a company that
knows more. Our answer is per-field provenance, which is more than the letter
requires and exactly what makes the DSAR answer easy. Second: erasure has to
stick. The law does not
[literally mandate a suppression list](https://ico.org.uk/for-organisations/direct-marketing-and-privacy-and-electronic-communications/direct-marketing-guidance/respect-peoples-preferences/),
but without one the next re-sync quietly resurrects the deleted contact and
puts you back in front of the same lawful-basis question with worse facts. The
ICO recommends one. We ship one; see
[privacy-and-consent.md](privacy-and-consent.md).

I understood the product line only after reading all three decisions: scale
changes the facts. A whole-market database combines sources and tells nobody,
and that is what every fine above punished. A salesperson researching one
prospect still needs the Art. 6 balancing like any other processing, but
walks into that test with the best facts available: a narrow purpose, minimal
fields, and the Art. 14(3)(b) notice riding along with the first outreach at
zero marginal cost. The trigger and the scale are what separate a defensible
lookup from the Bisnode pattern.

One more split to respect: *storing* a business email address is a GDPR
question. *Emailing* it is a separate marketing-law question. Germany's
[§7 UWG](https://www.gesetze-im-internet.de/englisch_uwg/englisch_uwg.pdf)
requires prior express consent for email marketing even B2B, with one narrow
exception (§7(3), existing customers, same-product context, opt-out at every
step). Your CRM can lawfully hold what you may not lawfully cold-email from
Frankfurt.

## 3. Vietnam: consent or nothing

I live in Vietnam, and we build Margince for companies there as well as in
Germany. Vietnamese law is not academic for us. It is also the strictest law
in this paper, and most Western vendors have not noticed it exists.

The timeline first. Vietnam ran on
[Decree 13/2023/ND-CP](https://thuvienphapluat.vn/van-ban/EN/Cong-nghe-thong-tin/Decree-No-13-2023-ND-CP-dated-April-17-2023-on-protection-of-personal-data/564343/tieng-anh.aspx)
until the end of 2025. Since 1 January 2026 the Personal Data Protection Law
applies ([Law 91/2025/QH15](https://english.luatvietnam.vn/dan-su/law-on-personal-data-protection-law-no-91-2025-qh15-405135-d1.html),
passed 26 June 2025), with implementing Decree 356/2025 in force the same day.

What the law says is harder than the GDPR. The model is consent plus a short
closed list of exceptions, and the law's "legitimate interest" basis is
defensive-only: it covers protecting rights against an act of infringement,
meaning fraud prevention and legal claims. Every law firm commentary I
checked
([Tilleke & Gibbins](https://www.tilleke.com/insights/vietnams-new-personal-data-protection-law-a-closer-look/),
[Baker McKenzie](https://connectontech.bakermckenzie.com/vietnam-decoding-vietnams-pdp-law-gdpr-inspired-rules-with-local-twists/),
[DLA Piper](https://www.dlapiperdataprotection.com/?t=law&c=VN),
[Rouse](https://rouse.com/insights/news/2025/vietnam-s-new-personal-data-protection-law-what-businesses-need-to-know))
concurs that it cannot carry prospecting or enrichment. There is no
publicly-available-data basis at all; making data public waives nothing.
Marketing needs consent, no soft opt-in, on top of the Decree 91/2020
anti-spam regime with its do-not-call register.

Article 7(6) prohibits buying and selling personal data, except where the law
itself provides otherwise. "Personal data is not a commodity." The penalty
rules (Article 8 and the sanctions regime) run up to ten times the revenue
gained from illegal data trading, and up to 5% of prior-year revenue for
violations of the cross-border transfer rules
([official text via the Ministry of Public Security](https://bocongan.gov.vn/media/bca-media/photo-library/20250728100402_a4c3955c-ea1d-48a0-9998-c262b14864af-Lu%E1%BA%ADt-B%E1%BA%A3o-v%E1%BB%87-d%E1%BB%AF-li%E1%BB%87u-c%C3%A1-nh%C3%A2n.pdf)).
Buying broker data about Vietnamese contacts is the single highest-risk act
in this entire space.

Enforcement is young. The Ministry of Public Security's A05 department ran
its first compliance campaign in late 2024, and no published fine has hit a
foreign B2B SaaS yet. I would not bet a product on that lasting. Vietnam's
data-trading black market is the stated reason this law exists, so scraping
and aggregation sit in the political crosshairs, not in a tolerated gray
zone.

The product consequence is blunt: a Vietnamese data subject needs consent
that the law recognizes, and Law 91 wants it voluntary, informed, specific
and verifiable. A handed-over business card starts a business relationship;
it does not prove consent to enrichment, marketing or onward disclosure. The
verifiable path is the confirm-your-details flow (A6 below), where the
contact sees the record and says yes on the record.

## 4. The Betriebsrat will read this paper too

In 2024 Germany's Federal Labour Court held that a retail headset system
needs works-council co-determination even though it stored nothing at all
([1 ABR 16/23](https://www.bundesarbeitsgericht.de/wp-content/uploads/2024/11/1-ABR-16-23.pdf)).
That is the doctrine in one image, and it is the reviewer German buyers bring
to the table that US vendors keep forgetting. Under
[§87(1)(6) BetrVG](https://www.gesetze-im-internet.de/betrvg/__87.html), any
technical system *capable* of monitoring employee performance or behaviour is
co-determined; intent is irrelevant and there is no de-minimis threshold. The
court has applied this to an Excel attendance list
([1 ABN 36/18](https://www.bundesarbeitsgericht.de/entscheidung/1-abn-36-18/)),
to SAP (1 ABR 45/11) and to Office 365
([1 ABR 20/21](https://www.hensche.de/bag-beschluss-vom-08.03.2022-1-abr-20-21-zustaendigkeit-des-gesamtbetriebsrats-bei-unternehmenseinheitlicher-nutzung-von-microsoft-office-365.html)).
A CRM with an audit trail and per-rep pipeline numbers is co-determined,
period. That part is routine: a German enterprise CRM rollout normally comes
with a Betriebsvereinbarung, and a well-made one can itself serve as the GDPR
basis for the employee-side processing, provided it meets the requirements
Art. 88(2) GDPR and
[§26(4) BDSG](https://www.gesetze-im-internet.de/englisch_bdsg/englisch_bdsg.html)
put on such agreements.

What decides whether that negotiation takes six weeks or eighteen months is
the feature list. Server-side enrichment of *external* contacts barely
registers; the only employee datum it touches is the audit-trail fact that
user X clicked. The scraping plugins fail on the three points works councils
actually fight: an extension in the employee's browser that watches what they
view, work forced through the employee's personal LinkedIn account, and
mining that turns the team's correspondence into per-employee conduct
profiles. The Kaspr fine hands any works council a ready-made argument that
the tool itself is legally toxic.

Since 2021 there is an AI layer on top. §90 BetrVG obliges the employer to
inform the works council at the *planning* stage of AI use, and the EU AI Act
classifies systems that monitor and evaluate worker performance as high-risk
(Annex III 4(b)). The compliance deadline for Annex III systems was moved by
the 2026 AI omnibus
([Regulation (EU) 2026/1744](https://www.steptoe.com/en/news-publications/steptechtoe-blog/eu-ai-act-amendments-enter-into-force.html))
from 2 August 2026 to 2 December 2027. A delayed deadline, not a cancelled
category. Our rule stands regardless of the date: the AI evaluates records,
deals and companies. Never a named rep's work quality.

And Margince itself? Squarely co-determined, and I will not pretend
otherwise. We capture connected mailboxes, including message bodies, because
a correspondence CRM is the product (see
[capture-connectors.md](capture-connectors.md) and
[ingress-gate-and-auto-capture.md](ingress-gate-and-auto-capture.md)), and
the [relationship graph](relationship-graph.md) computes from that
correspondence a per-colleague warmth toward each *contact*, because "who
here can open this door" is an account question a sales team genuinely has.
Both are designed to be governable: no browser extension, everything
server-side, no rep leaderboards and no performance scores, warmth bands
deliberately built not to be comparable across people or merged into a
ranking, exclusion lists for mailboxes and domains that must never be
captured, audit data access-restricted. The Betriebsvereinbarung then pins
the purpose in writing: account coverage yes, performance evaluation never,
with a Verwertungsverbot on top. That conversation is winnable because the
design gives the works council something to hold. A scraping plugin gives
them something to veto.

## 5. What we build, what we gate, what we refuse

The short version: we build A1 to A6 today. We gate B1 to B3 until the named
controls exist and counsel signs off. We refuse all five Tier C models. The
detail follows, and each item is rated on four axes: GDPR posture, Vietnam
compatibility, platform-contract exposure, works-council temperature.

### Tier A: build freely

**A1. Company enrichment from official sources.** A1 is easy until a natural
person appears. Data about the legal person as such sits outside the GDPR
(Recital 14): name, legal form, VAT number, registered address. The sources:

| Source | Data | Access | Limit |
|---|---|---|---|
| [VIES](https://ec.europa.eu/taxation_customs/vies/) | VAT validity, registered name and address in many member states | Free API; we store the consultation number as proof | Availability varies by state |
| [GLEIF LEI](https://www.gleif.org/en/about/open-data) | Legal entity plus ownership tree | Free API and bulk files, CC0 | Coverage thins outside finance |
| Swiss Zefix | Commercial register, daily change feed | Free REST API | None that bites |
| UK Companies House | Register, officers, filings | Free API and streaming | Rate limits only |
| German Handelsregister | Register data, free since the 2022 DiRUG reform | Web, per-lookup | Portal caps queries and bans bulk |
| Licensed aggregators (e.g. North Data) | Structured DACH register and financial data | Paid API | The clean bulk route |

The line to keep sharp: the moment a natural person appears inside that data,
a director, a sole trader, a named representative, it is personal data again.
Published under statutory duty, which is the strongest legitimate-interest
fact pattern there is, but personal data with all the duties attached.
Identifier spine: EUID, LEI, VAT ID, and the Vietnamese MST.

**A2. The counterparty's own website, read deeper.** Build; the chassis
ships. Margince already reads company sites (see
[company-context.md](company-context.md)), and a deep read already handles
the people a site names, in exactly the shape this paper argues for. A
stranger on the team page stages as a lead and stays staged until a human
accepts; no new person is ever auto-created. When the published person is
unmistakably someone the workspace already records at that company (exact
email match, or a single high-confidence name match), the read fills their
empty fields, role, title, profile link, evidence-backed with the source on
the row, and never overwrites what a human wrote. The clean extensions from
here: parse the Impressum, whose publication German law *mandates* (§5 DDG),
and consume newsroom RSS for company events, storing the signal plus a link
and never the cached full text. Polite crawling, robots.txt respected, and
the existing rule stands: a human click writes directly, an automatic read
stages.

**A3. Technical enrichment.** Build. MX and DNS records, TLS certificate
transparency logs, tech-stack fingerprints from the company's public pages.
Company-level signals that work identically in Hamburg and in Da Nang, with
the same caveat as A1: a personal name inside a certificate or a domain
record is personal data, so we keep what we store at the company level.

**A4. First-party person capture.** Build; mostly shipped. The contact sent
you their details, and Margince's capture already works this way: connected
mailboxes are processed automatically, a nightly pass lifts the stated fields
from email signatures (name, title, phone, nothing inferred), and workspace
exclusion lists keep whole addresses and domains out of capture entirely.
What this tier cleanly extends to next: vCard import and a per-mailbox switch
for the signature pass, so an organization can turn the granularity all the
way down. First party is the only person-data channel that scales cleanly.
See [capture-connectors.md](capture-connectors.md) and
[ingress-gate-and-auto-capture.md](ingress-gate-and-auto-capture.md).

**A5. LinkedIn as a link, plus the member's own export.** We store the
profile URL as a clickable link and never fetch it server-side. The user
reads the profile in their own browser and types what they learned into a
structured form. LinkedIn also gives every member a download of their own
data, and
[import-your-linkedin-network.md](../how-to/import-your-linkedin-network.md)
imports that `Connections.csv` deliberately small: as private graph substrate
("ghosts") for reach and warmth, visible only to the importer, never as
contact records, never creating people. I do not claim LinkedIn's terms
permit anything beyond this private graph use, so Margince stops there: no
contact creation, and no parser for pasted profile text. The moment software
copies the page for you, the scraping clause is back in play, and our line
is drawn on the safe side of it.

**A6. The "confirm your details" flow.** Build. Send the contact a link to
view and correct their own card. The same flow delivers the Art. 14
transparency notice and gives the contact an Art. 16 accuracy mechanism, and
because the consent is verifiable it also satisfies Vietnam's Law 91
standard. It actively de-risks everything else on this page.

### Tier B: build with controls, counsel signs off first

**B1. Widening person lookup beyond the counterparty's own site.** B1 stays
gated. The shipped shape described in A2 (staged leads, fill-only-empty,
evidence on every field, per-field provenance through the write shape; see
[write-backbone.md](write-backbone.md)) is the right chassis: one company at
a time, sources the person published for business contact, a human between
the suggestion and the record. New sources beyond the company's own site
(speaker bios, conference pages, Impressum contact lines of third parties)
enter only under the same staging discipline, and each widening needs its own
Art. 6 balancing on paper, not a generalized "single lookups are fine".
Before we widen anything, the missing controls land:

- the Art. 14 notice, generated and delivered with the first outreach or
  within one month of a staged lead being accepted;
- a justified retention rule with refresh-or-delete, starting from CNIL's
  three-year benchmark;
- the erasure suppression list consulted before a re-read can re-stage a
  deleted person;
- visibility restrictions honored wherever a platform offers them, which is
  precisely where Kaspr lost;
- Vietnamese subjects excluded and routed to A6.

**B2. Art. 14 notice automation as a customer feature.** The CRM generates,
sends and logs the notification our customer owes for any contact not
collected from the subject, with objection handling wired into the
suppression list. I have not found this at any vendor, and I think I know
why: their model cannot survive transparency. Ours is built on it. We do not
ship the notice texts until counsel has reviewed them market by market.

**B3. Licensed notified-database providers.** Providers like
[Cognism](https://www.cognism.com/compliance) and Dealfront run a "notified
database": they claim Art. 6(1)(f) with a documented balancing test and
discharge Art. 14 by emailing the data subjects. That model is
provider-asserted, not regulator-certified, and the customer remains an
independent controller with their own duties.

On our side, what a licensed provider asserts about a person lands as a claim
row with the purchasing run attached (`person_provider_claim`), separate from
the record itself, and a delete-data action removes the local claims and
scrubs the run metadata.

What does NOT exist yet, and goes on the same counsel checklist as the
provider contract: automatic purge when a provider is disconnected, and
plumbing for provider-side erasure signals to flow downstream. Until both
exist, B3 stays a gated integration, and never for Vietnamese subjects;
Article 7(6) forbids the trade itself.

### Tier C: refuse

- **Bulk background enrichment of a market.** The Bisnode and Lusha pattern:
  a standing shadow database of everyone, built before any user asked. No
  realistic Art. 14 story, scale wrecks the balancing test, and Italy has
  shown a regulator will order erasure of the entire national dataset.
  Reading one company's own site for a human working that company (A2) is
  the opposite end of the proportionality scale.
- **LinkedIn or Xing ingestion in any automated form.** For LinkedIn this
  breaches the quoted contract for everyone who agreed to it, replays the
  Kaspr fine, and the sanctioned API path is closed. "The user's browser
  does it" defends nothing. LinkedIn sues over exactly this model. For Xing
  I cite no contract clause; the GDPR analysis and the works-council
  analysis alone put it here, and that is enough.
- **Any browser extension.** Twice disqualified: LinkedIn's clause names
  plugins, and a tool that observes what an employee views in their browser
  is textbook §87(1)(6) surveillance. Even where some narrow extension might
  survive a lawyer's reading, this product line does not want to be in that
  argument.
- **Purchased lists and broker data of unclear provenance.** The recipient
  becomes controller of data with no lawful-basis chain. For Vietnamese
  subjects, add the trading ban and its ten-times-revenue fine.
- **Rep scoring.** We refuse it, and the refusal is precise because we DO
  compute from correspondence (section 4): no inferred employee traits, no
  conduct profiles, no performance scores, no surface that ranks reps
  against each other. The relationship graph answers "who can open this door
  for this account", per contact, in bands built not to be summed into a
  scoreboard. Whether mailbox processing passes §26 BDSG is a
  purpose-by-purpose assessment, and account coverage with those guardrails
  is a purpose we can defend in writing. Rep scoring is not, and it stays
  refused.

## 6. Sources and verification status

Primary decisions and rulings, verified against the issuing body where linked
above: CJEU C-621/22 (*KNLTB*); UODO v. Bisnode with the NSA ruling of
19 Sep 2023; CNIL v. Kaspr, EUR 240,000, 5 Dec 2024, plus the closure notice;
Garante v. Lusha, EUR 2M, Jul 2026; the Clearview AI fine series (IT, GR, FR,
NL, UK; UK in litigation); IMY v. the Swedish police; Irish DPC v. Meta,
EUR 265M; hiQ v. LinkedIn (9th Cir. 2022, N.D. Cal. Nov 2022, consent
judgment Dec 2022); LinkedIn v. Mantheos and v. ProAPIs; BAG 1 ABN 36/18,
1 ABR 45/11, 1 ABR 20/21, 1 ABR 16/23. Statutes: GDPR Arts. 5, 6(1)(f), 14,
15(1)(g), 17, 21, 88; Recitals 14 and 47; §87(1)(6), §90, §80(3) BetrVG; §26
BDSG; §5 DDG; §7 UWG; EU AI Act Annex III 4(b) as amended by Regulation (EU)
2026/1744; Vietnam Decree 13/2023/ND-CP, PDPL Law 91/2025/QH15 (Arts. 7(6),
8, 9), Decree 356/2025, Decree 91/2020. Vietnamese-law readings rest on the
official texts plus concurring commentary from Tilleke & Gibbins, DLA Piper,
Baker McKenzie, Rouse and DFDL.

On 27 Aug 2026 I reconciled all 26 findings of an adversarial fact-check into
this version. Counsel still needs to close four Tier B questions: the final
adoption status of the EDPB's Guidelines 1/2024 on legitimate interest (still
draft as of this writing); Vietnam's administrative sanctions decree (in flux
through 2026); the exact section numbering of LinkedIn's subscription
agreement; the §7 UWG detail per outreach channel. None of these changes a
tier placement.

## 7. Why this wins deals

This paper defines what Margince can sell with a straight face. The scraping
vendors made a bet: LinkedIn's enforcement stays slow, DPAs stay busy
elsewhere, no works council reads the CNIL's press releases. Every year that
bet gets worse. Since 2022 the count reads: one permanent injunction
(consented, and no less permanent for it), two more LinkedIn lawsuits, three
DPA fines over untold data subjects, one nationwide erasure order.

We took the other side of the bet. Verified registers, the counterparty's own
site, first-party signals, consent flows, and notification automation give
real data quality with an audit trail behind every field. Our market is DACH
companies with works councils, DPOs and regulators on the buying committee.
For them the compliance IS the product. And for Vietnamese contacts I have
yet to see another CRM with a considered answer at all.

So ask your current vendor two questions. Who indemnifies you when your reps'
LinkedIn accounts get banned? And who sends the Art. 14 notices for the ten
thousand contacts the plugin just imported? We have answers to both. They
have a Chrome extension.
