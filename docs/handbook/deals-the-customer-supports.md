<!-- prose:plain -->
<!-- describes: backend/internal/modules/deals/viability.go backend/internal/compose/dealcloselost.go backend/internal/modules/deals/closelost.go backend/internal/compose/dealverdictpass.go backend/internal/compose/dealverdict/guards.go backend/internal/modules/deals/verdictapply.go -->
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

## When the customer names a date, a price or a decision

Each night Margince also reads the mail, meetings, calls, notes and offers on
every open deal. It may then set three things for the owner:

- **The close date**, when the customer names the day they will decide or sign.
- **The amount**, when the customer or your offer names a price for the deal.
- **The win probability**, when the evidence clearly moves it, such as a
  verbal yes or a budget freeze.

Margince reads a deal again only when something new arrived on it, when its
close date passed, or when its owner changed a switch.

Margince reads only what everyone in the workspace may read. A message only
its participants can see never reaches the reason or the evidence.

Margince leaves a value alone in these cases:

- It is not sure enough of what the words say.
- A colleague changed the field after the newest message the change rests on.
- You set the win probability yourself.
- The new close date is in the past or more than 18 months ahead.
- An accepted offer fixes the amount. Margince may still set the deal to the
  exact amount of that offer.
- The price is not in the words of the message Margince cites.
- The price is in another currency and no exchange rate is stored for it.

A price in another currency is converted into the deal's currency at the stored
rate, through the base currency. It is rounded to whole units. The receipt
names the customer's words, the amount they named and the date of the rate.

Each change shows on the owner's **Home** under **Handled for you**, with the
old and new value, the reason and **Undo**. It stays there until the owner
accepts or undoes it. The deal's timeline gets one note for the night's
changes.

**Undo** puts the old value back. It is refused when someone changed the field
since. After an Undo, Margince does not make that change on the deal again
until something new arrives on it: a message, an offer or a stage move.

The switches are **Close dates**, **Deal amounts** and **Win probabilities**
among the automatic changes. Margince checks the switch again just before it
writes.
