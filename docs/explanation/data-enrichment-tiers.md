# What Margince Builds, Gates and Refuses

**The three tiers the enrichment position produces: what we build freely, what
we gate, and what we refuse outright.**


| | |
|---|---|
| **Author** | Lars Jankowfsky |
| **Date** | 27 August 2026 |
| **Scope** | European Union law (with German specifics) and Vietnamese law. Other jurisdictions can be added later; nothing here covers them yet. |
| **Status** | Position paper, adversarially fact-checked. This is research to brief counsel with, not legal advice. Every load-bearing claim carries a named source. |

---

Split out of [Why Margince Doesn't Scrape](data-enrichment-position.md), which
carries the legal argument this table is the conclusion of. Read that first: the
ratings below are only meaningful against the law it sets out, and every tier
here is a consequence of a claim made there.

## What we build, what we gate, what we refuse

Everything above compresses into three tiers. Each item is rated on four
axes: GDPR posture, Vietnam compatibility, platform-contract exposure,
works-council temperature.

### Tier A: build freely

**A1. Company enrichment from official sources.** Data about the legal
person as such sits outside the GDPR (Recital 14): name, legal form, VAT
number, registered address. [VIES](https://ec.europa.eu/taxation_customs/vies/)
VAT validation, free, returns the registered name and address for many member
states; we store the consultation number as proof.
[GLEIF LEI](https://www.gleif.org/en/about/open-data), CC0-licensed, free API
and bulk files, including the ownership tree. Swiss Zefix with its free REST
API and daily change feed. UK Companies House, free API and streaming. German
Handelsregister lookups, free since the 2022 DiRUG reform, but strictly
per-lookup: the portal caps queries and bans bulk retrieval, so bulk always
goes through a licensed aggregator such as North Data. And the line to keep
sharp: the moment a natural person appears inside that data, a director, a
sole trader, a named representative, it is personal data again. Published
under statutory duty, which is the strongest legitimate-interest fact pattern
there is, but personal data with all the duties attached. Identifier spine:
EUID, LEI, VAT ID, and the Vietnamese MST.

**A2. The counterparty's own website, read deeper.** Margince already reads
company sites (see [company-context.md](company-context.md)), and a deep read
already handles the people a site names, in exactly the shape this paper
argues for. A stranger on the team page stages as a lead and stays staged
until a human accepts; no new person is ever auto-created. When the published
person is unmistakably someone the workspace already records at that company
(exact email match, or a single high-confidence name match), the read fills
their empty fields, role, title, profile link, evidence-backed with the
source on the row, and never overwrites what a human wrote. The clean
extensions from here: parse the Impressum, whose publication German law
*mandates* (§5 DDG), and consume newsroom RSS for company events, storing the
signal plus a link and never the cached full text. Polite crawling, robots.txt
respected, and the existing rule stands: a human click writes directly, an
automatic read stages.

**A3. Technical enrichment.** MX and DNS records, TLS certificate
transparency logs, tech-stack fingerprints from the company's public pages.
Company-level signals that work identically in Hamburg and in Da Nang, with
the same caveat as A1: a personal name inside a certificate or a domain
record is personal data, so we keep what we store at the company level.

**A4. First-party person capture.** The contact sent you their details.
Margince's capture already works this way: connected mailboxes are processed
automatically, a nightly pass lifts the stated fields from email signatures
(name, title, phone, nothing inferred), and workspace exclusion lists keep
whole addresses and domains out of capture entirely. What this tier cleanly
extends to next: vCard import and a per-mailbox switch for the signature
pass, so an organization can turn the granularity all the way down. First
party is the only person-data channel that scales cleanly. See
[capture-connectors.md](capture-connectors.md) and
[ingress-gate-and-auto-capture.md](ingress-gate-and-auto-capture.md).

**A5. LinkedIn as a link, plus the member's own export.** We store the
profile URL as a clickable link and never fetch it server-side. The user
reads the profile in their own browser and types what they learned into a
structured form. And LinkedIn gives every member a download of their own
data; [import-your-linkedin-network.md](../how-to/import-your-linkedin-network.md)
imports that `Connections.csv` deliberately small: as private graph
substrate ("ghosts") for reach and warmth, visible only to the importer,
never as contact records, never creating people. Whether LinkedIn's terms
would tolerate more than that is exactly why we built no more than that.
What we will not build is a parser for pasted profile text. The moment
software copies the page for you, the scraping clause is back in play, and
our line is drawn on the safe side of it.

**A6. The "confirm your details" flow.** Send the contact a link to view and
correct their own card. One feature, three legal jobs: it is the Art. 14
transparency notice, the Art. 16 accuracy mechanism, and, because it collects
verifiable consent, the one enrichment channel that satisfies Vietnam's
Law 91 standard. It actively de-risks everything else on this page.

### Tier B: build with controls, counsel signs off first

**B1. Widening person lookup beyond the counterparty's own site.** The
shipped shape described in A2 (staged leads, fill-only-empty, evidence on
every field, per-field provenance through the write shape; see
[write-backbone.md](write-backbone.md)) is the right chassis: one company at
a time, sources the person published for business contact, a human between
the suggestion and the record. What stays gated on counsel is widening it.
New sources beyond the company's own site (speaker bios, conference pages,
Impressum contact lines of third parties) enter only under the same staging
discipline, and each widening needs its own Art. 6 balancing on paper, not a
generalized "single lookups are fine". Before we widen anything, the missing
controls land: the Art. 14 notice generated and delivered with the first
outreach or within one month of a staged lead being accepted, a justified
retention rule with refresh-or-delete (CNIL's three-year benchmark as the
starting point), the erasure suppression list consulted before a re-read can
re-stage a deleted person, visibility restrictions honored wherever a
platform offers them (that is precisely where Kaspr lost), and Vietnamese
subjects excluded and routed to A6.

**B2. Art. 14 notice automation as a customer feature.** The CRM generates,
sends and logs the notification our customer owes for any contact not
collected from the subject, with objection handling wired into the
suppression list. I have not found this at any vendor, and I think I know
why: their model cannot survive transparency. Ours is built on it. Needs
per-market legal review of the notice texts before shipping.

**B3. Licensed notified-database providers.** Providers like
[Cognism](https://www.cognism.com/compliance) and Dealfront run a "notified
database": they claim Art. 6(1)(f) with a documented balancing test and
discharge Art. 14 by emailing the data subjects. That model is
provider-asserted, not regulator-certified, and the customer remains an
independent controller with their own duties. On our side, what a licensed
provider asserts about a person lands as a claim row with the purchasing run
attached (`person_provider_claim`), separate from the record itself, and a
delete-data action removes the local claims and scrubs the run metadata.
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
- **Evaluating employees from their communications.** The refusal here is
  precise, because we DO compute from correspondence (section 4): no
  inferred traits or conduct profiles about employees, no performance
  evaluation from communication data, no surface that ranks reps against
  each other. The relationship graph answers "who can open this door for
  this account", per contact, in bands built not to be summed into a
  scoreboard. Whether mailbox processing passes §26 BDSG is a
  purpose-by-purpose assessment, and account coverage with those guardrails
  is a purpose we can defend in writing. Rep scoring is not, and it stays
  refused.
