# Back up an installation

Margince ships no backup tool. Whoever runs the installation takes the backups: the company itself, or the hosting
partner that runs it for them.

The cadence below meets the two recovery targets that [run-a-restore-drill.md](run-a-restore-drill.md) names.

## What to back up

| Part | Holds | Back it up |
|---|---|---|
| Postgres database | Every record, the audit log, settings, the event outbox and the restore-drill ledger | Yes, with point-in-time recovery or a dump at least every hour |
| Object store (`MARGINCE_BLOBSTORE_*`, or the folder in `MARGINCE_BLOBSTORE_PATH`) | Attachments, documents, exports, and the erasure journal under `erasure-suppression/` | Yes, at the same cadence as the database |
| `MARGINCE_KEYVAULT_ROOT_KEY` | The key that seals mailbox credentials, provider keys, the mail relay password and the licence copy in the `vault_secret` table | Yes, in your secret store, apart from the data backups |
| Deployment settings | `margince.yaml`, the DSNs, object store keys and `MARGINCE_LICENSE` | Yes, in your secret store or configuration repository |
| Redis (`MARGINCE_REDIS`) | The event stream, event dedupe markers, provider health and rate-limit counters | Not for the records: every event is written to the database first, in the same transaction as the change. Events that reached Redis and no reader has taken yet are lost with it |

Keep the root key apart from the database backup. A backup and its key in one place give anyone who gets the backup
every sealed credential. Without the key, a restored database that holds sealed secrets stops the api and the worker
at start. Nothing can open those secrets again: see
[configuration.md](../reference/storage-secrets-and-license.md#secret-vault-api-worker-connector-credentials).

## How often

- **Database:** turn on point-in-time recovery, or take a dump at least every hour. A daily dump alone cannot meet
  the one-hour target.
- **Object store:** copy it to a second place at least every hour. A restore needs the database and the object store
  from the same point in time, or records point at files that are missing.
- **Settings and keys:** back them up when they change.

Do not turn on object versioning in the live object store. Margince deletes objects to carry out erasure and
retention, and a versioned store keeps the old version of each one. The backup copy is where old versions live, for
a period you set and write down.

## Backups and erasure

A backup taken before an erasure still holds the erased contact's data. Two things keep a restore from bringing it
back:

- The worker job `suppression_journal_export` copies the suppression list (a hash of each erased address, never the
  address) to `erasure-suppression/` in the object store every hour.
- After a restore, `worker replay-erasures` erases those contacts again.

So never restore `erasure-suppression/` from a backup: keep the newest copy from the live store. Keep your own
record of completed erasure requests too, because the replay cannot cover an erasure the last export missed. The
steps are in [run-a-restore-drill.md](run-a-restore-drill.md).

Say in your privacy documents how long backups are kept. Erased data stays in a backup until that backup expires.

## Show that a restore works

A backup that was never restored is not evidence. Run a restore drill on a schedule you write down, for example
every quarter, and after each change to the backup setup. Follow [run-a-restore-drill.md](run-a-restore-drill.md).
It records each drill in the installation, and its step 5 shows where to read the result.

## Who does what

| Task | Margince | The operator |
|---|---|---|
| Take database and object store backups | No | Yes |
| Keep the root key and settings safe | No | Yes |
| Export the erasure journal every hour | Yes, when an object store is set | Keeps the newest copy out of any restore |
| Erase restored contacts again | Gives `worker replay-erasures` | Runs it before users get back in |
| Run restore drills | Records them and shows the result | Runs them and keeps the schedule |
| Report the last backup time | No: the card reads "Not observed by Margince" | Reads it from the backup tool |

A hosting partner that runs the installation for a customer is the operator. Agree in writing which of the
operator's tasks the partner does and which the customer keeps.
