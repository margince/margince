# Offers and the rate card

An **offer** is the priced document you put in front of a buyer. It belongs to a
deal and carries revisions rather than being edited in place once it has gone
out.

It starts in the deal's currency, and while it is a draft you can change that —
the currency is the offer's own, not a copy of the deal's that the product keeps
in step.

## Starting one

From the deal, **New offer**.

A deal with no currency cannot carry one, and the button says why rather than
failing later: "Price this deal first — an offer is written in the deal's own
currency."

## What an offer holds

A header — buyer company, template, valid-until date, intro text, terms text —
and a list of lines.

Each line carries a position, a description, a unit, a quantity, a unit price, a
discount percentage, a tax rate, and its own total. A line may be picked from a
product, or typed from nothing.

**Both the header and the lines are editable only while the offer is a draft.**
After that it is a record of what you sent.

## How the money is worked out

Entirely on the server, in whole minor units, with exact arithmetic — no
floating point anywhere.

Per line, in this order:

1. **Net** = quantity × unit price, less the discount.
2. **Tax** = net × the tax rate.
3. **Line total** = net + tax.

**Rounding happens per line, then the lines are summed.** That is what makes the
offer's totals reconcile to the lines a buyer can read off the page, rather than
being a few cents from them.

The offer's **Net**, **Tax** and **Gross** are those sums.

### Recurring lines

A line can be one-off or recurring. A recurring line names how often it repeats
— monthly, quarterly, every six months, or yearly — and how many periods the
buyer commits to.

Two more figures then appear:

- **Annual recurring** — the recurring lines annualised. It is worked from
  **net**, not gross.
- **Committed net** — one-off lines in full, plus each recurring line multiplied
  by the periods committed.

A line with no billing classification at all contributes to **neither**. That is
the ordinary case for anything written before the classification existed, and it
is why the two figures appear only when there is something recurring to report.
An offer with nothing recurring shows neither, rather than showing two zeros.

A line carrying no price is marked "unpriced — excluded from total" rather than
being counted as free.

## The four verbs

**Send.** "Send this offer to the buyer? The offer becomes read-only until the
buyer responds." Sending freezes the exchange rate onto the offer and takes a
snapshot of who the buyer and the issuer legally were at that moment.

**Accept.** "Mark this offer as accepted? The deal's amount and currency will be
updated to match this offer." That sync is the point — accepting is what makes
the deal say what the buyer agreed to. It needs permission to change both the
offer and the deal.

**Reject.** A reason is optional, and it is kept in the trail rather than on the
offer.

**Regenerate revision.** Mints the next revision as a fresh **draft**, copying
the header and every line verbatim, and marks the old revision superseded. One
thing does not come across: the **template**, which the new revision does not
carry over — re-pick it before rendering.
Nothing is re-derived from today's rate card: a price that changed last week
does not silently rewrite the offer you already sent.

You can render a **PDF** at any point, draft included. Where the deployment has
no file store wired, it says so plainly — "PDF rendering not available on this
deployment." — rather than failing as an error.

## What an offer refuses

- **An empty offer cannot be sent.** "the offer has no line items to send".
- **A repeating line must say how many periods.** "line 3 repeats, so the offer
  has to say how many periods the buyer commits to". It is checked on send and
  names the line by its position on the paper.
- **A product priced in another currency needs an explicit price.** You cannot
  quietly mix currencies inside one offer.
- **A figure too large to store exactly is refused**, rather than rounded into
  something that looks fine.
- **Sending with no exchange rate for the day is refused** — it never falls back
  to a rate of 1.

The billing shape has its own refusals, each stating the rule rather than the
error: "a one-off price has no billing interval, because there is nothing to
repeat"; "a commitment covers at least one period".

## What an agent may do

An agent can draft: create an offer, list and read offers, edit one, add and
remove lines, and archive one.

**An agent may not send, accept, reject, regenerate or render one.** Those are
human-only at the door — there is no staged path to them, and no approval card
to release. The money leaving your hand in a buyer's direction is a human's
decision.

## The rate card

**Settings → Products & offers.** "Rate-card entries that offer lines snapshot
from."

A product carries a name, an optional SKU, a description, a unit, a unit price
and currency, a default tax rate, and its billing shape — one-off or recurring,
and for recurring, the period.

Billing is allowed to be **unspecified**, and that is not the same as one-off.
Reading an unclassified price as a one-off would assert a classification nobody
made, so the product leaves it blank and says "Not specified".

### Snapshots, and why a line does not move

**A line takes its description, unit, price and tax rate from the product once,
when you insert it.** Editing the product afterwards never reaches back into an
offer. Archiving one says so: "Archive this product? Existing offer lines keep
their snapshot."

That is the whole design. An offer is a thing you said on a day, and a rate card
that could rewrite last quarter's paperwork would make it something else.

## Offer templates

A template is a branded layout, German or English, with at most one default per
locale.

**Three honest limits today.** An offer whose template has gone does **not**
fall back to your locale default — it renders from an empty layout. Header and
footer text typed into a template is
stored but never reaches the rendered PDF, and there is no field at all for the
terms text the renderer does print. A logo referenced in a template is not
fetched or embedded either — the renderer works offline by design.

If you need branded output now, treat the template as naming a locale rather
than as carrying your letterhead.
