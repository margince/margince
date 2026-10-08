# Performance and saved sales reports

## Where do I start my sales review?

Open **Analytics → Performance**.
Choose **Sales** for sales won, pipeline stages, sales by salesperson and time in stage, or
**SDR** for confirmed meetings held and accepted opportunities. Choose your record
scope and period. Stage charts require a single pipeline. A rep's personal scope
shows their own results; wider access is required to compare owners.

Sales won follows the selected close period and the current deal owner. Reassigning
a deal moves its live sales and target results to the new owner; saved snapshots
keep the owner and results captured then. Open pipeline and stage age describe
the current pipeline, with their expected-close window printed beside the chart.
A salesperson's target is for the month or fiscal quarter that contains the end of
the period you chose, or the current cutoff if that comes first, compared with that
month's or quarter's actual. Changing the event period does not turn today's pipeline into history.

“This month” and “This quarter” follow the installation’s calendar and reset when
that period begins. The dates beside the figures identify the period measured.
Custom ranges may span up to twelve months. If the end date is today or later,
actual results stop at the current reporting cutoff, shown above the charts;
future sales are not counted. Entirely future ranges are rejected; choose a start date before the cutoff.
Targets use the month or fiscal quarter containing the actual cutoff, even when
the requested custom end date is later. Saved editions keep their captured cutoff.

Every chart has a numeric alternative. Focus or select a mark to open its evidence;
press Escape to close the drawer. Restricted source records stay restricted.
Stage-age records stay within the selected reporting scope.

## What does each graph mean?

- **Sales won:** cumulative won value, a compatible previous-period series and a
  full-period target. The target is a reference, not an assumed daily pace.
- **Pipeline stages:** current open value by ordered stage, on a common scale.
- **Sales by salesperson:** actual and target for the same target period. Missing and zero
  targets are different states; a missing target does not mean poor performance.
- **Stage age:** median and 75th percentile for current open deals. Small cohorts
  are withheld. A percentile describes the population; it is not a stalled-deal alarm.
- **SDR outcomes:** weekly held meetings and accepted opportunities, counted
  separately. A held meeting keeps the host recorded when it was held. An older meeting
  with no such record, or only part of one, is credited to its current host; the
  metric’s coverage details say when this applies. An accepted opportunity stays credited
  to the SDR who created it after the deal changes owner. These two series are not
  a conversion rate.
- **Sales and SDR summaries:** period totals above their trends. Target progress
  appears only for assigned targets; SDRs without targets see totals without a
  quota column. Exact amounts, timezones and attribution remain available in
  reporting details.
- **Forecast:** won, supported open and additional upside are separate segments.
  The manager's forecast is a separate marker. Movement uses stored captures with their
  actual dates; a new context needs two captures before it can show movement.

Open **Definitions** to see the formulas, attribution rules, required fields and
coverage policy. Unavailable, partial and suppressed values are not measured zero.
Qualified pipeline needs qualifying stages configured prospectively by an administrator.

## How do I save or share a report?

Choose **Save report**, name it and choose **Visible to**. Use **Customize report** to change metrics, graphs or their order. The saved report
retains its selected graphs, their order, scope, pipeline and period rules.
Open it from **Reports** to see its current values. Duplicate a report to keep a
separate configuration; editing creates a revision. Readers need both access to the
report and access to the underlying figures. A shared report does not grant record access.

Forecast links can share a live view or an available stored capture. A recipient
still signs in and is checked against present-day permissions.

## How do I keep a weekly or monthly edition?

Open a saved report and choose **Save snapshot**, or configure a weekly or monthly
schedule with a timezone, day and time. **Activate schedule** starts it; **Save paused** keeps it inactive. Scheduled snapshots cover the previous completed period and appear in the report; they are not sent by email.
To schedule a report, it needs a retention policy for editions and a fixed scope you
are allowed to see. Ask an administrator if either is missing. The schedule pins the report revision you chose.
Editing the live report does not silently change that schedule. When its settings differ, choose **Use current report settings** to update the schedule.

Expand **History and schedules** for run history. It distinguishes queued, running, complete, partial, failed, skipped
and suspended runs. Use the shown reason to correct a failed run and retry it.
A schedule stops when its owner loses the necessary access. Fix the access, then
resume the schedule. A paused schedule stays paused until someone resumes it.

Select an edition in the history to see the values, targets, graphs and evidence
captured then. Later source edits do not recalculate it. Privacy erasure and retention
can remove evidence or expire the edition; those states remain visible.
Compare two adjacent complete editions to see paired values. Incompatible definitions,
changed populations, withheld evidence or incomplete periods prevent a misleading delta.

## How do I set targets or export figures?

Open **Targets** and choose sales won, qualified pipeline created, meetings held
or accepted opportunities. Select the owner or team, pipeline where supported, and
month or fiscal quarter. Choose a year and starting month; only valid fiscal-quarter months are offered. Each revision needs a reason. Filter the list by active/retired status and starting month. The team commitment stays
independent of individual allocations; the difference is shown as **Unallocated** or **Overallocated**.

Use **Export CSV**, beside **Save report**, for the report's current readings, or export a selected frozen
edition for its archived readings. Exports respect the same permissions and coverage
as the screen. For a bespoke analysis, open **Reports → New custom report** or ask your connected AI
to use Margince's governed reporting tools.

Metric definitions are available to readers. Administrators open **Reporting setup** separately to configure qualification stages and daily pipeline history.

## Why can Weekly figures differ?

Weekly sales and held-customer-meeting totals use the same metrics as Analytics.
The lead funnel counts recorded status transitions for the rep’s leads during the
week; it is a different population and date basis from meetings hosted during the
week. Older partial transitions are excluded from that funnel.

Saved reports keep the metric definitions they were made with, and Weekly
compares figures only when their definitions match.
