<!-- prose:plain -->
<!-- describes: backend/internal/modules/deals/viability.go backend/internal/modules/assurance frontend/src/features/analytics/analytics.forecast.enforcement.tsx -->
# Deals the customer still supports

The nightly check asks one more thing of every open deal: has the customer said
anything lately? It counts the days since the customer last wrote to you, or
since your last meeting with them took place. Mail you sent, a field you changed
and a task do not count.

After 60 days with no word from the customer, the deal gets a review in
**Checks before the call** on the **Forecast** section of **Analytics**. The
review is named **Customer has gone quiet**, and the owner of the deal gets one
task. The close date plays no part: a deal three months from its close date gets
the same review.

A reply from the customer does not end the review. The review stays until
somebody answers it, so that a person checks the next step and the close date.
Completing the task alone does not answer it, and no second task comes for the
same review.

## Count only deals the customer supports

An admin can turn on **Count only deals the customer supports** in **Checks
before the call**. It can be turned on once the first check has run. While it
is on:

- The forecast answer adds a line: won deals plus confirmed commits that the
  customer still supports.
- A deal with an open review is left out of that line. The screen says how many
  deals were left out and how much money they carry.
- A deal the last check could not judge is not counted either, and has a line of
  its own. That happens when the mailbox could not be read, when the deal is
  new, or when something on it changed after the check.
- The other figures do not change.
- The nightly close-date sweep stops lowering forecast categories.

Turning it off removes the extra line at once. The reviews and their tasks stay.
