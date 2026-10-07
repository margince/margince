<!-- prose:plain -->
# The pipeline: how a deal moves

The pipeline in Margince is the ladder of stages every deal moves through. Deals
live under **Deals** in the sidebar. That screen lists them as a **Table**, or
shows them as a **Board** with one column per stage. The **Pipeline** picker on
that screen names which ladder you are looking at.

## Deal tasks

### How do I move a deal to the next stage?
To move a deal to another stage in Margince, open the deal from **Deals** and click the stage you want on its **Stage** ladder. Or drag its card to another column on the **Board** view.
1. Open **Deals** in the sidebar and open the deal, or switch the list to **Board**.
2. Click the stage you want on the ladder, or drag the card onto that stage's column.
3. The dialog asks **Move to {stage}?**. Press **Confirm**, or **Cancel**.

The move is saved at once ("Moved to {stage}"); there is no save button. Dragging does not work on touch screens, so use the ladder on a touch screen. Also called: advance a deal, change deal stage, progress an opportunity.

### How do I move several deals to a stage at once?
To move many deals at once, tick them on the **Deals** list and use **Move to stage** in the bar that appears.
1. On **Deals**, tick the box of each deal.
2. Choose **Move to stage**, then **Pick a stage**, then **Move**.

This offers open stages only, and skips deals already on that stage. You cannot win or lose many deals at once: close each one on its own page. Only open deals that are not archived have a box to tick. Also called: bulk edit, mass update deal stage.

### How do I mark a deal as won?
To mark a deal as won in Margince, move it to a stage whose **Stage type** is **Won**. In the pipeline you start with, that is the stage called Won. There is no separate Won button.
1. Open the deal and click the Won stage on its **Stage** ladder. Or drag the card to the Won column on **Board**.
2. The dialog says "This closes the deal as won. Nothing changes until you confirm." Press **Confirm**.
3. With a signed contract attached, the deal closes. Without one, the dialog comes back asking **How was it won?**.
4. Pick an answer and press **Confirm** again. **Other** also needs **Details**.

Also called: close won, close a deal as won, win an opportunity.

### How do I mark a deal as lost?
When a customer or client says no, mark the deal as lost. Move it to a stage whose **Stage type** is **Lost** and give a **Lost reason**. There is no separate Lost button.
1. Open the deal and click the Lost stage on its **Stage** ladder. Or drag the card to the Lost column on **Board**.
2. Type the **Lost reason**. You must give one: **Confirm** stays off until the box says something.
3. Press **Confirm**.

The reason is kept on the deal. If the customer comes back later, reopen the deal instead of creating a new one. Pressing **Cancel** or Escape, or clicking outside, clears what you typed.
Also called: close lost, lose a deal, mark an opportunity lost, the customer said no.
Also called: the client declined, we lost the deal, the prospect said no.

### How do I reopen a closed deal?
To reopen a won or lost deal in Margince, open it and choose **More actions** → **Reopen**, then pick the open stage it goes back to.
1. Open the closed deal. Its ladder is grey: "This deal is closed. Reopen it to move it to another stage."
2. Choose **More actions** → **Reopen**.
3. Under **Move this deal back to an open stage**, pick a stage and press **Reopen**.

Reopening clears the close date and the exchange rate fixed at close, and the dialog does not say so. It needs permission to change the deal, and you cannot reopen an archived deal. Also called: undo a close, reopen a lost deal.

### How do I bring back a deal that went cold?
A deal that went cold is still open unless someone closed it. Margince marks an open deal **stalled** after more than 60 days with no activity, and real activity brings it back.
1. On **Deals**, turn on the **Stalled only** filter to find them.
2. Open the deal and send a mail, or use **Log activity** for a call, meeting or note.
3. If the buyer asked you to wait, edit the deal and set **Wait until** to a later date. The mark stays hidden until then.

Any real activity clears the mark; opening the record does not. If the deal was closed as lost, reopen it instead. Also called: revive, re-engage, stale deal.

### How do I change the pipeline stages?
To add, rename, reorder or remove stages, open the account menu → **Settings** → **Pipelines** (in the **Sales** group). Then choose the pipeline in the list.
1. To add a stage, choose **New stage**. Fill in **Name**, **Stage type** (Open, Won or Lost) and **Win probability** (0 to 100).
2. A new open stage goes after the other open stages.
3. To change a stage, choose **Edit stage** on its row.
4. To reorder, drag an open stage by its handle, or select the handle and press the up or down arrow.
5. The order is saved when you let go, and **Undo** in the note puts it back. Won and Lost always stay last.
6. To delete a stage, choose **Remove** → **Remove stage**. Move its deals off it first.

