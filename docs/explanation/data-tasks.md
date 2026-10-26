<!-- describes: backend/internal/platform/datatask backend/internal/compose/datataskengine.go backend/internal/compose/datataskoperator.go backend/internal/compose/jobs_datatask.go backend/api/jobs.yaml -->
# Background data corrections

A release carries large corrections as data tasks. The worker applies them in bounded transactions
while the API serves requests. Startup runs schema migrations and does not execute data tasks.

Each task has a stable ID, a code version, migration prerequisites and ordered steps. The worker seeds
one `data_task_run` row per step without replacing existing progress. Tasks run by ID, then by step
order. A paused or failed step holds later tasks. A missing migration prerequisite holds its step.
An older binary does not touch an unknown task or a cursor written by a newer task version.

One database advisory lock serializes batches across worker replicas. Each transaction has a
20-second deadline, a 15-second statement budget and a two-second lock wait. Domain writes, their
normal audit entries and progress commit together. A crash before commit rolls them all back.
The cursor advances over inspected rows, including rows the correction cannot change.

A failed batch increments its step's consecutive failure count. A timeout halves its batch limit,
with the declared minimum as a floor. Five failures without progress stop the step and notify live
admin and ops members whose current grants permit reading data tasks. Only vetted failure classes
are stored. A shutdown cancellation does not count as a failure. Reconciliation runs every ten
minutes and on worker start to restore interrupted execution.

Settings → System health shows inspected and changed counts separately. The bounded estimate counts
rows to inspect; it is not a prediction of how many will change. A capped estimate is a floor, and
an absent estimate is unknown. A failed read remains an error. The last-progress time moves only
when a batch commits. Admin and ops can pause, resume, retry and refresh an estimate. Controls carry
the ledger version the operator read, so a stale request cannot overwrite newer progress.

The first task stamps provable provider arrival dates from stored mail and then corrects acquisition
provenance and related privacy notices in one transaction. Unknown arrival dates remain no evidence.
Queued disclosures and completed notice decisions keep their state. The correction is safe to replay
from the beginning. Its versioned evidence scope is narrower than current runtime admission, which
also recognizes fellow recipients and colleague-sent copies.

Data reset preserves the task ledger. Existing feature-specific backfills remain with their owners;
this framework does not replace provider imports, participant backfills or geocoding passes.

## Upgrade limits

Task IDs are append-only throughout the supported upgrade range. A step logic change increments its
task version and restarts its cursor; the writer must therefore be safe to replay. A task is never
removed merely because one installation finished it.

Application reads must handle partially corrected data. A schema migration must not require a
background task to have finished, for example by adding a validated constraint or `NOT NULL` after
a backfill. Such constraints need a future minimum upgrade-from release that guarantees completion.
Core tasks are supported; custom and extension task registration needs a separate public contract.

There is no foreground executor or undo command. Reversing a correction requires another reviewed
correction. Automatic release tasks start without operator approval. A manually reviewed semantic
repair needs an explicit activation contract; an unactivated task must not be represented as a
paused automatic correction.
