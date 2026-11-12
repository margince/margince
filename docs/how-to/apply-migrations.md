<!-- prose:plain -->
# Apply migrations

Schema changes ship as SQL migrations built into the binary, in two folders. `backend/migrations/core/` is
owned by the main project. `backend/migrations/custom/` is owned by a fork, and the main project never
writes there. `cmd/migrate` applies both, in order, with the **owner role** DSN.
The role the app runs as never owns the schema.

## The main path

```sh
make db-up    # once: start the dev Postgres and create the app role
make migrate  # apply everything pending
```

`make migrate` runs:

```sh
MARGINCE_OWNER_DSN="<the owner DSN>" go run ./cmd/migrate up
```

The DSN reaches the command through the environment instead of the command line. It holds a password, and
any user on the machine can read a command line. The `make` target names its target database with the
password removed (`postgres://***@localhost:15432/margince`), so `migrate-down` says which database it is
about to roll back.

## Run the command by hand

```sh
MARGINCE_OWNER_DSN=<owner-dsn> migrate up
MARGINCE_OWNER_DSN=<owner-dsn> migrate down --steps 1
```

`--dsn <owner-dsn>` still works and comes before the environment. Use the environment, so the password stays
out of the process list.

- `up` applies every pending core and custom migration.
- `down` rolls back the newest `--steps` migrations (default 1).
  Migrations can roll back, but use `down` only as a dev tool.
  Shipped core migrations only add, and no one edits them.

With no `--dsn`, the DSN comes from `MARGINCE_OWNER_DSN`, else from `MARGINCE_DSN`. The owner value comes first
because every command here runs DDL, and only the owner may run `CREATE TABLE`, `CREATE ROLE` and `CREATE TRIGGER`.
In the rest of the product, `MARGINCE_DSN` is the app role (`NOSUPERUSER NOBYPASSRLS`, no DDL rights).
`MARGINCE_DSN` stays the last option, for an installation that runs everything under one account with
enough rights.

An empty `--dsn ""` is refused, so it does not go back to the environment. A script that passes an empty
value then stops. It does not run `down` or `drop-db` against any DSN the environment names.

## Write a migration

Follow this checklist. Fitness tests check several of these rules, so a missed one fails `make check` or
`make test-integration`.

1. **Create the pair.** `make migrate-create NAME=<name>` writes two files in `backend/migrations/core/`.
   They are `<unix-seconds>_<name>.up.sql` **and** `.down.sql`.
   You need both files; the runner refuses a missing `.down.sql`.
   The version is that time in seconds, so two branches cannot pick the same number.

   **Never edit a shipped core migration.** Migrations only add.
   To add a value to a `CHECK` list, write a new migration; do not change the old one.

   A migration must still sort after everything on `origin/main`. Another migration may merge while your
   branch waits. Then run `make migrate-create` again and move your SQL across. `make check` reports this
   case (`scripts/check-migration-versions.sh`).
2. **No table carries row-level security.**
   An installation holds one company, so a tenant filter separates nothing.
   A schema test reads the live schema and fails a table that declares a policy.
3. **Keep each enum in step.** A Go enum may mirror a new `CHECK (col IN (...))`.
   Then add the value to that Go `const` set too, or `enumsync_test.go` fails.
4. **`SET LOCAL lock_timeout` limits the wait, never the hold.**
   It limits how long a statement waits for a lock before it gives up.
   Once the statement has the lock, it holds it until the transaction ends.

   The runner normally puts each migration in one transaction. An index built with
   `CREATE INDEX CONCURRENTLY` runs outside it, as the section below shows. Take an `ALTER TABLE ... ADD COLUMN`, then an
   `UPDATE` of every row and a `CHECK`. The migration keeps `ACCESS EXCLUSIVE` while it fills every row.
   Every reader of that table waits that whole time. `lock_timeout` does not keep readers safe while
   you fill rows.

   On a table with real rows, you cannot fill the rows over many transactions here. One transaction per
   migration is the contract of the runner. So add the column with a `DEFAULT`, so no row is written again. Then fill the rows from app
   code or a job. Then add the `CHECK` as `NOT VALID` and `VALIDATE` it in a **migration of its own**.

   The two steps work only as two files. `NOT VALID` records the rule without a scan, and `VALIDATE` takes
   only `SHARE UPDATE EXCLUSIVE` for itself.

   In the same file, it asks for that lighter lock while the
   `ALTER` still holds `ACCESS EXCLUSIVE` until the commit. Readers and writers then wait as long as a
   plain `ADD CONSTRAINT` would have made them wait. `backend/gates/migrationvalidatesplit_test.go`
   refuses the same-file form. The applied migrations that have it are registered there, because no one
   edits an applied migration.

   `1787968162_a_brief_names_the_currency_it_normalized_against.up.sql` and
   `1787968163_the_brief_currency_check_is_validated.up.sql` are the worked pair. The second one explains
   why it is a file of its own.
5. **Reach the erase and SAR paths** if the table holds personal data (`piicoverage_test.go`).
6. **Record the schema the migrations build.** Run `make db-up`, then run this from `backend/`:

   ```sh
   go test -tags integration ./migrations -run TestMigrationsBuildTheCommittedSchema -update-catalog
   ```

   It writes `backend/migrations/testdata/head_catalog.txt` again. Commit that file with the migration.
   The test reads the database from `MARGINCE_TEST_DSN`, which `backend/Makefile` sets for its own targets.
