<!-- prose:plain -->
# Contracts and invoices

Contracts and invoices both live on a company's own page in Margince. The
**Contracts** panel on the **Documents** tab records the agreements you have
signed. The **Finance** tab mirrors what your accounting system has invoiced.

Contracts are entered by hand, one by one, by a user. Invoices are never
entered in Margince at all: they come only from your accounting system.

## Contracts

A contract in Margince carries a title, a contract number and the company. It
can also carry the deal and project it belongs to. Around that are the money, the
dates and the paper.

### How do I create a contract?
To create a contract in Margince, or upload a customer's signed contract document, open the company's **Documents** tab. Click **Add contract** in the **Contracts** panel, and drop the signed file on **Signed document**.
1. Open the company, choose **Documents**, then **Add contract**.
2. In **Record contract**, fill **Title** and **This value is** (both required).
   You may also fill **Value**, **Starts**, **Ends**, **Renews**, **Notice period (days)**, **Payment terms (days)** and **Signed**.
3. Click **Record contract**.
A new contract starts as **Draft**; without the Create permission there is no button.
Also called: add an agreement, upload a signed contract.

### How do I make a contract active or change its status?
To change a contract's status in Margince, open the company's **Documents** tab and use the contract row's **Contract actions** menu → **Change status**.
1. Open **Contract actions** (the menu on the contract's row).
2. Choose **Change status**, pick the **New status** (Draft, Active, Expired or Canceled), and click **Change status**.
A contract that is already Expired, Canceled or Superseded offers no status change, because those are final. Superseded cannot be chosen at all; renewing sets it.
Also called: activate a contract, mark a contract expired.

### How do I renew a contract?
To renew a contract in Margince, open the company's **Documents** tab and choose **Contract actions** → **Renew** on the contract's row.
1. The **Renew contract** form opens: "Creates a new contract with its own terms and marks this one superseded. Only the counterparty is kept."
2. Pick the **Deal** that won this term, or **No deal**.
3. Fill the new term (**Title**, value, dates) as for a new contract.
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
To edit a contract in Margince, open the company's **Documents** tab and choose **Contract actions** → **Edit**. Change the fields in **Edit contract** and click **Save changes**. To archive it, choose **Contract actions** → **Archive** and confirm "Archive this contract?".
"“{title}” leaves the lists and the company totals. The record and its history are kept; nothing is deleted."
Also called: correct a contract, remove a contract, delete a contract.

### The two value bases

A contract's value is either **the total for the whole term** or **12 months of
a contract with no end date**, and the record says which.

Contract values on different bases are **never added together**, because 36
months plus 12 months is not 48 months of anything.

Where a yearly figure is recorded, the contract form shows a **Monthly
equivalent** beside it. Margince works it out; you never type it. When the
monthly figure has to be rounded, it says so.

### The dates

A contract carries four dates: starts, ends, renews, signed. An empty end date
means a contract with no end date, and the form says so.

The form explains these fields:

- **Notice period (days)**: "Notice period for a cancellation. The renewal
  warning fires before this deadline, not the renewal date."
- **Payment terms (days)**: "Days the customer has to pay. 0 means due on
  receipt; leave empty if no terms are agreed." Blank and zero are different
  answers.
- **Signed**: "Only when someone confirms the signature. Never taken from the
  deal’s close date."

### Status is never guessed from a date

A contract has five statuses: **Draft, Active, Expired, Canceled, Superseded.**

**A contract's status never changes by date.** A human makes every change. A
term whose end date has passed while nobody has changed the status reads "Term
ended, status pending". It does not switch to Expired by itself.

On its own, the record shows whether the company is **under contract** as of
today, worked out from the dates. That reading can disagree with the status, and
both are shown. "The papers say active" and "the dates say we are covered" are
two different facts.

**Expired, Canceled and Superseded are final.** There is no way back out.
Superseded cannot be chosen at all: renewing sets it on the agreement it
replaces.

### What renewing changes
Renewing a contract creates the new contract and supersedes the old one in one
step. **The old contract keeps its terms.** Only its status changes to
Superseded, and it shows which contract replaced it. You may not be able to open the
company. Then the renewal still works and says what it did instead: "The renewal
keeps the same counterparty and records no deal."

### What a cancellation records
Recording a cancellation stores the date notice was given and the date it takes
effect. Nothing else changes; the status changes later, when somebody sets it.

The form refuses three cases: "Enter both dates.";
"Cancellation cannot take effect before notice was given."; "Cancellation
cannot take effect after the term ends."

### What makes a contract count as a signed win
[The pipeline](the-pipeline.md) closes a deal without asking how it was won when
the deal has a signed contract. There is more to it than first appears, and all
of it must hold:

- the contract is not archived, and is past draft;
- it carries a **signed date**;
- it has a live attachment filed under **Contract** or **Legal**, in the
  **Current** or **Final** state.

A contract record with no paper on it does not clear the bar.

Superseded, expired and cancelled contracts all still count. A deal won last year
stays won when the agreement later ends.

Margince offers no signing of its own, so this check does not show that anyone signed. It
checks the records, and the audit trail shows who entered what.

### Contracts and agents

**An agent cannot read or change a contract.** A wrong answer about contract
values and dates, given as if it were true, costs the most. So agents have no
access to contracts. Every contract action is done by a signed-in user.

## Finance and invoices

The **Finance** tab on a company mirrors invoices from your accounting system.
**The Finance tab is read-only.** Nothing in Margince creates or edits an
invoice, and no permission can allow it. A customer in your accounting system
never becomes a company record either; Margince only reads from it. The tab is
missing while the company is a target, prospect or opportunity: an account
nobody has ever invoiced has no money to report.

### How do I create an invoice?
You cannot create an invoice in Margince: it has no invoice form, no "New invoice" button and no invoicing API. Invoices exist only in your accounting system, and the company's **Finance** tab shows a read-only mirror of them.
To bill a customer, raise the invoice in your accounting system. Margince cannot connect a real accounting system yet;
the only finance source it ships is a demo provider that works offline. So **Recent invoices** on the **Finance** tab shows only its made-up demo invoices, never yours.
Also called: bill a customer, raise an invoice, send an invoice, credit note.

### How do I connect my accounting system?
You cannot connect an accounting system from Margince yet. The **Finance** tab has no connect button, and **Settings** has no finance connector. The only finance source Margince ships is a demo provider that works offline, set up outside the app.
Until one is connected, the **Finance** card reads "No accounting system connected."
Also called: connect finance, link accounting, sync invoices, connect a ledger, bookkeeping integration.

### What the Finance tab shows
- **Net invoiced · 12 months**: issued, with credits taken off. It is called net
  invoiced and never "revenue", because the two are different figures.
- **Open balance**, and how much of it is **Overdue**, with the overdue share.
- **Payment behavior**: "Typically {days} days after due". It is measured from
  the **due** date to payment rather than from the invoice date. A small line
  chart shows the days late per paid invoice, oldest first.
- **Recent invoices**: number, issued and due dates, amount, status.
- **Billing contacts**, read from your own records rather than from the
  accounting system.

Once a source is connected, the Finance card names where the figures came from
and when. It reads "From {provider} · synced {when}", or "From {provider} · not
yet synced" before its first sync. With no source connected it names none. For a
past customer the card is titled "Finance · historical".

### A blank is not zero

A figure that cannot be worked out is **left blank**. It is never shown as
zero. "€0 open" means every invoice is paid. No figure means you
do not know.

The same goes for totals. If one invoice has no rate to change its currency,
the whole total is left blank instead of leaving that invoice out.

### Connection states

| State | What it means |
|---|---|
| **No connection** | "No accounting system connected." |
| **Unmapped** | "Connected, but this company is not yet matched to a customer in the accounting system." |
| **Syncing** | "Figures appear after the first sync." |
| **Connected** | Working |
| **Stale** | Connected, but the last sync is old |
| **Error** | The sync failed |

### Invoice statuses

Invoice statuses in Margince are **Draft, Open, Partially paid, Paid, Overdue,
Disputed, Credited, Void.**

Margince works out **Overdue** against today rather than taking it from the
accounting system. So it is current even when the last sync is not.
