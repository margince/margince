# Read what a company publicly runs

Margince reads three free public sources — DNS records, certificate-transparency logs, and the
company's own homepage — and writes what it finds onto the company record as facts with evidence.
It answers a different question from the deep read beside it: a deep read says what a company
**tells** you, this says what it demonstrably **runs**.

This guide covers what it reads, how it is triggered, what you configure, and how to tell whether
it worked.

> **Nothing to press.** There is no lookup button. The reading is queued by the site read, and a
> scheduled sweep comes back round for companies whose picture went stale. If you want a company
> re-read now, read its site.

## What it reads

| Field | Source | Set | Example values |
|---|---|---|---|
| `mail_provider` | MX records | exactly one | `google_workspace`, `microsoft365`, `self_hosted`, `other` |
| `email_security` | TXT records | many | `spf`, `dmarc_reject`, `dmarc_quarantine`, `dmarc_none`, `dkim` |
| `hosting_provider` | A/AAAA, CNAME, PTR | exactly one | `hetzner`, `aws`, `cloudflare`, `ionos`, `strato`, `azure`, `google_cloud`, `other` |
| `operated_service` | certificate log | many | `webshop`, `careers`, `customer_portal`, `api`, `vpn`, `status_page` |
| `technology` | homepage fingerprint | many | `shopware`, `shopify`, `wordpress`, `typo3`, `matomo`, `google_analytics` |

Every value is an `organization_fact` row with `category='signal'` and `source='technical_lookup'`,
carrying the public record that proved it — the winning MX host, the proving subdomain, the matched
marker — so the record can always answer "how do you know?".

Extraction is **deterministic**. Table-driven classifiers and a hand-curated fingerprint ruleset,
no model call anywhere in the lane.

### Two lanes write `technology`, to different standards

The deep read also writes `technology` facts, and the two mean different things.

This lane writes only what the site **serves**: a response header, a cookie name, a script `src`, a
`<meta generator>`. `nginx` from `Header server: nginx` is a fact about what is running, and no
amount of prose can produce one.

The deep read writes what the company **states** it uses, read from page text by a model. That is
worth having — a stack a consultancy says it builds in, a platform named on a careers page — but it
is a weaker claim, and the vocabulary in `compose/sitereadvocab.go` is deliberately narrow about it:
the passage must assert this company's own use. A vendor merely named, compared, or offered as an
integration is not a technology fact, an analyst firm or its report (Gartner, Forrester) never is,
and a bare category (BI, CRM, ERP, PIM) is not a product.

Both land in the same section of the card, each carrying its own evidence, so a reader who opens the
mark can see which kind of claim they are looking at.

### The three sources

- **DNS** (`internal/platform/dnsread`) — MX, TXT (SPF at the root, DMARC at `_dmarc.`, DKIM at
  bounded well-known selectors), A and AAAA, CNAME, and a reverse `PTR` lookup for the hosting hint.
  Paced at one query per 200ms.
- **Certificate transparency** (`internal/platform/certlog`) — one `GET https://crt.sh/?q=%25.<domain>&output=json`
  per company. Every publicly trusted certificate must be published to append-only public logs, so
  the hostnames a company holds certificates for are already public record; reading them needs no
  agreement and no key. Paced at **one query per five seconds** — crt.sh is a single free service
  run on goodwill, and its operators have asked heavy users to be gentle rather than parallel.
- **Homepage** (`internal/platform/webread`) — the fingerprint (response headers, cookie names,
  script `src`s, `<meta generator>`) gathered during the fetch the site read already makes. Same
  fetcher, same user-agent, same robots.txt obedience as any other read.

### What is never stored

A certificate log publishes every hostname a company ever held a certificate for, and those include
people — `lars.example.de` is a normal thing to find. The subdomain classifier is an **allowlist**:
only first labels that name a *service* survive it, and it runs **before the cache write and before
the fact write**. A personal name in a certificate matches nothing and reaches no table. That is a
property of the classifier, not something a caller has to remember.

## What triggers it

Three paths, all landing on the same deduplicated job:

1. **Every site read.** When a deep read finishes and resolved a company, it queues the technical
   lookup (`compose/technicalonsiteread.go`). This covers both the automatic capture read and a
   human pressing **read the site** on the company record. This is the path that matters day to day.
2. **The scheduled sweep** (`technical_enrich_backfill`). Every 6 hours by default, it nominates up
   to 25 companies per workspace whose picture is missing or older than 7 days. Freshness is the
   point of this lane: unlike geocoding, which fires when an address is *written*, a company's mail
   provider changes at the **company** and no write on our side ever announces it. Only a scheduled
   pass observes a move.
3. **`POST /organizations/{id}/technical-enrich`** — the API surface, 202 with no body. No UI calls
   it; it exists for scripting.

The lookup **enqueues rather than runs inline**, because three services with a five-second pacer
would park a deep-read worker — the scarcest kind in the fleet — on a wait that has nothing to do
with crawling. It runs on its own single-threaded `technical_lookup` queue.

Jobs are deduplicated by args while queued or running, so a site read of a company the sweep just
nominated joins that lookup instead of asking the same three services twice. A human-triggered
lookup runs at a higher priority than a sweep nomination, so it never waits behind a batch.

The **domain is not a job argument**. The job reads whatever the company record holds when it runs.
A copy in the args would be a stale-lookup bug moved one layer out, and a way to point the lookup at
a domain the record never carried.

## Configure it

Two settings, both read by the **worker** at boot. The api does not read them — the enricher is
built in `cmd/worker/jobrunner.go`. Put them in `.env.local`, which `scripts/dev.sh` sources and
exports.

