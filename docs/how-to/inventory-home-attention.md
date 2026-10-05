# Inventory what history left in the Worklist

After a mail history import or a CRM migration, Home can fill with old requests,
privacy duties and proposals. `scripts/home-attention-inventory.sql` counts
those, so an operator can see which repair applies before changing anything.

The script only reads. It runs in a `READ ONLY` transaction and ends in
`ROLLBACK`. It prints counts and kinds, never a subject, body or address, so its
output can be shared with whoever decides the repair.

## Run it

Against an installation's database, as a role that can read its tables:

```
psql "$DATABASE_URL" -v horizon_days=90 -f scripts/home-attention-inventory.sql
```

Against your own dev stack, with the host port your stack's Postgres listens on:

```
bash scripts/dev-psql.sh 15432 margince -v horizon_days=90 < scripts/home-attention-inventory.sql
```

`horizon_days` is the waiting horizon to count against; the default is 90. The
product measures its own horizon per installation from how long its replies
take, between 14 and 365 days, and uses 90 only on thin history
(`backend/internal/modules/activities/waitinghorizon.go`). The script cannot
measure it, so count 2 is an approximation: it matches the Worklist only when
`horizon_days` equals the measured value.

## Read the counts

1. **Received mail by origin.** Mail a mailbox already held when it was first
   connected is imported history. Calendar connections are not counted, and a
   message several seats imported is counted once. "Arrival time not recorded" is mail with no
   provider receipt time stored, which includes older imports never
   backfilled; its origin is unknown, not current.
2. **Open confirmed requests by age.** Open and confirmed as the waiting lane
   reads them: not finished, settled or judged not sales, and either classified
   as asking us or taken by a human. Only a request inside the horizon, or one a
   human holds, belongs in daily work. A request a human holds has an open
   reminder that a human wrote or edited. A reminder the system filed does not
   count. Requests past the horizon and not held leave the Worklist by
   themselves and stay on the contact's timeline.
3. **Received mail whose sender is a seat.** Mail filed as inbound although a
   seat, one of its connected mailboxes or one of its claimed addresses sent
   it. This is the direction repair's
   candidate set; a high number usually means a seat's other address was not
   claimed when the history was imported.
4. **Unresolved privacy-notice duties.** A duty recorded after its own deadline
   came from history, not from a missed deadline. A `queued` duty is staged for
   delivery and stays off the Worklist unless delivery fails.
5. **Pending approvals by kind.** What the decision queue holds, without
   proposals past their expiry.
6. **Contact proposals on reserved test domains.** Proposals for addresses on
   domains that cannot receive real mail, such as `.test` or `example.com`.
7. **Machine seat addresses.** Addresses learned from delivery headers whose
   local part names a no-reply or notification sender. Every reader of a seat's
   addresses already skips them, so they change nothing; they are leftover rows
   to clean up. The count is a lower bound, because the product also recognises
   transactional sending domains.
8. **Evidence doubled across a duplicate pair.** Meetings, signals or documents
   cited by an open deal suggestion on both companies of an open duplicate
   pair. Merging the pair in the dedupe queue resolves it.

Run the script again after a repair. Each count should fall to what the repair
promised, and no other count should rise.
