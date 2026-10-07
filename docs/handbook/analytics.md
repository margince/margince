<!-- prose:plain -->
# Analytics and forecasting

**Analytics** in Margince is the reporting screen. It shows forecast, pipeline
reports, sales performance, time in stage, your own outcomes, delivery and data
coverage. It is in the sidebar under **Intelligence**. It answers two different
kinds of question and keeps them apart: what the records say, and what somebody
expects will happen.

Every number here opens the records it was made from. If a figure looks wrong,
open it rather than guessing.

## Common reporting tasks

### Where can I see my forecast?
To see your forecast in Margince, click **Analytics** in the sidebar (under **Intelligence**) and open the **Forecast** section.
1. Click **Analytics**, then **Forecast** under **Analytics sections**.
2. Choose the **Period**: **Quarter**, **Month** or **Week**.
3. Read the **Period forecast** and what it is made of: **Already won**, **Committed · confirmed dates** and **Best-case addition**.
   The forecast gap sets the manager’s total against won sales plus confirmed committed deals.
   Review **Checks before the call**; source details are under **Data and evidence checked**.
A manager records a call with **Update forecast**. Also called: sales forecast, projected revenue, quarter forecast, landing.

### How do I see my team's pipeline?
To see your team's pipeline in Margince, open **Analytics** and pick the team in the **Record scope** picker above the numbers.
A seat whose scope is its teams, such as a team lead's, is offered **My teams**, each of those teams and its own records. A seat that sees everything is also offered **Whole company**.
A rep who sees only their own records gets no picker, only "These numbers cover {scope}."
Also called: team forecast, my team's deals, manager view, team report.

### How do I see my pipeline report?
To see your pipeline report in Margince, click **Analytics** in the sidebar and open the **Pipeline** tab.
1. Click **Analytics**, then the **Pipeline** tab.
2. Read the totals at the top, then **Open deals by stage** (deals, value and weighted value per stage), **Forecast categories** and **Open deals per company**.
3. Press **Explain this number** on any card to see the deals behind it.
The **Deals** board itself also shows each stage's total and weighted total at the top of its column. Also called: pipeline report, sales pipeline, deals by stage, funnel report.

### What analytics sections are there?
The Analytics sections are **Performance**, **Forecast**, **Saved reports** and **Pipeline analysis**. **More analysis** holds **Data coverage** and **Custom reports**. Access decides which sections you get.
The **Forecast** section is where this period will land. The **Pipeline analysis** section holds the pipeline reports.

The **Performance** section shows outcome totals, target progress when targets are set, trends and pipeline charts.
The **My outcomes** section shows your own open deals and meetings, for a rep only.
The **Data coverage** section shows which sources the nightly check could read, for a seat allowed to see it. The **Delivery** section holds the project reports. Also called: reports, dashboards.

### How do I share a report view?
To share a forecast view in Margince, open **Analytics** → **Forecast** and press **Share view**. Choose **Live view** or **Snapshot**, press **Create link**, then **Copy link**.
The link is shown only once and stops working after 30 days. Whoever opens it must sign in, and sees only what their own access allows.

Only the Forecast section has **Share view**; Performance and saved sales reports offer CSV export. Opening a link shows the shared forecast under the current permissions of whoever opens it. Also called: send a report, share dashboard, report link.

### How do I close a shared forecast link?
To close a forecast link in Margince before its 30 days run out, open **Analytics** → **Forecast** and press **Shared links** beside **Share view**.
1. **Your shared links** lists every link you issued that still opens: **Live view** or **Snapshot**, and the records it shows.
   It also shows when the link was created and when it expires.
2. Press **Close link** on the link's row, then **Close link** again in **Close this link?**.

The row leaves the list, and anyone who opens the link is refused. Only you see and close the links you issued. Right after **Create link**, **Close link** in the **Your link** dialog does the same.
A link also stops on its own after 30 days, or when you lose forecast access. Also called: revoke a share, cancel a report link, stop sharing, see my shared links.

### How do I see a win rate or export a report?
**Performance** offers a closed win rate with a smallest group size, and a CSV export under its rules; see [Performance and saved sales reports](sales-reporting.md).
To get deal rows out, open **Filters and views** in the sidebar and choose **Deals** as the **Record type**. Make a filter, then press **Export CSV** or **Export JSON**. Also called: conversion rate, download report, export to Excel.

## The reporting sections

When sales reporting is turned on, **Reports**, **Targets** and **Definitions**
join the Performance view, which leads with graphs. Saving, scheduling and
comparisons of editions are in [Sales reporting](sales-reporting.md).

Two sections appear only for some seats, and when they are missing, that means
something:

- **My outcomes** appears only for a seat that sees only its own records, such
  as a rep. A manager never sees the tab, because the section answers for one
  seat and a manager's view covers more. If you open its address, it says so:
  "This section measures one user’s records. Wider sections cover the rest of
  the lens."
- **Data coverage** appears where you have permission to see it. It appears
  even on a new installation where no check has ever run, and the page then
  says so in words.

The section is part of the address, so a link to one opens it.

## Which records a number covers