### `MARGINCE_CERTLOG_BASE_URL` — required, or the whole lane is off

```sh
MARGINCE_CERTLOG_BASE_URL=public
```

`public` is a keyword, not a URL: it resolves to `https://crt.sh`. To use your own
certificate-transparency mirror, give the real base URL instead — it must answer the crt.sh query
shape (`/?q=%25.<domain>&output=json`).

**Empty or unset turns the entire feature off**, all three lanes together. That is deliberate: a
partial enricher would complete some lanes and never the others, and a lane that never completes
leaves its facts frozen at whatever the last full run saw — which on the record is indistinguishable
from a company that has not changed. An installation that should make no outbound lookups queries
nothing on purpose.

With it unset, the job kinds are never registered, and anything that enqueues a lookup produces a
job that retries against an unregistered kind:

```
job kind is not registered in the client's Workers bundle: technical_enrich_organization
```

### `--technical-backfill-interval` — how often to refresh

Defaults to **6h**. Runs on start. `0` turns the sweep off and leaves the lookup to the site read
that queues it.

### Restart after changing either

The worker is a compiled binary and reads both at boot. `make dev` again — Vite hot-reloads the SPA,
not the Go worker.

## Where it shows

The **Technik** card on the company record's **Profile** tab, inside **Daten & Werkzeuge** and below
the site-read card that queues it — a reader who just started a company research looks there for what
it brought back. Four sections: Mail · Website-Technik · Dienste · Hosting.

The card is a read with no controls. Each value carries an evidence mark showing the public record
behind it. When a source did not answer, a notice at the foot of the card names it — worth a line
because a missing service then means "not checked today" rather than "they have none", and a reader
deciding whether to trust "no webshop" deserves to know the certificate log has been down.

A human correcting a value rewrites the row's source to `human`, and the row stays on this card. The
card partitions by **field name**, never by source, so a corrected row is never dropped from both
cards or rendered on two.

Changed signals also appear on the company rail as a `technical_change` event.

## How a refresh reconciles

Each lane that **completed** is authoritative over its own fields: rows it no longer observes are
removed, so a company that moves Google → Microsoft 365 ends with exactly one mail provider rather
than two.

A lane that **failed** changes nothing. A certificate-log outage must never be recorded as "this
company operates no services". This is why the lane outcomes distinguish `empty` (the source
answered; the company publishes none of what this lane reads) from `failed` (the lookup did not
complete) and `refused` (robots.txt declined the homepage read).

Rows a human wrote are never removed by a machine refresh.

## Answers are cached, with a TTL

Cached per query name and record type, installation-global — a domain's DNS answer is the same for
every tenant. Negative results are recorded explicitly, so a company with no DMARC is not re-asked
every run.

| Kind | Trusted for |
|---|---|
| MX, TXT, DMARC | 24h |
| Address (A/AAAA), CNAME | 12h |
| DKIM, reverse (PTR) | 7 days |
| Certificate log | 24h |

Every TTL is shorter than the 7-day refresh cadence. That is the constraint rather than a
coincidence: a cache entry that outlived the refresh would make the sweep unable to observe the very
move it exists to catch. The cache holds **classified outcomes only** — the allowlist has already
run, so no raw certificate hostname is stored.

## Check that it worked

**In the app.** Open a company with a real domain, read its site, and watch the Technik card. The
lookup is queued when the read finishes, so the fields arrive shortly after the read closes, not
with it.

**Per lane.** `GET /organizations/{id}/technical-enrich/latest` reports what each of the three
sources last did, with attempt counts and last-success stamps. Per lane rather than per run, because
the three sources fail independently and one verdict would hide which of them is stale.

**In the job table.**

```sh
docker exec margince-postgres-1 psql -U margince_app -d margince \
  -c "select kind, state, attempt, left(coalesce(errors::text,''),200) \
      from river_job where kind like '%technical%' order by id desc limit 10;"
```

`state='retryable'` with the "not registered" error means `MARGINCE_CERTLOG_BASE_URL` is unset or
the worker has not been restarted since you set it.

**Against one domain, without a database.** There is no DB-less subcommand for this lane yet
(`worker siteread <url>` covers the deep read only). To exercise the classifiers directly, the
table-driven tests in `backend/internal/compose/techenrich_test.go` are the loop.

## When the card stays empty

- **The feature is off.** `MARGINCE_CERTLOG_BASE_URL` unset, or the worker not restarted since.
- **The company has no stored domain.** The lanes read the record's own domain and nothing else, so
  a company without one produces nothing. A seeded demo company with a fake domain resolves nothing
  and correctly writes nothing.
- **Nothing has read the site yet.** The lookup rides the site read. A company nobody has read waits
  for the sweep.
- **The company genuinely publishes little.** A small site on shared hosting with no subdomains and
  no recognisable markers is an ordinary `empty` result, not a failure.

## Limits worth knowing

- **One provider for the certificate lane.** crt.sh is frequently slow and sometimes down. Callers
  treat that as "this lane had nothing to say today", never as an authoritative empty answer. The
  `certlog.Client` interface is what keeps that from hardening into an assumption.
- **The fingerprint ruleset is small** (34 rules in `platform/techprofile/data/rules.json`)
  and German-SMB-biased. It grows by hand, not
  through a model.
- **No paid tech-stack datasets**, and no Wappalyzer import — those datasets and their forks are GPL,
  which is incompatible with this codebase's BUSL-1.1 licence.

## Related

- [add-a-job.md](add-a-job.md) — how the two job kinds and their queue are declared.
- [connect-an-mcp-client.md](connect-an-mcp-client.md) — reaching these facts as tools.
