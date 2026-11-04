<!-- prose:plain -->
<!-- describes: backend/internal/modules/deals/viability.go backend/internal/compose/dealcloselost.go backend/internal/modules/deals/closelost.go -->
# Deals the customer still supports

The nightly check asks one more thing of every open deal: has the customer said
anything lately? It counts the days since the customer last wrote to you, or
since your last meeting with them took place. Mail you sent, a field you changed
and a task do not count. A reply that turns the deal down does not count either.

Margince asks nobody to review such a deal. It closes the deal itself, as
below, and you can undo that.

## When the customer stops answering for 120 days

After 120 days with no word from the customer, Margince closes the open deal as
lost, with the reason **Stopped responding**. When the customer's last message
says no, Margince closes the deal at once, with the reason **Other**. An admin
can change the number of days with the setting `deals.silent_close_days`. It
must be 60 or more.

Margince closes a deal only when all of these hold:

- The deal has an owner who still works in the workspace.
- The owner's mailbox was read in the last two days.
- Every mailbox that brought in mail on the deal was read in the last two days,
  a colleague's included.
- The deal is not on legal hold.
- No wait the customer asked for, and no meeting they accepted, is still to come.

A message from the customer that only its participants can see also keeps the
deal open. Margince reads only its date.

Margince closes the deal for its owner. The close shows on the owner's **Home**
under **Handled for you**, with the reason and **Undo**. It stays there until the
owner accepts it or undoes it. The deal's timeline gets a note about the close.

**Undo** puts the deal back in the stage it was closed from. Changes made to the
deal since the close stay as they are. Undo is refused when someone moved the
deal to another stage after the close.

After an Undo, add a note on why the deal is still open. A note, a logged call
or a meeting after the Undo starts the 120 days again. With none, Margince
closes the deal again at the next check.

To stop it for your own deals, turn off **Close silent deals as lost** among
the automatic changes. [Approvals](approvals.md) says where the switches are.
