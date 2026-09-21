# The pipeline: how a deal moves

Deals live under **Deals** in the navigation, which draws the pipeline board.
("Pipeline" is the selector on that screen, naming which ladder you are looking
at.) This page covers what a stage is, how a deal moves, what closing does, and
what the numbers on screen mean.

## Pipelines and stages

A **pipeline** is a named ladder of stages. You can have more than one, and at
most one of them is the default. Clearing the default on the only pipeline that
has it leaves the installation with none, which the product allows and nothing
warns you about.

A **stage** carries four things:

- a **name** — yours to choose
- a **position** in the ladder
- a **semantic** — one of **Open**, **Won** or **Lost**
- a **win probability** — a whole number from 0 to 100

The semantic is the only fixed vocabulary. The names are entirely yours.

### The pipeline you start with

A new company gets one pipeline, called **Sales**, with six stages:

| Stage | Semantic | Win probability |
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

You edit all this at **Settings → Data model → Pipelines**. Only a human can —
an agent is refused outright, because the stage ladder is the ground truth that
every "should this deal move?" decision is judged against.

Removing a stage tells you what happens: the stages after it move up, past stage
changes stay readable, and deals still sitting on it have to move first.

## Moving a deal

Three ways: drag it on the board, use the stepper on the deal page, or select
several and use **Move to stage**.

The move is written immediately and confirmed — "Moved to Discovery". There is no
save button.

Two things to know:

- **The board's drag does not work on touch.** Use the stepper on a tablet.
- If two colleagues move the same deal at once, the second is refused rather than
  silently overwriting the first.

Bulk move offers open stages only, and skips deals already there. You cannot
close deals in bulk — that is one deal at a time, on purpose.

A deal can only move to a stage in its own pipeline.

## Closing a deal

Closing is a real event, and the app treats it as one. Moving to a won or lost
stage asks first:

> **Move to Lost?** This closes the deal as lost. Confirm first — nothing happens
> until you do.

### Losing

**A lost deal needs a reason.** The Lost reason box must say something before
Confirm lights up. If you cancel — or press Escape, or click outside — anything
you typed is cleared.

### Winning

This is the most opinionated part of the product, and worth quoting exactly:

> **Won asks what is behind the win.** Margince accepts a won deal two ways, and
> both are legitimate — it just refuses to record a win that says nothing at all.

**Either** there is a signed contract on the deal, in which case you press
Confirm and it closes with no further questions.

"Signed contract" is stricter than it sounds. The contract must be unarchived,
past draft, and carry a signed date — *and* it must have a live attachment filed
under Contract or Legal, in the Current or Final state. A contract record with no
paper on it does not clear the bar.

**Or** there is not. You will not see the question until you press Confirm: the
dialog submits, the server refuses a win it cannot evidence, and the dialog comes
back carrying the question:

> **How was it won?** This deal has no signed contract attached, so tell us how it
> was won. The answer is kept on the deal and counted in reports.

Five answers:

- On a purchase order
- Verbally, in person or by phone
- Renewed by email
- Imported from another system
- Something else — which then asks **"What was it?"**, because "something else"
  explains nothing on its own

The point is not paperwork. It is that "how many of our wins have no paper, and
why" becomes a question you can answer. A win with a contract carries no reason
at all, so the two are distinguishable in your reports.

The reason is shown back on the deal afterwards, on its identity line beside the
won badge — and where the answer was "something else", the detail you typed
shows with it.

One honest caveat today: the contract form has no deal field, so attaching a
contract to the deal you are winning takes a step the form does not offer, and
in practice many wins go through the reason instead.

### The outcome review

A closed deal carries an **Outcome review** panel: why the deal went the way it
did, recorded while the reasons are still fresh. It is offered, not demanded —
nothing about closing waits on it, and an unreviewed deal says so plainly: "No
review written yet."

The panel appears on a **closed** deal only. An open one has no outcome to
review, so the card is absent rather than empty.

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
  marked "About an earlier closing", so March's loss is never mistaken for
  June's win.
- **A deal closed before closings were recorded takes no new review.** Its
  existing ones stay readable — there is simply no closing to attach a new one
  to.

This is not the "How was it won?" answer above. That one is asked in the close
dialog, and only where there is no signed contract.

### Currency at close

When a deal closes in a currency other than your base currency, the exchange rate
is **frozen onto the deal** at that moment. That is what stops last quarter's
reported numbers from moving when rates change.

## Reopening

A closed deal's stepper is completely inert — every stage greyed out. The way
back is **Reopen**, in the header's **More actions** menu, which only appears on
a won or lost deal.

It asks which open stage to return to. It also **clears the close date and the
frozen exchange rate** on the way — which the dialog does not tell you, so know
it before you press.

Reopening is treated as seriously as closing, and for the same reason: it takes
revenue back out of a quarter that has already been reported.

