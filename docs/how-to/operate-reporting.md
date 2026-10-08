<!-- prose:plain -->
# Run sales reporting

## Set up reporting

1. Apply the additive migrations with the normal deployment steps. Back up the
   installation before you deploy. Old rows get no made-up event owners, rates or
   customer eligibility.
2. Know that reporting is on for every installation. The API, MCP and workers use the same
   evaluator, and no deployment switch chooses another analytics engine.
3. Review the role grants for `report_definition`, `report_edition`, `report_schedule`,
   `sales_target`, `reporting_framework` and `reporting_credit`. Grants on kinds of data,
   field masks and team oversight still limit the numbers. A read seat cannot change data.
4. In Analytics → Definitions, publish the template and stages that you agreed on.
   Mappings have versions, and they apply from now on, not to the past. Agree on target metrics and
   calendar periods before you set commitments.
5. Turn on an `erase` retention policy for `report_edition` before you turn on schedules.
   Choose at most 20 fixed team or pipeline capture contexts. The workspace and the contexts you set share one daily forecast capture job.
6. Save a view, capture an edition, look at the evidence of a mark, and export it.
   Then do it again as a reader with a smaller view. Check totals against the same context before you give more users access.
   Check the business time zone at the start and end of each day too.

## Upgrade from a flag-gated installation

If you turned reporting off after a pilot, call `POST /v1/admin/reporting/pause`
before you upgrade, and check that schedules are off. If you do not, schedules that are on
start again, and make up the runs they missed (up to 12 of them).
To remove the old settings key does not pause a schedule.

Old forecast captures without a population fingerprint cannot become a
reporting baseline that you can compare against. New captures build history from now on.
No backfill makes up a population for old data. An old error from a down migration
may say to turn off the old flag: use the schedule pause, which lasts, instead.

## Understand the workers

`report_schedule_sweep` turns runs whose time has come into records, and it publishes editions within limits.
The identity of a run includes the schedule revision and the time the run is for. Leases and
fences stop a stale try from taking the place of another publication. Tries wait longer each time, up to a limit.
Run history shows failure and pauses. When it makes up runs, it keeps the
12 newest runs, and records the older runs it skipped.

`forecast_snapshot_sweep` captures the fixed contexts you set, and the
workspace too. It removes contexts that show up twice. It works first on the context whose last try is oldest,
with limits for each context and for all of them. The status shows the last try, the
last capture that worked, a safe failure reason and the next capture it expects. After you fix a team or pipeline that someone removed,
try the job again through the job tools. A new period, or a changed population, needs
captures that match; it never uses the history of another team instead.

## Pause schedules or go back to old code

A human admin with the right grant can call `POST /v1/admin/reporting/pause`.
It turns off all reporting schedules until someone starts them again, and steps
their versions up. Check the changed schedules and the run history. Each schedule
needs its own decision to start again; a new start of the API or worker does not start it.

Keep the additive schema and the stored editions when you go back to old code. Do not run
migrations down to remove facts that the system stored. Down migrations work on an installation that no one used,
and refuse once reporting data exists. The daily forecast capture, privacy
erase, SAR and retention go on, even while reporting schedules are paused.

## Look into numbers that do not agree

Start with the evaluation key, scope, time zone, time window, pipeline, definition and
coverage on the answer. Sales won and the win rate follow the current deal owner.
Qualification and SDR outcomes use their own stated rules for who gets the credit. Charts of the current state
use their capture time. Target actuals have their own period.

Use the evidence endpoint with the evaluation key and time that the answer gave you. If
sources changed, the answer asks for a refresh, and it does not add new rows to
an old total. Saved editions read frozen contributions, not live records.

The one shared evaluator serves HTTP, `read_reporting`, exports and typed report
references. `compose_analytics_report` cells may name `metric_ref` (selection plus metric) or
`edition_ref` (edition ID plus metric), or an old saved query cell. One
reference must be there. Metric references carry the definition, unit, context and
coverage with the value. When you show them, keep the `partial` and `withheld` marks.

Requests stay within the dimensions the server supports, and 100,000 contributions for each metric.
When a request is over the limit, make its population smaller. Evidence, target, report and edition lists come
in pages. An edition page holds at most 5 items that the reader may see in full, to limit
the cost of reading evidence many times.

Publication checks the accountable human again, and holds changes to rights
through a generation fence in the publication transaction. Sign in counters do not
take that fence; changes to rights and erasure do. Planned work does not give that human more grants.

## Privacy and retention

Edition manifests and stored contributions take part in subject access and
erasure. A fence between publication and erasure stops old snapshots from putting back erased
content. Reads check the current grants again.

The win rate is worked out again from the retained numerator and denominator facts, under
the sample floor. Some old charts and comparisons are kept back: the ones of how long deals stay in a stage, and forecast charts.
The system keeps them back when it cannot show their first population safely. Retention
leaves a tombstone, and does not break legal holds on linked sources. Audit records
name editions and revisions, and do not copy their full frozen payloads.

## Pilot query budget

Run `IT_ARGS='-tags=integration,bench' make test-it DIR=backend/internal/compose RUN=TestReportingEvaluationPilotBudget`
against the integration database that is only for tests. It creates 10,000 deals through the
production writer, runs the evaluator once first, and measures 20 full evaluations of four
charts each. The pilot limit is `300 ms` at `p95`. The fixture has limits, and it
does not certify the full workload of a real customer. Run it again on the
hardware of the deployment, with owner and target counts like the real ones.

## What a reader can see, and what you can compare

Frozen editions keep the captured numbers, but they give no access to them that lasts.
On every read, current membership, source visibility and field masks limit the published
population. So if a reader no longer has access to a past team, an old edition can show them a smaller view.
A reader with limited rights gets no target numbers of other users on the team.
They also get no made-up rows for them with a value of 0, even when the edition has no contributions.

A live evidence or CSV request carries the evaluation receipt. If source changes
make it no longer valid, refresh the reading before you export. To serve rows that changed
under the old total would not add up. Frozen editions are the lasting
way to get a review you can do again. A comparison needs the same
population fingerprints and framework revisions, company views too.
When the roster or the framework changes, the system shows that as the reason a comparison is not available.
