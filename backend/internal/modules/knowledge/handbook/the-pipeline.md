# The pipeline: how a deal moves

The pipeline in Margince is the ladder of stages every deal moves through. Deals
live under **Deals** in the sidebar, which lists them as a **Table** or draws
them as a **Board** with one column per stage; the **Pipeline** selector on that
screen names which ladder you are looking at. This page covers how to move,
win, lose and reopen a deal, how stages are set up, and what the numbers mean.

## Common deal tasks

### How do I move a deal to the next stage?
To move a deal to another stage in Margince, open the deal from **Deals** and click the stage you want on its **Stage** ladder, or drag its card to another column on the **Board** view.
1. Open **Deals** in the sidebar and open the deal, or switch the list to **Board**.
2. Click the target stage on the ladder, or drag the card onto that stage's column.
3. The dialog asks **Move to {stage}?**. Press **Confirm**, or **Cancel**.
The move is saved at once ("Moved to {stage}"); there is no save button. Dragging does not work on touch screens, so use the ladder on a tablet. Also called: advance a deal, change deal stage, progress an opportunity.

### How do I move several deals to a stage at once?
To move many deals at once, tick them on the **Deals** list and use **Move to stage** in the bulk bar.
1. On **Deals**, tick the checkbox of each deal.
2. Choose **Move to stage**, then **Pick a stage**, then **Move**.
Bulk move offers open stages only and skips deals already on that stage. You cannot win or lose deals in bulk: close each one on its own page. Only open, unarchived deals have a checkbox. Also called: bulk edit, mass update deal stage.

### How do I mark a deal as won?
To mark a deal as won in Margince, move it to a stage whose **Stage type** is **Won** (in the starter pipeline, the stage called Won). There is no separate Won button.
1. Open the deal and click the Won stage on its **Stage** ladder, or drag the card to the Won column on **Board**.
2. The dialog says "This closes the deal as won. Nothing changes until you confirm." Press **Confirm**.
3. With a signed contract attached, the deal closes. Without one, the dialog returns asking **How was it won?**: pick an answer and press **Confirm** again. **Other** also needs **Details**.
Also called: close won, close a deal as won, win an opportunity.

### What do I do when a customer says no?
When a customer or client says no, mark the deal as lost: move it to the pipeline's **Lost** stage and type the **Lost reason** they gave, then press **Confirm**. The reason is kept on the deal. If they come back later, reopen the deal rather than creating a new one.
Also called: the client declined, we lost the deal, the prospect said no.

### How do I mark a deal as lost?
When a customer says no, record it by marking the deal as lost. To mark a deal as lost in Margince, move it to a stage whose **Stage type** is **Lost** and give a **Lost reason**. There is no separate Lost button.
1. Open the deal and click the Lost stage on its **Stage** ladder, or drag the card to the Lost column on **Board**.
2. Type the **Lost reason**. It is required: **Confirm** stays disabled until it says something.
3. Press **Confirm**.
Pressing **Cancel** or Escape, or clicking outside, clears what you typed. Also called: close lost, lose a deal, mark an opportunity lost, the customer said no, the client declined.

### How do I reopen a closed deal?
To reopen a won or lost deal in Margince, open it and choose **More actions** → **Reopen**, then pick the open stage it goes back to.
1. Open the closed deal. Its ladder is greyed out: "This deal is closed. Reopen it to move it to another stage."
2. Choose **More actions** → **Reopen**.
3. Under **Move this deal back to an open stage**, pick a stage and press **Reopen**.
Reopening clears the close date and the frozen exchange rate, which the dialog does not say. It needs permission to change the deal, and an archived deal cannot be reopened. Also called: undo a close, reopen a lost deal.

### How do I bring back a deal that went cold?
A deal that went cold is still open unless someone closed it. Margince marks an open deal **stalled** after more than 60 days without activity, and real activity brings it back.
1. On **Deals**, turn on the **Stalled only** filter to find them.
2. Open the deal and send a mail, or use **Log activity** for a call, meeting or note. Any real activity clears the flag; opening the record does not.
3. If the buyer asked you to wait, edit the deal and set **Wait until** to a future date. The flag stays hidden until then.
If the deal was closed as lost, reopen it instead. Also called: revive, re-engage, stale deal.

