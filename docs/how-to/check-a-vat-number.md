<!-- prose:plain -->
# Set up VAT number checks

The handbook page [records.md](../handbook/records.md) covers what a user does on the company. There, a
user types the **Register / VAT ID**, reads the receipt, and presses **Check again**. This page is for the
operator. It covers turning the register on, what a check does on the server, and what to do when no answer comes.

Margince can ask the EU VAT register whether the VAT ID a company gives is real, and keep the receipt. A
business that counts a sale as a sale inside the EU must **show** that it checked the other party. A tax
office accepts the *consultation number* the register gives, for the number asked about, on that day.

> **Off by default.** With no register in the config, Margince stores a VAT number as typed and never
> checks it.

## Turn the register on

The check needs a register to ask. Set it in `.env.local` at the root of the repository. The dev script
reads that file, and both the API and the worker get it from there, so no value goes into a config file:

```
MARGINCE_VAT_CHECK_BASE_URL=public
```

`public` is a short name for VIES, the service of the European Commission. Point it at a different URL
to use a proxy or a test service.

Each check runs under this installation's own VAT ID. That is the **Register / VAT ID** on
Settings → Company profile, once someone has typed or confirmed it. Margince does not use a number from
the website reader until someone confirms it. VIES gives a consultation number only for a check run
under the number of the one who asks. Without it, the check still answers, but the receipt says
*"None issued."*

> Start again with `make dev` after you change the value.

On a deployment that sets up programs with flags, not with environment values, it is also a flag. Set
`--vat-check-base-url` on **both** the API and the worker. The API decides whether to queue a check at
all, and the worker sends the request. So if you set it on one role only, the two disagree.

## What a check does

**Writing the number queues the first check**, in the same transaction as the write. A site read that
finds a number in a German *Impressum* does the same. Before the check, Margince drops case, spaces,
`.`, `-` and `/`, so `de 811 907 980` and `DE811907980` are the same ID. A number with no two-letter
country code is answered **Not valid** without a request.

Nothing asks again on a schedule: the product cannot see when a verdict goes stale. So the parts that
run on their own ask only about a number they do not know yet. A user asks again from the receipt, or
with a script:

```bash
curl -sS -b cookies.txt -X POST "$BASE/v1/companies/$COMPANY/vat-check"
# 202 Accepted, no body
```

A worker writes the answer, a few seconds later. The worker tries again when the service refused, and
waits when the register says when to come back. So a verdict can change a minute after the screen
stops waiting (about 15 seconds).

| Answer | Why |
|---|---|
| **429** *"This number was checked in the last few minutes…"* | The five-minute floor for each company. The answer on the record **is** that check. |
| **404** *"This company states no VAT number yet…"* | No number to check. Writing one checks it. |
| **404** *"This installation does not consult the VAT register…"* | The API role has no register in its config, so it queues nothing. |

The register is a shared public service. Margince asks it from one worker, at about one request every
two seconds, and its rules say it is for a check now and then. The five-minute floor, and the rule that
only a human can use this endpoint, keep an installation from being blocked for every user. **An agent
cannot ask.**

## When a check gets no answer

The API refuses when **it** has no register. But say the API has one and the **worker** does not. Then
the API queues the job, and the worker runs it, records nothing, and reports that it worked. The screen
waits, and no answer appears. A check that is missing is not a check that failed. So look at the
environment of the worker first, then run `make dev`.

See what is on record:

```bash
curl -sS -b cookies.txt "$BASE/v1/companies/$COMPANY/vat-check"
```

`404` means never checked. A body with `"status"` means an answer is on record. If the job has run and
nothing is there, the worker has no register to ask:

```bash
# the job completed, and recorded nothing
psql "$DSN" -c "SELECT state, attempt FROM river_job
                 WHERE kind = 'check_company_vat' ORDER BY id DESC LIMIT 3;"
```

A grey question mark the user can press means Margince could not **read** the stored check. That is a
network or server error, and it says nothing about the company.

## What is stored, and where

There is one row for each company, and each check replaces it. It holds the number as checked, the
verdict, the consultation number, the name and address the register holds, and both dates. The number
as checked sits next to the answer, so a profile field edited later cannot take over a receipt issued
for a different number. **Consulted on** is the date the *register* reported, which can differ from the
day this installation recorded it.

The history lives in the audit log. Every check writes an audit entry. A check that someone **asked
for** writes its own entry, which names who asked. The worker runs as the system, so without that
entry nothing would record which user used a check on this company.

## See also

- [configuration.md](../reference/configuration.md): every environment value and flag, in one table.
- [company-context.md](../explanation/company-context.md): where the profile fields of a company come
  from, with the site read that finds a VAT number in a German imprint.