Every Analytics reading covers a set of records, and the screen always says
which.

Where you have more than one to choose from, it is a picker labelled **Record
scope**. Where you have only one, which is the usual case for a rep, it is a
plain sentence instead: **"These numbers cover {scope}."**

- A seat that sees everything is offered **Whole company**, every live team,
  and its own records.
- A seat whose scope is its teams is offered **My teams**, each of its own live
  teams, and its own records.
- Everyone else is offered their own records.

Archived teams are never offered.

The scope picker rules Performance, Forecast and Custom reports. Pipeline
analysis and Delivery report cards use their own record scopes; changing the
picker does not change those cards.

A report that counts every record still checks a named owner. Filtering one to
somebody you may not measure is refused. Grouping one by owner counts only
the owners you may measure, and the answer says so.

## Under every report

Every Analytics report shows **"As of `{asOf}` · {zone}"**: the moment the
figures describe, and the time zone they were cut in. If either value is
missing, this line is not shown.

It names no currency, because one report can show deals in their own currency
beside totals turned into yours.

## Explain this number

Every Analytics report card carries **Explain this number**. It opens **How this
number is built** and gives you three things:

1. **The definition** of the filter, the grouping and the total, in words.
2. **Source rows**: the records behind the number, limited to what the report
   covered. Amounts are shown as money. Column names are put into your words
   where Margince knows them; others keep their original name.
3. **A warning where the figures may be old.** If the link did
   not say when the number was worked out, the figures were worked out again
   right now. The panel says so: "If an exchange rate changed since, they may
   not match the number you clicked."

Record numbers are hidden only when *every* row carries a name. If some rows
have no name, the numbers stay, so a record you may not open can still be
picked out.

Weighting is rounded per deal and then added up, here and on the board. So the
rows you open add up to the same number, with no rounding gap.

The Forecast section has no Explain control, because it is not built from
report cards.

## Deals

The **Pipeline** section of Analytics holds the pipeline reports, under totals
for the whole open pipeline.

**Open deals by stage**: one row per stage, in pipeline order. It shows the
stage, how many deals, and the value without and with weighting. Every figure
is turned into one base currency, so a stage is one row with no currency
column.

**Forecast categories**: a tile per category, with the raw total as the value
and the weighted total under it. A note that always shows, **How to read these
tiles**, says: "Each tile shows the raw total and, below it, the
probability-weighted total. Rounding is per deal, so it always matches Explain
this number."

The **No category** tile appears only when a deal falls in it. A tile with
nothing in it says **"No deals"**, never an empty mark.

**Open deals per company**: company, currency, open deals, value without
weighting. These rows stay in their own currency and are grouped by company
*and* currency. So two currencies on one company are two rows.

Where not every deal carries an amount, a note says so:
"{priced} of {total} priced".

## Performance

**Performance** shows sales won, target progress when targets are set, trends
and pipeline charts. Switch to **SDR outcomes** for held meetings and accepted
opportunities. Every chart opens its evidence and offers a table of numbers.
Small groups are held back rather than shown as zero. See [Performance and saved
sales reports](sales-reporting.md).

## My outcomes

The **My outcomes** section of Analytics shows your own work, in two panels.

**My open deals**: deals and value, both opening the Deals section.

**My meetings**: meetings *you host*, counted by where each stands today.
The states are **Booked, Held, No-show, Canceled.** The line under it states
the rule that readers often get wrong: "A held meeting no longer counts as
booked." The count has no date filter; use **Performance → SDR outcomes** for
period totals.

## Delivery

The **Delivery** section of Analytics holds the project reports.

**Projects by phase**: phase, projects, open deal value, won deal value.

**Project commitments**: project, phase, open, overdue.

**Projects inactive for 30 days**: project, phase, quiet since.

When there is nothing, the page says which nothing it means: "No projects yet.
A won deal creates one." or "No project in delivery has gone quiet."

## Data coverage

The **Data coverage** section of Analytics shows how much could be looked at,
not what was found. It says "Sources the nightly check could read, and how far. A read
source with no activity counts as checked; an unread source shows why."

It has three columns: source, state, checked through. The six sources are the
mailbox, the calendar, documents, contracts, offers and the old system you
moved from.

Five states:

| State | What it means |
|---|---|
| **Checked** | Read, and to a date |
| **Stale, nothing read recently** | Can be reached, but nothing recent came back |
| **Unavailable, the check could not read it** | It tried and failed |
| **Access needs renewal** | A permission ran out |
| **Not connected** | Never connected: nothing to fix, something to decide |

"Checked through" carries a date only for a checked source. A date on an
unread one would read as "checked up to then" when nothing was read at all.

A source missing from the list is one the run did not try. That is different
from one it tried and could not read.

If no check has ever run, the page says so rather than showing a healthy blank:
"No check has run yet. An unchecked installation is not the same as a healthy
one."

Problems with single records are shown in the review of the Forecast section.

## Forecast

The **Forecast** section of Analytics answers one question (where will this
period land?) and keeps three kinds of claim apart.

