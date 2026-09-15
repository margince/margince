# Analytics and forecasting

**Analytics** sits under Intelligence. It answers two different kinds of
question, and the product is careful to keep them apart: what the records
actually say, and what somebody believes will happen.

Every number here opens the rows it was built from. If a figure looks wrong,
open it rather than guessing.

## The six sections

**Forecast** · **Pipeline** · **Performance** · **My outcomes** ·
**Data coverage** · **Delivery**

Two of them are conditional, and their absence means something:

- **My outcomes** appears only for a seat whose own records are its whole
  world — a rep. A manager never sees the tab, because the section answers for
  one seat and a manager's lens covers more. Reaching its address anyway says
  so: "This view answers for one seat. Your lens covers more than your own
  records, so the wider sections carry your numbers."
- **Data coverage** appears where you hold the grant and the server answered at
  all — including on a fresh installation where no check has ever run, which the
  page then says in words rather than by being absent.

The section is part of the address, so a link to one opens it.

## Which records a number covers

Every reading is bounded by a population, and the screen always says which.

Where you have more than one to choose from, it is a picker labelled **"Which
records these numbers cover"**. Where you have exactly one — which is a rep's
usual case — it is a plain sentence instead: **"These numbers cover {scope}."**
A dropdown holding one option asks a question with one answer.

What you are offered comes from the server, labels and all, and the request is
checked again on the way in. Nothing in the browser decides who may measure
what.

- An unbounded seat is offered **Whole workspace**, every live team, and their
  own records.
- A team-scoped seat is offered **My teams**, each of their own live teams, and
  their own records.
- Everyone else is offered their own records.

Archived teams are never offered.

**One honest limit.** The picker governs the Forecast section. The report cards
in the other sections read under your own default population rather than the one
selected above them, so changing the picker does not move their figures.

## Under every report

**"As of {asOf} · {zone}"** — the moment the figures describe, and the timezone
they were cut in. It is drawn only when the server sent both. Half a frame is
worse than none.

It names no currency on purpose: one report prints rows in their own currency
beside blocks converted into yours, and a single currency label over both would
be false for one of them.

## Explain this number

Every report card carries **Explain this number**. It opens "How this number is
built" and gives you three things:

1. **The server's own definition** of the filter, the grouping and the
   aggregate — in words, not as a query.
2. **Source rows** — the actual rows, scoped exactly as the report was. Money in
   minor units is formatted; a column the vocabulary knows is translated, and one
   it does not keeps its wire name rather than being given a wrong one.
3. **A staleness warning where one is owed.** If the link did not say when the
   number was worked out, the figures were recalculated just now, and the panel
   says so: "If an exchange rate changed in between, they may not add up to the
   number you clicked."

Row ids are dropped only when *every* row carries a name. A partially labelled
set keeps them, so a row you may not read stays identifiable.

**Weighting is rounded per deal and then summed**, here and on the board, which
is why the drill-down reconciles exactly rather than being off by a rounding
error.

The Forecast section has no Explain control — it is not built from report cards.

## Pipeline

**Open pipeline by stage** — one row per stage, in pipeline order: the stage,
how many deals, unweighted and weighted. Every figure is converted server-side
into one base currency, so a stage is one row with no currency column.

**Forecast categories** — a tile per category, the raw total as the value and
the weighted total beneath it. A standing note says how to read them: "Each tile
shows the raw total and, beneath it, the probability-weighted total — rounded
per deal, so it always reconciles to Explain This Number."

The **No category yet** tile appears only when something actually falls in it. A
tile with nothing measured says **"Nothing measured"** — a word, never a dash.

**Open deals per company** — company, currency, open deals, unweighted. These
rows stay in their native currency and are grouped by company *and* currency, so
two currencies on one account are two rows.

Where not every deal carries an amount, a footnote says so: "{priced} of {total}
priced".

## Performance

**Won and lost** — outcome, deals, value, median days to close, P75 days to
close.

**Time in stage** — stage, deals, median and P75 days in stage. Open deals only,
aged from the last time each entered the stage it is in now.

Two honest refusals here:

- Below the engine's sample floor, a duration reads **"Too few to say"** rather
  than a number. A median drawn from a handful of deals moves further than the
  answer is worth.
- **No win rate is computed.** A percentage worked out in the browser would be a
  second answer to the question of what the cohort is, and the two would drift.

A stage the pipeline no longer carries reads **"Former stage"**.

## My outcomes

Your own work, two panels.

**My open pipeline** — deals and value, both opening the Pipeline section.

**My meetings** — meetings *you host*, counted by where each stands today:
**Booked, Held, No-show, Canceled.** The sub-line states the rule that trips
readers up: "a held meeting no longer counts as booked."

## Delivery

**Projects by phase** — phase, projects, open deal value, won deal value.

**Project promises** — project, phase, open, overdue.

**Projects gone quiet** — project, phase, quiet since.

When there is nothing, the page says which nothing it means: "No projects yet —
a won deal opens one," or "No delivering project has gone quiet."

## Data coverage

Not what was found — **how much could be looked at**. "Which sources the nightly
check could read, and how far. A quiet source that was read is checked; an
unread one says why."

Three columns: source, state, checked through. The six sources are the mailbox,
the calendar, documents, contracts, offers and the incumbent system.

Five states:

| State | What it means |
|---|---|
| **Checked** | Read, and to a date |
| **Stale — nothing read recently** | Reachable, but nothing recent came back |
| **Unavailable — the check could not read it** | It tried and failed |
| **Access needs re-granting** | A permission lapsed |
| **Not connected — nothing to fix, something to decide** | Never wired up |

**"Checked through" carries a date only for a checked source.** A date on an
unread one would read as "checked up to then" when nothing was read at all.

A source missing from the list is one the run did not attempt, which is a
different fact from one it attempted and could not read.

If no check has ever run, the page says so rather than showing a healthy blank:
"No check has run yet. A fresh installation has not been looked at — different
from one that was looked at and found healthy."

Record-level problems are not here. They are in the Forecast section's review.

## Forecast

The section answers one question — where will this period land — and it
distinguishes three different kinds of claim rather than blending them.

| Reading | What kind of claim it is |
|---|---|
| **Current call** | Somebody's judgement. No call reads "No call made", never a dash |
| **Supported by evidence** | The part with confirmed close dates behind it |
| **Already won** | Money that arrived |

The membership rules are worth knowing:

- **Won** counts by the day a deal actually closed, never the day it was
  expected to.
- **Open** means not won, carrying an expected close date, and that date falls
  in the window.
- **Evidence** means open, not provisional, and in Commit.
- A **provisional** close date is a guess. It is in the open pipeline and out of
  the evidence.

Where not every deal is priced, a callout says what that costs: "{priced} of
{eligible} deals carry an amount. The rest are real pipeline contributing
nothing to the totals above."

### The window

**Quarter, Month or Week.** Quarter and month are cut against your financial
year, so an April-opening year has Q1 running April to June. **Week is the
working week, Monday to Sunday**, and is not a division of the financial year —
asking for a week gets a week, or a refusal, never a quarter quietly returned in
its place.

### Recording a call

**"A call is what you believe will close. It records your number and changes no
deal."**

You give an expected total in the base currency and a supporting note. The call
is filed against the period *and* the population you are looking at.

**A call supersedes; it never overwrites.** The first call of a period
supersedes nothing, and that is recorded too. Each one commits the row, the
audit entry and the event together.

Who may record one: not a rep. The manager forecast is an assertion about a team
or the whole company, so a seat whose lens is its own records does not get the
editor at all. Nor does **My teams** — a forecast names one population, and that
lens covers several, so the action is withheld rather than guessing which team
you meant.

A call against a population you have no authority over answers **not found**,
not "forbidden" — naming the id would confirm it exists.

### Projected landing

Won plus what is still to come. What "still to come" means is an installation
setting, and the card names it:

- **Commit evidence** — confirmed close dates only. The default, and the
  strictest.
- **Weighted pipeline** — every open deal at its stage probability.
- **The manager's call** — the authored number *replaces* the projection rather
  than adding to what is already won.

The two halves are disjoint by construction, so nothing is counted twice. A sum
that would overflow is refused rather than wrapped.

Two caveats it will tell you about rather than hide:

- Nobody called the period on a manager-call installation, so it fell back to
  commit evidence — and says so.
- The call is **below** the money already won. It is shown as recorded rather
  than quietly corrected.

### Pipeline needed

How much open pipeline it would take to reach the reference, as a percentage
with a meter beneath it.

**The reference comes from outside the current pipeline, on purpose.** A manager
call outranks history; failing that, the median of the last four completed
comparable periods. The reason is stated in the product itself: pipeline divided
by a target derived from that same pipeline is a number that is always fine.

**This is not a target.** Margince has no target model, and a figure a rep is
measured against is a product decision nobody has made.

Where it cannot be computed it says which nothing it means, and shows no figures
at all — a zero here would read as a fully covered pipeline:

- **Nothing to measure against** — no call, and fewer than four comparable
  periods finished.
- **Too few closed deals** to read a conversion rate from.

Neither the landing nor this figure is shown under **My teams**: a landing
summed across books called separately would describe none of them.

### The receipt

**"Data and evidence checked"** — four counts, and the last two are deliberately
separate:

| Count | What it means |
|---|---|
| **Eligible deals** | Everything the reading considered |
| **Priced** | Of those, how many carry an amount at all |
| **Close date confirmed** | Has an expected close date, and it is not provisional |
| **Exchange rate missing** | Priced, but no rate could convert it |

An unpriced deal has no amount. A priced deal no rate could reach has one and
still contributes nothing. Blurring the two would hide half the problem.

The review — "What should be checked before the call?" — comes first, and the
receipt after it. A manager with ten minutes reads what needs doing; the receipt
is what they consult when a number looks wrong.

## Sharing a view

A **Share view** control sits beside the tabs on the Forecast section and mints
a link.

Read the model before you rely on it: **a share widens who knows a link exists,
never what anybody is allowed to read.** Whoever opens it must be signed in as
themselves, and the numbers are computed under *their* grants. Two colleagues
opening one link can correctly see different figures.

The link is shown once — "Copy it now — it cannot be read back" — and stops
working after 30 days. Every refusal is the same **not found**, whether the
token is unknown, expired, revoked, or issued by somebody who has since left:
distinguishing them would tell anyone working through guessed tokens which
guesses were closer.

An agent can neither mint one nor open one.

**Honest state of this feature today.** The link mints, but the rest is not
wired up: opening one lands on the ordinary Analytics screen under your own
population rather than the shared view, the frozen-state option cannot yet
succeed, there is no control to close a link early, and the CSV export behind it
is unreachable. Treat Share view as unfinished rather than as a way to get
figures to somebody outside your own seat.