A stage whose win probability is lower than the stage above it is marked, but the order is still saved. A role that cannot edit pipelines sees "Read-only. Your role cannot change pipelines or stages."

An agent is always refused. Also called: deal stages, sales process, customise the pipeline, reorder stages.

### How do I create another pipeline?
To add a pipeline, open **Settings** → **Pipelines** and choose **New pipeline**.
Give it a **Name** and choose **Default** or **Not default**. It starts with a Won and a Lost stage, so add its open stages with **New stage**. The list at the top shows every pipeline with the shape of its stages. Drag one by its handle to change the order pipelines are offered in. Choose **Make default** on a pipeline to make new deals go there.

New deals need a default pipeline. To stop using a pipeline, choose **Retire**. It leaves the pickers and the forms for new deals, its deals keep their stage and history, and **Restore** brings it back. You cannot retire the default pipeline until another one is the default. Margince does not delete pipelines. Also called: second sales process, separate pipeline.

### How is the weighted pipeline value calculated?
The weighted value of a deal in Margince is its value times its stage's **Win probability**, rounded for each deal. A column's or report's weighted total is the sum of those rounded numbers.

So a 10,000 EUR deal on a stage at 50 counts as 5,000. A Won stage counts at 100 and a Lost stage at 0. Because each deal is rounded before the sum, **Explain this number** always adds up to the total. The Analytics reports turn all money into your base currency first. Also called: weighted forecast, probability-weighted pipeline, expected value.

## Pipelines and stages

A **pipeline** is a ladder of stages with a name. You can have more than one,
and at most one of them is the default. If you clear the default on the only
pipeline that has it, the installation has none. Margince allows that, and
nothing warns you.

A **stage** carries four things:

- a **name**, which you choose
- a **place** in the ladder
- a **stage type**, one of **Open**, **Won** or **Lost**
- a **win probability**, a whole number from 0 to 100

The stage type is the only fixed choice. The names are all yours.

### The pipeline you start with

A new company in Margince starts with one pipeline, called **Sales**, with six
stages: Qualified, Discovery, Proposal, Negotiation, Won and Lost.

| Stage | Stage type | Win probability |
|---|---|---|
| Qualified | Open | 10 |
| Discovery | Open | 25 |
| Proposal | Open | 50 |
| Negotiation | Open | 75 |
| Won | Won | 100 |
| Lost | Lost | 0 |

Rename them, reorder them, add your own. Two rules you cannot change: a won stage
is always 100 and a lost stage is always 0.

You edit pipelines and stages at **Settings → Pipelines**, in the Sales group.
Only a human can edit them. An agent is refused, because every "should this deal
move?" idea is judged against the stage ladder.

Removing a stage tells you what happens. The stages after it move up, and past
stage changes still read well. Deals still on it have to move first.

## Moving a deal

A deal moves three ways. Drag it on the board, or click a stage on the ladder on
the deal page. Or select several and use **Move to stage**. Margince saves the
move at once and says so ("Moved to Discovery"), with no save button.

Dragging on the board does not work on touch, so use the ladder on a touch screen. If
two colleagues move the same deal at once, the second is refused, so it does not
write over the first. A deal can only move to a stage in its own pipeline, and
you close deals one at a time.

## Closing a deal

Closing a deal is a real event, and Margince handles it as one. Moving to a won
or lost stage asks first:

> **Move to Lost?** This closes the deal as lost. Nothing changes until you
> confirm.

### Losing

**A lost deal needs a reason.** The **Lost reason** box must say something
before **Confirm** works. If you cancel (or press Escape, or click outside),
anything you typed is cleared.

### Winning

> **Won asks what is behind the win.** Margince takes a won deal two ways. It
> refuses only a win that says nothing at all.

**Either** there is a signed contract on the deal. Then you press Confirm and it
closes with no more questions. "Signed contract" asks more than it sounds. The
contract must not be archived, must be past draft, and must carry a signed date.
It must also have a file attached that is not archived, filed under Contract or
Legal, in the Current or Final state. A contract record with no file on it, or
only archived files, is not enough.

**Or** there is not. You will not see the question until you press Confirm.
When you press Confirm without a signed contract, the dialog comes back with a
question:

> **How was it won?** No signed contract is attached. Record how the deal was
> won; the answer is kept on the deal and counted in reports.

The **How was it won?** list offers these answers:

