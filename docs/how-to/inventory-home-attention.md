<!-- prose:plain -->
# Inventory what history leaves in the Worklist

After a mail history import or a CRM migration, Home can fill with old requests,
privacy duties and proposals. `scripts/home-attention-inventory.sql` counts
these, so an operator can see which repair applies before any change.

The script only reads. It runs in a `READ ONLY` transaction and ends in
`ROLLBACK`. It prints counts and kinds, and never a subject, body or address. So you can
share what it prints with the user who decides the repair.

## Run it

Against the database of an installation, as a role that can read its tables:

```
psql "$DATABASE_URL" -v horizon_days=30 -f scripts/home-attention-inventory.sql
```

Against your own dev stack, with the host port of the Postgres of your stack:

```
bash scripts/dev-psql.sh 15432 margince -v horizon_days=30 < scripts/home-attention-inventory.sql
```

`horizon_days` is the waiting horizon to count against, and it is 30 by default. The
product measures its own horizon for each installation, from how long its replies
take, between 14 and 30 days. It uses 30 when the history is short
(`backend/internal/modules/activities/waitinghorizon.go`). The script cannot
measure it, so count 2 is close but not exact. It matches the Worklist only when
`horizon_days` is the same as the measured value.

## Read the counts

1. **Received mail by origin.** Mail that a mailbox holds from before its first
   connection is imported history.

   The counts leave out calendar connections, and count a message that more than one seat imported one time.
   `Arrival time not recorded` is mail with no stored time from the provider.
   That includes old imports that no backfill reached. Its origin is unknown, and not current.
2. **Open confirmed requests by how old they are.** These are open and confirmed in the way the waiting lane
   reads them.

   They are not finished, not settled, and not marked as not sales. The system marked them
   as asking us, or a human has taken them. Only a request in the horizon, or one a
   human holds, is daily work. A request a human holds has an open
   reminder that a human wrote or edited. A reminder the system filed does not
   count. Requests past the horizon that no one holds leave the Worklist on
   their own, and stay on the timeline of the contact.
3. **Received mail whose sender is a seat.** This is mail filed as inbound, but a seat is the sender.
   It is from a connected mailbox of the seat, or from one of its claimed addresses.

   This is the set the direction repair works on. Many rows here most likely mean
   a seat has another address that no one claimed at the history import.
4. **Open privacy notice duties.** A duty recorded after its own deadline
   is from history, not from a missed deadline.

   A `queued` duty waits for delivery, and stays off the Worklist unless delivery fails.
5. **Approvals that wait, by kind.** What the decision queue holds, without
   proposals past their end date.
6. **Contact proposals on test domains.** Proposals for addresses on
   domains kept for tests that cannot get real mail, such as `.test` or `example.com`.
7. **Machine seat addresses.** Addresses learned from delivery headers, whose
   local part names a no-reply or notice sender.

   Every reader of the addresses of a seat already skips them, so they change nothing.
   They are old rows to remove. The count can be short, because the product also knows
   the domains that send automated mail and email campaigns.
8. **Evidence counted twice in a duplicate pair.** Meetings, signals or documents
   that an open deal suggestion points to, on both companies of an open duplicate pair.

   When you merge the pair in the dedupe queue, this is fixed.

Run the script again after a repair. Each count should drop to what the repair
promised, and no other count should go up.

## Preview exact task repairs

Use `POST /admin/home-repair/preview` with `source: task` to get the bounded
review page. Its counts use the same source and date rules as current task reads.
The SQL counts describe history, not current work.

The page lists a source link fix, a generated date to clear, or old work to archive.
Each row names its change. Human
edits, pins and reminders stay. Missing source access grants no right to repair.
Unknown sources stay as they are. No repair marks a task as complete.

Review the page before `POST /admin/home-repair/activate`. The worker checks
source access, task version and the rule again. A change after review is skipped;
failed batches roll back with their audit and event. Resume uses the same reviewed
page. A traced source or date fix stays a machine change for later task reads.

Send `reviewed_actions` in the page's row order when `requires_action_review` is
true. A client sends names of actions it can show.
