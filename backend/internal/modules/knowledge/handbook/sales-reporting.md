# Performance and saved sales reports

## Where do I start my sales review?

Open **Analytics → Performance** when your administrator has enabled sales reporting.
Choose **Sales** for sales won, pipeline stages, sales by salesperson and time in stage, or
**SDR** for confirmed meetings held and accepted opportunities. Choose your record
scope and period. Stage charts require a single pipeline. A rep's personal scope
shows their own results; wider access is required to compare owners.

Sales won follows the selected event period. Open pipeline and stage age describe
the current pipeline, with their expected-close window printed beside the chart.
Salesperson targets use the containing month or fiscal quarter, with that period's own
actual. Changing the event period does not turn today's pipeline into history.

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
  separately. Acceptance stays credited to the originating SDR after ownership
  transfers. These two series are not a conversion rate.
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
schedule with a timezone, day and time. **Activate schedule** starts it; **Save paused** keeps it inactive. Scheduled snapshots cover the previous completed period and appear in the report; they are not sent by email. Scheduling requires an edition-retention
policy and a fixed, permitted scope. The schedule pins the report revision you chose.
Editing the live report does not silently change that schedule. When its settings differ, choose **Use current report settings** to update the schedule.

Expand **History and schedules** for run history. It distinguishes queued, running, complete, partial, failed, skipped
and suspended runs. Use the shown reason to correct a failed run and retry it.
A schedule stops when its accountable owner loses the necessary access. After
correcting access, explicitly resume it. Pausing survives a restart.

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

Use the report's CSV export for its current readings, or export a selected frozen
edition for its archived readings. Exports respect the same permissions and coverage
as the screen. For a bespoke analysis, open **More analysis → Custom reports** or ask your connected AI
to use Margince's governed reporting tools.

The Sales and SDR summaries show period totals above their trends. Target progress appears only for assigned targets; SDRs without targets see totals without a quota column. Exact amounts, timezones and attribution remain available in reporting details. Export CSV is beside Save report.

Metric definitions are available to readers. Administrators open **Reporting setup** separately to configure qualification stages and daily pipeline history.
