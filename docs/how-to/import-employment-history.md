<!-- prose:plain -->
# Import purchased employment history

The handbook page [records.md](../handbook/records.md) covers what a user does on the contact.
There, a user adds phones and emails, reads the **Companies** section, and uses **Edit employment** and
**Match company**. This page is for the operator: install, the match rules, the backfill and repair.

## Install and upgrade

1. Run the normal additive migrations before you start the new API and worker. The employment status
   migration adds the `status` and `precision` columns, the resolution ledger, and a marker on retained claims.
   Old relationships keep their old meaning.
2. Start the event relay, the normal workers and the consumer that enriches companies on its own. The
   employment sweep works through new retained claims of current employment and job history. It makes no
   new call to Surfe.
3. Check the setting that enriches companies on its own, and the daily research budget. A new company
   sends the normal event, which plans research through this policy. A link holds even when research is
   off or fails. Use the research view of the company to look at research and to try it again.
4. Add one empty phone, and check that it is still there after a refresh. Try a history link on a test
   contact. Confirm that the contact list on the company leaves out past and unknown work.
5. Preview the old purchases, then apply them in small batches, as the backfill section shows. The
   migration does not import old purchases when the app starts; the backfill does that.

## Match and review

An exact domain match uses the company that exists. A name alone needs a human to confirm it, even when it looks
like a name that exists. A valid domain that the provider gives can create a missing company, through the
identity checks that exist today. A name that looks the same, or a company label that only looks like a domain,
can never create a company.

Open entries stay in Companies. To close one, choose a company that exists, or confirm its website, with
**Match company**. The company you choose applies to every open role in the same company group. The dates
and status in that form change only the role that the form is for. Dismiss any evidence that should not become a
relationship.

When you import again, the import does not make a linked episode again, and it does not put back a relationship you removed.
A human change always comes first. Roles with different dates or titles stay separate. When you remove
purchased evidence, it takes back only the relationships the provider owns and no human changed. A
relationship a human changed by hand, and a company that others share, both stay.

Research runs on its own, through the company pipeline that exists today. An open identity needs a
match before someone can research the company. A linked company without a website needs one before
research can go on. A linked company that waits for a policy check does not prove that a research job
has run.

## Backfill old purchases

Use an admin account and the normal API client. Never put credentials into a script that you commit to
the repository.

- `GET /v1/contacts/{id}/employment-import` shows a preview of the retained history and the stored outcomes of one contact. It writes nothing and makes no call that costs money.
- `POST /v1/contacts/{id}/employment-import` with `{"action":"apply"}` updates that contact. `resolve` and `dismiss` take the `key` from the preview, as it is.
- `POST /v1/employment-import/backfill` with `{"apply":false,"limit":10}` shows a preview of the first batch. Read the `reports`; by default the call only reads.
- To apply, send `{"apply":true,"limit":10}`. Send the `next_cursor` it returns as `after` for the next batch, until `has_more` is false. A replay updates the ledger and returns the current outcomes. These endpoints do not cache HTTP answers.

Keep the batch reports as the record of the work. They name the linked episodes, and the ones that need
a match, need review, or that a human dismissed. You can run a batch again after a request breaks off. A failed
batch can still have saved some contacts first: try the same cursor again, then go on after its good
answer. Preview and apply buy no Surfe lookup. A new company can start research, as the settings and
budget of the install allow.

## Repair

When the worker starts again, it goes on with the retained claims it has not worked through. A wrong
purchase becomes a review item, and it does not block the valid roles. When a step fails for a short time, the worker tries again,
with more time between tries, up to 6 tries. Then the work stops, and the contact shows a warning for
an admin to review.

Fix the evidence or the settings, then apply that contact by hand to try again. Fix
open identities in the contact screen, then try again. Never repair an import by deleting a company that
others share.

To go back to the old version of the app, first stop the employment work and keep the schema with its data. If you need to,
take back only the imported data that no human changed, with the provider data tools. The
down migration refuses old or unknown rows with no dates, because the old reader would read them
another way. Never make that step pass by deleting customer history.
