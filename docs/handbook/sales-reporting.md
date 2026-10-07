<!-- prose:plain -->
# Performance and saved sales reports

## Where do I start my sales review?

Open **Analytics → Performance**. Choose **Sales** for sales won, pipeline stages, sales by salesperson and time in stage, or **SDR** for meetings held and accepted opportunities. Choose your record scope and time span. Stage charts need a single pipeline. A rep's own scope shows their own results; you need wider access to compare owners.

Sales won follows the close time span you picked and the deal owner of today. If a deal gets a new owner, its live sales and target results move to the new owner. Saved snapshots keep the owner and results from that time. Open pipeline and stage age show the pipeline as it is now, with the time window for expected closes printed next to the chart.

A salesperson's target is for the month or the business quarter that holds the end of the time span you picked. If the cutoff of today comes first, it is the month or quarter that holds that cutoff. It is compared with the real result for that month or quarter. Changing the time span does not turn today's pipeline into history.

“This month” and “This quarter” follow the calendar of your Margince and start again when that time span starts. The dates next to the numbers name the time span measured. A custom time span can be up to 12 months long. If the end date is today or later, real results stop at the reporting cutoff of today, shown above the charts. Future sales are not counted. A time span that is all in the future is refused; choose a start date before the cutoff.

Targets use the month or business quarter that holds the real cutoff, even when the custom end date you asked for is later. A saved edition keeps its own cutoff.

Every chart can also be read as numbers. Select a mark, with a click or the keys, to open the records behind it; press `Esc` to close the side panel. A record you may not see stays hidden. Stage age records stay inside the reporting scope you picked.

## What does each graph mean?

- **Sales won:** won value added up over time, and the time span before it when the two can be compared. It also shows a target for the full time span. The target is for the whole time span; it does not ask for the same amount each day.
- **Pipeline stages:** open value of today for each stage in order, all measured the same way.
- **Sales by salesperson:** real result and target for the same target time span. A missing target and a zero
  target are two different states; a missing target does not mean low results.
- **Stage age:** the middle age and the age that 75% of open deals stay under, for open deals of today. Small groups
  are held back. These numbers describe the whole group; they do not warn you that one deal has stopped moving.
- **SDR outcomes:** meetings held and accepted opportunities for each week, counted
  apart. A held meeting keeps the host that was recorded when it was held. An older meeting with no such record, or only part of one, is given to its host of today. The details for the number say when this happens. An accepted opportunity stays with
  the SDR who created it after the deal gets a new owner. These two lines do not
  show a rate of turning one into the other.
- **Sales and SDR summaries:** totals for the time span above their charts. Target progress
  shows only for given targets; an SDR without a target sees totals without a
  target column. Exact amounts, time zones and who gets credit stay in the
  reporting details.
- **Forecast:** won value, open value that is likely to close, and extra value that may close, each shown on its own.
  The manager's forecast is a mark of its own. Movement uses stored captures with their
  real dates; a new view needs two captures before it can show movement.

Open **Definitions** to see how each number is worked out, the rules for credit, the required fields and what the numbers cover. A value that is not there, only in part, or held back is not a measured zero. Qualified pipeline needs an admin to set up the qualifying stages first; they count only from then on.

## How do I save or share a report?

Choose **Save report**, name it and choose **Visible to**. Use **Customize report** to change numbers, graphs or their order. The saved report keeps its graphs, their order, scope, pipeline and time span rules. Open it from **Reports** to see its values of today. Copy a report to keep a setup of its own; editing creates a new version.

To read a report, a user needs access to the report and access to the numbers under it. A shared report does not give access to records.

A forecast link can share a live view or a stored capture. The user who gets it still signs in and is checked against the permissions of today.

## How do I keep a weekly or monthly edition?

Open a saved report and choose **Save snapshot**, or set up a weekly or monthly schedule with a time zone, day and time. **Activate schedule** starts it; **Save paused** keeps it stopped. A scheduled snapshot covers the last full time span and shows in the report; it is not sent by email.

To schedule a report, it needs a retention rule for editions and a fixed scope you may see. Ask an admin if either is missing. The schedule keeps the version of the report you picked. Editing the live report does not change that schedule behind your back. When their settings differ, choose **Use current report settings** to update the schedule.

Open **History and schedules** for past runs. It tells apart runs that are waiting, running, complete, partial, failed, skipped and suspended. Use the reason shown to fix a failed run and try it again. A schedule stops when its owner loses the access it needs. Fix the access, then start the schedule again. A paused schedule stays paused until someone starts it again.

Select an edition in the history to see the values, targets, graphs and records it held at that time. Later edits to the records do not work it out again. Privacy erasure and retention can remove records or end the edition; those states stay in view. Compare two complete editions that come one after the other to see their values side by side. When the two cannot be compared, no difference is shown. That happens when definitions changed, the group counted changed, records were held back or a time span is not complete.

## How do I set targets or export figures?

Open **Targets** and choose sales won, qualified pipeline created, meetings held or accepted opportunities. Select the owner or team, the pipeline where that is offered, and month or business quarter. Choose a year and a first month; only months that can start a business quarter are offered. Each new version needs a reason. Filter the list by active or retired status and by first month. The team target stays apart from the targets of each member; the difference shows as **Unallocated** or **Overallocated**.

Use **Export CSV**, next to **Save report**, for the report's numbers of today, or export a fixed edition for the numbers it stored. An export follows the same permissions and the same limits as the screen. For a report you build yourself, open **More analysis → Custom reports** or ask your connected AI to report through Margince.

Any user who reads a report can see the definitions of its numbers. Admins open **Reporting setup** on its own to set up qualifying stages and the daily history of the pipeline.

## Why can Weekly figures differ?

Weekly sales and totals of held customer meetings use the same numbers as Analytics. The lead funnel counts recorded status changes for the rep’s leads in the week. It counts a different group, on a different date, from meetings held in the week. Older status changes with parts missing are left out of that funnel.

Saved reports keep the number definitions they were made with, and Weekly compares numbers only when their definitions match.
