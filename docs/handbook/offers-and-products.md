<!-- prose:plain -->
# Offers and the rate card

An **offer** in Margince is the document with prices that you give a buyer. It belongs to a deal. Once it has been sent, you do not edit it; you make a new version of it instead.

An offer starts in the deal's currency, and while it is a draft you can change it. The currency belongs to the offer; changing the deal's currency later does not change it.

### How do I create an offer for a customer?
To create an offer in Margince, open the deal and click **New offer** in the **Offers** panel on its **Overview** tab.
1. Open **Deals** in the sidebar and open the deal.
2. Click **New offer**. Margince creates a draft offer in the deal's currency and opens it.
3. Click **Edit header** to set **Valid until**, **Template**, **Buyer company**, **Intro text** and **Terms text**.
4. Add lines under **Line items** (see the next question).
If the deal has no currency yet, the button is turned off and says: "Set the deal value first. An offer uses the deal’s currency."
Also called: quote, quotation, proposal, estimate.

### How do I add a product or a line to an offer?
To add a line to an offer in Margince, open the draft offer and fill the **Add line** row in the **Line items** panel.
1. Choose **Pick product** to copy a product from your rate card, or type the line yourself.
2. Fill **Description**, **Unit**, **Quantity** and **Unit price**; set **Discount %** and **Tax %** on the added line.
3. For a line that repeats, set **Billing** to **Recurring**, a **Billing period**, and **Periods** agreed.
4. Click **Add line**. **Remove** takes a line out again.
You can add or remove lines only while the offer is a draft.
Also called: line item, position, add an item to a quote.

### How do I send a quote to a customer?
To send an offer in Margince, open the draft offer and click **Send**, then confirm "Send this offer to the buyer?". Sending marks the offer as sent and locks it: "The offer becomes read-only until the buyer responds."

**Send does not email anything.** Nothing leaves Margince when you click it; it records that this version is the one you are sending. To get the offer to the buyer, click **Render PDF** and open it with **View PDF**. Then send that file yourself: added to an email, or uploaded to the deal and added to its [Deal Room](deal-rooms.md). You cannot send an offer with no lines.
Also called: email the proposal, issue the quote.

### How do I print or download an offer?
To print an offer or save it as a file, open the offer and choose **Render PDF**, then **View PDF** once it shows. Print or download the PDF from your browser; Margince has no print button of its own. The PDF holds the offer's intro text, lines, totals and terms text, plus the template's header and footer.
The **View PDF** button does not show when the offer's buyer is a company you cannot open, because the stored PDF may print its name. This is true even for a PDF you made yourself. Changing a draft's buyer removes its PDF; make it again for the new buyer.

Also called: print a quote, offer PDF, download the proposal, export an offer.

### What happens after I send an offer?
After an offer is sent, Margince waits for you to record the buyer's answer; the buyer cannot accept or reject it inside Margince. A sent offer shows three buttons:
- **Accept**: "Mark this offer as accepted?" The deal value and currency are updated to match this offer.
- **Reject**: "Mark this offer as rejected?", with a **Reason (optional)** you may fill or leave empty.
- **Regenerate revision**: starts the next version as a new draft, for an answer to the buyer or a change.
An accepted or rejected offer has no more buttons. **Render PDF** stays there in every state. **View PDF** shows once a PDF has been made, unless the offer's buyer is a company you cannot open.
Also called: the customer said yes, quote declined, revise the quote.

## What an offer holds

An offer holds a header (buyer company, template, valid-until date, intro text, terms text) and a list of lines.

Each line holds a position, a description, a unit, a quantity, a unit price, a discount %, a tax rate, and its own total. A line may be picked from a product, or typed from nothing.

You can edit the header and the lines **only while it is a draft**. After that the offer is a record of what you sent.

## How the money is worked out

Margince works out offer totals in whole units of the currency's smallest unit (such as the cent). The only rounding is the one on each line, below.

For each line, in this order:

1. **Net** = quantity × unit price, with the discount taken off.
2. **Tax** = net × the tax rate.
3. **Line total** = net + tax.

Each line is **rounded first**, then the lines are added up. So the offer's totals always match the lines a buyer can read off the page, to the currency's smallest unit.

The offer's **Net**, **Tax** and **Gross** are those totals.

### Recurring lines

A recurring offer line names how often it repeats: monthly, every three months, every six months, or yearly. It also names how many periods the buyer agrees to. A line can also be paid only once.

Two more numbers then show:

- **Annual recurring**: what the recurring lines come to in one year. It is worked out from
  **net**, not gross.
- **Committed net**: lines paid once in full, plus each recurring line times
  the periods agreed.

A line with no billing type counts in **neither** number. The two numbers show only when something repeats; an offer with nothing that repeats shows neither, instead of two zeros.

A line with no price is marked "unpriced, excluded from total". It is not counted as free.

## Send, accept, reject and revise

**Send** fixes the exchange rate on the offer and records who the buyer and the seller were in law at that time. It sends nothing to the buyer.

**Accept** sets the deal's amount and currency to the offer's gross total, so the deal shows what the buyer agreed to. It needs permission to change both the offer and the deal.

**Reject** takes a reason you may leave empty. The reason is kept in the audit log, not on the offer.

**Regenerate revision** creates the next version as a new **draft**. It copies the header and every line word for word, and marks the old version as replaced. The **template** is not carried over, so pick it again before you make the PDF. Prices are not updated from today's rate card: a price that changed last week does not change the offer you already sent.

You can make an offer **PDF** at any point, draft included. Where your Margince has no file storage set up, it says "PDF rendering is not available on this installation."

The PDF prints the offer's own **Intro text** above the lines and its **Terms text** under the totals. It leaves either part out when it is empty. The terms always come from the offer, never from the template.

## What an offer refuses

- **An empty offer cannot be sent.** "the offer has no line items to send".
- **A repeating line must name its periods.** "line 3 repeats, so the offer
  has to say how many periods the buyer commits to". It is checked on send and
  names the line by its position on the page.
- **Another currency needs its own price.** For a product priced in another
  currency, enter the price yourself. You cannot mix currencies inside one
  offer.
- **A number too large to store is refused**, not rounded.
- **No exchange rate for the day stops sending.** Margince never uses
  a rate of 1 in its place.

The billing type has its own refusals, and each one states the rule. One says "a one-off price has no billing interval, because there is nothing to repeat". Another says "a commitment covers at least one period".

## What an agent may do with offers

An agent can draft offers: create an offer, list and read offers, edit one, add and remove lines, and archive one.

An agent **may not send, accept, reject, regenerate or render** an offer. Only a human can do these. An agent cannot ask for approval to do them either, because what you promise a buyer is a human's choice.

## The rate card

The rate card in Margince is the **Products** list at **Settings → Products and offers**: "Price list entries that offer lines copy from." If you search for "rate card" in the command palette, it opens the same page; there is no rate-card screen of its own.

### How do I add a product or change the rate card?
To add a product to the rate card in Margince, open **Settings → Products and offers** and click **New product**.
1. Open the account menu at the top right and choose **Settings**, then **Products and offers**.
2. Click **New product**.
3. Fill **Name**, **Unit price** and **Currency** (all required). If you like, also fill **SKU**, **Description**, **Unit**, **Default tax rate %**, **Billing** and **Billing period**.
To change a price, use **Edit product** in its row's **…** menu; **Archive product** there stops using one. Without the right role the list reads "Read-only. Your role cannot change products."
Also called: price list, price book, catalogue, SKU.

A product holds a name, a SKU if you want one, a description, a unit, a unit price and currency, and a default tax rate. It also holds its billing type: one-time or recurring, and for recurring, the period.

You can leave product billing **unspecified**, which is not the same as one-time. Margince does not guess the type, so the product says "Not specified".

### Snapshots, and why a line does not move

An offer line **copies from the product once**, when you add it: the description, unit, price and tax rate. Editing the product later never changes an offer. Archiving one says so: "Archive this product? Existing offer lines keep their snapshot."

## Offer templates

An offer template is a PDF layout with your brand, in German or English. Each language has at most one default: "Branded PDF layouts for offers in German and English."

### How do I create an offer template?
To create an offer template in Margince, open **Settings → Products and offers** and click **New template** in the **Offer templates** panel.
1. Fill **Name** and choose the **Language** (German or English).
2. Set **Default for this language** to Yes or No.
3. If you like, fill **Header text** and **Footer text**. The PDF prints the header above the buyer and the footer at the end.
Then pick the template on an offer with **Edit header** → **Template**.
Also called: quote template, proposal layout, letterhead.

**Limits of offer templates.** An offer whose template no longer exists does not use your language default in its place; it is made from an empty layout. An archived template still works for its offers. Logos are not fetched, so put your letterhead in **Header text**.

An offer template has no terms field: terms belong to each offer, set under **Edit header** → **Terms text**.
