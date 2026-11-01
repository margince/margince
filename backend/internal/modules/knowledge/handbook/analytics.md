<!-- prose:plain -->
<!-- describes: frontend/src/features/analytics backend/internal/modules/reporting backend/internal/modules/forecasting -->
# Analytics and forecasting

**Analytics** in Margince is the reporting screen. It shows forecast, pipeline reports, sales performance, time in stage, your own outcomes, delivery and data coverage. It is in the sidebar under **Insights**. It answers two different kinds of question and keeps them apart: what the records say, and what somebody expects will happen.

Every number here opens the records it was made from. If a figure looks wrong, open it rather than guessing.

## Common reporting tasks

### Where can I see my forecast?
To see your forecast in Margince, click **Analytics** in the sidebar (under **Insights**) and open the **Forecast** section.
1. Click **Analytics**, then **Forecast** under **Analytics sections**.
2. Choose the **Period**: **Quarter**, **Month** or **Week**.
3. Read the **Period forecast** and what it is made of: **Already won**, **Committed · confirmed dates** and **Best-case addition**.
   The forecast gap sets the manager’s total against won sales plus confirmed committed deals.
   Review **Checks before the call**; source details are under **Data and evidence checked**.
A manager records a call with **Update forecast**. Also called: sales forecast, projected revenue, quarter forecast, landing.

### How do I see my team's pipeline?
To see your team's pipeline in Margince, open **Analytics** and pick the team in the **Record scope** picker above the numbers.
Every seat is offered **Whole company**, every live team and its own records; a seat whose scope is its teams also gets **My teams** first.
Totals are the same for everyone. You can read a team's totals without being in it, but only the owners you may measure show by name.
A rep opens on their own records.
Also called: team forecast, my team's deals, manager view, team report.

### How do I see my pipeline report?
To see your pipeline report in Margince, click **Analytics** in the sidebar and open the **Pipeline** tab.
1. Click **Analytics**, then the **Pipeline** tab.
2. Read the totals at the top. Then read **Open deals by stage**, **Open pipeline by close month**, **Forecast categories** and **Open deals per company**.
3. Press **Explain this number** on any card to see the deals behind it.
The **Deals** board itself also shows each stage's total and weighted total at the top of its column. Also called: pipeline report, sales pipeline, deals by stage, funnel report.

### What analytics sections are there?
The Analytics tabs are **Overview**, **Performance**, **SDR outcomes**, **Leads**, **Partners**, **Forecast**, **Pipeline**, **My outcomes** and **Delivery**. At the end of the tab row, **Reports** opens the saved reports and the **Setup** menu holds **Targets**, **Metric definitions** and **Data coverage**. Access decides which tabs and menu entries you get. The date range and pipeline you choose stay set when you change tabs.

The **Forecast** section is where this period will land. The **Pipeline** section holds the pipeline reports. When you can see no deal yet, Performance, Pipeline and Forecast say **No deals to report on yet** and offer **Open Deals**.

The **Performance** section shows outcome totals, target progress when targets are set, trends and pipeline charts. It also shows a **Needs your attention** list when forecast checks, unpriced deals or unread data sources need you.

The list names up to five open forecast checks for the records you picked. Each shows the deal, what the check found, the owner and the money in question. High checks come first, then the largest amounts. **Review** opens the forecast review. When there are more checks, one line counts them all.

A check is **Out of date** when a record it read changed after it ran. It reads deals with their offers, contracts and activities, buying signals with their company, stages and forecast calls. The verdict and findings stay as the check left them, with the mark beside them, and the verdict loses its colour. **Check again** reads the changes now, and the next nightly check reads them too. A check that could not read every source, the mailbox included, clears no mark. Until a check reads every source, the review says it cannot tell whether anything changed.

A change to one deal also starts a check of that deal alone. When it lands, it clears that deal's mark, and the screen does not wait for it. A manager forecast is checked against the pipeline it was recorded over. **Forecast call needs review** names the call and the deals that moved.

A deal moved if its amount changed, it entered or left the period, or it left or came back to the qualified forecast. Age alone never flags a call. If the team, base currency or financial year changed since, the finding says the call cannot be compared. A call recorded before Margince kept what it was made against says that what moved since cannot be said.