- On a purchase order
- Verbally, in person or by phone
- Renewed by email
- Imported from another system
- Other, which then needs **Details**, because "other" says nothing on its own

This lets you answer "how many of our wins have no paper, and why". A win with a
contract carries no reason at all, so your reports can tell the two apart. The
reason shows on the deal's top line beside the won badge, with the detail you
typed if the answer was Other.

One thing missing today: the contract form has no deal field. So attaching a contract to
the deal you are winning takes a step the form does not offer. Many wins go
through the reason instead.

### The outcome review

A closed deal in Margince carries an **Outcome review** panel. It holds why the
deal went the way it did, written down while you still know the reasons. You do
not have to fill it in. Closing never waits on it, and a deal with no review says "No
review written yet." An open deal has no outcome to review, so it has no panel.

Margince also offers you the review the moment you close, in a dialog that opens
with the close dialog's own reason already filled in. If you say no there, the
panel keeps waiting on the deal.

An administrator writes the questions at **Settings → Outcome reviews**: one
set for won, one for lost. Each question has its answer type, whether an answer
must be given, and its choices if it offers any. Writing a review is logging an
activity, so it needs the same permission as logging one.

Rules worth knowing:

- Editing the questions changes later reviews only. A review already written
  keeps the questions it was asked and the answers given.
- A review is for one closing of the deal. Close, reopen and close again,
  and there is a new outcome to review. The older review still reads well,
  marked "From an earlier close", so nobody takes a deal lost in March for a
  deal won in June.
- A deal closed before Margince recorded closings takes no new review. The
  reviews it already has still read well.

The outcome review is separate from the "How was it won?" answer. That one is
asked in the close dialog, and only when there is no signed contract.

### Currency at close

When a deal closes in a currency other than your base currency, the exchange rate
is **fixed onto the deal** at that moment. So the numbers of past months do not
move when rates change.

## Reopening

Reopening a closed deal is how a won or lost deal goes back into the pipeline.
On a closed deal the stage ladder does nothing, and every stage is grey.
**Reopen** sits in the header's **More actions** menu, which only appears on a
won or lost deal. It asks which open stage to go back to. On the way it **clears
the close date and the exchange rate fixed at close**, and the dialog does not
tell you.

Margince handles reopening the same way as closing, because it takes
money back out of a time that was already reported.

Reopening a won deal does not delete a partner's commission on it. Margince adds
a row that takes it back instead, as [Partners and commission](partners.md)
explains.

## Stalled deals

**A deal in Margince is stalled when it is open and nothing has touched it for
more than 60 days.** The deal card then carries a **stalled** badge. "Touched"
means real activity (a mail, a meeting, a note). Opening the record does not
count. At 60 days a deal is not yet stalled; past 60 it is.

A **wait until** date that is still to come hides the stalled mark, and the mark
comes back on its own after that date. It does not hide a close date that has
passed, which is a different thing and stays in view.

There is a second, shorter time: **19 days** with no activity makes a deal
*quiet*. The morning screens notice that well before the deal reaches the 60
days for stalled. "Quiet" and "stalled" say different things about the same
deal, and the text beside a deal always names the time it used. The Worklist's
**Deals at risk** work runs on the 19 days instead of the stalled mark.

Say you pressed **Dismiss** on a deal in the Worklist. It stays out of later
lists until a new activity is linked to it after you dismissed it. Then it comes back,
and says why. Nothing else (a stage move, an offer running out) brings a
dismissed deal back.

## Where a deal came from

A deal carries an **acquisition source**: the business channel it came from. It
is a different field from a lead source. A lead source records how a record
reached Margince; an acquisition source records which channel won the business.

The list is for an administrator, at **Settings → Acquisition sources**. A name
you add makes a key that never changes after that, so renaming it later keeps
the reports behind it whole. You can retire a source instead of deleting it.
Deals that already name it still read "(retired)" instead of going empty. A deal
that names none reads "Not set".

## Forecast category

A deal's forecast category is separate from its stage, and is your call. You set it when you edit the deal, to **Commit**, **Best case**,
**Pipeline** or **Omitted**.

Two more appear in reports, but nobody chooses them: **Slipped** and **No
category**. Margince marks a Commit or Best case deal Slipped when its close date
has passed, is missing, or is still only a guess. Nobody sets it; it is what the
dates say.

## Reading the numbers

The deals board in Margince loads 100 deals at a time. But the totals at the top
of each column count every matching deal, the ones not loaded yet too. Each
column header carries the stage name, its win probability, how many deals are in
it, the stage total, and under that the weighted total.

