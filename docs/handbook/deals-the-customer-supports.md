<!-- prose:plain -->
<!-- describes: backend/internal/modules/deals/viability.go backend/internal/modules/assurance frontend/src/features/analytics/analytics.forecast.enforcement.tsx -->
# Deals the customer still supports

The nightly check asks one more thing of every open deal: has the customer said
anything lately? It counts the days since the customer last wrote to you, or
since your last meeting with them took place. Mail you sent, a field you changed
and a task do not count.

After 60 days with no word from the customer, the deal gets a review in
**Checks before the call** on the **Forecast** section of **Analytics**. The
review is named **Customer support needs review**, and the owner of the deal gets one
task. The close date does not change this: a deal three months from its close
date gets the same review.

A reply that turns the deal down does not count. When the last message from the
customer says no, the deal gets the review at once, without waiting for 60 days.

A reply from the customer does not end the review. The review stays until
somebody answers it, so that somebody checks the next step and the close date.
Completing the task alone does not answer it, and no second task comes for the
same review.

A review takes two answers. **Value is correct** keeps the deal counted until a
date you choose, at most 90 days from today, and asks for a reason. **Remind
later** keeps the review open until the day you pick. When an override ends,
the deal is not counted until the next check looks at it again.

An answer to the older check, **Buyer has gone quiet**, still
holds. Until it ends, the deal gets no review. A reminder from that check holds
for at most 90 days from the day it was given.

## Count only deals the customer supports

An admin can turn on **Count only deals the customer supports** in **Checks
before the call**. It can be turned on once the first check has run. Beside
the switch, the admin sees how many deals it would leave out right now. If
that number changes before the admin switches, the switch is refused and shows
the new number. While it
is on:

- The forecast answer adds a line: won deals plus confirmed commits that the
  customer still supports.
- A deal with an open review is left out of that line. The screen says how many
  deals were left out and how much money they carry.
- A deal the customer still talks about is left out too when it has no next
  step. The same goes for a close date that was only guessed or has passed.
  Such a deal gets no review, and it has a line of its own.
  Set the close date or plan the next step, and it counts again after the next
  check.
  While the customer has asked you to wait, the wait is the next step, but the
  close date must not fall before the wait ends.
- A deal the last check could not judge is not counted either, and has a line of
  its own. That happens when the mailbox could not be read, when the deal is
  new, or when something on it changed after the check.
- The other figures do not change.
- The nightly close-date sweep stops lowering forecast categories.

Turning it off removes the extra line at once. The reviews and their tasks stay.
