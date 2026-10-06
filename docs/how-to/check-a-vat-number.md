# Check a company's VAT number

Margince can ask the EU VAT register whether a company's stated VAT ID is real, and keep the receipt.
This guide is **UI-first**: you drive it from the company record, with the equivalent `curl` shown
alongside for scripting and verification.

A business treating a sale as intra-EU has to be able to **show** it verified its counterpart. A tax
authority accepts the *consultation number* the register issues, tied to the number asked about and the
day it was asked.

> **Off by default.** An operator turns it on. Without a register configured, a VAT number is stored
> as stated and never verified. Everything below does nothing until you
> [turn it on](#turn-the-register-on).

## Where the UI lives

On a **company record**, in the right-hand **Details** panel, on the **Register / VAT ID** row.

- The number itself is an inline field: click it to type or correct one.
- Beside it sits a **shield mark** carrying the verdict. Green is valid, red is not valid, grey is
  either "not checked yet" or "the register did not answer".
- Clicking the shield opens the receipt: **Register answer**, **Number consulted**, **Registered to**,
  **Consulted on**, **Consultation number**, and the button that asks again.

The verdict shows without opening anything.

## Turn the register on

The check needs a register to ask. Set it in `.env.local` at the repository root. The dev script sources
that file and both the api and the worker inherit it, so no value lands in a config file:

```
MARGINCE_VAT_CHECK_BASE_URL=public
```

`public` is a shorthand that resolves to the European Commission's own VIES service. Point it at a
different base URL to use a proxy or a test double.

Each check is made under this installation's own VAT ID: the **Register / VAT ID** on Settings → Company
profile, once someone has entered or confirmed it. A number the website reader proposed is not used until
someone confirms it. VIES issues a consultation number only for a check made under a requester's number.
Without one the check still runs and still answers, but with no proof attached, and the card says *"None
issued."* If you rely on these checks for filings, confirm the VAT ID.

> Restart with `make dev` after changing the variable.

It is also a plain command-line flag on a deployment that configures processes instead of
environments. Set `--vat-check-base-url` on **both** the api and the worker: the worker makes the request,
and the api decides whether to queue one at all, so setting it on one role only makes the two disagree.

## Give a company a VAT number

1. Open the company. In the right-hand **Details** panel, find **Register / VAT ID**.
2. Click **Add VAT ID** (or the existing number to correct it), type the number, press Enter.

Or:

```bash
curl -sS -b cookies.txt -X PATCH \
  "$BASE/v1/companies/$COMPANY/profile-fields/register_vat" \
  -H 'content-type: application/json' \
  -d '{"value":"DE811907980"}'
```

**Writing the number queues the first check.** You do not have to ask separately: a number that has just
been stated has not been verified, so the consultation is queued in the same transaction as the write. A
site read that finds a number in a German *Impressum* does the same thing.

The number is normalised before it is consulted (case, spaces, dots, hyphens and slashes are dropped), so
`de 811 907 980` and `DE811907980` are the same ID.

## Read the answer

The shield beside the number carries it. Open it for the whole receipt.

| Verdict | What it means |
|---|---|
| **Valid** | The register recognises the number. **Read Registered to before you trust it** (see below). |
| **Not valid** | The number does not hold up. A typo, a value that is not VAT-ID shaped at all, a number since deregistered, or one that was never real. |
| *(grey, "not checked yet")* | Nobody has asked. Distinct from having asked and been told no. |

**Valid does not mean "belongs to this company."** A VAT ID copied from a website's imprint is often
somebody else's (a template reused, a subsidiary's number left in place). It is a *real* number, so the
register returns **Valid**. **Registered to** exposes it: the name the register holds against that
number. If it is not the company you think you have, the number is somebody else's, whatever the verdict
says. That is why the receipt shows the name beside the verdict.

**Consulted on** is the date the *register* reported, which can differ from the day this installation
recorded it. A receipt attests to when the register was asked.

## Ask again

Nothing re-asks on a schedule. The product cannot observe a verdict going stale, so the automatic lanes
consult only about a number they have not seen, and a stored answer stands until somebody asks for a
fresh one.