If a partner earned commission on the win, reopening does not delete it. A
reversal row is added and the original is marked **Reversed**. A deal that was
won, reopened and won again shows three rows. Nothing is rewritten.

## Stalled deals

**A deal is stalled when it is open and nothing has touched it for 60 days.**

"Touched" means real activity — a mail, a meeting, a note. Not someone opening
the record.

Exactly 60 days is not yet stalled; past 60 is.

You can suppress it. Setting a **wait until** date in the future hides the stalled
flag, and it comes back on its own afterwards. It does not hide an overdue close
date — that is a different problem and stays visible.

There is a second, shorter window: **19 days** without activity makes a deal
*quiet*, which the morning surfaces notice well before it meets the 60-day
stalled bar. "Quiet" and "stalled" are different claims about the same deal, and
the copy beside a deal always names the window it used.

Home's **Deals at risk** lane runs on the 19-day window, not the stalled flag.
The deals board has a **Stalled only** filter.

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

Separate from the stage, and a judgement rather than a fact. Four values:

- **Commit**
- **Best case**
- **Pipeline**
- **Omitted**

Two more appear in reports but are never chosen by a contact: **Slipped** and **No
category yet**. Slipped is the server's own reading of a Commit or Best case
whose close date has passed, gone missing, or is still only provisional. Nobody
sets it; it is what the dates say.

## Reading the numbers

The board loads 100 deals at a time, but the header totals are computed over
**every** matching deal, not just the loaded page.

Each column header carries the stage name, its win probability, how many deals
are in it, the stage total, and beneath it the weighted total.

The two totals are withheld — the header says "Loaded only" — when a tag filter
is applied, or when no owner filter is set. A partial total presented as a whole
one is the error that rule exists to prevent.

**Weighting is rounded per deal and then summed.** That is why the drill-down
always reconciles exactly rather than being off by a rounding error.

Every deal report has an **Explain this number** control that shows the rows the
figure was built from. If a number looks wrong, open it rather than guessing.

The deal reports are **Open pipeline by stage**, **Forecast**, **Open deals per
company**, **Won and lost**, and **Time in stage**. They live under Analytics.

## Stage automation: the evidence before trusting a move

Margince can propose stage moves. Before a transition is trusted to move deals
by itself, there is a record of how its proposals actually went, at
**Settings → Stage automation**.

The report itself is read-only — it is the evidence. Below it sit the rules that
decide what each transition may actually do, and changing one of those needs
permission to edit pipelines. The page says so: "You can see what each
transition has earned. Changing what one may do needs permission to edit
pipelines."

Per pipeline, per transition, over a window of days, it reports how many
proposals somebody **answered**, how many are **still open**, and how many
**ran out** — nobody answered before the window closed, which is not a
rejection: nobody disagreed and nobody looked. Of the answered ones it splits
accepted as proposed, accepted after edits, rejected, and **undone or
corrected** — a move somebody reversed, or whose evidence they marked wrong.
Those four read as percentages of what was answered, not as counts.

It also reports **days observed**, from the first answered proposal to the
last, and says why that matters: "A good rate earned in one afternoon is not a
record."

## Badges on a deal card

- **stalled** — nothing for 60 days
- **archived**
- **single-threaded** — you know one contact at this account
- **staged** — the AI proposed something on this deal that nobody has accepted

In practice the board populates **stalled** and **archived**; the other two are
defined but are not drawn on the production board today.

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

Archiving is not closing. A closed deal is a finished piece of business; an
archived deal has left your lists.

Archived deals leave every list and report. **Show archived** lets you see one,
read-only. There is no way to bring one back from the app.

A deal has a checkbox only while it is open and unarchived. Closed or archived —
either one — and it cannot be selected for a bulk action.

**Two deals cannot be merged.**

## Partners on a deal

If a partner is involved, name them and say how:

- **Brought us this deal** — earns commission
- **Helped on a deal we already had** — no commission

Naming a partner without saying which treats it as "brought us the deal".
Commission accrues on that first case only.

### The commission ledger

A partner's own page carries **Commission** — "What this partner has earned on
deals they brought" — with a row per deal: the deal, what was earned, the rate,
the deal value it was taken from, and a status. Above it sits **Still owed**.

Four statuses, and a row moves through them by somebody deciding:

| Status | What it means |
|---|---|
| **Accrued** | Earned, nobody has approved it |
| **Approved** | Approved for payment |
| **Paid** | Paid |
| **Reversed** | Undone, with the original left standing |

The verbs are Approve, Mark as paid and Reverse. Where a decision is not yours,
the cell says **"Not yours to decide"** rather than offering a button that
refuses.

Nothing earned yet reads "Nothing earned yet".

The partner's margin tier is frozen onto the commission at the moment it accrues,
so changing a partner's tier later does not rewrite what they have already
earned. A partner with no tier earns nothing, and no commission row is written at all —
which is different from writing a zero. "We owe them nothing" and "we owe them
nothing yet" are not the same claim, and a zero row would blur them.
