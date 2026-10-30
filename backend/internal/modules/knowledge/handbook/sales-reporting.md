<!-- prose:plain -->
<!-- describes: frontend/src/features/analytics frontend/src/features/settings/lossreasons.tsx backend/internal/modules/deals/lossreasonmapping.go backend/internal/modules/reporting backend/internal/modules/forecasting backend/internal/compose/analyticsquery backend/internal/compose/weekly -->
# Performance and saved sales reports

## Where do I start my sales review?

Open **Analytics → Performance** for sales won, pipeline stages, sales by salesperson and time in stage. The **SDR outcomes** tab shows meetings confirmed as held and accepted opportunities. Choose your record scope and time span. Stage charts need a single pipeline. A rep's own scope shows their own results; you need wider access to compare owners.

Sales won follows the close time span you picked and the deal owner of today. If a deal gets a new owner, its live sales and target results move to the new owner. Saved snapshots keep the owner and results from that time. Open pipeline and stage age show the pipeline as it is now, with the time window for expected closes printed next to the chart.

Above the figures, a few sentences sum up the position. They say what was won against the target, where the quarter should end, and what is still open. They are written from the figures below them, and a figure that was not measured is left out.

The row of figures at the top shows sales won and open pipeline, with the number of open deals and the date it was read. With the time span set to this quarter, it also shows **Forecast landing**. That is where the deal forecast expects this fiscal quarter to end, and it needs forecast access. Click sales won or open pipeline to open the records behind it. Click **Forecast landing** to open the **Forecast** section, where each part links to its deals.

A salesperson's target is for the month or the fiscal quarter that holds the end of the time span you picked. If the cutoff of today comes first, it is the month or quarter that holds that cutoff. It is compared with the real result for that month or quarter. Changing the time span does not turn today's pipeline into history.

“This week”, “This month” and “This quarter” follow the calendar of your Margince and start again when that time span starts.

A week starts on Monday at 00:00. At that exact moment the week holds nothing yet, and Analytics says so instead of showing zeros. With “This week”, a monthly target still counts the whole month up to the cutoff.

The dates next to the numbers name the time span measured. A custom time span can be up to 12 months long. If the end date is today or later, real results stop at the reporting cutoff of today, shown above the charts. Future sales are not counted. A time span that is all in the future is refused; choose a start date before the cutoff.

Targets use the month or fiscal quarter that holds the real cutoff, even when the custom end date you asked for is later. A saved edition keeps its own cutoff.

Every chart can also be read as numbers. Select a mark, with a click or the keys, to open the records behind it; press `Esc` to close the side panel. A record you may not see stays hidden. Stage age records stay inside the reporting scope you picked.

## How far is this quarter from its target?

With the time span set to this quarter, **This quarter against its target** sits under the row of figures. It holds the forecast landing against the sales won target of the same scope and quarter:

- **Gap to target** is the target minus the forecast landing. A manager's call is the whole quarter, so it is not added to what is already won.
- **Left to win** is the target minus sales won so far, and never below zero.
- **Coverage, all open deals** is the open value expected to close this quarter, over what is left to win.
- **Coverage, qualified deals** counts only the deals the qualified forecast keeps; see [Deals the customer still supports](deals-the-customer-supports.md). While the qualified forecast is off, it says **Not switched on**.

When nothing is left to win, the panel says **Target reached** and shows no coverage. When it cannot compare, it says why:

- No sales won target is set for this scope, pipeline and quarter. A missing target is not a target of zero.
- A deal amount has no exchange rate, so the totals would be too low.
- You cannot see every deal in the scope, or the amount of one.
- **Show individual performance** is off, and one colleague besides you makes up the figures.
- The manager's call is below the sales already won.
- The time span is not this quarter, or the scope is several teams at once.
- One pipeline is picked, and the target covers all pipelines, or the landing is a manager's call.
- The target was set in a currency that is no longer the base currency.

When another time span, pipeline or team would compare, a button under the notice switches the page to it.

This comparison is not **Deal value needed** on the **Forecast** page. That figure holds the pipeline against a call or past quarters, never against a target.

## What does each graph mean?

- **Sales won:** won value added up over time, and the time span before it when the two can be compared. It also shows a target for the full time span. The target is for the whole time span; it does not ask for the same amount each day.
- **Pipeline stages:** open value of today for each stage in order, all measured the same way, with the number of priced deals in each stage.
- **Sales by salesperson:** real result and target for the same target time span. A missing target and a zero
  target are two different states; a missing target does not mean low results.
  The chart says whom it credits and how much of the target time span has passed.
- **Stage age:** the middle age and the age that 75% of open deals stay under, for open deals of today. Small groups
  are held back. These numbers describe the whole group; they do not warn you that one deal has stopped moving.
- **SDR outcomes:** meetings held and accepted opportunities for each week, counted
  apart. A held meeting keeps the host that was recorded when it was held, and the day it was held on. A meeting entered with only a date stays on that date when it is moved later. An older meeting with no such record, or only part of one, is given to its host of today.

  Some older meetings were saved before Margince kept whether a start had a time. One of those that starts at 12:00 UTC may count on the day next to it. The details for the number say when either happens. An accepted opportunity stays with
  the SDR who created it after the deal gets a new owner. These two lines do not
  show a rate of turning one into the other.
- **Leads:** new leads by origin and lead status changes by the new status. Leads qualified into a deal show out of all that came in. First response time is the middle wait. Lead figures take no pipeline.
- **Partners:** pipeline created and sales won on deals that name a partner. Each is split by partner and by whether the partner sourced or influenced the deal. Commission earned is split by partner, and active partners are counted as things stand. A partner company you cannot open shows as "Partner you cannot see".
- **Sales and SDR summaries:** totals for the time span above their charts. Target progress
  shows only for given targets; an SDR without a target sees totals without a
  target column. Exact amounts, time zones and who gets credit stay in the
  reporting details.
