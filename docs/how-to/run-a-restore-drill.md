# Run a restore drill

A restore drill restores a backup into a spare database, checks that the copy works, and records how long it took.
Margince publishes two recovery targets: lose at most one hour of data, and be back within four hours. A drill
record is the evidence that a restore meets them. The record lives in the `restore_drill` table of the live
installation, owned by the `continuity` module (`backend/internal/modules/continuity`).

Margince ships no backup or restore tool. The steps below that copy data use your own tool. The steps that record
the drill use `margince-migrate`, the same binary that runs migrations and `reset-password`, under the owner DSN.

## Before you start

- Pick the backup and the point in time it restores to. With point-in-time recovery, this is the time you ask
  the tool for. With a nightly dump, it is the time the dump started.
- Prepare a spare database and a spare place in the object store. Do not restore over the live installation. A
  restore rolls back every table in the database it lands in, `restore_drill` included, so a drill restored over
  the live database deletes its own record.
- Have the owner DSN of the live installation at hand. Both recording commands write to the live database.

## 1. Start the drill

Run this when the restore begins:

```sh
margince-migrate drill-start --dsn "$MARGINCE_OWNER_DSN" --by "Dana Ops" --restored-to 2026-10-09T03:00:00Z --note "quarterly drill"
```

It prints the drill id. Keep it for step 4. The start time is the database clock at this moment, so start the drill
when the restore starts. `--restored-to` must be RFC 3339 and must not lie in the future.

## 2. Restore the backup into the spare database

Use your backup tool to restore the database into the spare database. Restore the object store to the same point
in time. A database without its attachment files is half a restore.

Leave out the `erasure-suppression/` prefix of the object store. It holds the erasure journal, and the replay below
needs the newest copy. Copy that prefix from the live object store instead.

Then bring the restored schema up to the running release:

```sh
margince-migrate up --dsn "$SPARE_OWNER_DSN"
```

A restore to a point before an erasure brings the erased contact's data back. The database's own suppression list
rolls back with it. The worker job `suppression_journal_export` copies that list to the object store every hour. The
`replay-erasures` command of the worker program erases again every live contact that the list or the copy names.

The worker refuses a database whose recorded release differs from its own. Only the api records the release, when it
starts. So if the backup was taken on an older release, record the current one first:

1. Start one api of the running release against the spare database and object store.
2. Send no user traffic to it, and stop it once it has started.

If the backup was taken on the running release, skip these two steps.

Then run the replay. Point every setting at the spare database and the spare object store, never at the live ones:

```sh
export MARGINCE_DSN="$SPARE_APP_DSN"
export MARGINCE_BLOBSTORE_ENDPOINT="$SPARE_BLOBSTORE_ENDPOINT"
export MARGINCE_BLOBSTORE_BUCKET="$SPARE_BLOBSTORE_BUCKET"
export MARGINCE_BLOBSTORE_ACCESS_KEY="$SPARE_BLOBSTORE_ACCESS_KEY"
export MARGINCE_BLOBSTORE_SECRET_KEY="$SPARE_BLOBSTORE_SECRET_KEY"
export MARGINCE_KEYVAULT_ROOT_KEY="$MARGINCE_KEYVAULT_ROOT_KEY"
worker replay-erasures
```

An installation that keeps its objects on local disk sets `MARGINCE_BLOBSTORE_PATH` to the spare folder instead of the
`MARGINCE_BLOBSTORE_*` service settings. `MARGINCE_DSN` is the runtime app role, not the owner. The root key is the
one the installation sealed its secrets with.

The command prints three things:

- the time of the journal export it read, and how many entries that export held;
- a warning that erasures completed after that time are not covered;
- how many contacts it erased again.

The journal ends at its last export. Take every erasure request completed after the printed time from your own record
of the requests. Erase each of those contacts again by hand in the app.

On a drill the count is often zero, and zero is a valid result. A contact under legal hold is not erased. The command
names it and exits with an error after it has erased the others.

The command refuses to run without an object store, because the journal lives there. It also refuses a journal with no
entry, which most often means the settings point at the wrong store. Pass `--empty-journal-ok` only if the installation
has never erased anyone.

For a real restore, do not let users back in until `replay-erasures` has run without an error. The erasures after the
printed time must be done by hand as well.

## 3. Check the restored copy

Choose checks that would fail on a broken restore, and note them. For example:

- `margince-migrate workspace-exists --dsn "$SPARE_OWNER_DSN"` prints `true`.
- Point a second api at the spare database and object store, sign in, and open a record with an attachment.
- Compare the count of contacts, companies and deals with the live installation as of the restore point.

## 4. Finish the drill

Close the drill as `passed` or `failed`:

```sh
margince-migrate drill-finish --dsn "$MARGINCE_OWNER_DSN" --drill <id> --outcome passed --note "copy opened; counts matched"
```

The finish time is the database clock at this moment. A drill closes once; a second `drill-finish` on it fails. If
you stop a drill halfway, close it as `failed` and say why in `--note`. A failed drill stays in the ledger as
evidence.

## 5. Read the result

Open **Settings** → **System health** and read the **Restore drills** card, or call `GET /v1/admin/recovery-health`.
Both need the admin or ops role. The card shows the last drill's outcome, the time to recover, and the data lost, each
against its target. The time to recover is finish minus start. The data lost is start minus the restore point.

The **Last backup** row reads "Not observed by Margince", because no backup tool reports to Margince. Check your
backup tool for when the last backup ran.

When you are done, drop the spare database and remove the spare object store place.
