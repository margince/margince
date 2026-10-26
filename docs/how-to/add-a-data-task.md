# Add a background data task

Use a data task for a correction whose cost grows with stored records. Keep schema changes in
migrations. Read [Background data corrections](../explanation/data-tasks.md) for execution and
upgrade rules.

1. Add the schema prerequisites as additive migrations. Do not make a migration depend on the
   correction's result.
2. Add a stable `core/<unix-second>_<name>` task to `dataTaskCatalog` in
   `backend/internal/compose/datataskengine.go`. Declare its version, title, prerequisites and
   ordered steps. Existing task IDs stay registered.
3. Give each step a starting batch limit, a minimum limit, a bounded estimate and its audit policy.
   An unaudited derived-column write must name the runtime writer that follows the same policy.
4. Implement domain writes in the owning module's exported transaction function. The adapter takes
   the executor's `pgx.Tx`; it must not call a store that starts another transaction or perform
   network I/O. Preserve the module's audit and event policy.
5. Page over an indexed key and advance over every inspected row. A batch that changes nothing may
   still have more rows to inspect. Exclude already corrected rows from writes so a nil-cursor
   replay changes nothing. The estimate must inspect at most its supplied cap plus one row.
6. Test rollback after domain writes, body replay, malformed input and bounded paging through the
   real executor. Add the owning module's real-path regressions. Run the database integration lanes
   as well as `make check`.

The executor supplies a system actor and durable run ID for audit attribution. Keep the run ID.
A reviewed task rebuilds its operator from current authority before using ordinary domain writers.
Keep raw causes and record content out of progress, job arguments, metrics and notices.

## Check failures

| Failure | What to change |
|---|---|
| Invalid registration | Supply a unique core ID, positive version, named steps, finite limits and callbacks. |
| Missing audit policy | Choose per-record auditing or explain the existing derived-column exception. |
| No cursor progress | Advance over inspected rows even when none changed. |
| Missing prerequisite | Declare the core migration that supplies the schema the writer reads. |
| Changed ledger version | Refresh progress before controlling the step; do not retry a stale overwrite. |
| Unbounded migration rewrite | Use a data task, or document a finite small-table exception. |
| Removed task ID | Keep the released task in the worker catalog. |
| Table ownership | Put the write in the owning module and inject it through compose. |

## Observe a correction

Read Settings → System health, or use the same catalog from the command line:

```sh
margince-migrate data-tasks status --dsn "$DSN"
margince-migrate data-tasks estimate --dsn "$DSN"
margince-migrate data-tasks run --wait --dsn "$DSN"
```

`run` queues reconciliation and waits for the worker when requested. It never runs a batch in the
CLI process. The database credentials confer operator authority; the HTTP controls require a human
session and the `data_task` read or update grant. A wait stops with an error on a paused or failed
step. Use System health to review the cause before resuming or retrying.

## Run corrections without the worker

On a restored copy whose worker must not start, run the corrections in the CLI process instead:

```sh
margince-migrate data-tasks execute --dsn "$DSN"
```

`execute` uses the same engine and catalog as the worker. It starts no job queue and no event lane,
so no correction reaches a mailbox, a model or an enrichment vendor. The events the corrections
write wait in the outbox for the next worker. It stops at a step waiting longer than 30 seconds to
retry, and after `--max-batches` batches (100000 by default).

It prints a verdict before the steps. The command fails unless every step finished. The verdict
names each step awaiting review, blocked, failed or unfinished. It also names each finished step
that inspected no rows: such a step proves nothing about rows it never saw.

## Require a reviewed scope

Set `Manual`, `ReviewVersion` and `Review` on a task that needs an operator’s decision. The callback
must recheck the supplied identities, versions, permissions and dispositions on its transaction.
A digest identifies the plan; current domain checks decide whether the plan is still valid.

Use `Engine.Activate` from a human-only handler with the observed ledger version. Keep the scope
bounded and free of source content. The worker receives the scope and operator in `BatchIn`.
Rebuild that operator’s current authority before each batch. A revoked object grant must stop work.
A changed record can be skipped with a reason and run ID after any partial write has rolled back.

Unreviewed tasks and reviews invalidated by a rule change do not hold up automatic corrections.
The progress report names the need for review. A new activation preserves the previous run and
starts a new cursor. A worker restart under the same rules retains the current cursor and scope.

## Keep startup bounded

Core migrations after the rewrite gate's pinned baseline cannot update or delete existing rows,
merge records, or copy rows with `INSERT … SELECT` without a reviewed exception. A table created in
that migration has no exemption: filling it still scans the source. Anonymous `DO` blocks, procedure calls, queries, bulk copy, materialized-view refresh and `TRUNCATE` need an
exception too: they can change records without a visible row writer. The gate strips procedural
bodies because creating a function does not execute it. A statement after trigger creation also
needs review: even a small insert or schema change may then call a trigger that rewrites many rows.

Use a data task for an unbounded correction. For a small, finite catalog such as built-in role
permissions, add one line to `backend/migrationrewrites.txt`:

```text
<version>_role_permissions.up.sql | role | exactly six built-in role documents
```

The first field is the complete migration filename. The second lists the written tables, separated
by commas; use `*` for a procedural call, top-level query or bulk removal. The last field explains the finite bound.
Unused exceptions and exceptions naming a different set of tables fail. This is a large-rewrite
guard, not proof that a migration is schema-only.

The task registry gate compares released task IDs on `origin/main` with the worker's current
catalog. Keep IDs as static literals so the source census can read them. Removing a registration
fails even if its constant remains in the code. Retirement requires a declared minimum upgrade-from
release and is not supported yet.