### How do I change the pipeline stages?
To add, rename, reorder or remove stages, open the account menu → **Settings** → **Pipelines** (in the **Sales** group).
1. On the pipeline, choose **New stage**, or **Edit stage** on an existing one.
2. Fill **Name**, **Stage type** (Open, Won or Lost), **Win probability** (0 to 100) and **Position**, its place in the ladder.
3. To delete a stage, choose **Remove** → **Remove stage**. Move its deals off it first.
A role that cannot edit pipelines sees "Read-only. Your role cannot change pipelines or stages." An agent is always refused. Also called: deal stages, sales process, customise the pipeline.

### How do I create another pipeline?
To add a pipeline, open **Settings** → **Pipelines** and choose **New pipeline**.
Give it a **Name**, choose **Default** or **Not default**, then add stages with **New stage**. New deals need a default pipeline. To stop using a pipeline, choose **Retire**: it leaves pickers and new-deal forms, its deals keep their stage and history, and **Restore** brings it back. The default pipeline cannot be retired until another one is the default. Margince does not delete pipelines. Also called: second sales process, separate pipeline.

### How is the weighted pipeline value calculated?
The weighted value of a deal in Margince is its value multiplied by its stage's **Win probability**, rounded per deal. A column's or report's weighted total is the sum of those rounded figures.
For example, a 10,000 EUR deal on a stage at 50 weighs 5,000. A Won stage counts at 100 and a Lost stage at 0. Rounding each deal before summing is why **Explain this number** always reconciles exactly. The Analytics reports convert every amount to your base currency first. Also called: weighted forecast, probability-weighted pipeline, expected value.

## Pipelines and stages

A **pipeline** is a named ladder of stages. You can have more than one, and at
most one of them is the default. Clearing the default on the only pipeline that
has it leaves the installation with none, which the product allows and nothing
warns you about.

A **stage** carries four things: a **name**, yours to choose; a **position** in
the ladder; a **stage type**, one of **Open**, **Won** or **Lost**; and a **win
probability**, a whole number from 0 to 100. The stage type is the only fixed
vocabulary. The names are entirely yours.

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
is always 100 and a lost stage is always 0. Those are not opinions about your
sales process, they are what the words mean.

Pipelines and stages are edited at **Settings → Pipelines**, in the Sales group.
Only a human can — an agent is refused outright, because the stage ladder is the
ground truth that every "should this deal move?" decision is judged against.

Removing a stage tells you what happens: the stages after it move up, past stage
changes stay readable, and deals still sitting on it have to move first.

## Moving a deal

A deal moves three ways: drag it on the board, click a stage on the ladder on
the deal page, or select several and use **Move to stage**. The move is written
immediately and confirmed — "Moved to Discovery" — with no save button.

The board's drag does not work on touch, so use the ladder on a tablet. If two
colleagues move the same deal at once, the second is refused rather than
silently overwriting the first. A deal can only move to a stage in its own
pipeline, and closing is one deal at a time, on purpose.

## Closing a deal

Closing a deal is a real event, and Margince treats it as one. Moving to a won
or lost stage asks first:

> **Move to Lost?** This closes the deal as lost. Nothing changes until you
> confirm.

### Losing

**A lost deal needs a reason.** The **Lost reason** box must say something
before **Confirm** lights up. If you cancel — or press Escape, or click outside —
anything you typed is cleared.

### Winning

> **Won asks what is behind the win.** Margince accepts a won deal two ways, and
> both are legitimate — it just refuses to record a win that says nothing at all.

**Either** there is a signed contract on the deal, in which case you press
Confirm and it closes with no further questions. "Signed contract" is stricter
than it sounds. The contract must be unarchived, past draft, and carry a signed
date — *and* it must have a live attachment filed under Contract or Legal, in
the Current or Final state. A contract record with no paper on it does not
clear the bar.

**Or** there is not. You will not see the question until you press Confirm: the
dialog submits, the server refuses a win it cannot evidence, and the dialog comes
back carrying the question:

> **How was it won?** No signed contract is attached. Record how the deal was
> won; the answer is kept on the deal and counted in reports.

The **How was it won?** picker offers five answers: On a purchase order;
Verbally, in person or by phone; Renewed by email; Imported from another system;
and Other — which then needs **Details**, because "other" explains nothing on
its own.

The point is that "how many of our wins have no paper, and why" becomes a
question you can answer. A win with a contract carries no reason at all, so the
two are distinguishable in your reports. The reason is shown back on the deal's
identity line beside the won badge, with the detail you typed where the answer
was Other.

