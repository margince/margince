<!-- prose:plain -->
# Your week

The **Weekly** view of **Home** looks back on a closed week and plans the next one. The day,
**Morning** and the Worklist are in [Your day](your-day.md).

## The weekly review

The weekly review in Margince is on **Home → Weekly**. Margince writes it once, on the Monday after
the week it covers, and marks it **Frozen**. You cannot make it again: the counts are what they
were.

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

Behind a button sit more parts: a scorecard, the forecast and how the week moved it, and the
observations.

### What the review will not claim
The weekly review counts recorded work, and says plainly where the record is short.
- It counts the work you hold, even for a manager. Owners and team members are fixed when the review
  is made.
- An older review keeps the rules it was made with. One from before tasks completed were counted
  shows no figure, not a zero.
- With nothing to coach on, it says no priority was found. It says nothing about how hard someone worked.
- A team total is left out when a member's week is missing, or is in another currency.
- A view against an earlier week names that week, which may not be the week just before.

### The scorecard

**Leads and meetings**: leads moved on, recorded replies (with how many missed the target), meetings
held (with booked and missed meetings under them).

**Deals** shows stage moves on and back, and the middle value of the days a deal spent in the stage
it left. It shows how many open deals have a next step, and how many have more than one contact. It
shows how many have a close date that is not a guess. And it shows forecast moves up against moves
down.

Two rows appear only when a count is not complete:

- **Meetings without history**: "Before history began · counts are minimums".
- **Deals not rebuilt**: "Affected by an erasure · counts are minimums".

Margince cannot count deals touched by a deletion for the week. It lists them on their own.

### Observations to review

**Observations to review** holds at most four observations. Each names the deals it was read from,
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
do, and what you need to do it. A commitment with a **Due date** joins your Worklist and **Focus**
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

## The team's week

A team's week in Margince is on **Home → Weekly** with the scope set to
**Team**. A lead looking at a team sees the same week from the other side. It shows reply rates,
meetings with a recorded next step, commitments kept, won and lost, and how many members were
counted.

**Monday agenda** has one line per teammate, the most urgent first. Each line gives the reason it is
there:
- **Help requested**: they asked for help
- **Response targets missed**
- **Plan commitments missed**
- **Deal recovery**: deals at risk
- **Strong week**: worth copying
- **Follow-up evidence missing**: meetings with no next step
- **No priority identified**: a quiet week

**Copy agenda** copies it in one press.

The page carries two notes about missing data:

- Where member data is missing, it says for how many team members, and how many members the numbers
  cover. It is never hidden behind a button.
- Where nobody was measured, it says there is no member data for this week, so nothing is measured.
  The page does not show a row of 0s.

A team's week is for the team's lead, and for a role that looks after every team (Admin and
Management, as they come). Reaching every record is not enough. A read-only seat sees the whole
company and its team's live work on Morning, and is still not offered the team's week. The week's
picker lists the teams you lead, or every team if your role looks after them all. A seat that may
open no team's week sees that this is not theirs. A lead asking about a team they are not on gets
**not found**.
