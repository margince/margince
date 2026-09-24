# Contracts and invoices

Contracts and invoices both live on a company's own page in Margince: the
**Contracts** panel on the **Documents** tab records the agreements you have
signed, and the **Finance** tab mirrors what your accounting system has
invoiced.

Contracts are entered by hand, one by one, by a user. Invoices are never
entered in Margince at all — they come only from the accounting mirror.

## Contracts

A contract in Margince carries a title, a contract number, the company, and
optionally the deal and project it belongs to. Around that sit the money, the
dates and the paper.

### How do I create a contract?
To create a contract in Margince, open the company's **Documents** tab and click **Add contract** in the **Contracts** panel.
1. Open the company from **Companies** in the sidebar, choose **Documents**, then **Add contract**.
2. In **Record contract**, fill **Title** and **This value is** (both required); optionally **Value**, **Starts**, **Ends**, **Renews**, **Notice period (days)**, **Payment terms (days)** and **Signed**.
3. Drop the signed PDF on **Signed document** and click **Record contract**.
A new contract starts as **Draft**. Without the Create permission on contracts the button is not shown.
Also called: add an agreement, upload a contract.

### How do I make a contract active or change its status?
To change a contract's status in Margince, open the company's **Documents** tab and use the contract row's **Contract actions** menu → **Change status**.
1. Open **Contract actions** (the menu on the contract's row).
2. Choose **Change status**, pick the **New status** — Draft, Active, Expired or Canceled — and click **Change status**.
A contract that is already Expired, Canceled or Superseded offers no status change, because those are terminal. Superseded cannot be chosen at all; renewing sets it.
Also called: activate a contract, mark a contract expired.

### How do I renew a contract?
To renew a contract in Margince, open the company's **Documents** tab and choose **Contract actions** → **Renew** on the contract's row.
1. The **Renew contract** form opens: "Creates a new contract with its own terms and marks this one superseded. Only the counterparty is kept."
2. Pick the **Deal** that won this term, or **No deal**.
3. Fill the new term — **Title**, value, dates — as for a new contract.
4. Click **Renew**. The old contract becomes **Superseded** and the new one starts as **Draft**.
A contract that is already superseded cannot be renewed again.
Also called: extend a contract, contract renewal.

### How do I cancel a contract?
To cancel a contract in Margince, open the company's **Documents** tab and choose **Contract actions** → **Cancel contract** on the contract's row.
1. The **Record cancellation** form opens: "The customer stays under contract until the effective date. This records notice and does not change the status."
2. Fill **Notice given** and **Takes effect** (both required).
3. Click **Record cancellation**.
The status does not change. When the term is over, set it to **Canceled** with **Change status**.
Also called: terminate a contract, give notice.

### How do I edit or archive a contract?
To edit a contract in Margince, open the company's **Documents** tab and choose **Contract actions** → **Edit**; change the fields in **Edit contract** and click **Save changes**. To archive it, choose **Contract actions** → **Archive** and confirm "Archive this contract?".
"“{title}” leaves the lists and the company totals. The record and its history are kept; nothing is deleted."
Also called: correct a contract, remove a contract, delete a contract.

### The two value bases

A contract's value is either **the total for the whole term** or **12 months of
an open-ended contract**, and the record says which.

That distinction is load-bearing: **contract figures on different bases are
never summed, because thirty-six months plus twelve months is not forty-eight
months of anything.**

Where an annual figure is recorded, the contract form shows a **Monthly
equivalent** beside it. It is a reading, never something you type, and it says
when the division left a remainder rather than presenting an approximation as
exact.

### The dates

A contract carries four dates: starts, ends, renews, signed. An empty end date
means an open-ended contract, and the form says so.

Three carry rules worth knowing, and the form states each:

- **Notice period (days)** — "Notice period for a cancellation. The renewal
  warning fires before this deadline, not the renewal date."
- **Payment terms (days)** — "Days the customer has to pay. 0 means due on
  receipt; leave empty if no terms are agreed." Blank and zero are different
  answers.
- **Signed** — "Only when someone confirms the signature. Never taken from the
  deal’s close date."

### Status is never inferred from a date

A contract has five statuses: **Draft, Active, Expired, Canceled, Superseded.**

**No contract status is ever derived from the calendar.** Every move is asserted
by a human. A term whose end date has passed while nobody has said so reads
"Term ended, status pending" rather than quietly flipping itself to expired.

Separately, the record shows whether the company is **under contract** as of
today, worked out from the dates. That reading can disagree with the status, and
both are shown, because "the paperwork says active" and "the dates say we are
covered" are two different facts.

**Expired, Canceled and Superseded are terminal.** There is no way back out, and
Superseded cannot be chosen at all — it is what renewing does to the agreement it
replaces.

### What renewing changes
Renewing a contract creates the successor and supersedes the predecessor in one
step. **The predecessor's terms are never touched** — only its status and the
pointer to what replaced it. If you cannot open the company, the renewal still
works and says what it did instead: "The renewal keeps the same counterparty and
records no deal."

### What a cancellation records
Recording a cancellation stores the date notice was given and the date it takes
effect. Nothing else moves — the status changes later, when somebody says so.

Three refusals, each a rule rather than an error: "Enter both dates.";
"Cancellation cannot take effect before notice was given."; "Cancellation
cannot take effect after the term ends."

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
so the whole resource is closed to a credential. Every contract action is done
by a signed-in user.

## Finance and invoices

The **Finance** tab on a company mirrors invoices from your accounting system.
**The Finance card is read-only by construction** — there is no create or update
action anywhere in the product, not a permission you could grant. An accounting
customer never becomes a company record either; the mirror reads, and never
merges. The tab is absent while the company is a target, prospect or
opportunity: an account nobody has ever invoiced has no money to report.

### How do I create an invoice?
You cannot create an invoice in Margince. Margince has no invoice form, no "New invoice" button and no invoicing API; invoices exist only in your accounting system, and the company's **Finance** tab shows a read-only mirror of them.
To bill a customer, raise the invoice in your accounting system. Once that system is connected and the company is matched, the invoice appears under **Recent invoices** on the **Finance** tab after the next sync.
Also called: bill a customer, raise an invoice, send an invoice, credit note.

### What the Finance tab shows
- **Net invoiced · 12 months** — issued minus credited. It is called net
  invoiced and never "revenue", because they are not the same claim.
- **Open balance**, and how much of it is **Overdue**, with the overdue share.
- **Payment behavior** — "Typically {days} days after due", measured from the
  **due** date to settlement rather than from the invoice date, with a sparkline
  of days late per settled invoice, oldest first.
- **Recent invoices** — number, issued and due dates, amount, status.
- **Billing contacts**, read from your own records rather than from the
  accounting system.

The Finance card always names where the figures came from and when: "From
{provider} · synced {when}". For a former customer the card is titled "Finance ·
historical".

### Absent is not zero

**Every finance figure is withheld rather than zeroed when it cannot be
computed.** "€0 open" means the customer is square with you. No figure means you
do not know.

The same rule at a larger grain: one invoice with no conversion rate withholds
the whole total rather than reporting a sum that quietly omits it.

### The six states, kept apart

| State | What it means |
|---|---|
| **No connection** | "No accounting system connected." |
| **Unmapped** | "Connected, but this company is not yet matched to a customer in the accounting system." |
| **Syncing** | "Figures appear after the first sync." |
| **Connected** | Working |
| **Stale** | Connected, but the last sweep is old |
| **Error** | The sweep failed |

### Invoice statuses

Invoice statuses in Margince are **Draft, Open, Partially paid, Paid, Overdue,
Disputed, Credited, Void.**

**Overdue** is worked out against today by Margince rather than taken from the
accounting system, so it is current even when the last sweep is not.

### Honest state of the finance feature today

**The only finance source this build ships is an offline demo provider** —
plausible generated invoices, not a real accounting system. The card names the
provider, so you can always see which you are looking at.

There is a **Connect finance** button on the card, and it does nothing. Do not
plan around connecting a real ledger from here yet.
