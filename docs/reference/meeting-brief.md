<!-- prose:plain -->
# The meeting brief

The brief is the file a user reads to get ready for one booked meeting, at
`GET /activities/{id}/meeting-brief`. It answers two questions in one read:
**what we know** about this meeting, in `sections`, and **what to do** in the
room, in `plan`.

## The three rules that always hold

These hold when all else changes. Each is stated in
`backend/internal/compose/meetingbrief/doc.go` and checked by the tests next to
it.

1. **No cache, ever.** `generated_at` is always the time of the read. A reader
   opens this in the minutes before a meeting. A stored brief could miss a promise
   logged an hour before, which already changed the deal. There is no cache table, no stored hash and no route to build it again.
2. **Every sentence is cited or dropped.** A sentence whose sources do not point
   to records the caller can open is dropped *whole*. It is never shown without
   a source. The rule lives in `internal/compose/claims` and is shared with the
   company and contact briefs.
3. **An empty section is not there**: a section with nothing to say is not shown. A
   reader never scans a header that turns out to hold nothing.

## What the caller sees is what the caller could open

The brief is built under the caller's own scope, from the same gated reads the
contact page serves. It can only describe records that caller could open; there
is no path with more rights.

Where a grant keeps something out, the brief **says so** in `omitted` and does
not stay quiet. It names two sources today:

| `source` | What is missing |
|---|---|
| `deal_room` | What the buyer has been doing in the Deal Room. Without it, a brief reads as if the buyer of the deal has not acted at all. |
| `activity_history` | Conversations with this company that the caller may not read. The company arc is built from the rest. A short arc that does not say it is short reads as a company with not much activity. |

## The plan

`plan` is the part a rep acts on. Its fields, and what each is for:

| Field | What it answers |
|---|---|
| `meeting_type` | What kind of meeting this is, with how sure the model is. `unknown` is a real answer and becomes the first question. |
| `objective` | The result to work for, plus a one-line note not to push for it too hard. |
| `opening` | The first thing to say. |
| `top_risk` | The one thing that can change the conversation, with what to say, what to show and what not to do. |
| `likely_asks` | What they are likely to ask us, each bound to something they said. |
| `questions` | What to ask them, best first. |
| `scenarios` | What the meeting may turn into, and what to do then. |
| `account_arc` | The few parts of the relationship that still count today. |
| `advance` | The least, the best and the back-up ways to close. |
| `unknowns` | What the record does not say, each with the question that would answer it. |

### `readiness`

`plan.readiness` is `outline` or `prepared`. `prepared` means the plan carries a
risk with its response, at least two likely asks and at least three questions.
That is enough to lead a screen with. `outline` is the base shape that the code
builds with no model.

A client leads with the plan at `prepared`. At `outline` it keeps the cited
summary in front, because a plan that is not complete should not hide the
cited summary.

The code works out `readiness` after it checks the sources. A plan whose risk
was dropped for a source that does not point to a record is an `outline`, even if it was
complete before.

### An unknown comes from what the record does not hold

An unknown is a fact about the record ("no one captured how the decision is
made"), never a fact about the writer. A model that leaves out a field does not
make one. So an empty `unknowns` means the record answered every question.

## The coaching part

`plan.manager_coaching` is there only for a lead who reads the meeting of a
member of their team. Two questions decide it, in the same order
`notices.RaiseCoachNotice` asks them:

1. **May this seat coach at all?** `auth.RequireCoach`: a human (not an agent,
   not a Deal Room buyer) who holds `team_lead.create`. The seed gives it to
   `admin`, `management` and `manager`. A `rep` holds nothing on it; else a rep
   on a team would coach the rest of the team.
2. **Is there someone here to coach?** A live team shared with a colleague
   in the meeting. This goes through the team member seam the Worklist reads.
   A lead who is in the meeting is still allowed to coach. A lead in the meeting
   who coaches their rep through it is the normal case.

A caller who passes the first question but carries no user id fails the read.
It does not get a brief with no coaching.

When either answer is no, the call returns the rep's brief with no error,
because the caller asked for a brief and may have one. When the team member
check breaks, the read fails. A check that failed and answered "no coaching"
would look the same as a correct no.

**Coaching adds no new read.** A lead gets the brief they would get with no
coaching, under their own grants, row scope and baseline, with one more
object attached. `TestCoachingAddsAnObjectAndChangesNothingElse` reads twice as one
user, once with the team member seam wired and once without. It then goes over the
plan `struct` field by field, through Go `reflect`, and compares every field but
the coaching.

A lead and their rep do not see the same brief. Every part of this surface is
scoped to the caller. `readLastSpoke` keys the `since you last spoke` line on
the id of the reader. The history runs through the activity scope of the reader.
A lead scoped to the team reaches rows that a rep scoped to their own records
does not.

Two readers of one meeting get two briefs. Coaching adds a reading of the own
brief of the reader, attached over the complete plan. So the coaching cannot
describe a meeting that the plan under it does not support.

## How the sources are read

The code reads in two passes, because one query cannot both be fast over a
year and carry message bodies.

1. **Rank** (`history.go`). Up to 200 conversations over 12 months, as
   data about the messages: dates, subjects, thread keys.

   It is gated the way the contact history is. The find clause decides whether
   a row is visible. The clause on who a message goes to decides whether its
   content comes back. A row the caller
   may not read still counts. It keeps its date, adds no subject, and counts
   in what `activity_history` reports as missing.
2. **Pull text** (`excerpts.go`). The bodies of the top threads, with a limit
   of six threads, four messages each and 1200 signs per message.
   Gated on the stricter content clause.

Between the two passes, threads are merged (`threads.go`: a line of replies is one
conversation). They are then grouped into moments, split by silences longer
than 21 days (`arc.go`). Moments are ranked by the kind of signal, not by how
many messages they hold. An agreement ranks above every other signal, and each
signal counts once per moment. Five normal threads never score above the one
conversation where someone made a promise.

## One brief, two surfaces

The `prep_for_meeting` MCP tool serves this brief and does not build its own.
So an agent and the user it acts for read the same brief. The binding is
`internal/compose/meetingbriefseam.go`. It takes the service the server runs,
because the model lane is bound to that one. A second service would serve
agents the base shape with no model, while the contact page has model text.

Every record the plan cites counts against the agent's read budget, for the
same reason. The arc reaches a year of history that the sections never
name. If only the sections counted, the read with the most data would cost the
least.

## With no model

With no model lane set up, the whole brief is built by code with no model, in
the same shape. `generated_by`, on the brief and again on the plan, says which
wrote it. The two can be different: the plan can drop back to the base shape while
a model wrote the sections, and the other way too.
