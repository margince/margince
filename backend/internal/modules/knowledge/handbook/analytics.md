# Analytics and forecasting

**Analytics** in Margince is the reporting screen: forecast, pipeline reports,
sales performance, time in stage, your own outcomes, delivery and data coverage. It
sits in the sidebar under **Intelligence**. It answers two different kinds of
question, and the product is careful to keep them apart: what the records
actually say, and what somebody believes will happen.

Every number here opens the rows it was built from. If a figure looks wrong,
open it rather than guessing.

## Common reporting tasks

### Where can I see my forecast?
To see your forecast in Margince, click **Analytics** in the sidebar (under **Intelligence**) and open the **Forecast** section.
1. Click **Analytics**, then **Forecast** under **Analytics sections**.
2. Choose the **Period**: **Quarter**, **Month** or **Week**.
3. Read the **Period forecast** and its composition: **Already won**, **Committed · confirmed dates** and **Best-case addition**. The forecast gap compares the manager’s total with won sales plus confirmed committed deals. Review **Checks before the call**; source details are under **Data and evidence checked**.
A manager records a call with **Update forecast**. Also called: sales forecast, projected revenue, quarter forecast, landing.

### How do I see my team's pipeline?
To see your team's pipeline in Margince, open **Analytics** and pick the team in the **Record scope** picker above the numbers. A seat scoped to its teams, such as a team lead's, is offered **My teams**, each of those teams and its own records; a seat that sees everything is also offered **Whole company**. A rep who sees only their own records gets no picker, just "These numbers cover {scope}."
Also called: team forecast, my team's deals, manager view, team report.

### How do I see my pipeline report?
To see your pipeline report in Margince, click **Analytics** in the sidebar and open the **Pipeline analysis** section.
1. Click **Analytics**, then the **Pipeline analysis** tab.
2. Read **Open deals by stage** (deals, value and weighted value per stage), **Forecast categories** and **Open deals per company**.
3. Press **Explain this number** on any card to see the deals behind it.
The **Deals** board itself also shows each stage's total and weighted total in its column header. Also called: pipeline report, sales pipeline, deals by stage, funnel report.

### What analytics sections are there?
The main Analytics sections are **Performance**, **Forecast**, **Saved reports** and **Pipeline analysis**. **More analysis** holds **Data coverage** and **Custom reports**. Access determines which sections are available.
**Forecast** is where this period will land. **Pipeline analysis** holds the pipeline reports. **Performance** shows outcome totals, target progress when assigned, trends and pipeline charts. **My outcomes** shows your own open deals and meetings, for a rep only. **Data coverage** shows which sources the nightly check could read, for a seat allowed to see it. **Delivery** holds the project reports. Also called: reports, dashboards.

### How do I share a report view?
To share a forecast view in Margince, open **Analytics** → **Forecast**, press **Share view**, choose **Live view** or **Snapshot**, press **Create link**, then **Copy link**.
The link is shown only once and stops working after 30 days. Whoever opens it must sign in, and sees only what their own access allows. Only the Forecast section has **Share view**; Performance and saved sales reports offer CSV export. Opening a link shows the shared forecast under the recipient’s current permissions. Also called: send a report, share dashboard, report link.

### How do I close a shared forecast link?
To close a forecast link in Margince before its 30 days run out, open **Analytics** → **Forecast** and press **Shared links** beside **Share view**.
1. **Your shared links** lists every link you issued that still opens: **Live view** or **Snapshot**, the population it shows, and when it was created and when it expires.
2. Press **Close link** on the link's row, then **Close link** again in **Close this link?**.

The row leaves the list and anyone who opens the link is refused. Only you see and close the links you issued. Right after **Create link**, **Close link** in the **Your link** dialog does the same. A link also stops on its own after 30 days, or when you lose forecast access. Also called: revoke a share, cancel a report link, stop sharing, see my shared links.