7. **Give each new table its owner** in `tableOwners` (`backend/gates/tableownerregistry_test.go`).
   Each entry is one line, such as `"my_table": "internal/modules/<name>",`.
8. **Rebuild the [table list](../reference/entity-model/README.md).** It reads `head_catalog.txt` and
   `tableOwners`, so it comes last: `cd backend && go test ./gates -run EntityModel -update-entity-model`.
   A module that owned no table before now needs the line
   `// Tables owned: docs/reference/entity-model/<name>.md.` in its `doc.go`.
   `<name>` is the name of the module's directory.
9. **Apply and check.** Run `make migrate`, then `make check` or `make test-integration`.

Schema that only a fork needs goes in `backend/migrations/custom/`. It has its own tracking table and
applies after core. Its files are named `YYYYMMDDHHMMSS` and its columns start with `x_`. It stays as it is
when the fork merges from the main project.

## Migrations that lock a busy table

Some migrations take a lock on a whole table, and every other user of that table waits on it. Such a
migration sets a short `lock_timeout`, so on a busy installation it fails, and does not wait without end.
Run it when the app is not busy, and run `migrate up` again if it timed out.

- `1790401426_bookings_reserve_their_exact_interval` builds the rule that stops two booking holds sharing
  time. It locks the whole activity table, and waits at most three seconds for that lock.

## Build an index while the app still writes

Put `-- pgmigrate:no-transaction` on a line by itself in the up file of such a migration.
The [runner](../../backend/internal/platform/dbmigrate/notransaction.go) records the version after
all statements finish, so each statement must be safe to run again.

A failed build can leave behind an index Postgres never uses. The
[indexes that let a delete find its child rows](../../backend/migrations/core/1794471014_parent_deletes_find_their_children.up.sql)
put `DROP INDEX CONCURRENTLY IF EXISTS` before each build, so a second try builds it again.
The down file drops its indexes with `DROP INDEX CONCURRENTLY` too.

## Why no one edits a shipped migration

On the `up` path an applied version never runs again. Only `migrate down` runs a version again, by
hand. So an edit to an applied migration changes what a new installation gets, while every database already
in use keeps working the old way. Nothing shows this. An installation can end up without a
row fill, with no sign that it is missing.

Two edits to applied migrations exist, and each one was safe for a stated reason:

1. **The tenant scope work** edited applied migrations.
   It shipped with repair migrations that only add, so it reached every database in use.
2. **The baseline merge** put one baseline file each in place of the history of core and custom.
   It has no repair half, because no installation then held data that could not be built again.
   It stops an old database instead.
   The baseline uses version `0001` again, and on such a database that row names a migration that no longer exists.
   So `dbmigrate.assertLedgerMatches` refuses and tells the operator to run `make dev-fresh`.

Neither case sets a rule for others. The second needed every database to be one that could be built again.
`scripts/migration-baseline.sh verify` also needed to prove that the baseline builds the same schema as the
history, byte for byte. Without both, never edit a shipped migration.

## A migration that moved to a new version

A sync can give a develop migration a new version above what main carries. A database that applied the
old version has already run that SQL. If the new version counted as not applied, `migrate up` would run
the SQL again and fail.

So every such move is declared in `backend/migrations/renumbered.txt`, one line per move:
`core old_version new_version name old_checksum`. The checksum is the `content_digest` a database
records for the old version. Declare a move only when the SQL did not change at all.
A database may hold an earlier text of the moved SQL that builds the same schema. Then add that text's
checksum under the old version to `equivalentContent` in `backend/internal/platform/dbmigrate/equivalence.go`,
keyed by the new version.

`migrate up` moves the ledger row of a declared old version to the new version. It does this first, under
the migration lock, in one transaction for each folder. It moves a row only if its checksum proves
the database ran the SQL that now ships, or the text such an entry names.

Otherwise it stops, names the move and applies nothing. That happens when the row has no checksum,
when the checksum differs, or when the ledger already holds the new version with other SQL. It prints how many rows it moved.

`scripts/check-migration-versions.sh` fails a move that `renumbered.txt` does not declare. It matches by
name, so a move fails even when the SQL stayed the same. It also fails a declared move whose SQL
changed, unless an `equivalentContent` entry accepts the old text at the new version. It asks
`backend/tools/check-equivalentmove`, which reads that list the way `migrate up` does.

An older binary started after this move runs the moved migrations again and stops with an error. To go
back to an older binary, restore the database from a copy taken before the upgrade.

## Data tasks

Move work whose cost grows with records into a
[background data task](add-a-data-task.md). The worker runs small transactions and saves its place;
API startup only applies schema changes. Never ship a migration that requires a background task to
have completed. Reads must be correct before the task has finished.


## Email date index

The index uses `activity_participant`. Apply the schema before starting the new worker.
Run the background corrections described in [Background corrections](add-a-data-task.md#observe-a-correction).
The reader uses the exact history query until this task finishes.

Wait for the task to finish before starting a history import. The worker also repairs missing
date fields after the task ends. Database triggers keep each row's date and visibility equal to its activity.

The index reads `address_domain_folded`. It finds both a domain and names under it.
Erasure clears that field, so the row leaves the index.

## Move an extension's migration version

An extension can declare a version move in `migrations/renumbered.txt`, embedded with its SQL files.
Use the same five-column format as `backend/migrations/renumbered.txt`.
Its first column is the unit's `ext_<name>` value, from `dbmigrate.NamespaceFor`.
The loader checks the old content before moving the recorded version.
The migration-version gate checks extension folders too.
Without that file, the extension declares no moves.
