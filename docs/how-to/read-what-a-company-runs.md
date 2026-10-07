<!-- prose:plain -->
# Read what a company publicly runs

Margince reads free public sources: DNS records, certificate transparency logs, and the pages of the
company's own site. It writes what it finds on the company record, as facts with evidence. A deep read says
what a company **tells** you; this reading says what you can prove it **runs**.

> **No button.** There is no lookup button. The site read puts the reading in the queue, and a
> scheduled sweep comes back to companies whose data is stale. If you want to read a company
> again now, read its site.

## What it reads

| Field | Source | Set | Example values |
|---|---|---|---|
| `mail_provider` | MX records | one | `google_workspace`, `microsoft365`, `self_hosted`, `other` |
| `email_security` | TXT records | many | `spf`, `dmarc_reject`, `dmarc_quarantine`, `dmarc_none`, `dkim` |
| `hosting_provider` | A/AAAA, CNAME, PTR | one | `hetzner`, `aws`, `cloudflare`, `ionos`, `strato`, `azure`, `google_cloud`, `other` |
| `operated_service` | certificate log | many | `webshop`, `careers`, `customer_portal`, `api`, `vpn`, `status_page` |
| `technology` | site-page fingerprint | many | `shopware`, `shopify`, `wordpress`, `typo3`, `matomo`, `google_analytics` |

Every value is a `company_fact` row with `category='signal'` and `source='technical_lookup'`. It carries
the public record that proved it: the MX host that won, the subdomain that proved it, or the matched
marker. So the record can always answer "how do you know?".

The reading is **deterministic**. It uses classifiers that read from tables, and a set of fingerprint rules
that humans wrote by hand. No part of the lane calls a model.

### Two readers write `technology`, to different standards

The site read makes `technology` facts twice, from the same crawl, and the two mean different things.

**The fingerprint** reads what the site **serves**: an answer header, a cookie name, a script `src`, a
`<meta generator>`, or a marker in the markup. `nginx` from `Header server: nginx` is a fact about what
runs, and page text cannot give that fact.

**The model** reads what the company **states** it uses, from the page text. Examples are a stack that a
service company says it builds with, or a platform named on a careers page. That is a second tier of claim. So the
vocabulary in `compose/sitereadvocab.go` requires the passage to state that this company itself uses it. A
vendor that is only named, compared, or listed as an integration is not a technology fact. A research company
or its report (Gartner, Forrester) never is, and a category alone (BI, CRM, ERP, PIM) is not a product.

Both go in the same part of the card, each with its own evidence. So a reader who opens the mark can see
which kind of claim they look at.

### The sources

- **DNS** (`internal/platform/dnsread`): MX, TXT (SPF at the root, DMARC at `_dmarc.`, DKIM at a set list of
  selectors), A and AAAA, and CNAME. It also makes a reverse `PTR` lookup for a sign of the host. It runs at one
  query each `200ms`.