- **Forecast:** won value, Commit value whose close date someone confirmed, and extra value that may close, each shown on its own.
  The manager's forecast is a mark of its own. Movement uses stored captures with their
  real dates; a new view needs two captures before it can show movement.

Open **Definitions** to see how each number is worked out, the rules for credit, the required fields and what the numbers cover. A value that is not there, only in part, or held back is not a measured zero. Qualified pipeline needs an admin to set up the qualifying stages first; they count only from then on.

## Why were deals lost?

Ask a question about won and lost deals, and group it by **Loss reason from the list**. Group by **How the loss reason was set** to see where each answer came from:

- **Picked at close**: the person closing the deal picked the reason.
- **Mapped from a note**: an administrator matched the deal's older note to a reason.
- **Note not mapped**: the deal has only a note, and nobody has matched it yet.

Deals lost before the list existed have only a note. Under **Settings → Loss reasons**, **Older loss notes** lists each note once, with how many deals carry it. An administrator picks the reason each note counts as and presses **Apply to deals**. Margince updates those deals in the background. Each note stays as it was written, and a reason picked at close is never replaced.

## How do I save or share a report?

Choose **Save report**, name it and choose **Visible to**. A **Purpose** tells readers what the report is for and shows under its name. Use **Customize report** to change numbers and graphs. Under **Charts and order**, put graphs into titled sections, each graph taking a whole row or half of one. Drag a graph by its handle to change its place, or press Tab to reach the handle and use the arrow keys. **Preview** shows a graph with this report's figures before you add it.

**Add question** places a saved custom report with no filters, answered for each reader. Pick one of yours from the list, or paste the link to anyone's. An edition keeps the answer its sender was given, if a retention rule for editions exists. A reader of the edition sees only the rows whose records they may open.

The saved report keeps its graphs and their layout, scope, pipeline and time span rules. Open it from **Reports** to see its values of today. Copy a report to keep a setup of its own; editing creates a new version.

To change a shared report without readers seeing it half done, choose **Save as draft** in **Edit report**. Readers keep the published report. Use **Preview draft** to see it, then **Publish draft** or **Discard draft**. While a draft is open, edits go into it. An admin can choose **Preview as** to read the report as a role would, limited to what the admin may read.

Under **History and schedules**, **Versions** lists who saved each version and when. **Restore version** publishes an earlier version again as the newest one. Snapshots and schedules keep the version they were made from. Only someone who may edit the report sees drafts and versions.

To read a report, a user needs access to the report and access to the numbers under it. A shared report does not give access to records.

A forecast link can share a live view or a stored capture. The user who gets it still signs in and is checked against the permissions of today.

## How do I keep a weekly or monthly edition?

Open a saved report and choose **Save snapshot**, or set up a weekly or monthly schedule with a time zone, day and time. **Activate schedule** starts it; **Save paused** keeps it stopped. A scheduled snapshot covers the last full time span and shows in the report; it is not sent by email.

To schedule a report, it needs a retention rule for editions and a fixed scope you may see. Ask an admin if either is missing. The schedule keeps the version of the report you picked. Editing the live report does not change that schedule behind your back. When their settings differ, choose **Use current report settings** to update the schedule.

Open **History and schedules** for past runs. It tells apart runs that are waiting, running, complete, partial, failed, skipped and suspended. Use the reason shown to fix a failed run and try it again. A schedule stops when its owner loses the access it needs. Fix the access, then start the schedule again. A paused schedule stays paused until someone starts it again.

Select an edition in the history to see the values, targets, graphs and records it held at that time. Later edits to the records do not work it out again. Privacy erasure and retention can remove records or end the edition; those states stay in view. Compare two complete editions that come one after the other to see their values side by side. When the two cannot be compared, no difference is shown. That happens when definitions changed, the group counted changed, records were held back or a time span is not complete.

## How do I set targets or export figures?

Open **Targets** and choose a figure that takes a target: sales won, qualified pipeline created, meetings held, accepted opportunities, leads qualified or partner pipeline. Select the owner or team, and month or fiscal quarter. A sales or partner figure can also be held to one pipeline; a lead or SDR figure counts every pipeline.

Choose a year and a first month; only months that can start a fiscal quarter are offered. Each new version needs a reason. Filter the list by active or retired status and by first month. The team target stays apart from the targets of each member; the difference shows as **Unallocated** or **Overallocated**.

A team lead, management or an admin sets targets. Every other seat with access to **Targets** sees its own targets, the targets of its teams and the company targets. It sees no button to add one. When none is set yet, the list says who sets them. Your own progress shows under **Analytics** → **My outcomes**.

Use **Export CSV**, next to **Save report**, for the report's numbers of today. Export a fixed edition for the numbers and question answers it stored. The `label` column names each figure as the screen does, in your language. The `metric` column keeps a fixed id that stays the same in every language. An export follows the same permissions and the same limits as the screen. For a report you build yourself, open **Reports** → **New custom report** or ask your connected AI to report through Margince.

Any user who reads a report can see the definitions of its numbers. Admins open **Reporting setup** on its own to set up qualifying stages and the daily history of the pipeline.

## Why can Weekly figures differ?

Weekly sales and totals of held customer meetings use the same numbers as Analytics. The lead funnel counts recorded status changes for the rep’s leads in the week. It counts a different group, on a different date, from meetings held in the week. Older status changes with parts missing are left out of that funnel.

Saved reports keep the number definitions they were made with, and Weekly compares numbers only when their definitions match.