### How do I see a win rate or export a report?
**Performance** offers a closed win rate with a minimum cohort and governed CSV export; see [Performance and saved sales reports](sales-reporting.md). To get deal rows out, open **Filters and views** in the sidebar, choose **Deals** as the **Record type**, build a filter, then press **Export CSV** or **Export JSON**. Also called: conversion rate, download report, export to Excel.

## The reporting sections

**Performance** · **Forecast** · **Saved reports** · **Pipeline analysis**

**More analysis** holds **Data coverage** and **Custom reports**. **Delivery** and **My outcomes** appear when the user has the relevant access.

When sales reporting is enabled, **Reports**, **Targets** and **Definitions**
join the graph-first Performance view. [Sales reporting](sales-reporting.md) covers
saving, scheduling and comparing editions.

Other Analytics sections are conditional, and their absence means
something:

- **My outcomes** appears only for a seat whose own records are its whole
  world — a rep. A manager never sees the tab, because the section answers for
  one seat and a manager's lens covers more. Reaching its address anyway says
  so: "This section measures one user’s records. Wider sections cover the rest
  of the lens."
- **Data coverage** appears where you hold the grant and the server answered at
  all — including on a fresh installation where no check has ever run, which the
  page then says in words rather than by being absent.

The section is part of the address, so a link to one opens it.

## Which records a number covers

Every Analytics reading is bounded by a population, and the screen always says
which.

Where you have more than one to choose from, it is a picker labelled **Record
scope**. Where you have exactly one — which is a rep's
usual case — it is a plain sentence instead: **"These numbers cover {scope}."**
A dropdown holding one option asks a question with one answer.

What you are offered comes from the server, labels and all, and the request is
checked again on the way in. Nothing in the browser decides who may measure
what.

- An unbounded seat is offered **Whole company**, every live team, and their
  own records.
- A team-scoped seat is offered **My teams**, each of their own live teams, and
  their own records.
- Everyone else is offered their own records.

Archived teams are never offered.

The scope picker governs Performance, Forecast and Questions. Pipeline analysis
and Delivery report cards use their own authorized populations; changing the
picker does not change those cards.

A report that counts every record still checks a named owner. Filtering one to
somebody you may not measure is refused. Breaking one down by owner counts only
the owners you may measure, and the answer says so.

## Under every report

Every Analytics report shows **"As of {asOf} · {zone}"** — the moment the figures describe, and the timezone
they were cut in. It is drawn only when the server sent both. Half a frame is
worse than none.

It names no currency on purpose: one report prints rows in their own currency
beside blocks converted into yours, and a single currency label over both would
be false for one of them.

## Explain this number

Every Analytics report card carries **Explain this number**. It opens **How this
number is built** and gives you three things:

1. **The server's own definition** of the filter, the grouping and the
   aggregate — in words, not as a query.
2. **Source rows** — the actual rows, scoped exactly as the report was. Money in
   minor units is formatted; a column the vocabulary knows is translated, and one
   it does not keeps its wire name rather than being given a wrong one.
3. **A staleness warning where one is owed.** If the link did not say when the
   number was worked out, the figures were recalculated just now, and the panel
   says so: "If an exchange rate changed since, they may not match the number
   you clicked."

Row ids are dropped only when *every* row carries a name. A partially labelled
set keeps them, so a row you may not read stays identifiable.

**Weighting is rounded per deal and then summed**, here and on the board, which
is why the drill-down reconciles exactly rather than being off by a rounding
error.

The Forecast section has no Explain control — it is not built from report cards.

## Deals

The **Pipeline analysis** section of Analytics holds the pipeline reports.

**Open deals by stage** — one row per stage, in pipeline order: the stage,
how many deals, unweighted and weighted. Every figure is converted server-side
into one base currency, so a stage is one row with no currency column.

**Forecast categories** — a tile per category, the raw total as the value and
the weighted total beneath it. A standing note, **How to read these tiles**, says: "Each
tile shows the raw total and, below it, the probability-weighted total. Rounding
is per deal, so it always matches Explain this number."