One honest caveat today: the contract form has no deal field, so attaching a
contract to the deal you are winning takes a step the form does not offer, and
in practice many wins go through the reason instead.

### The outcome review

A closed deal in Margince carries an **Outcome review** panel: why the deal went
the way it did, recorded while the reasons are still fresh. It is offered, not
demanded — nothing about closing waits on it, and an unreviewed deal says so
plainly: "No review written yet." An open deal has no outcome to review, so the
card is absent rather than empty.

You are also offered the review at the moment you close, in a modal that arrives
with the close dialog's own reason already filled in. Declining it there costs
nothing — the panel keeps waiting on the deal.

An administrator writes the questions at **Settings → Outcome reviews** — one
set for won, one for lost. Each question carries its answer type, whether an
answer is required, and its choices where it offers any. Writing a review is
logging an activity, so it takes the permission that logging one takes.

Three rules worth knowing:

- **Editing the questions changes future reviews only.** A review already
  written keeps the questions it was asked and the answers given.
- **A review belongs to a closing, not to a deal.** Close, reopen and close
  again and there is a new outcome to review; the older review stays readable,
  marked "From an earlier close", so March's loss is never mistaken for
  June's win.
- **A deal closed before closings were recorded takes no new review.** Its
  existing ones stay readable.

This is not the "How was it won?" answer, which is asked in the close dialog
and only where there is no signed contract.

### Currency at close

When a deal closes in a currency other than your base currency, the exchange rate
is **frozen onto the deal** at that moment. That is what stops last quarter's
reported numbers from moving when rates change.

## Reopening

Reopening a closed deal is how a won or lost deal goes back into the pipeline.
A closed deal's stage ladder is inert, every stage greyed out; **Reopen** sits in
the header's **More actions** menu, which only appears on a won or lost deal.
It asks which open stage to return to, and **clears the close date and the
frozen exchange rate** on the way, which the dialog does not tell you.

Reopening is treated as seriously as closing, because it takes revenue back out
of a quarter that has already been reported.

Reopening a won deal does not delete a partner's commission on it; a reversal
row is added instead, as [Partners and commission](partners.md) explains.

## Stalled deals

**A deal in Margince is stalled when it is open and nothing has touched it for
more than 60 days.** The deal card then carries a **stalled** badge. "Touched"
means real activity — a mail, a meeting, a note — not someone opening the
record. Exactly 60 days is not yet stalled; past 60 is.

Setting a **wait until** date in the future hides the stalled flag, and it comes
back on its own afterwards. It does not hide an overdue close date — that is a
different problem and stays visible.

There is a second, shorter window: **19 days** without activity makes a deal
*quiet*, which the morning surfaces notice well before it meets the 60-day
stalled bar. "Quiet" and "stalled" are different claims about the same deal, and
the copy beside a deal always names the window it used. The Worklist's **Deals
at risk** work runs on the 19-day window, not the stalled flag.

A deal suggestion you pressed **Dismiss** on in the Worklist stays out of later
queues until a new activity is linked to the deal after you dismissed it. Then
it comes back, and says why. Nothing else — a stage move, an expiring offer —
brings a dismissed deal back.

## Where a deal came from

A deal carries an **acquisition source** — the business channel it is
attributed to. It is deliberately not the same field as a lead source: a lead
source records how a record reached Margince, an acquisition source records
which channel earned the business.

The list is an administrator's, at **Settings → Acquisition sources**. A label
you add mints a key from it that never changes afterwards, so renaming the
label later keeps the reporting behind it intact. A source can be retired
rather than deleted — deals already attributed to it still read "(retired)"
rather than going blank. A deal that names none reads "Not set".

## Forecast category

A deal's forecast category is separate from its stage, and a judgement rather
than a fact. You set it when you edit the deal, to **Commit**, **Best case**,
**Pipeline** or **Omitted**.

Two more appear in reports but are never chosen by anyone: **Slipped** and **No
category**. Slipped is the server's own reading of a Commit or Best case whose
close date has passed, gone missing, or is still only provisional. Nobody sets
it; it is what the dates say.

## Reading the numbers

The deals board in Margince loads 100 deals at a time, but the column header
totals are computed over **every** matching deal, not just the loaded page.
Each column header carries the stage name, its win probability, how many deals
are in it, the stage total, and beneath it the weighted total.

