<!-- prose:plain -->
# Partner programs

## In short

Some deals come to you because someone else brought them. It may be an agency
that recommended you, a consultancy that sells you into its clients, or a
hosting company whose customers need your software.

A partner program is how you keep track of those partners and pay them. In
Margince you can:

- Mark a company as a partner and record what kind of partner they are.
- Agree a share: 15%, 20% or 25% of the deals they bring you.
- Tag a deal with the partner who brought it, and say what they did: brought
  it to you, or helped with one you already had.
- See what a partner is owed. Win a deal they brought and Margince works out
  their share by itself and adds it to their ledger.
- Answer "what have we earned?", the question every partner eventually asks,
  from one screen on their company page.

Partners cannot log in to see their own numbers; see *What this cannot do*
below.

Margince does not pay anyone. You settle a partner in whatever system you pay
partners from; Margince holds the record of what was earned, agreed and
settled. Marking an entry *Paid* here says your finance system already paid it.

Below, one deal is followed from start to finish. For the setup form field by
field (every role, every status, every relationship stage), see
[handbook/partners.md](../docs/handbook/partners.md).

## The one thing to get right

An ordinary deal has one company in it: the one buying.

A partner deal has two:

- the **customer**, who is buying, and
- the **partner**, who brought it to you and gets a share.

They are different companies. If you find yourself putting the same company in
both, stop: that is a company buying for itself, and no one is owed anything.

Margince keeps the two apart on every screen. A company's **Deals** tab shows
what *it* is buying. Its **Partner** tab shows what it has *brought you*.

## 1. Make a company a partner

Open the company, go to its **Partner** tab, and choose **Make this a
partner**.

Two fields matter to begin with:

**Partner role**: what sort of partner they are. *Hosting* if they run the
software for their clients, *Consulting* if they advise clients and bring you
in, *Strategic* for anything broader.

**Margin tier**: their share of the deals they bring:

| | |
|---|---|
| **Intro (15%)** | they make the introduction and hand it over |
| **Active collaboration (20%)** | they work the deal alongside you |
| **Partner closed (25%)** | they run the sale and close it themselves |

If you leave the tier blank, they never earn anything. The record looks
complete, deals get attributed to them, and no money is ever calculated.
Margince will not guess a rate you have not agreed, and nothing warns you
either, so set it when you set the role.

The rest of the form (certification, relationship stage, next step) is for
managing the relationship, not the money.
[The handbook explains each one.](../docs/handbook/partners.md)

## 2. Say who brought the deal

Open the deal and edit it, or fill these in as you create it. Two fields, and
they only appear if you have at least one partner:

**via partner**: who brought it. Only actual partners are offered, so you
cannot pick an ordinary customer by mistake.

**Partner attribution**: appears once you have chosen someone:

- **Sourced the deal (earns commission)**: they found it.
- **Influenced an existing deal (no commission)**: they pitched in, and
  it is recorded, but there is no share in it for them.

Skip the second field and Margince assumes they brought it, which is the usual
case.

Set **Company** to the customer and **via partner** to the partner. The deal
then reads:

> **€10,000.00 · Northgate GmbH · via VietnamPartner JSC**

The value, who is buying, and who brought them. You can add **via partner** as
a column in the deals list too, from the column picker. It cannot be sorted
yet.

## 3. Win it

Move the deal to **Won** as you normally would: drag its card into the Won
column.

If there is no signed contract on the deal, Margince asks **"How was it
won?"** (verbally, on a purchase order, and so on). That is a record-keeping
question, not a commission one. Any answer lets the win through.

That is all you do. Winning the deal is what pays the partner.

## 4. See what they earned

Back on the partner's company page, the **Partner** tab now has two panels
under their record.

**Partner deals**: everything attributed to them, won or still open,
with the customer each one was for. This is the only place these deals show up
on this company's page, because the deals themselves belong to the customers.

**Commission**: what they have earned. One row per deal, with the amount, the
rate, and the deal value it came from. A €10,000 deal at *Partner closed
(25%)* shows **€2,500.00**.

Both the deal and the customer are links, so any number can be traced back to
the work behind it. The commission appears a second or two after the win;
refresh if the panel still looks empty.

## Reading the ledger

Every entry has a status:

| | |
|---|---|
| **Accrued** | earned, not yet agreed; every entry starts here |
| **Approved** | signed off |
| **Paid** | your finance system has paid it |
| **Reversed** | cancelled, by you or because the deal was reopened |

To move an entry along, use the steps its row offers. An accrued entry offers
**Approve**, an approved one **Mark as paid**, and anything still live
**Reverse**. Each asks you to confirm first, because pressing the same
button again does not undo it. Reversing asks why, and the reason travels with
the entry so it can be explained to the partner later.

If your seat cannot decide commissions, you see no buttons. The column says so
instead of going blank, so you can tell "nothing to decide here" from "not
yours to decide".

**Outstanding** sits above the ledger: everything accrued or approved,
totalled per currency. Two currencies are never added together, because the
sum would mean nothing.

Reopening a won deal does not delete the commission. Margince adds a reversal
row and marks the original *Reversed*, leaving both on the page. Win it again
and a new entry appears. So a deal that was won, reopened and won again shows
three rows, and that is correct. The ledger is a history of what happened, not
a snapshot of what is true today.

The rate is fixed at the moment a deal is won. Move a partner to a different
tier and it changes what their *next* deals earn; it never rewrites what an
old one already paid.

## What this cannot do

- **Margince does not move money.** Approving and marking paid are record
  keeping: they say what your finance system has agreed and settled.
- **Partners cannot see any of this.** There is no partner login, and none is
  planned. When a partner asks what they have earned, someone on your side
  opens their company page and tells them.
- **No tier, no commission.** Win a deal a partner with no margin tier
  sourced and no entry appears, with no warning, because there is no rate to
  apply. If you expected a commission and see none, check the tier on their
  Partner tab.
- **Assistants can read partners** (tier, certification, stage and the partner
  list) but cannot change them. Setting a partner's terms is a human act.

## Where next

- [Setting up partners, field by field](../docs/handbook/partners.md)
- [Working deals in general](../docs/handbook/the-pipeline.md)
