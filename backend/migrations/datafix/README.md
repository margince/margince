<!-- prose:plain -->
# Data fixes

One-time rewrites of data in an installation that exists. They are **not** migrations.
`margince-migrate` never runs this folder. The run time of a data fix grows with the data of an
installation, and migrations run when the API starts.

An operator runs a data fix once, after the release that needs it is live. Run it at a quiet time,
and in batches where the installation is large. Each file states what it rewrites, and is idempotent: to run
it again changes nothing.

| File | Needed when | What it does |
|---|---|---|
| `2026-10-03_mail_sent_from_a_former_address_owes_no_notice.up.sql` | the installation captured mail a seat sent from another address of theirs before that mail was read as outbound | re-labels qualifying `unknown_legacy` acquisitions of that mail's To and Cc recipients as `mailbox_history` and closes their open notice cases; the activities stay as captured |
| `2026-10-03-2_mail_delivered_to_a_seat_is_their_import.up.sql` | the installation captured list, group or Bcc mail before capture read `Delivered-To` | writes the `capture_import` row such mail never got, where the original's trusted `Delivered-To` names the seat; its future data task must include a stamp step for arrival times |
| `2026-10-03-3_a_contact_capture_withdrew_owes_no_notice.up.sql` | a verdict withdrew capture-made contacts before retraction ended their duties | closes the open notice cases of contacts a capture verdict archived |
| `2026-10-03-4_open_duties_the_captured_mail_already_answers.up.sql` | capture-made duties opened before the counterparty verdict's contacts were settled by their own mail, and before sent mail's To and Cc counted | settles open capture-made duties whose contact wrote to us, or was on To or Cc of mail the seat sent before connecting |
| `2026-10-03-5_a_group_post_is_its_authors_mail.up.sql` | the installation captured Google Group posts before capture read their author | names the X-Original-From author as counterparty and sender of each stored group post, and clears the bulk flag the group's own unsubscribe links set |
| `2026-10-06_an_imported_emails_sender_is_who_its_headers_name.up.sql` | the installation logged or imported emails before the participant-role fix, so every linked contact was recorded as the sender (margince#6914) | gives each linked contact of a live, unrestricted email with stated headers and at least one contact-only participant row the role its address appears on — a contact the headers do not name keeps the receiving side (cc inbound, to otherwise), bcc rows are never touched; the only delete collapses two identical contact-only rows into one, and any email whose stated address ever belonged to more than one contact is skipped whole |

Where you need more than one, run them in the order of this table. Each later one reads what an
earlier one wrote. The older-mail notice correction runs through the background data-task worker;
watch its progress in Settings → System health. A completed correction is not restarted when another
fix creates more imports: a future Delivered-To task must include its own arrival-stamp step.

## Running one

Each file expects to run in one transaction (`SET LOCAL lock_timeout` holds only inside one). Run it
like this, so that a failure rolls all of it back:

```
psql "$DSN" -v ON_ERROR_STOP=1 --single-transaction -f <file>
```

On a large installation, run the work in batches instead of the whole file at once. For example,
limit the first statement to a range of `capture_import.id` per run. Repeat until a run changes
nothing.
