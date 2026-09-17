# Your day and your week

**Home** is where a day starts. It has two views — **Morning** and **Weekly** —
and two scopes, **Mine** and **Team**. The team scope appears only for a seat
whose row scope reaches past its own records.

The **Worklist** is the work queue behind Home. It has no navigation row of its
own: you open it from Home, and it arrives as a panel over it. The old `#/worklist`
address still works and lands you in the same place.

## The Worklist

One ranked list. **The order is the server's**, and the screen never re-sorts
it.

### The three bands

Work is grouped into bands, drawn in this order:

| Band | When it is clear it says |
|---|---|
| **Now** | "No urgent interruptions. Check the remaining work below." |
| **Keep momentum** | "Nothing agreed is drifting." |
| **Build pipeline** | "No new pipeline work waiting." |

Review work is the fourth kind, and it does not draw a band in the queue — it has
its own panel beneath, headed **To review**.

A band you have cleared still shows its heading and says so — but only when the
whole day is loaded. Half a day cannot honestly report an empty band.

Within a band, work due tomorrow, this week or later carries its own heading.
Overdue and today's work sits under the band heading, because that is what the
band already means.

**A crowded row moves down, never up.** When one band fills, its overflow drops
to Keep momentum rather than being promoted past work that was already urgent.

### Why a row is where it is

Each row explains itself three ways, in sentences rather than under headings:

- **The facts that put it there** — a buyer wrote last, a promise is due, a
  meeting is soon, nobody has replied in so many days.
- **Why it outranks the row below**, written out: "Above the next because you
  pinned it."
- **What happens if you leave it**: "If you do nothing, they keep waiting."

Ranking is a fixed ladder of comparisons, not a score: a pin first, then the
band, then whether the row was crowded, then what **kind** of work it is — waiting on
somebody, a promise, a risk to real money, something agreed, something blocking
a colleague, or routine — then its deadline, then expected revenue, and so on
down. **Levels are hard.** Nothing
adds up into a single number you cannot take apart.

### Pinning

A pin moves a row to the top of **Now** and keeps it there. It is yours alone —
pinning reorders what you can already read, and changes nothing for anybody
else.

**A pin never expires.** It survives the row it names, and nothing takes one off
for you — with one exception: fifty is the most you can hold, and pinning a
fifty-first drops your oldest pin to make room.

### Taking something off the list

Three verbs, and they reach different distances:

| Verb | What it does | Who it affects |
|---|---|---|
| **Snooze** | Comes back tomorrow, in 3 days, in 7 — or when they reply | **Your list only** |
| **Not mine** | "Off your list. Whoever owns it still sees it." | **Your list only** |
| **Not a customer** | "Off everyone's list." | **Everybody** |

"Not a customer" is judged on the **thread**, not the message, so the next reply
in it arrives already judged.

Each confirms what it did — "Back on your list tomorrow", "Back on your list
when they reply" — and each offers **Undo**. The undo is scoped the way the verb
was: clearing your own snooze never re-admits a thread a colleague ruled out for
the whole company.

A reply that arrived *before* you snoozed does not lift the snooze.

### "This list has moved"

The list holds still while you work it. When something arrives or is dealt with
underneath you, a notice says which: "{arrived} more since you started. They wait
for a refresh so this list holds still."

It is an event, not a warning — the page is correct, merely incomplete. The
Refresh button appears only when something actually arrived.

### How complete is this?

The footer says one of two things:

- **"{shown} of {considered} shown"** — a plain fraction.
- **"{shown} shown · {sources} sources have more"** — where a source stopped at
  its reading limit.

The second one prints **no fraction**, deliberately. The denominator would be a
floor, and "200 of 200 shown · 1 source has more" contradicts itself.

If a source could not be read at all, a callout says so above the queue: "This
is not the whole day". And the empty state changes with it — not "Nothing is
waiting on you" but **"Nothing is waiting among the sources that answered."**

That distinction is the page in miniature. A quiet day and an unread source look
identical unless the product says which it means.

### Handled for you

**"Handled for you"** lists what was done on your behalf in the last 24 hours:
what happened, about which record, when, and a **way back**.

When there is nothing: "Nothing was done on your behalf today."

The way back restores the record to what it was before the change. It is offered
on deals; other kinds of receipt show the row without one. A receipt already
reversed says **"Already put back"** rather than vanishing, so you can see that
it was.

## For a team lead

More appears for a seat whose scope reaches past its own records, and where it
appears matters.

On **Home → Team** you get the team board and the coaching suggestions. The other
two live in the Worklist panel, which carries its own scope dial — mine,
unassigned, team, all — and **"What the queue is not showing" appears only at
All**.

**What needs me** — team work that has crossed a line: a first reply is late,
revenue at risk, nobody has taken it, or the same thing keeps failing. Each row
names **what it was judged against**, so the threshold is visible rather than
implied. You can take a row on from here.

**Team work needing attention** — a row per teammate: waiting on a reply, deals
at risk, past due, promises due. Unassigned work gets its own row.

**Worth a word this morning** — at most one suggestion per teammate, and the
most urgent kind rather than the biggest number.

