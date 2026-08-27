# What Margince Builds Instead of Scraping

**The tiered feature position that follows from
[data-enrichment-position.md](data-enrichment-position.md): what is built
freely, what is gated behind counsel, and what is refused.**

Read that paper first for the law this rests on — the LinkedIn contract, the
GDPR duties, Vietnam's consent standard, and the works-council footing. This
page is the answer it produces, and carries the sources for both.

---

## 1. What we build, what we gate, what we refuse

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

## 2. Sources and verification status

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

## 3. Why this wins deals

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
