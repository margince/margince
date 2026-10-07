# Apply migrations

Schema changes ship as embedded SQL migrations in two namespaces:
`backend/migrations/core/` (upstream-owned) and
`backend/migrations/custom/` (fork-owned; upstream never writes there).
`cmd/migrate` applies both, in order, with the **owner-role** DSN; the
runtime app role never owns schema.

## The golden path

```sh
make db-up    # once: start the dev Postgres and create the app role
make migrate  # apply everything pending
```

`make migrate` runs:

```sh
MARGINCE_OWNER_DSN="<the owner DSN>" go run ./cmd/migrate up
```

The DSN reaches the command through the environment instead of argv, because it
carries a password and argv is world-readable. The recipe announces its target with the
credential stripped (`postgres://***@localhost:15432/margince`), so `migrate-down`
says which database it is about to revert.

## Direct invocation

```sh
MARGINCE_OWNER_DSN=<owner-dsn> migrate up
MARGINCE_OWNER_DSN=<owner-dsn> migrate down --steps 1
```

`--dsn <owner-dsn>` still works and takes precedence; prefer the environment so
the credential stays out of the process list.

- `up` applies every pending core + custom migration.
- `down` reverts the most recent `--steps` migrations (default 1).
  Migrations are written reversible, but treat `down` as a dev tool:
  shipped core migrations are additive-only and are never edited.

With no `--dsn`, the DSN comes from `MARGINCE_OWNER_DSN`, else `MARGINCE_DSN`.
The owner variable takes precedence because every verb here runs DDL, and
tables, roles and triggers need owner privileges to create. `MARGINCE_DSN` is the
app role elsewhere in the product (`NOSUPERUSER NOBYPASSRLS`, no DDL rights). `MARGINCE_DSN` remains the last resort for an
installation running everything under one sufficiently-privileged credential.

An explicitly empty `--dsn ""` is refused instead of falling through to the
environment, so a wrapper passing an unset variable aborts instead of running
`down` or `drop-db` against whatever the ambient DSN names.

## Writing a migration

Follow this checklist. Fitness tests enforce several of these obligations, so
missing one fails `make check` or `make test-integration`.

1. **Scaffold the pair.** `make migrate-create NAME=<name>` writes
   `<unix-seconds>_<name>.up.sql` **and** `.down.sql` in `backend/migrations/core/`. Both halves are
   mandatory (the runner rejects a missing `.down.sql`). The version is the unix second, so two
   branches cannot pick the same number.
   **Never edit a shipped core migration.** Migrations are additive; extend a `CHECK` vocabulary
   with a new migration instead of rewriting the old one.

   A migration must still sort after everything on `origin/main`. A branch that sat while another
   migration merged re-runs `make migrate-create` and moves its SQL across. `make check` reports
   that (`scripts/check-migration-versions.sh`).
2. **No table carries row-level security.** An installation holds one company, so a tenant
   predicate separates nothing; a schema fitness test derived from the live schema fails a table
   that declares a policy.
3. **Keep enums in sync.** A new `CHECK (col IN (...))` that a Go enum mirrors means extending that
   Go const set, or `enumsync_test.go` fails.
4. **Reach erasure + SAR** if the table holds PII (`piicoverage_test.go`), and record the table in the
   owning module's `doc.go` "Tables owned" list (`tableownership_test.go`).
5. **`SET LOCAL lock_timeout` bounds the wait, never the hold.** It caps how long a statement
   queues for a lock before giving up. Once granted, the lock is held until the transaction ends.
   The runner wraps each migration in one transaction, so an `ALTER TABLE ... ADD COLUMN` followed
   by a full-table `UPDATE` and a `CHECK` keeps `ACCESS EXCLUSIVE` for the whole backfill. Every
   reader of that table blocks for the duration. Do not rely on `lock_timeout` to protect readers
   during a backfill.

   On a table with real rows, the backfill cannot be batched here: one transaction per migration is
   the runner's contract. Land the column with a `DEFAULT` and no rewrite, then backfill from
   application code or a job, then add the `CHECK` as `NOT VALID` and `VALIDATE` it in a
   **migration of its own**.

   The separate file is what makes the two-step work. `NOT VALID` records the constraint without
   scanning, and `VALIDATE` drops to `SHARE UPDATE EXCLUSIVE` for itself only. Run in the same
   file, it asks for that lighter lock while the `ALTER`'s `ACCESS EXCLUSIVE` is still held to
   commit, so readers and writers queue as long as a plain `ADD CONSTRAINT` would have made them.
   `backend/gates/migrationvalidatesplit_test.go` refuses the same-file form; the applied
   migrations that carry it are registered there, because an applied migration is never edited.

   `1787968162_a_brief_names_the_currency_it_normalized_against.up.sql` and
   `1787968163_the_brief_currency_check_is_validated.up.sql` are the worked pair, and the second
   explains why it is a file of its own.
6. **Apply and verify.** Run `make migrate`, then `make check` / `make test-integration`.

Fork-local schema goes in `backend/migrations/custom/`, which has its own tracking table and applies
after core (`YYYYMMDDHHMMSS`-named, `x_`-prefixed columns) and survives upstream merges untouched.

## Why a shipped migration is never edited

On the `up` path an applied version never re-runs; only `migrate down` lets a
version execute again, by hand. Editing an applied migration therefore changes
what a fresh installation gets while every deployed database keeps the old
behaviour, and nothing reports the difference. An installation can end up
missing a backfill with no sign that it is missing.

Two edits to applied migrations exist, and each was safe for a stated reason:

1. **The tenant-scope sweep** edited applied migrations and shipped with
   additive repair migrations, so every deployed database was reached.
2. **The baseline consolidation** replaced core's and custom's histories with
   one baseline file each. It carries no repair half because no installation
   then held data that could not be rebuilt. It stops a stale database instead:
   the baseline reuses version `0001`, whose ledger row on such a database names
   a migration that no longer exists, so `dbmigrate.assertLedgerMatches` refuses
   and tells the operator to run `make dev-fresh`.

Neither generalizes. The second needed every database to be rebuildable, and
`scripts/migration-baseline.sh verify` had to prove the baseline builds the same
schema as the history, byte for byte. Without both, never edit a shipped
migration.
