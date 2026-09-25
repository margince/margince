# Offers and the rate card

An **offer** in Margince is the priced document you put in front of a buyer. It
belongs to a deal and carries revisions rather than being edited in place once
it has gone out.

An offer starts in the deal's currency, and while it is a draft you can change
that — the currency is the offer's own, not a copy of the deal's that the
product keeps in step.

### How do I create an offer for a customer?
To create an offer in Margince, open the deal and click **New offer** in the **Offers** panel on its **Overview** tab.
1. Open **Deals** in the sidebar and open the deal.
2. Click **New offer**. Margince creates a draft offer in the deal's currency and opens it.
3. Click **Edit header** to set **Valid until**, **Template**, **Buyer company**, **Intro text** and **Terms text**.
4. Add lines under **Line items** (see the next question).
If the deal has no currency yet, the button is disabled and says: "Set the deal value first. An offer uses the deal’s currency."
Also called: quote, quotation, proposal, estimate.

### How do I add a product or a line to an offer?
To add a line to an offer in Margince, open the draft offer and fill the **Add line** row in the **Line items** panel.
1. Choose **Pick product** to copy a product from your rate card, or type the line yourself.
2. Fill **Description**, **Unit**, **Quantity** and **Unit price**; set **Discount %** and **Tax %** on the added line.
3. For a repeating charge, set **Billing** to **Recurring**, a **Billing period**, and **Periods** committed.
4. Click **Add line**. **Remove** takes a line out again.
Lines can only be added or removed while the offer is a draft.
Also called: line item, position, add an item to a quote.

### How do I send a quote to a customer?
To send an offer in Margince, open the draft offer and click **Send**, then confirm "Send this offer to the buyer?". Sending marks the offer as sent and freezes it: "The offer becomes read-only until the buyer responds."
**Send does not email anything.** Nothing leaves Margince when you click it; it records that this version is the one you are sending. To get the offer to the buyer, click **Render PDF**, open it with **View PDF**, and send that file yourself — attached to an email, or uploaded to the deal and added to its [Deal Room](deal-rooms.md).
An offer with no lines cannot be sent.
Also called: email the proposal, issue the quote.

### How do I print or download an offer?
To print an offer or save it as a file, open the offer and choose **Render PDF**, then **View PDF** once it appears, and print or download the PDF from your browser. Margince has no separate print button. The PDF carries the offer's intro text, lines, totals and terms text, plus the template's header and footer.
Also called: print a quote, offer PDF, download the proposal, export an offer.

### What happens after I send an offer?
After an offer is sent, Margince waits for you to record the buyer's answer; the buyer cannot accept or reject it inside Margince. A sent offer shows three buttons:
- **Accept** — "Mark this offer as accepted?" The deal value and currency are updated to match this offer.
- **Reject** — "Mark this offer as rejected?", with an optional **Reason (optional)**.
- **Regenerate revision** — starts the next revision as a fresh draft for a counter-offer or change.
An accepted or rejected offer has no further buttons. **Render PDF** stays available in every state, and **View PDF** appears once a PDF has been rendered.
Also called: the customer said yes, quote declined, revise the quote.

## What an offer holds

An offer holds a header — buyer company, template, valid-until date, intro
text, terms text — and a list of lines.

Each line carries a position, a description, a unit, a quantity, a unit price, a
discount percentage, a tax rate, and its own total. A line may be picked from a
product, or typed from nothing.

**Both the header and the lines of an offer are editable only while it is a
draft.** After that it is a record of what you sent.

## How the money is worked out

Offer totals are worked out entirely on the server, in whole minor units, with
exact arithmetic — no floating point anywhere.

Per line, in this order:

1. **Net** = quantity × unit price, less the discount.
2. **Tax** = net × the tax rate.
3. **Line total** = net + tax.

**Rounding happens per line, then the lines are summed.** That is what makes the
offer's totals reconcile to the lines a buyer can read off the page, rather than
being a few cents from them.

The offer's **Net**, **Tax** and **Gross** are those sums.

### Recurring lines

A recurring offer line names how often it repeats — monthly, quarterly, every
six months, or yearly — and how many periods the buyer commits to. A line can
also be one-off.

Two more figures then appear:

- **Annual recurring** — the recurring lines annualised. It is worked from
  **net**, not gross.