- **Certificate transparency** (`internal/platform/certlog`): one `GET https://crt.sh/?q=%25.<domain>&output=json`
  for each company. Every public certificate must go to public logs that you can only add to. So the host names on the
  certificates of a company are already public. To read them needs no agreement and no key, and it runs at
  **one query each 5 seconds**. The [crt.sh](https://crt.sh) service is free and runs on good will. The team that runs it
  has asked users not to send many queries at the same time.
- **Site pages** (`internal/platform/webread` + `compose/sitetechnology.go`): the fingerprint of **every page
  that the site read fetched**, and not only the start page. The fingerprint covers answer headers, cookie
  names, script `src` values, `<meta generator>`, and the markup itself. A shop system shows itself on `/shop`, a portal on `/kunden`, and a
  careers platform on `/karriere`. A fetch of only the start page sees no page of the three. The crawl already
  fetched these pages, so this costs no more requests. The evidence names the page that proved it.

### What is never stored

A certificate log publishes every host name on any certificate of a company. These include
the names of humans: `lars.example.de` is a normal thing to find. The subdomain classifier is an
**allowlist**. Only first labels that name a *service* pass it, and it runs **before the cache write and
before the fact write**. A human name in a certificate matches nothing and reaches no table, for any
caller.

## What starts it

Each lane has its own fetcher, so different things start them.

**The page lane is in the site read.** When a crawl ends and has matched a company, the read
matches every page it fetched against the rules. Then it writes the technology facts itself
(`compose/sitetechnology.go`). No job, no queue, no second request: the pages are already in hand. This
covers both the capture read that runs on its own, and a human who clicks **read the site**.

The DNS and certificate lanes **run as their own job**, and these things put that job in the queue:

1. **Every site read**, right after the page lane (`compose/technicalonsiteread.go`).
2. **The scheduled sweep** (`technical_enrich_backfill`).

   By default, it runs every 6 hours, and takes up to 25 companies in each workspace whose data is missing,
   or older than 7 days. It keeps the data up to date. Geocoding starts when someone *writes* an address. But
   the mail provider changes at the **company**, and no write on our part shows the change. Only a scheduled
   pass sees a move.
3. **`POST /companies/{id}/technical-enrich`**: the API path, `202` with no body. No screen calls it; it
   exists for scripts.

The DNS and certificate lanes **wait in a queue instead**. A certificate log that waits 5 seconds between
queries would keep a deep read worker waiting, for no reason linked to the crawl. And the system has few
workers of that kind. So they run on their own `technical_lookup` queue,
one job at a time.

**The sweep does not refresh technology.** It refreshes mail, hosting and the services a company runs. A
company that changes from Shopware to Shopify keeps the old row until someone reads its site again.
Only a crawl can see what the pages declare.

The queue drops a copy of a job that waits or runs with the same args. Say a user reads the site of a
company that the sweep has put in the queue. Then the read uses that lookup, and does not ask the same two
services twice. A lookup that a human starts runs before a sweep job, so it never waits behind a batch.

The **domain is not a job argument**. The job reads the domain the company record holds when it runs. A
copy in the args would be the same stale lookup bug, moved one layer out. It would also be a way to point
the lookup at a domain that was never on the record.

## Set it up

There are two settings. The address of the certificate log is an environment value that the **worker**
reads when it starts. The API does not read it, because `cmd/worker/jobrunner.go` builds the enricher. Put
it in `.env.local`, which `scripts/dev.sh` reads and exports. The time between sweeps is an admin setting
in Settings → System health. A running worker sees a change without a new start.

### `MARGINCE_CERTLOG_BASE_URL`: required, or the whole lane is off

```sh
MARGINCE_CERTLOG_BASE_URL=public
```

`public` is a key word, not a URL: it means `https://crt.sh`. To use your own copy of a certificate
transparency service, give its real base URL instead. It must answer queries of the [crt.sh](https://crt.sh) shape
(`/?q=%25.<domain>&output=json`).

**Empty or unset turns both lanes off.** An enricher with only some lanes would complete these lanes, and
never the others. A lane that never completes leaves its facts frozen at the last full run. On the
record, that looks the same as a company that has not changed. An installation that should make no outbound
lookups leaves it unset.

When it is unset, the job kinds are never registered. Each step that puts a lookup in the queue makes a job
that tries again and again against a kind that is not registered:

```
job kind is not registered in the client's Workers bundle: technical_enrich_company
```

The worker reads `MARGINCE_CERTLOG_BASE_URL` when it starts. Run `make dev` again after you change it.
Vite shows changes to the app as you type, but not to the Go worker.

### Set the time between refreshes

This is an admin setting, **Technical lookup sweep (seconds)**, in Settings → System health. It is 21600
seconds (6 hours) by default. It runs at start. `0` turns the sweep off, and leaves the lookup to the site
read that puts it in the queue. A running worker checks for changes every minute.

## Where it shows

The **Technology** card is on the **Profile** tab of the company record. It is below the site read card that
starts it, where a reader who has started company research looks for the results. Its parts are **Mail**,
**Website technology**, **Services** and **Hosting**.

The card only shows data, and has no controls. Each value has an evidence mark that shows the public record
behind it. When a source gave no answer, a notice at the end of the card names it. Then a missing service
means `not checked today`, and not `they have none`. So a reader who decides whether to trust `no webshop`
knows that the certificate log was down.

When a human fixes a value, the source of the row changes to `human`, and the row stays on this card. The
card sorts rows by **field name**, never by source. So the system never drops a fixed row from both cards,
and never shows it on two.

Changed signals also show on the company rail as a `technical_change` event.

## How a refresh changes the facts

Each lane that **completed** decides its own fields. It removes the rows it no longer sees. So a company
that moves from Google to Microsoft 365 ends with one mail provider, and not two.

A lane that **failed** changes nothing. A certificate log outage must never be recorded as "this company
runs no services". This is why the lane outcomes keep three cases separate. `empty` means the source answered,
and the company publishes nothing that this lane reads. `failed` means the lookup failed to complete.
`refused` means `robots.txt` refused the page read.

A refresh by the system never removes rows that a human wrote.

## The cache keeps answers, with a TTL

The cache keys on query name and record type, for the whole installation. The DNS answer for a
domain is the same for every tenant. The cache also records empty answers, so the system does not ask again
on every run for a company with no DMARC.

| Kind | Trusted for |
|---|---|
| MX, TXT, DMARC | `24h` |
| Address (A/AAAA), CNAME | `12h` |
| DKIM, reverse (PTR) | 7 days |
| Certificate log | `24h` |

Every TTL is shorter than the refresh every 7 days. A cache entry that lasted longer than the refresh would
keep the sweep from seeing the move it is there to catch. The cache holds **only the outcomes of the classifier**. The
allowlist has already run, so no certificate host name is stored as it was.

## Check that it worked

**In the app.** Open a company with a real domain, read its site, and watch the **Technology** card. The
lookup goes in the queue when the read ends. So the fields come soon after the read closes, not with it.

**For each lane.** `GET /companies/{id}/technical-enrich/latest` reports the last run of each of the three
sources, with try counts and the time of the last good run. It reports for each lane, and not for each run,
because the three sources fail on their own. One verdict would not show which of them is stale. The `homepage`
lane is the site page one, which the crawl writes.

**In the job table.**

```sh
docker exec margince-postgres-1 psql -U margince_app -d margince \
  -c "select kind, state, attempt, left(coalesce(errors::text,''),200) \
      from river_job where kind like '%technical%' order by id desc limit 10;"
```

`state='retryable'` with the "not registered" error means `MARGINCE_CERTLOG_BASE_URL` is unset. Or the
worker has not started again after you set it.

**Against one domain, without a database.** There is no command for this lane that runs without a database
yet (`worker siteread <url>` covers only the deep read). To test the classifiers on their own, use the table
tests in `backend/internal/compose/techenrich_test.go` as the loop.

## When the card stays empty

- **The feature is off.** `MARGINCE_CERTLOG_BASE_URL` is unset, or the worker has not started again after that.
- **The company has no stored domain.** The lanes read the domain of the record and nothing else, so a
  company without one gives nothing. A demo company from seed data with a made-up domain finds nothing, and
  rightly writes nothing.
- **Nothing has read the site yet.** The lookup follows the site read. A company that no one has read
  waits for the sweep.
- **The company publishes few signals.** A small site on shared hosting, with no subdomains and no markers the
  rules know, is a normal `empty` result, not a failure.

## Limits worth knowing

- **One provider for the certificate lane.** The [crt.sh](https://crt.sh) service is slow in many cases, and down at times. Callers count that as
  "this lane has nothing to say today", never as an empty answer they can trust. The `certlog.Client`
  interface is what keeps that from turning into a rule we never check.
- **The fingerprint rules are few** (`platform/techprofile/data/rules.json`), and they are made for small
  German companies. Humans add to them by hand, not through a model.
- **No paid data sets of technology stacks**, and no Wappalyzer import. These data sets and their forks are GPL,
  which cannot go with the BUSL-1.1 license of this code.

## See also

- [add-a-job.md](add-a-job.md): how the two job kinds and their queue are declared.
- [connect-an-mcp-client.md](connect-an-mcp-client.md): how to reach these facts as tools.
