<!-- prose:plain -->
<!-- describes: backend/internal/modules/deals/viability.go backend/internal/compose/dealcloselost.go backend/internal/modules/deals/closelost.go backend/internal/compose/dealverdictpass.go backend/internal/compose/dealverdict/guards.go backend/internal/modules/deals/verdictapply.go backend/internal/modules/deals/dealrevive.go backend/internal/compose/dealscoutrevive.go -->
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

- **The close date**, when a message names the day or the month the customer
  will decide or sign. Margince sets a date only inside a day, a month or a
  quarter the words of the message name.
- **The amount**, when the customer or your offer names a price for the deal.
  Margince sets the price the words state, to the cent.
- **The win probability**, when the evidence clearly moves it. Margince may
  lower it for any reason the evidence gives. It never raises it above the
  probability of the deal's stage.

Margince reads a deal again only when something new arrived on it, its close
date passed, or its owner changed a switch. A deal whose price had no exchange
rate is read again once the rate is stored.

Margince reads only what everyone in the workspace may read. A message only
its participants can see never reaches the reason or the evidence.

Margince leaves a value alone in these cases:

- It is not sure enough of what the words say.
- A colleague changed the field after the message the change rests on arrived.
- You set the win probability yourself.
- The new close date is in the past, more than 18 months away, or not in the
  words of the message.
- An accepted offer fixes the amount. Margince may still set the deal to the
  exact amount of that offer.
- The price is not in the words of the message, or the words state another
  price.
- The price is in another currency and no exchange rate is stored for it.
- Something on the deal changed while Margince was reading it. The deal is
  read again the next night.

A price in another currency is converted into the deal's currency at the stored
rate, through the base currency. It is rounded to whole units. The receipt
shows the customer's words, the amount they named and the date of the rate.

A close date from the customer is no longer an estimate, so Margince marks it
as confirmed. **Undo** marks it as an estimate again.

Each change shows on the owner's **Home** under **Handled for you**, with the
old and new value, the reason and **Undo**. It stays there until the owner
accepts or undoes it. The deal's timeline gets one note for the night's
changes.

**Undo** puts the old value back. It is refused when someone changed the field
since. After an Undo, Margince does not make that change on the deal again.
That lasts until a new message, offer or stage move arrives on the deal.

The switches are **Close dates**, **Deal amounts**, **Win probabilities**,
**Stage moves**, **New deals** and **Reopened deals** among the automatic changes. Margince checks the switch again
just before it writes.

## When the deal reached a later stage

Margince may move an open deal to a later stage. It never moves a deal back,
and never to won or lost. It moves a deal only across stages that list
criteria. A message, meeting or note on the deal must show each of the
criteria of each stage the deal leaves. **Undo** moves the deal back, unless someone moved it since. An admin
can stop all stage moves in the installation settings.

## When the customer asks for an offer

Margince looks for a company with no open deal where a customer asked for an
offer or a proposal. If the company has an owner, Margince opens the
deal for that owner at the first stage. It sets an amount only when the
customer wrote a number and a currency. Home, the morning mail and the deal show
what it read, and when.

**Undo** archives the deal. Margince then does not open a deal on the same
evidence; a new request opens one again. If your team added notes or tasks to
the deal, Undo asks first.

A company with no owner gets a suggestion instead, as does one whose owner
switched **New deals** off or left.

A company whose last closed deal was lost never gets a second deal beside it.
Margince reopens the lost deal instead, as below.

## When a lost deal's customer comes back

When a customer asks for an offer again after their deal was lost, Margince
reopens the lost deal for its owner. It takes the last closed deal of the
company, and only when that deal was lost. The deal goes back to the first open
stage of its pipeline, never to won. Margince counts only messages, meetings and
documents that reached it after the deal was lost.

Margince reopens a deal only when all of these hold:

- The company has no open deal.
- The deal has an owner who still works in the workspace, with **Reopened
  deals** switched on.
- The deal is not on legal hold.
- Nobody on your team closed the deal in the last 30 days. A close by Margince
  does not count.
- Everyone in the workspace may still read the evidence.

The reopen shows on the owner's **Home**, in the morning mail and on the deal,
with the reason and **Undo**. Only the owner reads the reason. The deal's
timeline gets a note about the reopen.

**Undo** closes the deal as lost again, with the reason and the close date it
had. Undo is refused when someone moved the deal to another stage since. If
your team added notes or tasks to the deal since, Undo asks first. After
an Undo, Margince reopens the deal only for evidence that arrived after it.

With no owner, an owner who left, or the switch off, the suggestion stays for
your team to decide. Evidence that reached Margince before the deal was lost is
not used.