The totals count every deal you may see, which are the deals the board shows as
cards. They are held back when a tag filter is on ("Loaded deals only. No total
while a tag filter is on."). They are also held back when the owner filter names
someone whose numbers you may not measure ("Loaded deals only. This owner’s
totals are outside what you may measure."). You may measure yourself, the
members of teams you lead, and anyone if your access covers the whole company.

Every deal report has an **Explain this number** control that shows the rows the
number was built from. If a number looks wrong, open it.

The deal reports are **Open deals by stage**, **Forecast categories**, **Open
deals per company**, and the sales and time-in-stage reports in **Performance**. They
live under **Analytics**; see [Analytics and forecasting](analytics.md).

## Stage automation: the evidence before trusting a move

Stage automation in Margince means the product suggests stage moves, and a stage
move may later move deals by itself. Before you trust it to, there is a record of
how its ideas really went, at **Settings → Stage automation** (in the Sales
group).

The report itself is read-only. Under it sit the rules that decide what each
stage move may do, and changing one of those needs permission to edit
pipelines. The page says so: "You can see each transition’s record. Changing a
transition requires permission to edit pipelines."

Under **Transition rules**, turning a stage move on does not start moving deals.
Margince keeps asking until the record is good enough. Then it moves deals by
itself and tells you after. A stage move that Margince has stopped shows
**Resume**, which still has to reach the same bar.

It reports three counts for each pipeline and stage move, over a number of
days. It counts how many ideas someone **Reviewed**, how many are **Still
open**, and how many **Expired**. An expired idea is one nobody answered in time; it does
not count as a no.

The reviewed ones are split into **Accepted as proposed**, **Accepted after edits**,
**Rejected**, and **Undone or corrected**. That last one is a move that was
undone, or whose evidence they marked wrong. Those four are shown as a share of
what was reviewed.

It also reports **Days observed**, from the first reviewed idea to the last. It
says why that counts: "A good rate from one afternoon is not a track record."

### How do I set up stage automation?
To let Margince move deals between stages by itself, open **Settings** → **Stage automation** (in the **Sales** group). Switch a stage move on under **Transition rules**.
1. Pick the pipeline and read each stage move's record: **Reviewed**, **Accepted as proposed**, **Rejected**, **Undone or corrected**, **Days observed**.
2. Under **Transition rules**, turn on the stage move you trust.

Nothing moves at once. Margince keeps suggesting moves for you to check until the record is good enough, then moves deals and tells you after. Changing a rule needs permission to edit pipelines. Also called: automatic stage moves, pipeline automation.

## Badges on a deal card

A deal card on the board can carry four badges:

- **stalled**: nothing for 60 days
- **archived**
- **single-threaded**: you know one contact at this company
- **staged**: the AI suggested something on this deal that nobody has accepted

Today the board shows only **stalled** and **archived**.

A deal is single-threaded when you have written with only one contact at the
company. Contacts listed on the deal do not count: a deal can list many of them
and still be single-threaded.

## The mail line on a deal card

Under the deal's name, a card says when mail last moved on the deal, and which
way. A letter means a message they sent, and an arrow one you sent. Rest the
mouse on it to see the last few subjects without opening the deal. **View all
activity** opens the deal's own timeline.

It counts the mail everybody in the company can read, the same way the stalled
badge does. A message shared only with the humans on it does not move the line.
So two colleagues always read the same date off the same card.

## Archiving a deal

Archiving a deal is different from closing it. A closed deal is a finished
part of your business; an archived deal has left your lists.

To archive a deal, open it and choose **More actions** → **Archive deal**, or
tick several on the list and use **Archive**. Archived deals leave every list and
report. **Show archived** lets you see one, read-only. To bring one back, open
it with **Show archived**, find the archive entry in its history and press
**Undo**. A closed or archived deal has no box to tick for acting on many at
once. You cannot merge two deals.

### Can I merge two duplicate deals?
No. Margince has no merge for deals: merging covers contacts and companies only. When one deal was entered twice, keep the best one and archive the other with **More actions** → **Archive deal**. Archiving carries nothing over, so move what you need first. Link its emails to the deal you keep with **Relink**, and note anything else on that deal. Also called: duplicate deal, combine two opportunities.

## Partners on a deal

A deal in Margince can name the partner that found it, under **via partner**.
Under **Partner attribution** it says whether the partner **Sourced the deal** or
only **Influenced an existing deal**. Commission builds up only on a sourced deal
that is won. Setting up partners, naming one on a deal, and approving their
commission or paying it are on [Partners and commission](partners.md).