- **Committed net** — one-off lines in full, plus each recurring line multiplied
  by the periods committed.

A line with no billing classification at all contributes to **neither**. That is
the ordinary case for anything written before the classification existed, and it
is why the two figures appear only when there is something recurring to report.
An offer with nothing recurring shows neither, rather than showing two zeros.

A line carrying no price is marked "unpriced, excluded from total" rather than
being counted as free.

## The four verbs

**Send** freezes the exchange rate onto the offer and takes a snapshot of who the
buyer and the issuer legally were at that moment. It delivers nothing to the
buyer — there is no transport behind it.

**Accept** syncs the deal's amount and currency to the offer's gross. That sync
is the point — accepting is what makes the deal say what the buyer agreed to. It
needs permission to change both the offer and the deal.

**Reject** takes an optional reason, and it is kept in the trail rather than on
the offer.

**Regenerate revision** mints the next revision as a fresh **draft**, copying
the header and every line verbatim, and marks the old revision superseded. One
thing does not come across: the **template**, which the new revision does not
carry over — re-pick it before rendering.
Nothing is re-derived from today's rate card: a price that changed last week
does not silently rewrite the offer you already sent.

You can render an offer **PDF** at any point, draft included. Where the
installation has no file store wired, it says so plainly — "PDF rendering is not
available on this installation." — rather than failing as an error.

The rendered PDF prints the offer's own **Intro text** above the lines and its
**Terms text** under the totals, and leaves either section out when it is
empty. The terms always come from the offer, never from the template.

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

## What an agent may do with offers

An agent can draft offers: create an offer, list and read offers, edit one, add
and remove lines, and archive one.

**An agent may not send, accept, reject, regenerate or render an offer.** Those
are human-only at the door — there is no staged path to them, and no approval
card to release. The money leaving your hand in a buyer's direction is a human's
decision.

## The rate card

The rate card in Margince is the **Products** list at **Settings → Products and
offers**: "Price list entries that offer lines copy from." Searching the command
palette for "rate card" opens the same page; there is no separate rate-card
screen.

### How do I add a product or change the rate card?
To add a product to the rate card in Margince, open **Settings → Products and offers** and click **New product**.
1. Open the account menu at the top right and choose **Settings**, then **Products and offers**.
2. Click **New product**.
3. Fill **Name**, **Unit price** and **Currency** (all required), and optionally **SKU**, **Description**, **Unit**, **Default tax rate %**, **Billing** and **Billing period**.
To change a price, use **Edit product** on its row; to retire one, **Archive product**.
Without the right role the list reads "Read-only. Your role cannot change products."
Also called: price list, price book, catalogue, SKU.

A product carries a name, an optional SKU, a description, a unit, a unit price
and currency, a default tax rate, and its billing shape — one-time or recurring,
and for recurring, the period.

Product billing is allowed to be **unspecified**, and that is not the same as
one-time. Reading an unclassified price as one-time would assert a
classification nobody made, so the product leaves it blank and says "Not
specified".

### Snapshots, and why a line does not move

**An offer line takes its description, unit, price and tax rate from the product
once, when you insert it.** Editing the product afterwards never reaches back
into an offer. Archiving one says so: "Archive this product? Existing offer
lines keep their snapshot."

That is the whole design. An offer is a thing you said on a day, and a rate card
that could rewrite last quarter's paperwork would make it something else.

## Offer templates

An offer template is a branded PDF layout, German or English, with at most one
default per locale: "Branded PDF layouts for offers in German and English."

### How do I create an offer template?
To create an offer template in Margince, open **Settings → Products and offers** and click **New template** in the **Offer templates** panel.
1. Fill **Name** and choose the **Locale** (de-DE or en-US).
2. Set **Default for locale** to true or false.
3. Optionally fill **Header text** and **Footer text**; the PDF prints the header above the buyer and the footer at the end.
Then pick the template on an offer with **Edit header** → **Template**.
Also called: quote template, proposal layout, letterhead.

**Two honest limits of offer templates today.** An offer whose template has
gone does **not** fall back to your locale default — it renders from an empty
layout; an archived template, by contrast, keeps rendering. A logo referenced
in a template is not fetched or embedded either — the renderer works offline by
design, so put your letterhead in **Header text**.

An offer template has no terms field: terms belong to each offer, set under
**Edit header** → **Terms text**.