The two totals are withheld when a tag filter is applied ("Loaded deals only.
No total while a tag filter is on.") or when the list is not filtered to **My
deals** ("Loaded deals only. Filter to My deals for the total."). A partial
total presented as a whole one is the error that rule exists to prevent.

Every deal report has an **Explain this number** control that shows the rows the
figure was built from. If a number looks wrong, open it rather than guessing.

The deal reports are **Open deals by stage**, **Forecast categories**, **Open
deals per company**, **Won and lost**, and **Time in stage**. They live under
**Analytics**; see [Analytics and forecasting](analytics.md).

## Stage automation: the evidence before trusting a move

Stage automation in Margince means the product proposes stage moves, and a
transition may later move deals by itself. Before it is trusted to, there is a
record of how its proposals actually went, at **Settings → Stage automation**
(in the Sales group).

The report itself is read-only — it is the evidence. Below it sit the rules that
decide what each transition may actually do, and changing one of those needs
permission to edit pipelines. The page says so: "You can see each transition’s record.
Changing a transition requires permission to edit pipelines."

Under **Transition rules**, turning a transition on does not start moving deals.
Margince keeps asking until the record meets the threshold, then moves deals
automatically and tells you afterwards. A transition Margince has suspended shows
**Resume**, which still has to meet the threshold.

Per pipeline, per transition, over a window of days, it reports how many
proposals somebody **Reviewed**, how many are **Still open**, and how many
**Expired** — nobody answered before the window closed, which is not a
rejection: nobody disagreed and nobody looked. Of the reviewed ones it splits
**Accepted as proposed**, **Accepted after edits**, **Rejected**, and **Undone or
corrected** — a move somebody reversed, or whose evidence they marked wrong.
Those four read as percentages of what was reviewed, not as counts.

It also reports **Days observed**, from the first reviewed proposal to the
last, and says why that matters: "A good rate from one afternoon is not a track
record."

### How do I set up stage automation?
To let Margince move deals between stages by itself, open **Settings** → **Stage automation** (in the **Sales** group) and switch a transition on under **Transition rules**.
1. Pick the pipeline and read each transition's record: **Reviewed**, **Accepted as proposed**, **Rejected**, **Undone or corrected**, **Days observed**.
2. Under **Transition rules**, turn on the transition you trust.
Nothing moves at once: Margince keeps proposing moves for review until the record meets the threshold, then moves deals and tells you afterwards. Changing a rule needs permission to edit pipelines. Also called: automatic stage moves, pipeline automation.

## Badges on a deal card

A deal card on the board can carry four badges: **stalled** (nothing for 60
days), **archived**, **single-threaded** (you know one contact at this account)
and **staged** (the AI proposed something on this deal that nobody has
accepted). In practice the board populates **stalled** and **archived**; the
other two are defined but are not drawn on the production board today.

On single-threaded: seats on a deal are not evidence of contact. A deal can carry
five stakeholders and still be single-threaded, because only actual exchanged
messages count as knowing someone.

## The mail line on a deal card

Under the deal's name a card says when mail last moved on the deal and which way
— an envelope for a message they sent, an arrow for one you sent. Rest the
pointer on it to see the last few subjects without opening the deal; **View all
activity** opens the deal's own timeline.

It counts the mail everybody in the workspace can read, the same way the stalled
badge does. A message shared only with its participants does not move the line,
so two colleagues always read the same date off the same card.

## Archiving a deal

Archiving a deal is not closing it. A closed deal is a finished piece of
business; an archived deal has left your lists.

To archive a deal, open it and choose **More actions** → **Archive deal**, or
tick several on the list and use **Archive**. Archived deals leave every list and
report. **Show archived** lets you see one, read-only. There is no way to bring
one back from the app. Closed or archived, a deal has no checkbox for a bulk
action. **Two deals cannot be merged.**

### Can I merge two duplicate deals?
No. Margince has no merge for deals: merging covers contacts and companies only. When one opportunity was entered twice, keep the better deal and archive the other with **More actions** → **Archive deal**. Archiving carries nothing across, so move what you need first: relink its emails to the kept deal with **Relink**, and note anything else on the kept deal. Also called: duplicate deal, combine two opportunities.

## Partners on a deal

A deal in Margince can name the partner that brought it under **via partner**,
and say under **Partner attribution** whether they sourced it or only
influenced it. Commission accrues on a sourced win only. Setting up partners,
crediting one on a deal, and approving or paying their commission are on
[Partners and commission](partners.md).