| Reading | What kind of claim it is |
|---|---|
| **Manager forecast** | Somebody's view. No forecast reads "No manager forecast submitted" |
| **Committed · confirmed dates** | Open Commit deals with a confirmed expected close date in the period |
| **Already won** | Value of deals that closed won in the period |

Which deals count:

- **Won** counts by the day a deal closed, never the day it was expected to.
- **Open** means not won, carrying an expected close date, and that date falls
  in the period.
- **Committed · confirmed dates** means open, not provisional, and in Commit.
- A **provisional** close date is a guess. It is in the open pipeline and out of
  the evidence.

The manager forecast is a total for the full period. Its gap sets it against
sales already won plus committed open deals with confirmed close dates. That
is in the same currency and period, and never against the open deals alone.
What the forecast is made of is not guaranteed revenue.

Where not every deal is priced, a box titled **Not every deal is priced**
says what that costs: "{priced} of {eligible} deals are priced. Unpriced deals
add nothing to the totals above."

### The window

The forecast **Period** is **Quarter**, **Month** or **Week**. Quarter and
month follow your financial year. So in a year that opens in April, the first
quarter runs April to June. **Week** is the working week, Monday to Sunday, and is not a
part of the financial year. Asking for a week gets a week or a refusal, never a
quarter in its place.

### Recording a call

**"A call is the amount you expect to close. It records your number and changes
no deal."**

To record a call, press **Update forecast**. Fill **Expected total for this
period** (in the base currency) and a **Supporting note**, then press **Save
forecast**. The call is filed against the period *and* the records you are
looking at.

Each new call is kept beside the ones before it. None is written over, and the
first call of a period is recorded the same way.

Who may record one: not a rep. The manager forecast is a claim about a team or
the whole company. So a seat that sees only its own records does not get the
editor at all. Nor does **My teams**: a forecast names one team or the whole
company, and **My teams** covers several. Margince does not guess which team.

A call for records you have no authority over answers **not found** rather
than "forbidden", so it does not confirm that they exist.

### Projected landing

The **Landing** card is won plus what is still to come. What "still to come"
means is the **Projection basis** installation setting, and the card names it:

- **Commit evidence**: confirmed close dates only. The default, and the
  strictest.
- **Weighted**: every open deal at its stage probability.
- **Manager’s call**: the recorded number *replaces* the projection rather
  than adding to what is already won.

No deal is counted in both parts.

The card tells you about two caveats under **Landing has a caveat**:

- "No call recorded for this period, so this shows commit evidence."
- "The call is below the amount already won. It is shown as recorded, not
  corrected."

### Deal value needed

The **Deal value needed** card shows how much open pipeline it would take to
reach the reference. It shows this as a number out of 100, with a bar under it.

The reference comes from outside the current pipeline. A manager call comes
first. Without one, it is the middle value of the last four finished periods of
the same kind. Measuring open deals against a figure worked out from those same
deals would always look like enough.

This figure is not a target. Targets are set on their own under **Analytics →
Performance → Targets**; see [Performance and saved sales reports](sales-reporting.md).

Where it cannot be worked out, it says **No coverage figure for this period**
and why. It shows no figures at all, because a zero here would read as a
pipeline with all it needs:

- No call is recorded, and not yet four periods of the same kind have
  finished.
- "Too few closed deals for a conversion rate."

Neither the landing nor this figure is shown under **My teams**. A total of
teams that made their own calls would describe none of them.

### The receipt

**Data and evidence checked**, the forecast receipt, shows four counts. The last
two are kept apart:

| Count | What it means |
|---|---|
| **Eligible deals** | Everything the reading looked at |
| **Priced** | Of those, how many carry an amount at all |
| **Close date confirmed** | Has an expected close date, and it is not provisional |
| **Exchange rate missing** | Priced, but no rate could turn it into the base currency |

An unpriced deal has no amount. A priced deal that no rate could turn has one,
and still adds nothing. Counting them together would hide part of the problem.

The review, **Checks before the call**, comes first and the receipt after it.
A manager with 10 minutes reads what needs doing; the receipt is what they
open when a number looks wrong.

## Sharing a view

To share an Analytics view, press **Share view** beside the tabs on the
**Forecast** section. **Share this view** asks **What the link shows** (**Live
view** or **Snapshot**). Then **Create link** creates it and **Copy link**
copies it. **Snapshot** is offered when the chosen scope has a stored
capture.

A share lets more colleagues know a link exists; it never changes what anybody
is allowed to read. Whoever opens it must sign in with their own account, and the
numbers are worked out under *their* permissions. Two colleagues opening one
link can rightly see different figures.

The link is shown once ("The link is shown only once. Copy it now; it cannot be
retrieved later.") and stops working after 30 days.

**Shared links**, beside **Share view**, opens **Your shared links**. It lists
every link you issued that still opens, with its kind and the records it shows.
It also shows when each was created and when it expires. **Close link** on a
row ends that link at any time, after **Close this link?** asks. Only you can
see and close your own links. The list never shows a link's address, so a lost
link cannot be copied from it.

A link also stops when it expires or when you lose forecast access. A seat
without permission to share sees neither button.

A link that no longer works shows **not found**, no matter the reason.

An agent can neither create one nor open one.
