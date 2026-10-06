# Data fixes

One-time rewrites of existing installation data. They are **not** migrations:
`margince-migrate` never runs this folder, because their run time grows with an
installation's data and migrations run at api boot.

An operator runs a data fix once, after the release that needs it is live, at a
quiet time, and batched where the installation is large. Each file states what it
rewrites and is idempotent: running it again changes nothing.

| File | Needed when | What it does |
|---|---|---|
| `1790871111_mail_a_mailbox_already_held_owes_no_notice.up.sql` | the installation captured mail before migration 1790871110 | stamps `capture_import.provider_received_at` from the stored original's top `Received` header, then re-labels qualifying `unknown_legacy` acquisitions as `mailbox_history` and closes their open notice cases |
| `1790871111_mail_a_mailbox_already_held_owes_no_notice.down.sql` | rolling that fix back | reopens the cases it closed and restores the acquisition kinds |
| `2026-10-03_mail_sent_from_a_former_address_owes_no_notice.up.sql` | the installation captured mail a seat sent from another address of theirs before that mail was read as outbound | re-labels qualifying `unknown_legacy` acquisitions of that mail's To and Cc recipients as `mailbox_history` and closes their open notice cases; the activities stay as captured |
| `2026-10-03-2_mail_delivered_to_a_seat_is_their_import.up.sql` | the installation captured list, group or Bcc mail before capture read `Delivered-To` | writes the `capture_import` row such mail never got, where the original's trusted `Delivered-To` names the seat; run `1790871111` again afterwards to stamp their arrival times |
| `2026-10-03-3_a_contact_capture_withdrew_owes_no_notice.up.sql` | a verdict withdrew capture-made contacts before retraction ended their duties | closes the open notice cases of contacts a capture verdict archived |
| `2026-10-03-4_open_duties_the_captured_mail_already_answers.up.sql` | capture-made duties opened before the counterparty verdict's contacts were settled by their own mail, and before sent mail's To and Cc counted | settles open capture-made duties whose contact wrote to us, or was on To or Cc of mail the seat sent before connecting |
| `2026-10-03-5_a_group_post_is_its_authors_mail.up.sql` | the installation captured Google Group posts before capture read their author | names the X-Original-From author as counterparty and sender of each stored group post, and clears the bulk flag the group's own unsubscribe links set |
| `2026-10-06_an_imported_emails_sender_is_who_its_headers_name.up.sql` | the installation logged or imported emails before the participant-role fix, so every linked contact was recorded as the sender (margince#6914) | gives each linked contact of an email with stated headers the role its address appears on, and collapses a party described twice — by contact or seat and again by bare address — into one row |

Where several are needed, run them in the order of this table: each later one
reads what an earlier one wrote, and `1790871111` comes again after the
`Delivered-To` backfill.

## Running one

Each file assumes it runs in one transaction (`SET LOCAL lock_timeout` holds only
inside one). Run it like this, so that a
failure rolls everything back:

```
psql "$DSN" -v ON_ERROR_STOP=1 --single-transaction -f <file>
```

On a large installation, run the work in batches instead of the whole file at
once (for example by restricting the first statement to a range of
`capture_import.id` per run), and repeat until a run changes nothing.

## Limits

- `1790871111…down.sql` is the rollback of the whole feature. Besides
  undoing the fix, it relabels every capture-created `mailbox_history`
  acquisition that the sent-mail rule does not cover, including ones the running
  application created correctly, and opens notice cases for them. Use it only
  together with reverting the received-mail rule. It also leaves the stamped
  `provider_received_at` values in place.
