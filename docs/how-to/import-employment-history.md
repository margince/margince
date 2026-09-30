# Import purchased employment history

What a user does on the contact — adding phones and emails, reading the
**Companies** section, **Edit employment** and **Match company** — is in the
handbook, [records.md](../handbook/records.md). This page is the operator's:
installation, matching rules, backfill and recovery.

## Installation and upgrade

1. Run the normal additive migrations before starting the updated API and worker. The employment status migration adds nullable status/precision columns, the resolution ledger and the retained-claim processing marker. Existing relationships preserve their previous semantics.
2. Start the event relay, normal workers and the company auto-enrichment consumer. The employment sweep processes newly retained current-employment and job-history claims without calling Surfe again.
3. Check the existing company auto-enrichment setting and daily research budget. Company creation emits the normal event that schedules research through this policy. Linking is durable even if research is disabled or fails. Use the company's research view to inspect and retry research.
4. Verify one empty phone can be added and survives refresh. Rehearse history linking on a test contact and confirm the reciprocal company contact list excludes former/unknown work.
5. Preview existing purchases, then apply bounded batches as described below. The migration leaves pre-upgrade purchases for this deliberate backfill; it does not bulk-import them on startup.

## Matching and review

An exact domain identity reuses the company. A name alone requires confirmation, even if it resembles an existing name. A valid explicit provider domain can create a missing company using the existing identity checks. Name similarity or a company label that merely resembles a domain cannot authorize creation. Unresolved entries remain visible in Companies: choose an existing company or confirm its website using **Match company**. One company choice applies to all unresolved roles in the same employer group; dates and status in that form correct the selected role only. Dismiss evidence that should not become a relationship.

Repeating an import does not recreate a linked episode or undo a removal. Human corrections remain authoritative. Roles with different dates/titles remain distinct. Removing purchased evidence retracts untouched provider-owned relationships only; manually edited relationships and shared companies survive.

Research runs independently through the existing company pipeline. An unresolved identity needs matching before a company can be researched. A linked company without a website needs one before research can proceed. A linked company awaiting policy evaluation is not proof of a completed research job.

## Backfill existing purchases

Use an authenticated administrator and the ordinary API client. Do not paste credentials into a script committed to the repository.

- `GET /v1/contacts/{id}/employment-import` previews one contact's retained history and recorded outcomes without writes or paid calls.
- `POST /v1/contacts/{id}/employment-import` with `{"action":"apply"}` reconciles that contact. `resolve` and `dismiss` address the opaque `key` returned by preview.
- `POST /v1/employment-import/backfill` with `{"apply":false,"limit":10}` previews the first batch. Inspect `reports`; the default is read-only.
- Apply with `{"apply":true,"limit":10}`. Supply the returned `next_cursor` as `after` for the next batch until `has_more` is false. Replays reconcile the ledger and return current outcomes; these endpoints do not cache HTTP responses.

Keep the batch reports as the operational record. They identify linked, needs-match, needs-review and dismissed episodes. Repeating a batch is safe after an interrupted request. A failed batch can have committed earlier contacts: retry the same cursor, then continue after its successful response. No Surfe lookup is purchased by preview or apply; creating companies can trigger research subject to the installation's settings and budget.

## Recovery

Restarting the worker resumes unprocessed retained claims. Malformed purchases become visible review items without blocking valid roles. Transient failures retry with exponential delays, up to six attempts; then processing stops and the contact shows an administrator-review warning. Correct the evidence or configuration and apply that contact explicitly to retry. Correct unresolved identities in the contact UI and retry. Do not repair an import by deleting a shared company.

For application rollback, stop employment processing first and retain the populated schema. Retract only untouched imported contributions through the provider-data controls if needed. The down migration refuses undated historical/unknown rows that would acquire a different meaning under the old reader. Do not force that rollback by deleting customer history.
