# Relationships, introductions and research

Three related questions: who here already knows somebody, how you get a warm
introduction to somebody you do not know, and what Margince can read about a
company from its own website.

## Who here knows them

A contact's page shows which colleagues have corresponded with them, strongest
first, with the number of exchanges behind each. When nobody has: "Nobody here
has corresponded with them yet."

The order is the answer, and it is never re-sorted on screen.

### How strength is worked out

One formula, from captured cadence, **computed at read and never stored**:

**Recency × Frequency × Reciprocity**, scored out of 100.

- **Recency** halves every 30 days since the last exchange.
- **Frequency** saturates at 20 interactions in 90 days — the twenty-first adds
  nothing.
- **Reciprocity** rewards a two-way exchange over a one-sided one, and never
  falls below a quarter.

**There is no way to override it by hand, and that is deliberate.** It is a
reading of what actually happened, not a field somebody fills in.

Two states that look alike and are kept apart: a contact you have **never**
spoken to carries no score at all, while one you spoke to and let go cold carries
a low one. Rendering both as zero would lose the distinction that matters.

The record also shows the direction split — how much came in against how much
went out — so "six two-way exchanges" never reads the same as "six unanswered
sends".

### A caution about the bands

The product uses more than one vocabulary for relationship warmth, on purpose,
because the things being measured are not comparable: one colleague's contact
with somebody is a different question from the company's contact with them.

So do not read "Weak" on one screen as the same claim as "barely in contact" on
another. Each band belongs to the surface it is drawn on.

### The network map

A contact's **Network** tab draws who can reach them, in four lanes: our team,
their company, who they talk to, and the target.

It counts and never quotes: "{total} interactions in 90 days · {inbound} in,
{outbound} out", with "Counts only — the messages themselves stay on the
timeline."

Where the map is incomplete it says so rather than drawing a smaller truth —
"{count} more not shown", "Some colleagues are not shown."

## Asking for an introduction

**Ways in** ranks the routes to somebody, best first, and explains the ranking
rather than asserting it: "Pick the one you can actually use — the second is here
because the first is not always available."

**A direct route beats an indirect one however warm the indirect looks.** After
that, two-way beats one-sided, then volume.

Each route carries its verdict and its evidence — "Ask {name} — they already
write to each other", with the exchanges and the date behind it. Where there is
nothing in 90 days it says so rather than leaving a blank.

Only a **colleague** can carry an introduction. The map draws contact-to-contact
edges, but they are never offered as routes.

A route you have already asked about is marked **Already asked**, and one that
was turned down **Declined before** — so a route the product would refuse is
never offered.

### Making the ask

Three things, and the form is explicit about who reads which:

- **Why you are asking** — required. "Your colleague reads this, not the contact."
- **What is in it for them** — the reason the contact would want the
  conversation.
- **A note your colleague can forward** — "The only part the reader reads. Write
  it so it can be pasted as it stands."

You can also ask permission to mention their name instead, and say what should
happen **if they say no**: nothing further, ask to use their name, or try the
next route.

Nothing here drafts for you.

### What your colleague sees, and the four answers

The ask reaches them two ways: on that contact's Network tab, and as an item in
their own daily queue. The second exists because an ask nobody happened to look
for simply expired.

Four answers, and no others:

| Answer | What it means |
|---|---|
| **I will introduce you** | They make the introduction |
| **You may use my name** | You reach out yourself and mention them |
| **Ask someone else** | They name a colleague better placed |
| **Not this time** | The ask closes |

**"You may use my name" is not a weaker yes.** Nothing in the product turns lent
permission into a handshake that happened, and the two are counted apart all the
way through.

A suggestion of somebody else does not become an ask on its own — you make a new
one, so nobody is asked without agreeing to be.

### After the answer

You mark the introduction made, or the name used — and which of the two gets
recorded comes from the state of the ask, never from a choice you make.

**Whether they replied is never something you tick.** It is observed from
captured mail: an inbound message, from that contact themselves, after the
handshake. A checkbox would make the product's best number the one claim nobody
had evidence for.

An ask waits **7 days** and then lapses on its own, which frees the route.

You can withdraw your own ask; your colleague declines rather than withdrawing.
Either way the row stays, because an ask that was made is a thing that happened.

### Who can see an ask

The two parties, and nobody else. A third colleague sees an empty list rather
than a refusal — whether somebody was asked about a contact is exactly the fact
the row protects.

**An agent may not ask, answer, complete or withdraw an introduction.** The
product states why: asking a colleague for a favour is a human's act, and an
agent holding a human's credential is not that human deciding to spend their
goodwill. An agent may read who knows whom.

## Researching a company

From a company: **Start company research**, or **Read the website again** once
there is a read.

> It reads the company's website for the domain, industry, size, locations and
> likely decision-makers, then suggests a first move. Findings are staged for
> your review — nothing is written until you accept.

### What it reads, and what it will not

It walks the company's **own** site: the home page, the imprint, about, team,
services, products and contact pages, plus what it finds linked from those.

**Which pages to read is decided by the product, never by the model.** Page
content can influence at most which same-site links exist — it can never talk the
crawl into leaving the site or raising its own budget.

An address on another domain is recorded as skipped and never fetched. The site's
own `robots.txt` is honoured.

### Every way a read stops

| It says | What happened |
|---|---|
| **Done** | It ran out of pages to read |
| **Read up to the page limit** | 60 pages, or fewer where an operator set a lower ceiling |
| **Read up to the size limit** | 32 MB across the whole crawl |
| **Read up to the time limit** | Four minutes |
| **Waiting for AI budget** | The company's allowance is spent. "Resumes automatically {when}" |
| **Failed** | With a plain cause, and another attempt scheduled where one would help |
| **Cancelled** | Withdrawn before it ran |

A failure names the cause in ordinary words — the site asked this crawler not to
read the page; bot protection refused it; the certificate could not be verified;
the domain does not resolve. Those sentences are the product's own, never a
provider's.

A read started automatically is capped harder than one you ask for: 12 pages
rather than 60.

### Nothing is written until you accept

Every finding arrives as a **staged proposal** in your approval inbox — one for
the company's facts, and one per contact found on a team page.

**Every field carries the verbatim passage it was read from, or it is left out.**
There is no guessing: a value the page does not clearly state is omitted rather
than filled in.

### The related readings

- **Dossier** — what this company *is*, written only from facts already on file,
  every sentence citing one.
- **Account scan** — a reading of one account *for you*, citing records and
  quoting words. It is per-reader and never shared, because it is assembled from
  what you can see.
- **Growth fit** — what this company is worth to *you*, banded, with both
  data-completeness counts shown.
- **VAT check** — asks the EU register and keeps the receipt. Never asked reads
  as never asked, not as a failure.
- **Hierarchy rollup** — a parent's numbers including the children you can read,
  with any you cannot **named as excluded** rather than silently dropped from the
  sum. Where no exchange rate is available it refuses rather than assuming one.

The dossier, the scan and the growth fit are yours alone — an agent cannot read
any of them, because each is built from one reader's own view.

### When it is not available

Where the deployment has wired no crawler, the panel says "Site reading is not
configured on this server." The button stays visible, so if you press it and get
that sentence, it is your installation rather than the company's website.