The **No category** tile appears only when something actually falls in it. A
tile with nothing in it says **"No deals"** — a word, never a dash.

**Open deals per company** — company, currency, open deals, unweighted. These
rows stay in their native currency and are grouped by company *and* currency, so
two currencies on one account are two rows.

Where not every deal carries an amount, a footnote says so: "{priced} of {total}
priced".

## Performance

**Performance** shows sales won, target progress when assigned, trends and pipeline
charts. Switch to **SDR outcomes** for held meetings and accepted opportunities.
Every chart opens its evidence and offers a numeric table. Small cohorts are
withheld rather than shown as zero. See [Performance and saved sales reports](sales-reporting.md).

## My outcomes

The **My outcomes** section of Analytics shows your own work, in two panels.

**My open deals** — deals and value, both opening the Deals section.

**My meetings** — meetings *you host*, counted by where each stands today:
**Booked, Held, No-show, Canceled.** The sub-line states the rule that trips
readers up: "A held meeting no longer counts as booked."

## Delivery

The **Delivery** section of Analytics holds the project reports.

**Projects by phase** — phase, projects, open deal value, won deal value.

**Project commitments** — project, phase, open, overdue.

**Projects inactive for 30 days** — project, phase, quiet since.

When there is nothing, the page says which nothing it means: "No projects yet.
A won deal creates one." or "No project in delivery has gone quiet."

## Data coverage

The **Data coverage** section of Analytics shows not what was found but **how
much could be looked at**: "Sources the nightly check could read, and how far. A
read source with no activity counts as checked; an unread source shows why."

Three columns: source, state, checked through. The six sources are the mailbox,
the calendar, documents, contracts, offers and the incumbent system.

Five states:

| State | What it means |
|---|---|
| **Checked** | Read, and to a date |
| **Stale, nothing read recently** | Reachable, but nothing recent came back |
| **Unavailable, the check could not read it** | It tried and failed |
| **Access needs renewal** | A permission lapsed |
| **Not connected** | Never wired up: nothing to fix, something to decide |

**"Checked through" carries a date only for a checked source.** A date on an
unread one would read as "checked up to then" when nothing was read at all.

A source missing from the list is one the run did not attempt, which is a
different fact from one it attempted and could not read.

If no check has ever run, the page says so rather than showing a healthy blank:
"No check has run yet. An unchecked installation is not the same as a healthy
one."

Record-level problems are not here. They are in the Forecast section's review.

## Forecast

The **Forecast** section of Analytics answers one question — where will this
period land — and it distinguishes three different kinds of claim rather than
blending them.

| Reading | What kind of claim it is |
|---|---|
| **Manager forecast** | Somebody's judgement. No forecast reads "No manager forecast submitted" |
| **Committed · confirmed dates** | Open Commit deals with a confirmed expected close date in the period |
| **Already won** | Value of deals actually closed won in the period |

The membership rules are worth knowing:

- **Won** counts by the day a deal actually closed, never the day it was
  expected to.
- **Open** means not won, carrying an expected close date, and that date falls
  in the window.
- **Committed · confirmed dates** means open, not provisional, and in Commit.
- A **provisional** close date is a guess. It is in the open pipeline and out of
  the evidence.

Where not every deal is priced, a callout headed **Not every deal is priced**
says what that costs: "{priced} of {eligible} deals are priced. Unpriced deals
add nothing to the totals above."

### The window

The forecast **Period** is **Quarter**, **Month** or **Week**. Quarter and month are cut against your financial
year, so an April-opening year has Q1 running April to June. **Week is the
working week, Monday to Sunday**, and is not a division of the financial year —
asking for a week gets a week, or a refusal, never a quarter quietly returned in
its place.

### Recording a call

**"A call is the amount you expect to close. It records your number and changes
no deal."**

To record a call, press **Update forecast**, fill **Expected total for this period**
(in the base currency) and a **Supporting note**, then press **Save forecast**. The call
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