The **Reports** section lists saved reports and starts a custom report with **New custom report**. Above them, **Prepared reports** offers ready reports to **Preview**, **Save a copy** of, or **Open** where they already live. A seat that cannot read saved reports gets a **Custom reports** button there instead. A custom report on deals can also group and filter by a deal or company field. An admin turns such a field on under **In reports** in Settings → **Fields**. **Your saved questions**, under a new custom report, lists the custom reports you saved, newest first, each named by what it asks.

A company field shows as its name with "(company)", and groups each deal by its company's value. A deal whose company you cannot open is left out of that grouping. Under **Group by**, the report says how many of the deals you can read have a value in each such field. A grouping by a field few deals hold puts most deals in the empty group. If an admin turns the field off or retires it, a saved report that uses it says so instead of showing numbers. Ask again without the field, or ask an admin to turn it back on.

A deal's own amount is in that deal's currency, and amounts in two currencies cannot be added. So a custom report that totals, averages, or takes the smallest, largest or middle deal amount must group by currency or filter to one currency. For one total across currencies, measure the amount in the base currency, which converts each deal at its own rate. A deal whose currency has no rate is left out of that total. A count of the base amount says how many deals it covers. A saved report that adds amounts across currencies says so when you open it, instead of showing a number.

The **My outcomes** section shows your own targets, open deals and meetings, for a rep only. The **Data coverage** section shows which sources the nightly check could read, for a seat allowed to see it. The **Delivery** section holds the project reports. Also called: reports, dashboards.

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
To get deal rows out, open **Filters and views** → **New filter** → **Deals** and make a filter. Then choose **⋯** (**More for this filter**) → **Export CSV** or **Export JSON**. Also called: conversion rate, download report, export to Excel.

## The reporting sections

When sales reporting is turned on, **Reports** and the **Setup** menu (**Targets** and **Metric definitions**) follow the Analytics tabs. The Performance, SDR outcomes, Leads and Partners tabs lead with graphs. Saving, scheduling and comparisons of editions are in [Sales reporting](sales-reporting.md).

Two sections appear only for some seats. **My outcomes** is for a seat that sees only its own records, such as a rep. Under a wider view, it says it measures one user’s records. **Data coverage** is for a seat with permission to see it, even on a new installation where no check has run yet. The section is part of the address, so a link to one opens it.

## Overview

Analytics opens on **Overview**, the first tab, which has two views: **Sales review** and **My prospecting**. A reader who measures a team also gets **Team prospecting**. The view a link names opens first, then the one you chose last. With neither, a reader who measures their own records in a company set up for SDR teams gets **My prospecting**. Everyone else gets **Sales review**. Each view shows its readings, a few sentences on what they mean, and what needs you.

**Sales review** opens on this fiscal quarter. The record scope and the period stand at the top. The pipeline and the expected close window sit behind **Filters**, and two chips under the controls always show what they hold. **Save view**, **Report templates** and **Export CSV** sit behind **More**. The line under the filters names the whole quarter, such as "1 October – 31 December", and the time the results run up to.

**Sales won** leads, beside a line of its total over the period. Under it you see how much of the quarter's target it reached and what is still to win. **Projected shortfall** is the target minus the expected finish. With enough history, the same point of the last period shows too. With no sales target there is no gap to show, and **Set targets** shows to a seat that may set them. A target set for a different scope, such as one team, says why it cannot compare and offers a view that can.

Three readings follow. **Expected finish** names what it counts, such as **Won + confirmed commit** or **Manager’s call**, and a call it does not use is shown, not added. **Coverage** names both of its amounts: the open value due to close this quarter, over what is still to win. **Closed win rate** is the share of closed deals that were won. **Summary** then says where the quarter stands and how many forecast checks are open, and each sentence opens what it rests on. While a figure or the checks could not be read, no sentence says the quarter is on target.

**Needs your attention** names the three most urgent open forecast checks. Each says which deal to review and why, who owns it (or **Unassigned**), when it was found and how much money it puts in question. **Answer** opens that check on the forecast review, and a seat that may only read it gets **Open**. Below, all open checks are counted by kind, in checks and in deals, and **Show** lists one kind. The checks cover every pipeline and close date of the record scope. A data source the last check could not read shows as **Connection maintenance**, apart from the checks.

A bar in **Summary** shows what the expected finish is made of, the target, and the deals it left out. **Modelled quarter finish** is the span that 8 in 10 runs of the forecast end in, not a margin around the expected finish. It is marked on the same bar, because it also counts open deals by their probability to close, so it can sit higher. On another period, **Show this quarter** brings the comparison back. **All open pipeline** and the open pipeline by stage follow **Summary**, and sales won by team member, in name order, follows **Needs your attention**. With **Show individual performance** off, the team member chart shows no names, and **Since the last capture** compares once there are two captures.

