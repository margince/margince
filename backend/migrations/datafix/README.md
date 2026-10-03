# Data fixes

One-time rewrites of existing installation data. They are **not** migrations:
`margince-migrate` never runs this folder, because their run time grows with an
installation's data and migrations run at api boot (margince#6692).

An operator runs a data fix once, after the release that needs it is live, at a
quiet time, and batched where the installation is large. Each file states what it
rewrites and is idempotent: running it again changes nothing.

| File | Needed when | What it does |
|---|---|---|
| `1790871111_mail_a_mailbox_already_held_owes_no_notice.up.sql` | the installation captured mail before migration 1790871110 | stamps `capture_import.provider_received_at` from the stored original's top `Received` header, then re-labels qualifying `unknown_legacy` acquisitions as `mailbox_history` and closes their open notice cases |
| `1790871111_mail_a_mailbox_already_held_owes_no_notice.down.sql` | rolling that fix back | reopens the cases it closed and restores the acquisition kinds |
| `2026-10-03_mail_sent_from_a_former_address_owes_no_notice.up.sql` | the installation captured mail a seat sent from another address of theirs before that mail was read as outbound | re-labels qualifying `unknown_legacy` acquisitions of that mail's To and Cc recipients as `mailbox_history` and closes their open notice cases; the activities stay as captured |

## Running one

Each file assumes one transaction, as the migration runner used to give it
(`SET LOCAL lock_timeout` only holds inside one). Run it like this, so that a
failure rolls everything back:

```
psql "$DSN" -v ON_ERROR_STOP=1 --single-transaction -f <file>
```

On a large installation, run the work in batches instead of the whole file at
once (for example by restricting the first statement to a range of
`capture_import.id` per run), and repeat until a run changes nothing.

## Limits

- `1790871111…down.sql` was written as the rollback of the whole feature. Besides
  undoing the fix, it relabels every capture-created `mailbox_history`
  acquisition that the sent-mail rule does not cover, including ones the running
  application created correctly, and opens notice cases for them. Use it only
  together with reverting the received-mail rule. It also leaves the stamped
  `provider_received_at` values in place.