Open the shield and press **Check again** (or **Check with the register**, on a company never consulted).

```bash
curl -sS -b cookies.txt -X POST "$BASE/v1/companies/$COMPANY/vat-check"
# 202 Accepted, no body
```

The button then goes **busy**, saying *"Asking the register — the answer appears here once it replies."*
You cannot press it again while it is busy. The request is accepted in milliseconds and the register
answers seconds later, so a second press would either queue a duplicate consultation or meet the rate
floor below and refuse you over your own in-flight request.

A background worker writes the answer, so the mark looks again a few times over the following seconds
and you do not need to reload.

**The busy state lasts about fifteen seconds.** If the answer has not landed by then, the button frees up,
so a register that never replies does not leave you with a control you cannot press. The consultation may
still be running: the worker retries a service that refused, and obeys a register that said when to come
back. A verdict can therefore change a minute after the button came back. Reopen the shield to see it;
the five-minute floor stops you spending a second consultation in the meantime.

### Why a request can be refused

| Answer | What to do |
|---|---|
| **429** *"This number was checked in the last few minutes…"* | Wait. The answer on the record **is** that check. The floor is five minutes per company. |
| **404** *"This company states no VAT number yet."* | Add one in the Details panel; the write checks it automatically. |

**An unconfigured register is never refused**, and that is the trap. The api role always accepts the
request and queues the job; the register itself lives on the **worker**. A worker with no register
configured runs the job, records nothing (an absent check is not a failed one), and reports success. On
an installation that was never [turned on](#turn-the-register-on), the button works, the busy state runs
its fifteen seconds, and no answer ever appears. Nothing on screen says why, so suspect this first when a
check produces silence.

The register is a shared public service consulted on one worker at roughly one request every two
seconds, and its terms describe it as intended for occasional verification instead of bulk lookup. The
five-minute floor and the human-only restriction on this endpoint keep an installation from being blocked
for everybody: **an agent cannot press this button.**

## What a number that changed looks like

The VAT field stays editable after a check, so a receipt can end up beside a number nobody consulted.
When that happens the panel says so: *"The number on this record has changed since this check. Ask the
register again to check the new one."* The old verdict is still shown, because it is still true about the
old number.

## Troubleshooting

**The shield never appears.** The row draws it only once a number is stated. An empty VAT field has
nothing to verify.

**I pressed the button and nothing came back.** Almost always the register is not configured on the
**worker**. Nothing refuses the request in that case (see the note under [Ask again](#ask-again)), so the
only symptom is silence. Check the worker's own environment instead of the api's, and run `make dev`
after changing it: the api and worker are compiled binaries.

Confirm what is on record with:

```bash
curl -sS -b cookies.txt "$BASE/v1/companies/$COMPANY/vat-check"
```

`404` means never consulted; a body with `"status"` means an answer is on record. If the job ran and
still nothing landed, the worker had no register to ask:

```bash
# the job completed, and recorded nothing
psql "$DSN" -c "SELECT state, attempt FROM river_job
                 WHERE kind = 'check_company_vat' ORDER BY id DESC LIMIT 3;"
```

**A real number reads Not valid.** Check its shape first. A VAT ID starts with a two-letter
country code. `122323235sdf` has none, so no request is made and the answer is **Not valid** without the
register ever being asked. Then check **Registered to**: a real number belonging to somebody else answers
**Valid**, and only the name exposes it.

**A grey question mark I can press.** The check could not be **read**: a network or
server fault, which says nothing about the company. Pressing it reads again.

## What is stored, and where

One row per company, replaced on each check: the number as consulted, the verdict, the consultation
number, the name and address the register holds, and both dates. The number as consulted is kept beside
the answer so a profile field edited afterwards cannot inherit a receipt issued for a different number.

The history is the audit log's. Every check writes an audit entry, and a check somebody **asked for**
writes its own entry naming who asked. The worker runs under a system principal, so without that entry
nothing would record which user spent a consultation on this company.

## See also

- [configuration.md](../reference/configuration.md): every env var and flag, in one table.
- [company-context.md](../explanation/company-context.md): where a company's profile fields come from,
  including the site read that finds a VAT number in a German imprint in the first place.