The **Landing** card is won plus what is still to come. What "still to come"
means is the **Projection basis** installation setting, and the card names it:

- **Commit evidence** — confirmed close dates only. The default, and the
  strictest.
- **Weighted** — every open deal at its stage probability.
- **Manager’s call** — the authored number *replaces* the projection rather
  than adding to what is already won.

The two halves are disjoint by construction, so nothing is counted twice. A sum
that would overflow is refused rather than wrapped.

Two caveats it will tell you about rather than hide, under **Landing has a
caveat**:

- "No call recorded for this period, so this shows commit evidence."
- "The call is below the amount already won. It is shown as recorded, not
  corrected."

### Deal value needed

The **Deal value needed** card shows how much open pipeline it would take to
reach the reference, as a percentage with a meter beneath it.

**The reference comes from outside the current pipeline, on purpose.** A manager
call outranks history; failing that, the median of the last four completed
comparable periods. The reason is stated in the product itself: pipeline divided
by a target derived from that same pipeline is a number that is always fine.

**This is not a target.** Margince has no target model, and a figure a rep is
measured against is a product decision nobody has made.

Where it cannot be computed it says **No coverage figure for this period** and
which nothing it means, and shows no figures at all — a zero here would read as
a fully covered pipeline:

- No call is recorded, and fewer than four comparable periods have finished.
- "Too few closed deals for a conversion rate."

Neither the landing nor this figure is shown under **My teams**: a landing
summed across books called separately would describe none of them.

### The receipt

**Data and evidence checked**, the forecast receipt, shows four counts, and the last two are deliberately
separate:

| Count | What it means |
|---|---|
| **Eligible deals** | Everything the reading considered |
| **Priced** | Of those, how many carry an amount at all |
| **Close date confirmed** | Has an expected close date, and it is not provisional |
| **Exchange rate missing** | Priced, but no rate could convert it |

An unpriced deal has no amount. A priced deal no rate could reach has one and
still contributes nothing. Blurring the two would hide half the problem.

The review — **Checks before the call** — comes first, and the
receipt after it. A manager with ten minutes reads what needs doing; the receipt
is what they consult when a number looks wrong.

## Sharing a view

To share an Analytics view, press **Share view** beside the tabs on the
**Forecast** section. **Share this view** asks **What the link shows** — **Live
view** or **Snapshot** — then **Create link** mints it and **Copy link** copies
it.

Read the model before you rely on it: **a share widens who knows a link exists,
never what anybody is allowed to read.** Whoever opens it must be signed in as
themselves, and the numbers are computed under *their* grants. Two colleagues
opening one link can correctly see different figures.

The link is shown once — "The link is shown only once. Copy it now; it cannot be
retrieved later." — and stops working after 30 days.

**Shared links**, beside **Share view**, opens **Your shared links**: every link
you issued that still opens, with its kind, the population it shows, and when it
was created and when it expires. **Close link** on a row ends that link at any time,
after **Close this link?** asks. Only you can see and close your own links. The
list never shows a link's address, so a lost link cannot be copied from it. A
link also stops when it expires or when you lose forecast access, and a seat
without the permission to share sees neither button.

Every refusal is the same **not found**, whether the
token is unknown, expired, revoked, or issued by somebody who has since left:
distinguishing them would tell anyone working through guessed tokens which
guesses were closer.

An agent can neither mint one nor open one.

Sharing is available for the quarter forecast. Live links open the shared forecast under the recipient's current permissions.
The snapshot choice is available when the selected scope has a stored capture.
Use **Shared links** to review issued links and **Close link** to revoke one.

The manager forecast is a full-period total. Its gap compares against already-won sales plus committed open deals with confirmed close dates, in the same currency and period. It is not compared only with the open component. The composition is not guaranteed revenue. **My outcomes → My meetings** counts meetings you host by their current status, without a date filter; use **Performance → SDR outcomes** for period totals.
