# Operate sales reporting

## Set up reporting

1. Apply additive migrations using the normal deployment procedure. Back up the
   installation before deploying. Existing historical rows are not given invented
   event owners, rates or customer eligibility.
2. Reporting is available on every installation. API, MCP and workers use the same
   evaluator; no deployment switch selects a different analytics implementation.
3. Review role grants for `report_definition`, `report_edition`, `report_schedule`,
   `sales_target`, `reporting_framework` and `reporting_credit`. Data-object grants,
   field masks and team oversight still constrain the numbers. Read seats cannot mutate.
4. In Analytics → Definitions, publish the agreed template and qualifying stages.
   Mappings are versioned and effective prospectively. Agree target metrics and
   calendar periods before setting commitments.
5. Enable an `erase` retention policy for `report_edition` before enabling schedules.
   Select at most 20 fixed team/pipeline capture contexts. Workspace and configured-context forecast captures share one daily job.
6. Save a view, capture an edition, inspect a mark's evidence, export it and repeat
   under a narrower reader. Validate totals and business-timezone boundaries against
   the same context before widening access.

## Upgrade from a flag-gated installation

If reporting was disabled after a pilot, call `POST /v1/admin/reporting/pause`
before upgrading and verify that schedules are disabled. Otherwise enabled
schedules resume their bounded catch-up (up to twelve missed occurrences).
Removing the retired configuration key does not pause a schedule.

Older forecast captures without a population fingerprint cannot become a
comparable reporting baseline. New captures establish history prospectively;
no backfill invents a population for old observations. Historical down-migration
errors may mention disabling the retired flag: use durable schedule pause instead.

## Understand the workers

`report_schedule_sweep` materializes due runs and publishes bounded editions.
Run identity includes the schedule revision and intended occurrence. Leases and
fences prevent a stale attempt from replacing another publication. Attempts use
bounded backoff; run history exposes failure and suspension. Catch-up retains the
newest twelve occurrences and records skipped older occurrences.

`forecast_snapshot_sweep` captures configured fixed contexts as well as the
workspace. Contexts are deduplicated and processed least-recently attempted first,
with per-context and overall budgets. Status includes last attempt, actual last
capture, a safe failure reason and next expected capture. Retry the existing job
through the operational job interface after correcting a removed team/pipeline.
A new quarter or changed population needs compatible captures; it never falls back
to a different team's history.

## Pause schedules or roll back

An authorized human administrator can call `POST /v1/admin/reporting/pause`.
It durably disables all reporting schedules and advances their versions. Verify
the changed schedules and run history. Each schedule needs an explicit decision
to resume; restarting API or worker does not resume it.

Keep the additive schema and stored editions during a code rollback. Do not reverse
migrations to remove collected facts. Down migrations support an unused installation
and refuse once the affected reporting data exists. Daily forecast capture, privacy
erasure, SAR and retention continue independently of paused reporting schedules.

## Investigate a discrepancy

Start with the evaluation key, scope, timezone, interval, pipeline, definition and
coverage on the answer. Sales won and win rate follow the current deal owner;
qualification and SDR outcomes use their stated attribution rules. Current-state
charts use their capture time. Target actuals have their own containing period.
Use the evidence endpoint with the returned evaluation key and timestamp. If
sources changed, the response asks for refresh rather than attaching new rows to
an old total. Saved editions read frozen contributions instead of live records.

The common evaluator serves HTTP, `read_reporting`, exports and typed report
references. `compose_analytics_report` cells may name `metric_ref` (selection plus metric) or
`edition_ref` (edition ID plus metric), or a legacy saved-query cell; exactly one
reference is required. Metric references carry definition, unit, context and
coverage with the resolved value. Keep partial/withheld metadata when rendering them.

Requests are bounded to supported dimensions and 100,000 contributions per metric;
large populations must be narrowed. Evidence, target, report and edition lists are
paginated. Edition pages contain at most five fully authorized results to bound
the multiplied evidence-read cost. Publication rechecks the accountable human and holds authority changes
through a generation fence in the publication transaction. Login counters do not
take that fence; authority changes and erasure do. Scheduled work does not elevate that human's grants.

## Privacy and retention

Edition manifests and normalized contributions participate in subject access and
erasure. A publication/erasure fence prevents old snapshots from restoring erased
content. Reads recheck current grants. Win rate is re-derived from retained numerator and denominator facts, subject to
the sample floor. Historical stage-age and forecast charts and comparisons are
withheld when their original population cannot be represented safely. Retention
leaves an expired tombstone and respects linked-source legal holds. Audit records
identify editions and revisions without copying their complete frozen payloads.

## Pilot query budget

Run `IT_ARGS='-tags=integration,bench' make test-it DIR=backend/internal/compose RUN=TestReportingEvaluationPilotBudget`
against the disposable integration database. It creates 10,000 deals through the
production writer, warms the evaluator and measures twenty complete four-chart
evaluations. The pilot ceiling is 300 ms at the 95th percentile. The initial local
run measured 81 ms median and 88 ms at the 95th percentile. This is a bounded
pilot fixture, not certification of the full mid-market workload; rerun on the
deployment's hardware and representative owner/target cardinality.

## Disclosure and comparison boundaries

Frozen editions preserve the captured figures, not a permanent grant to them.
Current membership, source visibility and field masks intersect the published
population on every read. Losing access to a former team can therefore narrow
an old edition. A restricted reader receives neither peer target allocations
nor synthesized zero-value peer rows, even when the edition has no contributions.

A live evidence or CSV request carries the evaluation receipt. If source changes
invalidate it, refresh the reading before exporting; serving newly changed rows
under the old total would not reconcile. Frozen editions provide the durable
alternative for repeatable review. Comparisons deliberately require matching
population fingerprints and framework revisions, including company views;
roster or framework changes are shown as a reason a comparison is unavailable.
