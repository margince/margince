# Contracts and invoices

Both live on a company's own page: **Contracts** records the agreements you have
signed, **Finance** mirrors what your accounting system has invoiced.

## Contracts

A contract carries a title, a contract number, the company, and optionally the
deal and project it belongs to. Around that sit the money, the dates and the
paper.

### The two value bases

A contract's value is either **the total for the whole term** or **twelve months
of an open-ended agreement**, and the record says which.

That distinction is load-bearing: **figures on different bases are never summed,
because thirty-six months plus twelve months is not forty-eight months of
anything.**

Where an annual figure is recorded, the form shows a **monthly equivalent**
beside it. It is a reading, never something you type, and it says when the
division left a remainder rather than presenting an approximation as exact.

### The dates

Starts, ends, renews, signed. An empty end date means an open-ended agreement,
and the form says so.

Two carry rules worth knowing, and the form states each:

- **Notice period** — "How much notice a cancellation needs. The renewal warning
  fires before this deadline, not before the renewal date."
- **Payment terms** — "0 means due on receipt; leave blank if nobody has agreed
  terms." Blank and zero are different answers.
- **Signed** — "Only when a human knows it was signed — never taken from a
  deal's close date."

### Status is never inferred from a date

Five statuses: **Draft, Active, Expired, Cancelled, Superseded.**

**No status is ever derived from the calendar.** Every move is asserted by a
human. A term whose end date has passed while nobody has said so reads "Term
ended — status change pending" rather than quietly flipping itself to expired.

Separately, the record shows whether the company is **under contract** as of
today, worked out from the dates. That reading can disagree with the status, and
both are shown, because "the paperwork says active" and "the dates say we are
covered" are two different facts.

**Expired, Cancelled and Superseded are terminal.** There is no way back out, and
Superseded cannot be chosen at all — it is what renewing does to the agreement it
replaces.

### Renewing

"Creates a new agreement and marks this one superseded. Its own terms — nothing
carries over but the counterparty."

The successor and the supersede land in one step, and **the predecessor is never
edited**. If you cannot open the company, the renewal still works and says what
it did instead: it keeps the counterparty and records no deal.

### Recording a cancellation

"The customer stays under contract until the effective date — this records
notice, not a state change."

You give the date notice was given and the date it takes effect. Nothing else
moves — the status changes later, when somebody says so.

Three refusals, each a rule rather than an error: both dates are needed,
cancellation cannot take effect before notice was given, and it cannot take
effect after the term already ends.

### Archiving

"It leaves the lists and the account totals. The record and its history stay, so
what was true stays answerable — nothing is deleted."

### What makes a contract count as a signed win

[The pipeline](the-pipeline.md) closes a deal without asking how it was won when
the deal has a signed contract. The bar is stricter than it sounds, and all of
it must hold:

- the contract is not archived, and is past draft;
- it carries a **signed date**;
- it has a live attachment filed under **Contract** or **Legal**, in the
  **Current** or **Final** state.

A contract record with no paper on it does not clear the bar.

Superseded, expired and cancelled contracts all still count. A deal won in March
is not un-won because the agreement later ended.

One thing this is not: **proof of signature.** Margince has no e-signature. This
is a record-keeping gate, and the audit trail is what separates an honest entry
from a careless one.

### Contracts and agents

**An agent cannot touch a contract at all — not even to read one.** Contract
values and dates are exactly the figures a confident wrong answer damages most,
so the whole resource is closed to a credential.

## Finance

The Finance card mirrors invoices from your accounting system. **It is read-only
by construction** — there is no create or update action anywhere in the product,
not a permission you could grant. An accounting customer never becomes a company
record either; the mirror reads, and never merges.

### What it shows

- **Net invoiced · 12 months** — issued minus credited. It is called net
  invoiced and never "revenue", because they are not the same claim.
- **Open balance**, and how much of it is **overdue**, with the overdue share.
- **Payment behaviour** — "Typically {days} days after due", measured from the
  **due** date to settlement rather than from the invoice date, with a sparkline
  of days-late per settled invoice, oldest first.
- **Recent invoices** — number, issued and due dates, amount, status.
- **Billing contacts**, read from your own records rather than from the
  accounting system.

The card always names where the figures came from and when: "From {provider} ·
synced {when}".

### Absent is not zero

**Every figure is withheld rather than zeroed when it cannot be computed.** "€0
open" means the customer is square with you. No figure means you do not know.

The same rule at a larger grain: one invoice with no conversion rate withholds
the whole total rather than reporting a sum that quietly omits it.

### The six states, kept apart

| State | What it means |
|---|---|
| **No connection** | Nothing is wired up |
| **Unmapped** | Connected, but this company is not matched to a customer yet |
| **Syncing** | "Figures appear once the first sweep lands" |
| **Connected** | Working |
| **Stale** | Connected, but the last sweep is old |
| **Error** | The sweep failed |

### Invoice statuses

**Draft, Open, Part paid, Paid, Overdue, Disputed, Credited, Void.**

**Overdue** is worked out against today by Margince rather than taken from the
accounting system, so it is current even when the last sweep is not.

### Honest state of this feature today

**The only source this build ships is an offline demo provider** — plausible
generated invoices, not a real accounting system. The card names the provider, so
you can always see which you are looking at.

There is a **Connect finance** button on the card, and it does nothing. Do not
plan around connecting a real ledger from here yet.
