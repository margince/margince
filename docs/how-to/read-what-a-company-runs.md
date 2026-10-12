<!-- prose:plain -->
# Turn on the public technology lookup

The handbook page [Relationships, introductions and research](../handbook/relationships-and-research.md)
covers what a user sees: the **Technology** card on a company. This page is for the operator: the
setting that turns the lookup on, what queues it, and how to check it ran.

The lookup reads free public sources: DNS records, certificate transparency logs, and the pages the
site read already fetched. It writes `company_fact` rows with `category='signal'` and
`source='technical_lookup'`, in five fields: `mail_provider`, `email_security`, `hosting_provider`,
`operated_service` and `technology`. No part of it calls a model.

## The three lanes

- **DNS** (`internal/platform/dnsread`): MX, TXT (SPF, DMARC at `_dmarc.`, DKIM at a set list of
  selectors), A, AAAA, CNAME and a reverse `PTR`. One query each `200ms`.
- **Certificate transparency** (`internal/platform/certlog`): one
  `GET https://crt.sh/?q=%25.<domain>&output=json` for each company, at one query each 5 seconds. The
  [crt.sh](https://crt.sh) service is free and runs on good will, and its team has asked users not to
  send many queries at once.
- **Site pages** (`internal/platform/webread` + `compose/sitetechnology.go`): the fingerprint of every
  page the site read fetched. It costs no extra request.

The subdomain classifier is an **allowlist**, and it runs before the cache write and the fact write. A
certificate host name that names a human, such as `lars.example.de`, reaches no table.

The model vocabulary in `compose/sitereadvocab.go` also writes `technology`, from what the company states
on its pages. It requires the passage to say the company itself uses the product.

## What starts it

The page lane runs inside the site read: when a crawl matches a company, it writes the technology facts
itself. The DNS and certificate lanes run as their own job, on their own `technical_lookup` queue, one job
at a time. A certificate log that waits 5 seconds between queries would otherwise hold a deep read worker.

These put the job in the queue:

1. **Every site read**, right after the page lane (`compose/technicalonsiteread.go`).
2. **The scheduled sweep** (`technical_enrich_backfill`). By default it runs every 6 hours. It takes up to
   25 companies in each workspace whose data is missing or older than 7 days. A mail provider changes at
   the company, and no write here shows the move. Only a scheduled pass sees it.
3. **`POST /companies/{id}/technical-enrich`**: `202` with no body. No screen calls it; it exists for
   scripts.

The sweep does not refresh `technology`; only a crawl can see what the pages declare.

The queue drops a copy of a job with the same args that waits or runs. A lookup a human starts runs
before a sweep job. The domain is not a job argument: the job reads the domain on the company record
when it runs. So a stale or foreign domain cannot ride in the args.

## Set it up

### `MARGINCE_CERTLOG_BASE_URL`: required, or the whole lane is off

The **worker** reads it when it starts; the API does not, because `cmd/worker/jobrunner.go` builds the
enricher. In development, put it in `.env.local`, which `scripts/dev.sh` reads and exports.

```sh
MARGINCE_CERTLOG_BASE_URL=public
```

`public` is a key word for `https://crt.sh`. To use your own certificate transparency service, give its
base URL instead. It must answer queries of the crt.sh shape (`/?q=%25.<domain>&output=json`).

Empty or unset turns the DNS and certificate lanes off. An enricher with only some lanes would leave
the missing lanes' facts frozen. On the record, that looks like a company that has not changed. An
installation that should make no outbound lookups leaves it unset.

When it is unset, the job kinds are never registered. Each enqueue then makes a job that retries against
a kind that is not there:

```
job kind is not registered in the client's Workers bundle: technical_enrich_company
```

Run `make dev` again after you change it. Vite reloads the app as you type, but not the Go worker.

### Set the time between refreshes

The admin setting **Technical lookup sweep (seconds)**, in **Settings → System health**, is 21600 (6 hours)
by default. `0` turns the sweep off and leaves the lookup to the site read. A running worker checks for
changes every minute.

## How a refresh changes the facts

Each lane that **completed** decides its own fields and removes the rows it no longer sees. A lane that
**failed** changes nothing, so an outage is never recorded as "this company runs no services". The lane
outcomes keep `empty`, `failed` and `refused` (`robots.txt` refused the page read) apart. A refresh never
removes a row a human wrote. Such a row's source is `human`, and the card splits rows by field, not by
source, so it stays on the card.

## The cache

The cache keys on query name and record type, for the whole installation. The DNS answer for a
domain is the same for every tenant. It records empty answers too.

| Kind | Trusted for |
|---|---|
| MX, TXT, DMARC | `24h` |
| Address (A/AAAA), CNAME | `12h` |
| DKIM, reverse (PTR) | 7 days |
| Certificate log | `24h` |

Every TTL is shorter than the 7-day refresh, or the cache would hide the move the sweep is there to catch.
The cache holds only classifier outcomes, after the allowlist ran.

## Check that it worked

**For each lane.** `GET /companies/{id}/technical-enrich/latest` reports the last run of each source, with
try counts and the time of the last good run. The `homepage` lane is the site page one, which the crawl
writes.

**In the job table.**

```sh
docker exec margince-postgres-1 psql -U margince_app -d margince \
  -c "select kind, state, attempt, left(coalesce(errors::text,''),200) \
      from river_job where kind like '%technical%' order by id desc limit 10;"
```

`state='retryable'` with the "not registered" error means `MARGINCE_CERTLOG_BASE_URL` is unset, or the
worker has not restarted since you set it.

**Without a database.** There is no command for this lane yet (`worker siteread <url>` covers only the deep
read). Use the table tests in `backend/internal/compose/techenrich_test.go` as the loop.

## Limits worth knowing

- **One provider for the certificate lane.** crt.sh is often slow and sometimes down. Callers read that as
  "nothing to say today", never as a trusted empty answer; the `certlog.Client` interface keeps it so.
- **The fingerprint rules are few** (`platform/techprofile/data/rules.json`), written by hand for small
  German companies.
- **No paid technology data sets**, and no Wappalyzer import. Those data sets and their forks are GPL,
  which cannot go with the BUSL-1.1 license of this code.

## See also

- [add-a-job.md](add-a-job.md): how the two job kinds and their queue are declared.
- [connect-an-mcp-client.md](connect-an-mcp-client.md): how to reach these facts as tools.
