# Check a company's VAT number

Margince can ask the EU VAT register whether a company's stated VAT ID is real, and keep the receipt.
This guide is **UI-first**: you drive it from the company record, with the equivalent `curl` shown
alongside for scripting and verification.

Two things make this worth doing rather than trusting the number on a page. A VAT ID copied from a
website's imprint is often somebody **else's** — a template reused, a subsidiary's number left in place —
and only the register's own answer exposes that. And a business treating a sale as intra-EU has to be
able to **show** it verified its counterpart; what a tax authority accepts is the *consultation number*
the register issues, tied to the number asked about and the day it was asked.

> **This is off unless an operator turns it on.** Without a register configured, a VAT number is stored
> exactly as stated and never verified. [Turn it on](#turn-the-register-on) is the first section for a
> reason: everything below does nothing until it is done.

## Where the UI lives

On a **company record**, in the right-hand **Details** panel, on the **Register / VAT ID** row.

- The number itself is an inline field — click it to type or correct one.
- Beside it sits a **shield mark** carrying the verdict. Green is valid, red is not valid, grey is
  either "not checked yet" or "the register did not answer".
- Clicking the shield opens the receipt: **Register answer**, **Number consulted**, **Registered to**,
  **Consulted on**, **Consultation number**, and the button that asks again.

The verdict is readable **without opening anything**. That is deliberate — whether a stated number holds
up is the fact you want at a glance, and a number validating to a company nobody recognises is the
finding this exists for.

## Turn the register on

The check needs a register to ask. Set it in `.env.local` at the repository root — the dev script sources
that file and both the api and the worker inherit it, so no value lands in a config file:

```
MARGINCE_VAT_CHECK_BASE_URL=public
MARGINCE_VAT_CHECK_REQUESTER=DE123456789
```

`public` is a shorthand, not a URL: it resolves to the European Commission's own VIES service. Point it
at a different base URL to use a proxy or a test double.

`MARGINCE_VAT_CHECK_REQUESTER` is **this installation's own VAT ID**, and it is separately optional. VIES
issues a consultation number only for a check made under a requester's number, so without it the check
still runs and still answers — it just comes back with no proof attached, and the card says
*"None issued."* If you intend to rely on these checks for filings, set it.

> **Restart after backend config.** The api is a compiled binary — changing an env var needs `make dev`
> again to take effect. Vite hot-reloads the SPA but not the Go api, so a stale api keeps answering
> happily and the feature breaks exactly like a bug in your own work.

Both flags are also plain command-line flags (`--vat-check-base-url`, `--vat-check-requester`) on the
worker, for a deployment that configures processes rather than environments.

## Give a company a VAT number

1. Open the company. In the right-hand **Details** panel, find **Register / VAT ID**.
2. Click **Add VAT ID** (or the existing number to correct it), type the number, press Enter.

Or:

```bash
curl -sS -b cookies.txt -X PATCH \
  "$BASE/v1/organizations/$ORG/profile-fields/register_vat" \
  -H 'content-type: application/json' \
  -d '{"value":"DE811907980"}'
```

**Writing the number is what queues the first check.** You do not have to ask separately: a number that
has just been stated has not been verified, so the consultation is queued in the same transaction as the
write. A site read that finds a number in a German *Impressum* does the same thing.

The number is normalised before it is consulted — case, spaces, dots, hyphens and slashes are dropped —
so `de 811 907 980` and `DE811907980` are the same ID.

## Read the answer

The shield beside the number carries it. Open it for the whole receipt.

| Verdict | What it means |
|---|---|
| **Valid** | The register recognises the number. **Read Registered to before you trust it** — see below. |
| **Not valid** | The register does not recognise it. A typo, a number that has been deregistered, or one that was never real. |
| **Register did not answer** | A fact about the **lookup**, never about the company. A member state's register was offline, or the number was not VAT-ID shaped so no request was made. |
| *(grey, "not checked yet")* | Nobody has asked. Distinct from having asked and been told no. |

**Valid does not mean "belongs to this company."** A VAT ID copied from another company's imprint — a
template reused, a subsidiary's number left in place — is a *real* number, so the register returns
**Valid**. What exposes it is **Registered to**: the name the register holds against that number. If it
is not the company you think you have, the number is somebody else's, whatever the verdict says.

That is why the name is shown beside the verdict rather than folded away, and it is the single most
useful line in the receipt.

**Consulted on** is the date the *register* reported, not the day this installation recorded it — a
receipt attests to when the register was asked.

## Ask again

Nothing re-asks on a schedule. A verdict going stale is not an event the product can observe, so the
automatic lanes consult only about a number they have not seen — which means a stored answer stands until
somebody asks for a fresh one.

Open the shield and press **Check again** (or **Check with the register**, on a company never consulted).

```bash
curl -sS -b cookies.txt -X POST "$BASE/v1/organizations/$ORG/vat-check"
# 202 Accepted, no body
```

The button then goes **busy**, saying *"Asking the register — the answer appears here once it replies."*
It is not pressable again while it is: the request is accepted in milliseconds and the register answers
seconds later, and a second press would either queue a duplicate consultation or meet the rate floor
below and refuse you over your own in-flight request.

The answer arrives on its own — it is written by a background worker, so the mark looks again a few
times over the following seconds rather than making you reload.

**The busy state lasts about fifteen seconds, not forever.** If the answer has not landed by then the
button frees up, because a register that never replies must not leave you with a control you cannot
press and no reason given. The consultation may still be running: the worker retries a service that
refused, and a register that told us when to come back is obeyed. So a verdict can change a minute after
the button came back — reopen the shield to see it, and the five-minute floor stops you spending a
second consultation in the meantime.

### Why a request can be refused

| Answer | What to do |
|---|---|
| **429** *"This number was checked in the last few minutes…"* | Wait. The answer on the record **is** that check. The floor is five minutes per company. |
| **404** *"This company states no VAT number yet."* | Add one in the Details panel; the write checks it automatically. |

There is deliberately **no refusal for an unconfigured register**, and this is the trap worth knowing.
The api role always accepts the request and queues the job; the register itself lives on the **worker**.
A worker with no register configured runs the job, records nothing — an absent check is not a failed one
— and reports success. So on an installation that was never [turned on](#turn-the-register-on), the
button works, the busy state runs its fifteen seconds, and no answer ever appears. Nothing on screen says
why, which is the first thing to suspect when a check produces silence.

The register is a shared public service consulted on one worker at roughly one request every two
seconds, and its terms describe it as intended for occasional verification rather than bulk lookup. The
five-minute floor and the human-only restriction on this endpoint are both there to keep an installation
from being blocked for everybody — **an agent cannot press this button.**

## What a number that changed looks like

The VAT field stays editable after a check, so a receipt can end up beside a number nobody consulted.
When that happens the panel says so: *"The number on this record has changed since this check. Ask the
register again to check the new one."* The old verdict is still shown, because it is still true — about
the old number.

## Troubleshooting

**The shield never appears.** The row draws it only once a number is stated. An empty VAT field has
nothing to verify.

**I pressed the button and nothing came back.** Almost always the register is not configured on the
**worker**. Nothing refuses the request in that case — see the note under
[Ask again](#ask-again) — so the only symptom is silence. Check the worker's own environment rather than
the api's, and remember `make dev` after changing it: the api and worker are compiled binaries.

Confirm what is on record with:

```bash
curl -sS -b cookies.txt "$BASE/v1/organizations/$ORG/vat-check"
```

`404` means never consulted; a body with `"status"` means an answer is on record. If the job ran and
still nothing landed, the worker had no register to ask:

```bash
# the job completed, and recorded nothing
psql "$DSN" -c "SELECT state, attempt FROM river_job
                 WHERE kind = 'check_organization_vat' ORDER BY id DESC LIMIT 3;"
```

**The answer is always "Register did not answer".** Check the number's shape. A VAT ID starts with a
two-letter country code — `122323235` is not one, and no request is made for a value that cannot be a VAT
ID. Recorded as unanswered rather than invalid, because the register said nothing.

**The shield shows a grey question mark I can press.** The check could not be **read** — a network or
server fault, not a fact about the company. Pressing it reads again.

## What is stored, and where

One row per company, replaced on each check: the number as consulted, the verdict, the consultation
number, the name and address the register holds, and both dates. The number as consulted is kept beside
the answer on purpose — a profile field edited afterwards must not silently inherit a receipt issued for
a different number.

The history is the audit log's. Every check writes an audit entry, and a check somebody **asked for**
writes its own entry naming who asked — the worker itself runs under a system principal, so without that
row "a person spent a consultation on this company" would be answerable from nothing.

## See also

- [configuration.md](../reference/configuration.md) — every env var and flag, in one table.
- [company-context.md](../explanation/company-context.md) — where a company's profile fields come from,
  including the site read that finds a VAT number in a German imprint in the first place.