**My prospecting** opens on this week, and **Your results** leads with the outcome your company counts. That is **Accepted handoffs** when an SDR hands prospects on, and **Leads qualified** when nobody does. When Margince cannot read which applies, it says so and offers **Retry**. Beside the figure, your last six weeks show one column each, and a week with no reading stays empty. Under it, **Monthly targets** names its own month, so a week is never set against a month. Three readings follow: **Meetings held**, **First response time** with the leads still waiting, and **Booked meetings** for 14 days.

A zero, a hidden figure, a failed one and a day that cannot be read each say so in their own words. **Summary** and **Do next** sit beside your results, and **Open** on a Worklist item goes to its record, where you complete it. **Handoffs waiting on others** names each recipient, how long the handoff has waited since it was last sent, and whether the recipient can still act. A handoff sent back to you shows the reason given; without SDR handoffs, **Meetings to prepare** lists your booked meetings instead. Lower down, **How your outcomes developed** shows each week, and **What happened to your handoffs** follows the ones you sent in the period. **Save view** keeps the view as a report under **Reports** without changing Overview, and **Report templates** opens prepared reports such as **My prospecting progress**.

**Team prospecting** has the record scope, a **Date range** of this or last week or month, and **More**. Its readings lead with accepted handoffs where the company uses SDR handoffs, and with qualified leads where it does not. **Where the team stands** says how the rounds handed on in the period, or the leads that arrived in it, ended so far. A share shows only when at least five rounds or leads are behind it. Under it come what holds the team up: waiting handoffs, handoffs sent back or turned down, and leads waiting for a reply. Each opens its own list, and the reasons count the decisions made in the period.

**Members** has one row per colleague you may measure, in name order, with the columns of the company’s way of working. With qualified leads as the main outcome, a row shows qualified leads, meetings held, leads waiting for a reply and the member’s own monthly target. A lead counts for the colleague who owns it now, a handoff for whoever handed it on, and the team is today’s team. The rows need not add up to the team: a lead with no owner is in no row, and a shared meeting is in each. A figure your role may not see is named under the table, never shown as zero, and with **Show individual performance** off no row shows. A total of one colleague’s work is held back, and a team of you and one colleague hides its readings and movement, with the reason.

Each view keeps its own record scope, pipeline, period, close window and dates. **All pipelines** on Sales review is still set when you come back from the other views. A link that names a view and its filters opens what it names and changes nothing you chose. What you chose is kept per account in this browser until you sign out. The links out of a view carry its filters, and Back returns to the same filters, place and card. Forecast covers every pipeline, so it says so first when the figure you came from counts one pipeline of several.

A link to figures you may not see says **This link names figures you cannot open** instead of showing other figures.

## Which records a number covers

Every Analytics reading covers a set of records, and the screen always says which. Where you have more than one to choose from, it is a picker labelled **Record scope**. Where you have only one, which is the usual case for a rep, it is a plain sentence instead: **"These numbers cover {scope}."**

Every seat is offered **Whole company**, every live team and its own records.
Totals for won, pipeline, forecast and meetings are the same for everyone. A seat whose scope is its teams also gets **My teams** first. Archived teams are never offered. A rep still opens on their own records.

A total counts the whole scope you picked. An owner chart, ranking or list shows only the owners you may measure, and the answer says so. A scope with no deals says so instead of showing zeros, and points you to **Whole company** if you own none.

The scope picker rules Performance, Forecast, Pipeline and Custom reports, so these sections count the same records. Delivery does not follow the picker: it covers every project you can read, and says so on the page.

