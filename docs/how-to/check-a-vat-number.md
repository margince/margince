<!-- prose:plain -->
# Check a company's VAT number

Margince can ask the EU VAT register whether the VAT ID a company gives is real, and keep the receipt.
This guide starts **from the screen**: you work from the company record. The same step as a `curl`
command sits next to it, for scripts and for checks.

A business that counts a sale as a sale inside the EU must **show** that it checked the
other party. A tax office accepts the *consultation number* the register gives. That number goes with
the VAT number you asked about, and to the day you asked.

> **Off by default.** An operator turns it on. With no register in the config, Margince stores a VAT
> number as typed and never checks it. Nothing below works until you
> [turn it on](#turn-the-register-on).

## Where the screen is

On a **company record**, in the **Details** panel on the right, on the **Register / VAT ID** row.

- The number itself is a field you edit in place: click it to type a number or to correct one.
- Next to it is a **shield mark** that shows the verdict. Green is valid, red is not valid. Grey is either
  "not checked yet" or "no answer from the register".
- A click on the shield opens the receipt: **Register answer**, **Number consulted**, **Registered to**,
  **Consulted on**, **Consultation number**, and the button that asks again.

You see the verdict without a click.

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
the website reader until someone confirms it. VIES gives a consultation number only for a check
run under the number of the one who asks.

Without that number the check still runs and still answers, but with no proof, and the receipt says
*"None issued."* If you need these checks for tax reports, confirm the VAT ID.

> Start again with `make dev` after you change the value.

On a deployment that sets up programs with flags, not with environment values, it is also a
flag. Set `--vat-check-base-url` on **both** the API and the worker. The worker sends the request, and
the API decides whether to queue one at all. So if you set it on one role only, the two disagree.

## Give a company a VAT number

1. Open the company. In the **Details** panel on the right, find **Register / VAT ID**.
2. Click **Add VAT ID**, or click the number to correct it. Type the number, and press Enter.

Or:

```bash
curl -sS -b cookies.txt -X PATCH \
  "$BASE/v1/companies/$COMPANY/profile-fields/register_vat" \
  -H 'content-type: application/json' \
  -d '{"value":"DE811907980"}'
```

**To write the number queues the first check.** You do not have to ask for it. A new number is not
checked yet, so Margince queues the check in the same transaction as the write. A
site read that finds a number in a German *Impressum* does the same thing.

Before the check, Margince drops case from the number, and drops each space, `.`, `-` and `/`. So
`de 811 907 980` and `DE811907980` are the same ID.

## Read the answer

The shield next to the number shows the answer. Open it for the whole receipt.

| Verdict | What it means |
|---|---|
| **Valid** | The register knows the number. **Read Registered to before you trust it** (see below). |
| **Not valid** | The number does not hold up. It may be a typing error, a value that does not have the shape of a VAT ID, a number no longer on the register, or one that was never real. |
| *(grey, "not checked yet")* | No one has asked. This is not the same as an answer of no. |

**Valid does not mean "belongs to this company."** A VAT ID copied from the imprint of a website often
belongs to someone else. It may come from a page copied from another site, or be the number of a company it
owns that was kept in place. It is a *real* number, so the register says **Valid**.

**Registered to** shows the problem: it is the name the register holds for that number. If it is not
the company you think it is, the number belongs to someone else, whatever the verdict says. That is why
the receipt shows the name next to the verdict.

**Consulted on** is the date the *register* reported. It can be different from the day this installation
recorded it. A receipt proves when the register was asked.

## Ask again

Nothing asks again on a schedule. The product cannot see when a verdict goes stale. So the parts that run
on their own ask only about a number they do not know yet. A stored answer stands until someone asks for a
new one.

Open the shield and press **Check again**, or **Check with the register** on a company never checked.

```bash
curl -sS -b cookies.txt -X POST "$BASE/v1/companies/$COMPANY/vat-check"
# 202 Accepted, no body
```

The button then shows that it is **busy**, and says *"Asking the register — the answer appears here
once it replies."* You cannot press it again while it is busy. Margince accepts the request in
less than a second, and the register answers seconds later. So a second press would either queue a second
check, or meet the rate floor below and refuse you because of your own open request.

A worker writes the answer. So the mark looks again a few times over the next seconds, and you do not
need to open the page again.

**The busy state lasts about 15 seconds.** If the answer has not come by then, the button is free
again. So a register that never replies does not leave you with a button you cannot press.

The check may still be running. The worker tries again when the service refused, and it waits when the
register says when to come back. So a verdict can change a minute after the button is free. Open the
shield again to see it. The five-minute floor stops you from spending a second check in that time.

### Why a request can be refused

| Answer | What to do |
|---|---|
| **429** *"This number was checked in the last few minutes…"* | Wait. The answer on the record **is** that check. The floor is five minutes for each company. |
| **404** *"This company states no VAT number yet."* | Add one in the Details panel; the write checks it on its own. |

**A register with no config is never refused**, and nothing tells you so. The API role always accepts the
request and queues the job, but the register itself lives on the **worker**. A worker with no register
in its config runs the job, records nothing, and reports that it worked. A check that is missing is not
a check that failed.

On an installation that was never [turned on](#turn-the-register-on), the button works, the busy state
runs its 15 seconds, and no answer appears. Nothing on the screen says why. So when a check
gives silence, look at this first.

The register is a shared public service. Margince asks it from one worker, at about one request every two
seconds. Its rules say it is for a check now and then, not for many checks at once. Two things keep an
installation from being blocked for every user: the five-minute floor, and the rule that only a human
can use this endpoint. **An agent cannot press this button.**

## What a number that changed looks like

You can still edit the VAT field after a check, so a receipt can end up next to a number no one checked.
When that happens, the panel says so: *"The number on this record has changed since this check. Ask the
register again to check the new one."* It still shows the old verdict, because that verdict is still
right about the old number.

## When it does not work

**The shield never appears.** The row shows it only once the record has a number. An empty VAT field has
nothing to check.

**I pressed the button and nothing happened.** Most of the time, the register has no config on the
**worker**. Nothing refuses the request in that case (see the note under [Ask again](#ask-again)), so the
only sign is silence. Check the environment of the worker, not the one the API reads. Run `make dev`
after you change it, because the API and the worker are built programs.

See what is on record with:

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

**A real number reads Not valid.** Check its shape first. A VAT ID starts with a country code of two
letters. `122323235sdf` has none, so Margince sends no request, and the answer is **Not valid** without a
question to the register. Then check **Registered to**. A real number that belongs to someone else
answers **Valid**, and only the name shows the problem.

**A grey question mark I can press.** Margince could not **read** the check: there was a network or
server error, which says nothing about the company. A press reads it again.

## What is stored, and where

There is one row for each company, and each check replaces it. It holds the number as checked, the
verdict, the consultation number, the name and address the register holds, and both dates. The number as
checked sits next to the answer. So a profile field you edit later cannot take over a receipt that was
issued for a different number.

The history lives in the audit log. Every check writes an audit entry. A check that someone **asked for**
writes its own entry, which names who asked. The worker runs as the system, so without that entry
nothing would record which user used a check on this company.

## See also

- [configuration.md](../reference/configuration.md): every environment value and flag, in one table.
- [company-context.md](../explanation/company-context.md): where the profile fields of a company come
  from, with the site read that finds a VAT number in a German imprint.