**What the queue is not showing** — five reasons work is held back, each with
what it costs you: too old for the queue ("Nobody decided this. They wrote
months ago and were never answered"), attached to no record, from one of our own
domains, judged not sales work, or set aside by you.

When there is nothing held back, it says so and it is good news: "Nothing is
being held back. Every waiting customer reaches somebody's queue."

You can also add a note to a teammate's queue — "A short note that lands in
their Worklist."

## The weekly review

Written once, on the Monday after the week it describes, and **frozen**. There
is no regenerate: the counts are what they were.

If there is none yet: "No weekly review yet — the first one is written on the
Monday after your first full week."

Above everything sits the rule the whole page is built on:

> **Recorded CRM work for this closed week. Missing records do not establish
> inactivity.**

### What it counts

Won, lost, stage changes, tasks completed and carried over, plan commitments
kept, recorded lead responses, and meetings with a linked follow-up. Each shows
against the prior week — and where there is no prior week, the comparison is
**omitted** rather than shown as a change from zero.

Behind a disclosure sit the deeper readings: a scorecard, the forecast outlook
and how the week moved it, and the observations.

### The scorecard

**Leads and meetings** — leads moved forward, recorded responses (with how many
breached the target), meetings held (with booked and no-show beneath).

**Deals** — stage advances and regressions, median days in stage, how many open
deals carry a next step, how many have more than one contact, how many carry a
firm close date, and forecast upgrades against downgrades.

Two rows appear only when they have something to say, and both exist to stop you
reading a number as complete when it is not:

- **Meetings without history** — "These predate the meeting history, so the
  counts above are a floor."
- **Deals we could not rebuild** — "Their week sits behind an erasure, so the
  counts above are a floor."

That second one is the honest edge of the whole feature. To count a population
as it stood on Friday, the product rewinds each deal through its own history. If
any step of that rewind sits behind an erasure, the deal is not counted at all
and is reported separately — rather than being counted wrongly.

### Observations to review

At most four, each citing the deals it was read from, and each tagged:
**Positive outcome**, **Unsuccessful outcome**, **A pattern**, or **Worth
trying**.

The caveat is part of the feature, not a disclaimer bolted on: "These
observations describe associations in recorded work; they do not establish what
caused the outcome."

Two empty states, and they mean different things: **"Nobody has read this week
yet"** and **"Not enough recorded evidence for useful observations."** The second
has a floor behind it — fewer than three citable rows and no model is asked at
all.

An installation with no AI model configured still gets the whole review. It
simply never gets the observations or the written summary, and says so.

## Planning a week

**This week's commitments** — what you said you would do, and what you need to
do it.

Each commitment carries a label, an optional linked record, and an optional due
date. Ticking one stages it; nothing is written until you press Save, and if
some fail, the ones that failed stay ticked with a count.

### The four states

**Open**, **Done**, **Dropped** — and **Missed**.

You set the first three. **You cannot mark yourself missed**: that is written by
the week closing, on anything still open. It is the week's verdict, not
something you declare about yourself.

**Dropped** is your decision, and it counts as neither kept nor owed.

### What the week is up against

Two free-text fields: what could get in the way, and available capacity —
"Anything the calendar does not know — leave, travel, a launch."

Each has **three** states rather than two: not written yet, **"Nothing to
name"**, or the text. An unanswered question and an answer of "nothing" are
different facts.

If the week already looks full, it says so without refusing anything: "That week
is already full. {committed} things are already booked and you have written
{commitments} commitments. Something will have to give."

### Asking your lead for help

Any commitment can carry a request: **"What do you need from your lead?"** While
it stands and nothing has come back, the row reads "Help requested · awaiting a
response." The answer arrives on that same row, with the name of whoever wrote
it.

Know how this actually travels: **nothing in the product notifies your lead.**
The request is found when they open your plan, or on the next Monday, where it
becomes the first thing on their agenda for you. If you need an answer sooner
than that, ask them directly as well.

(An installation that has wired its own automation to Margince's events can be
told about the ask; nothing built in does that.)

A lead may answer a commitment and nothing else — not settle it, reword it, or
drop it.

## The team's week

A lead reviewing a team sees the same week from the other side: response rates,
meetings with a recorded next step, commitments kept, won and lost, and how many
members were counted.

**Monday agenda** — one line per teammate, in priority order, each with the
reason it is there: asked for help, response targets missed, plan commitments
missed, deal recovery, worth copying, follow-up evidence missing, or no priority
identified. It copies to the clipboard in one press.

Two honest notes carried on the page itself:

- Where snapshots are missing: "Snapshots are missing for {count} team members.
  These figures cover {counted} members." It is never hidden behind a
  disclosure.
- Where nobody was measured: "No rep snapshots are available for this week.
  Performance is not measured." — rather than a page of zeroes.

A team's week needs a grant that reaches past your own rows, and the two ways of
lacking one answer differently. A seat that reaches only its own records is told
plainly that this is not theirs. A seat that *could* read a team, asking about
one it does not lead, gets **not found** — exactly what a team that does not
exist returns, so who leads what cannot be mapped by trying.
