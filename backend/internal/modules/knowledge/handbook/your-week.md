<!-- prose:plain -->
<!-- describes: frontend/src/features/brief/brief.weekly.tsx frontend/src/features/brief/brief.weekly.scorecard.tsx frontend/src/features/brief/brief.plan.tsx frontend/src/features/worklist/brief.teamplan.tsx backend/internal/compose/weekly backend/internal/modules/weeklyplan -->
# Your week

The **Weekly** view of **Home** looks back on a closed week and plans the next one. The day,
**Morning** and the Worklist are in [Your day](your-day.md).

## The weekly review

The weekly review in Margince is on **Home → Weekly**, and reads as a report. Margince writes it
once, on the Monday after the week it covers, and marks it **Frozen**. You cannot make it again: the
counts are what they were.

Its title is the week it covers. Above it are the calendar week and when the report was frozen.
Beside the title, the two arrows **Earlier week** and **Later week** step through the weeks that have
a report. Under it, a card holds what Margince wrote about the week, marked as written by AI. The
report's parts are:
- **At a glance**: four figures. Deals won shows what they were worth; the other three show their
  share as a bar. Each shows its change on the week before, where there is one.
- **Closed deals**: the deals won and lost, each with what it closed at, and a bar of won against
  lost.
- **Pipeline movement**: the deals that changed stage, and the stage each reached.
- **Forecast**: one table of the forecast for the week, month and quarter.
- **Figures**: every figure beside the week before, and the scorecard.
- **Observations**: what worked in the week and what did not, where Margince found something.
- **The week ahead**: your plan for this week.

**Closed deals**, **Pipeline movement**, **Observations** and **The week ahead** are left out when
they hold nothing. **Forecast** and **Figures** are always there. The last line says when the next report
comes.

If there is none yet: "No weekly review yet. The first one is created on the Monday after your first
full week."

Above everything sits this rule:

> **Recorded CRM work for this closed week. Missing records do not establish
> inactivity.**

### What it counts
The weekly review counts won, lost, stage changes, and tasks completed and carried over. It counts
plan commitments kept, recorded replies to leads, and meetings with a linked next step. Each shows
next to the week before. Where there is no week before, the review leaves that out, instead of
showing a change from nothing.
- The week is the local week, Monday to Sunday, in the company's time zone. A task done at 00:00 on
  the next Monday counts for the next week.
- Tasks completed include tasks with no date and older late ones. They are counted apart from the
  share of tasks due that week.
- A task carried over was due in or before the week, and was not done when the week closed.
- A reply to a lead counts even when it came after the target. The missed target is still counted on
  its own.
- A meeting has a next step only when a task linked to it was made before the week ended. A task on
  the same contact is not enough.

### What the review will not claim
The weekly review counts recorded work, and says plainly where the record is short.
- It counts the work you hold, even for a manager. Owners and team members are fixed when the review
  is made.
- An older review keeps the rules it was made with. One from before tasks completed were counted
  shows no figure, not a zero.
- With nothing to coach on, it says no priority was found. It says nothing about how hard someone worked.
- A view against an earlier week names that week, which may not be the week just before.

### The scorecard

The **Weekly scorecard** is one table. Each row has a count and what the count rests on.

For leads and meetings it shows leads moved on, how many replies missed the target, and meetings
held (with booked and missed meetings beside them).

For deals it shows stage moves on and back, and the middle value of the days a deal spent in the stage
it left. It shows how many open deals have a next step, and how many have more than one contact. It
shows how many have a close date that is not a guess. And it shows forecast moves up against moves
down.

Two rows appear only when a count is not complete:

- **Meetings without history**: "Before history began · counts are minimums".
- **Deals not rebuilt**: "Affected by an erasure · counts are minimums".

Margince cannot count deals touched by a deletion for the week. It lists them on their own.

### Observations

**Observations** holds at most four observations. Each names the deals it was read from,
and each has a tag: **Positive outcome**, **Unsuccessful outcome**, **Pattern**, or **Experiment**.

Each carries a warning: the observations show links in recorded work, and do not show what made the
outcome happen.

Two empty states mean different things. **"This week has not been analyzed yet."** means the reading
has not run. **"Not enough recorded evidence for useful observations."** means there were fewer than
three records to name, so no model was asked.

An installation with no AI model set up still gets the whole review. It only never gets the
observations or the written summary, and says so.

## Planning a week

**This week’s commitments**, on **Home → Weekly**, is your plan for the week: what you said you will
do, and what you need to do it. A commitment with a **Due date** joins your Worklist and **First**
on that day, in the company's time zone. Done ones, ones due later and ones with no date stay out.

Each commitment has a name, and can have a linked record and a due date. Ticking one marks it but
does not save it. Nothing is written until you press **Save {count} changes**. If some fail, the
ones that failed stay ticked, with a count.

### The four states

**Open**, **Done**, **Dropped** and **Missed**.

You set the first three. You cannot mark a commitment missed: Margince marks open commitments missed
when the week closes.

**Dropped** is your decision, and it counts as not kept and not owed.

### What the week is up against
**Constraints this week** holds two fields for your own text. **Risks** asks what you expect could
go wrong, in your own words. **Available capacity** asks for anything the calendar does not show,
such as time off.

Each has three states: **Not written yet**, **"Nothing to name"**, or the text. A question nobody
answered and an answer of "nothing" are different facts.

If the week already looks full, it says so under **Week already full**. It refuses nothing:
"{committed} items are already booked and {commitments} commitments are planned. Something will not
fit."

### Asking your lead for help

To ask your lead for help, press **Ask for help** on a commitment, answer
**"What do you need from your lead?"** and press **Send**. While it is open and nothing has come
back, the row reads "Help requested · awaiting a response". The answer comes on that same row, with
the name of whoever wrote it.

**Nothing in the product tells your lead.** They find the request when they open your plan, or on
the next Monday, where it is the first thing on their list for you. If you need an answer before
that, ask them yourself as well.

(An installation that has connected its own automation to the events Margince sends can let them
know about the request; nothing built in does that.)

A lead may answer a commitment and nothing else. A lead cannot close it, change its words or drop it.
