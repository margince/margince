<!-- prose:plain -->
<!-- describes: backend/internal/platform/jobs/health.go backend/internal/platform/jobs/healthassessment.go backend/internal/platform/jobs/healthhistory.go backend/internal/compose/jobhealth.go backend/internal/compose/jobhealthhistory.go -->
# Job surfaces

The two readers of the job table: the `/metrics` families and `GET /v1/admin/job-health`. The other
operator endpoints are in [operator-endpoints.md](operator-endpoints.md).

## Reading the job surfaces

Two readers over one table, `river_job`, answer two separate questions. `cmd/api` serves both, and
both read at request time, not from a count kept in the process. The job table covers the whole
fleet. So no scrape of the api would see a counter kept inside `cmd/worker`. And the own copy of
the api would report a zero that looks right. The worker never serves a job table gauge
again, and its `--observe-addr` ([the operator surface of the worker](operator-endpoints.md#the-operator-surface-of-the-worker))
reports on the process, not the fleet.

**`/metrics`: is a queue growing?** Gauge families over the job table:

| Family | Labels | Meaning |
|---|---|---|
| `margince_job_queue_depth` | `queue`, `workspace_id` | `available` + `scheduled` + `retryable` + `pending`: work nobody has done yet |
| `margince_job_running` | `queue`, `workspace_id` | running now |
| `margince_job_discarded` | `kind`, `workspace_id` | all its runs used up; will never run unless someone steps in |
| `margince_job_cancelled` | `kind`, `workspace_id` | stopped by request, before all its runs were used. Counted separately from discarded because the operator does something else about it. The sweep pair counts either as a missed workspace |
| `margince_job_oldest_queued_age_seconds` | `queue`, `workspace_id` | how long the oldest job that can run and that no worker holds has waited |
| `margince_sweep_workspaces` | `sweep` | workspaces with a child of that fleet pass still on record |
| `margince_sweep_workspaces_failed` | `sweep` | those whose newest child that ended is discarded or cancelled. A next run that is `pending` or running is not an outcome, so it never counts as the result of the pass |
| `margince_sweep_units` | `sweep`, `unit` | the same reading one level down, for the dispatchers that fan out per **connection** or per **build**: units with a child still on record |
| `margince_sweep_units_failed` | `sweep`, `unit` | those whose newest child that ended is discarded or cancelled, as for the workspace pair above |
| `margince_job_failures` | `kind`, `class` | failing work (`retryable` or discarded) by what failed: the same class the failure list shows. `unclassified` is a failure whose recorded text nothing knows, the normal shape of an outage nobody has listed. Cancelled work is not counted, since a stop someone asked for is not an outage |

`margince_job_failures` lets you alert on an outage. Without a class, a monitor sees only the
discarded count go up. It goes up the same way for a provider outage, a revoked credential and a
bug, and each of those needs its own answer. The vocabularies limit how many series it can have. For each failing kind, that is every class the
core declares, plus the own classes of each composed unit, plus the reserved one.

The workspace pair counts each workspace once, and some dispatchers fan out below that level. The
unit pair reports only the kinds whose declared `fan_out_unit` is smaller than a workspace. For every
other kind the unit is the workspace, so the two pairs would carry the same numbers.

**The two pairs overlap; never add them up.** Both report a kind that works per connection, at two
levels, because its rows carry a workspace ID and also a connection ID. The workspace reading answers
*is every tenant covered*; the unit reading answers *did every unit of the pass run*.
`margince_sweep_units_failed{sweep="telegram_poll"}` and
`margince_sweep_workspaces_failed{sweep="telegram_poll"}` can both be above zero for one dead
connection. Alert on the level you mean. Use `... > 0 or ... > 0` if you need either to page you,
never `+`.

The `sweep` label on both pairs is the **child** kind, such as `sweep="telegram_poll"` and not
`telegram_poll_sweep`. The child is what the rows hold, and to map back to the dispatcher would need
a table kept by hand. On a dashboard, a join from a sweep series to `margince_job_declared_info` on
the kind lands on the catalog entry of the child. That entry carries no `fan_out_unit`, because the
label is declared on the dispatcher. The unit pair carries its level in its own `unit` label, so you
need no join to read it.

One more family, `margince_job_unrecognised_state{state,queue,workspace_id}`, shows up only when
work sits in a state this output does not sort. It is a sign to look into, not a series to graph.
So it is missing (not zero) the rest of the time.

Two more families read the **declaration**, `backend/api/jobs.yaml`, where every job kind this build
runs is declared. Every gauge above is a view of `river_job` at scrape time, so it can only name a
kind that has rows. That turns three cases into one gap. They are a declared kind with no work, a kind nobody wired,
and rows of a kind the contract no longer declares.

| Family | Labels | Meaning |
|---|---|---|
| `margince_job_declared_info` | `kind`, `role`, `queue`, `fan_out_unit`, `timeout_seconds` | one series per declared kind, with the value 1: the catalog, written whether or not the job table holds a row of that kind |
| `margince_job_unrecognised_kind` | `kind` | rows whose kind the contract does not declare: an old kind that stays on in the retention of River. It shows only when such work exists |

Together they tell the three cases from each other. A kind in the catalog with no depth series has no work. A
kind missing from the catalog, with rows, is no longer used. A kind in no place at all was never
wired. Join
an alert against `margince_job_declared_info`, and do not take a missing depth series to mean zero
work.

Its labels are those of the declaration. A label the declaration does not govern is dropped, not
filled in, because an alert will act on any number it gets:

- `queue` is missing where the insert options of a kind belong to its callers, not to the contract.
  The file records a queue for every kind, but binds one only where it gives the options. A kind its
  callers own takes its queue from enqueue sites all over the code. To publish that number would
  hide the drift between what is declared and what runs, which this surface is there to find.
- `timeout_seconds` is `-1` where the kind runs with no deadline by design. These are the two embed
  passes: their backlog limits them, and they must stay outside the rescuer of River. It is missing
  where the clock limit is a dial an operator sets, worked out when the worker registers. The file
  marks that case `not knowable here at all`. It is never `0`, because zero is the one-minute River
  default, and would look the same as a gap by design.
- `fan_out_unit` says what one child of a dispatcher stands for (a workspace, a connection, or a
  build). It is missing for a kind that fans out to nothing.

The declaration states three more things no gauge can. Know them when you read the row of a kind
in `river_job`:

- **Every kind has a timeout someone picked.** A kind with none fails generation, and does not run
  on the one-minute River default. A worker cannot answer for its own clock limit: the declared
  value is what River gets.
- **`fault:` governs log-and-return-nil.** It says whether a worker may log a failure and return
  nil. When it is missing (most kinds), the worker may not, so a green row means the work passed.
  The kinds that declare it name the lasting retry policy that keeps the green row true. Some cases
  are the backoff of a connector sidecar, or the own state of a run row. For those, a completed job
  means "this run is over", not "the work passed".
- **`args:` says what the payload holds.** It says what each field of the payload of a kind carries.
  River stores args as written in a table with no workspace column, so a job names a row and the
  worker reads it. Every field is declared an ID, or waived as a scalar with the reason a value that
  is not an ID is safe there. A field whose *name* reads like content (`Body`, `Subject`,
  `RecipientEmail`) owes a written reason even when it is an ID.

  To read the args of a job in an incident should never turn up message bodies or addresses. If it
  does, that is the bug.

Before you build an alert on these:

- **An empty `workspace_id` means a dispatcher**, both ways. *Empty* means the `workspace_id` key is
  missing or JSON null in the args of the job. That is how the args of a fan-out job look. A job
  that does tenant work always names its workspace.

  A row whose key is *there but an empty string* is badly formed. It shows up under
  `workspace_id="malformed_workspace_id"`, so it reads as something wrong, not as dispatcher work.
  The label carries the ID, never a name: the output endpoint has no path that hides data.
- **A job for later adds depth, not age.** It is queued but not past due. A queue that
  holds only running or discarded rows reports no age series at all. A running job is already
  claimed, and a discarded one never will be. So not either is "the oldest that can run and that
  no worker holds". The endpoint reports `null` for the same rows.
- **The sweep pair is per workspace.** This table has no "last pass". River handles a conflict on a
  key that must be one of a kind by updating the row that is there. So a child still live from the last fan-out is
  merged into it, and writes no new row. A reading keyed by batch would report a dispatcher retried
  in the middle of the fleet as covering a part of the workspaces it covers. Any child of that kind
  counts the workspace as covered, and its newest child that ended is its outcome.
- **Retention can make a sweep series smaller.** It can get smaller, or end, because of the River
  retention: the cleaner deletes done rows on its own schedule. A missing series is right; a
  made-up zero would look like "the fleet is empty".
- **Both `_failed` series see only what River sees.** They count rows that ended `discarded` or
  `cancelled`. Some kinds record their own failure and return `nil`, declared as
  `fault.nil_after_logging` in `backend/api/jobs.yaml`. This is true of `capture_sync` and
  `voice_build`, whose retry timing belongs to their own sidecar.

  Those end green, so a failure they handled does not reach either pair. For them a zero here means
  "River found no dead rows"; their own domain state has the last word. This holds for the whole sweep
  reading.
- **Some dispatchers need the unit pair.** For them the workspace pair is one level too high, which
  is what `margince_sweep_units_*` is for. Gmail sync, Gmail watch and the Telegram poll fan out per
  **connection**; the voice-build retry fans out per **build**. A workspace that holds two
  connections makes two child jobs per pass.

  Say the broken one failed before the good one passed. Then the newest child of the workspace that
  ended is the good one, and the workspace pair reports zero failures while a connection is dead.
  The unit pair counts each connection on its own, and reports the failure. Read the workspace pair
  for fleet coverage, and the unit pair for whether every unit of a pass ran. These kinds show up in
  both (see the note above on never adding them up).

**`/metrics` covers the whole fleet.** The endpoint is scoped. The output carries the ID of every
workspace and every kind, because an operator who scrapes a service is outside the tenant line.
That is the reason the ID is let in, and nothing past the ID. Keep `/metrics` behind the same access
control as any other operator surface, and never proxy it to a tenant.

**`GET /v1/admin/job-health`: whose work failed for good, and why?** Requires `job_health:read` and a human
session. The middleware refuses an agent passport, and the handler refuses it again. For each kind,
it reports the counts of waiting, running, retrying and dead jobs, and the oldest waiting age. It
also lists up to 50 of the newest failures.

- **Scoped to the workspace of the caller**, plus the dispatcher rows with no tenant.
  `river_job` has no workspace column, so the handler sets the scope itself. The rows with no tenant
  are a closed set of declared dispatcher kinds. A row with no tenant that is not in that set is
  dropped.
- **The failure `reason` is a checked sentence.** It comes from the closed vocabulary of the job
  layer. `river_job.errors` holds what a worker returned, no matter what. A worker that skipped
  the fault seam stored its own cause as it was, which often names an address or record a provider refused.

  One fixed sentence replaces anything outside the closed vocabulary. Margince never reads the panic
  trace River stores. A row that recorded no cause (a job cancelled before it ran) says so. It does
  not claim a failure that never happened.

## System health summary and history

`GET /v1/admin/system-health` reads each source only after checking its grant.
A source that fails leaves the others readable. Each check has its own time; the report is not one database snapshot.

The queue [rule](../../backend/internal/platform/jobs/healthassessment.go) is shared by summary and detail reads.
A runnable wait past a fixed declared interval needs work. With no declared interval, the wait is shown without claiming it is on time.
Only a recorded future attempt supports a retry state. A canceled run alone raises no alert.
Missing history does not prove completion, so an empty history cannot show recovery.

`GET /v1/admin/job-health/history` returns grouped totals and a bounded page of runs still kept.
The page key binds the reader, workspace, filters and start time. Runs use IDs from high to low to avoid duplicates while paging.

The time window covers finished runs; current retries stay in view at any age.
Each page reads current outcomes and retention. Later pages say that the set may have changed.
A group names a kind and state, not a shared cause. Error text uses the same checked words as the existing detail read.