A report that counts every record still checks a named owner. Filtering one to
somebody you may not measure is refused. Grouping one by owner counts only
the owners you may measure, and the answer says so. With **Show individual performance** off, grouping or filtering by a colleague is refused. So is a figure holding only one colleague's work besides yours. See [Can we stop Margince from showing each colleague's performance?](compliance.md#can-we-stop-margince-from-showing-each-colleagues-performance).

## Under every report

Every Analytics report shows **"As of `{asOf}` · {zone}"**: the moment the
figures describe, and the time zone they were cut in. If either value is
missing, this line is not shown.

It names no currency, because one report can show deals in their own currency
beside totals turned into yours.

Activity reports count meetings and tasks, never email. Messages, calls and
notes are not counted either. An activity report or custom question that asks
for one of those kinds is refused, and the refusal names the kinds you can use.

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

Record IDs are hidden only when *every* row carries a name. If some rows
have no name, the IDs stay, so a record you may not open can still be
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

**Open pipeline by close month**: deals, value and weighted value per expected
close month, in calendar order. **No close date** comes last.

**Open deals per company**: company, currency, open deals, value without
weighting, and share of open pipeline. These rows stay in their own currency
and are grouped by company *and* currency. So two currencies on one company are
two rows.

The share is of the open pipeline in the same currency. It shows only when the
list holds every row and no deal is hidden from you.

Where not every deal has an amount, a note says "{priced} of {total} priced".

## Performance

**Performance** shows sales won, target progress when targets are set, trends and pipeline charts. **View records**, at the foot of every chart, opens its evidence, and each chart offers a table of numbers. Small groups are held back rather than shown as zero. See [Performance and saved sales reports](sales-reporting.md).

The **SDR outcomes** tab shows held meetings, accepted opportunities, leads qualified, first response time and the leads still waiting for a first reply. **Leads** shows new leads, status changes and the same three lead figures. **Partners** shows partner pipeline, sales won, commission and active partners. A tab is not offered when your role cannot read its figures.

## My outcomes

The **My outcomes** section of Analytics shows your own work, in up to six panels.

**My targets**: each figure that can take a target, against your own target for this month or this fiscal quarter. A card reads "10 of 15", with the dates the target covers and "67% of target" under it. A figure with no target says **No target set** and shows the month so far. Only targets on your own name for all pipelines show here; team, company and single-pipeline targets are listed under **Targets**. **My open deals**: deals and value, both opening the Deals section.

**My meetings**: meetings *you host*, counted by where each stands today. The states are **Booked, Held, No-show, Canceled.** The line under it states the rule that readers often get wrong: "A held meeting no longer counts as booked." The count has no date filter; use **Analytics → SDR outcomes** for period totals.

**My leads this month**: leads qualified, first reply time for answered leads only, and the leads still waiting for a first reply. The definition of leads qualified is shown above the figures. Where the workspace is not set up for SDR handoffs, the panel also shows **Meetings coming up**.

**Handoffs waiting on others** appears only when Analytics is set up for SDR teams. It counts the handoffs you sent that nobody has decided, the longest wait, those in the shared queue and those returned to you. A handoff returned and sent again waits from the new sending. One whose recipient can no longer act is counted as blocked, and a manager must reassign it. A prospect you cannot open is listed without its name. Only the age is shown, because no handoff target time is set.

**Waiting on you**: the first items of your Worklist, in its order. When a source could not be read, the panel says the list may be missing items and never says that nothing needs you.

## Delivery

The **Delivery** section of Analytics holds the project reports.

**Projects by phase**: phase, projects, open deal value, won deal value. **Open project tasks**: project, phase, open tasks, overdue tasks. **Projects inactive for 30 days**: project, phase, quiet since.

When there is nothing, the page says which nothing it means: "No projects yet. Create one under
Projects with New project. A won deal moves its project to Delivering." or "No project in delivery has gone quiet."

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

To record a call, press **Update forecast**. Fill **Expected total for this period** (in the base currency) and a **Supporting note**, then press **Save forecast**. The call is filed against the period *and* the records you are looking at.

Each new call is kept beside the ones before it. None is written over, and the first call of a period is recorded the same way.

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
- **Weighted**: every open deal at its probability ([how it is set](forecast-learning.md)).
- **Manager’s call**: the recorded number *replaces* the projection rather
  than adding to what is already won.
  **From the deal records** shows won plus commit evidence beside it, never added.

No deal is counted in both parts.

The card tells you about two caveats under **Landing has a caveat**:

- "No call recorded for this period, so this shows commit evidence."
- "The call is below the amount already won. It is shown as recorded, not
  corrected."

### Deal value needed

The **Deal value needed** card shows how much open pipeline it would take to
reach the reference. It shows this as a number out of 100, with a bar under it.

The reference comes from outside the current pipeline. A manager call comes
first. Without one, it takes the amounts won in the last four finished periods
of the same kind. It sorts them and takes the average of the two middle ones. Measuring open deals against a figure worked out from those same
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

A deal whose customer stopped answering gets a review here, whatever its close
date: see [Deals the customer still supports](deals-the-customer-supports.md).

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

A link that no longer works shows **not found**, no matter the reason. An agent can neither create one nor open one.
