# The pipeline: how a deal moves

The **Pipeline** is where deals live. This page covers what a stage is, how a
deal moves, what closing does, and what the numbers on screen mean.

## Pipelines and stages

A **pipeline** is a named ladder of stages. You can have more than one, and
exactly one of them is the default.

A **stage** carries four things:

- a **name** — yours to choose
- a **position** in the ladder
- a **semantic** — one of **Open**, **Won** or **Lost**
- a **win probability** — a whole number from 0 to 100

The semantic is the only fixed vocabulary. The names are entirely yours.

### The pipeline you start with

A new organization gets one pipeline, called **Sales**, with six stages:

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

You edit all this at **Settings → Data model → Pipelines**. Only a person can —
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
- If two people move the same deal at once, the second is refused rather than
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

**Or** there is not, and the dialog asks:

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

Two honest caveats about this today: the contract form has no deal field and
contracts are created as drafts with no control to move them out of draft, so in
practice most wins currently go through the reason. And the reason is not shown
back on the deal afterwards.

### Currency at close

When a deal closes in a currency other than your base currency, the exchange rate
is **frozen onto the deal** at that moment. That is what stops last quarter's
reported numbers from moving when rates change.

## Reopening

A closed deal's stepper is completely inert — every stage greyed out. The way
back is the **Reopen** button in the header, which only appears on a won or lost
deal.

It asks which open stage to return to, and **clears the close date and the frozen
exchange rate** on the way.

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

There is a second, shorter window used by the morning surfaces to notice a deal
going quiet at about three weeks, before it meets the stalled bar. "Quiet" and
"stalled" are different claims about the same deal, and the app names which one
it means.

Stalled deals appear on Home, and Pipeline has a **Stalled only** filter.

## Forecast category

Separate from the stage, and a judgement rather than a fact. Four values:

- **Commit**
- **Best case**
- **Pipeline**
- **Omitted**

Two more appear in reports but are never chosen by a person: **Slipped** and **No
category yet**. Slipped is the server's own reading of a Commit or Best case
whose close date has passed or gone missing. Nobody sets it; it is what the dates
say.

## Reading the numbers

The board loads 100 deals at a time, but the header totals are computed over
**every** matching deal, not just the loaded page.

Each column header carries the stage name, its win probability, the stage total,
and beneath it the weighted total.

**Weighting is rounded per deal and then summed.** That is why the drill-down
always reconciles exactly rather than being off by a rounding error.

Every deal report has an **Explain this number** control that shows the rows the
figure was built from. If a number looks wrong, open it rather than guessing.

The reports available on deals are: Deals by stage, Forecast, and Open deals
per company.

## Badges on a deal card

- **stalled** — nothing for 60 days
- **single-threaded** — you know one person at this account
- **staged** — something is waiting in the approval inbox for this deal
- **archived**

On single-threaded: seats on a deal are not evidence of contact. A deal can carry
five stakeholders and still be single-threaded, because only actual exchanged
messages count as knowing someone.

## Archiving a deal

Archiving is not closing. A closed deal is a finished piece of business; an
archived deal has left your lists.

Archived deals leave every list and report. **Show archived** lets you see one,
read-only. There is no way to bring one back from the app.

Closed and archived deals have no checkbox — they cannot be selected for bulk
actions.

**Two deals cannot be merged.**

## Partners on a deal

If a partner is involved, name them and say how:

- **Brought us this deal** — earns commission
- **Helped on a deal we already had** — no commission

Naming a partner without saying which treats it as "brought us the deal".
Commission accrues on that first case only.

The partner's margin tier is frozen onto the commission at the moment it accrues,
so changing a partner's tier later does not rewrite what they have already
earned. A partner with no tier earns nothing, and that is recorded as a skip
rather than a zero — the two are different facts.
