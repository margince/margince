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
4. **Reach the erase and SAR paths** if the table holds personal data (`piicoverage_test.go`).
   Add the table to the "Tables owned" list in the owning module's `doc.go` (`tableownership_test.go`).
5. **`SET LOCAL lock_timeout` limits the wait, never the hold.**
   It limits how long a statement waits for a lock before it gives up.
   Once the statement has the lock, it holds it until the transaction ends.

   The runner puts each migration in one transaction. Take an `ALTER TABLE ... ADD COLUMN`, then an
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
6. **Apply and check.** Run `make migrate`, then `make check` or `make test-integration`.

Schema that only a fork needs goes in `backend/migrations/custom/`. It has its own tracking table and
applies after core. Its files are named `YYYYMMDDHHMMSS` and its columns start with `x_`. It stays as it is
when the fork merges from the main project.

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
