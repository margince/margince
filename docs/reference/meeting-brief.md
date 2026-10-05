# The meeting brief

The prep dossier for one booked meeting, at
`GET /activities/{id}/meeting-brief`. It answers two questions in one read:
**what is known** about this meeting, in `sections`, and **what to do** in the
room, in `plan`.

## The three invariants

These hold whatever else changes. Each is stated in
`backend/internal/compose/meetingbrief/doc.go` and enforced by the tests beside
it.

1. **No cache, ever.** `generated_at` is always the instant of the read. A
   reader opens this in the minutes before walking into a room, and a brief
   served from a stored artifact would describe a state of play that a
   commitment logged an hour ago has already moved past. There is no cache
   table, no fingerprint and no refresh route.
2. **Every sentence is cited or dropped.** A sentence whose citations do not
   resolve to records the caller can open is dropped *whole* rather than shown
   uncited. The rule lives in `internal/compose/claims` and is shared with the
   account and contact briefs.
3. **A section with nothing to say is absent**, never present-and-empty. A
   reader never scans a heading that turns out to hold nothing.

## What the caller sees is what the caller could open

The brief is assembled under the caller's own scope, from the same gated reads
the contact page serves. It can only describe records that caller could open
themselves; there is no privileged path.

Where a grant keeps something out, the brief **says so** in `omitted` rather
than staying silent. Two sources are named today:

| `source` | What is missing |
|---|---|
| `deal_room` | The buyer's own activity in the Deal Room. A brief that cannot see it reads like a brief about a deal whose buyer has done nothing. |
| `activity_history` | Conversations in this account the caller may not read. The account arc is built from the rest, and a thin arc that does not say it is thin reads as a quiet account. |

## The preparation plan

`plan` is the part a rep acts on. Its fields, and what each is for:

| Field | What it answers |
|---|---|
| `meeting_type` | What kind of room this is, with a confidence. `unknown` is a real answer and becomes the opening question. |
| `objective` | The outcome to earn, plus the one-line reminder not to force it. |
| `opening` | The first thing to say. |
| `top_risk` | The one thing that can change the conversation, with what to say, show and avoid. |
| `likely_asks` | What they are likely to ask us, each grounded in something they said. |
| `questions` | What to ask them, ranked. |
| `scenarios` | What the meeting may turn into, and what to do then. |
| `account_arc` | The few stretches of the relationship that still bear on today. |
| `advance` | Minimum, best and fallback ways to close. |
| `unknowns` | What the record does not say, each with the question that closes it. |

### Readiness

`plan.readiness` is `outline` or `prepared`. `prepared` means the plan carries a
risk with its response, at least two likely asks and at least three questions,
which is enough to lead a surface with. `outline` is the deterministic skeleton.

A client leads with the plan at `prepared` and keeps the cited summary in front
at `outline`, because a half-built plan should not hide the cited summary.

Readiness is computed after grounding: a plan whose risk was dropped for an
unresolvable citation is an outline, however complete it was before.

### Unknowns come from absence

An unknown is a fact about the record ("nobody captured the decision route"),
never a fact about the writer. A model omitting a field does not produce one,
so an empty `unknowns` means the record answered every question.

## The coaching layer

`plan.manager_coaching` is present only for a lead reading a teammate's meeting.
Two questions decide it, in the same order `notices.RaiseCoachNotice` asks
them:

1. **May this seat coach at all?** `auth.RequireCoach`: a human (not an agent,
   not a Deal Room buyer) holding `team_lead.create`, seeded to `admin`,
   `management` and `manager`. A `rep` holds nothing on it, because a rep on a
   team would otherwise coach their teammates.
2. **Is there anybody here to coach?** A live team shared with a colleague
   seated in the meeting, through the same membership seam the Worklist reads.
   Being seated yourself does not disqualify you: a lead in the room coaching
   their rep through it is the ordinary case.

Both refusals return the rep's brief with no error, because the caller asked for
a brief and may have one. A membership check that breaks does fail the read,
because a broken check answering "no coaching" would look the same as a correct
denial.

**Coaching introduces no new read.** A lead gets the brief they would have got
anyway, under their own grants, row scope and baseline, with one more object
attached. `TestCoachingAddsAnObjectAndChangesNothingElse` reads twice as one
user, once with the membership seam wired and once without, and walks the plan
struct reflectively to compare every field but the coaching.

A lead and their rep do not see the same brief. This surface is caller-scoped
throughout: `readLastSpoke` keys "since you last spoke" on the reader's own id,
the history runs through the reader's own activity scope, and a team-scoped lead
reaches rows an own-scoped rep does not. Two readers of one meeting get two
briefs. Coaching adds a reading of the reader's own brief, attached over the
finished plan, so the coaching cannot describe a meeting the plan under it does
not support.

## How the evidence is read

Two passes, because one query cannot both be cheap over a year and carry
message bodies.

1. **Rank** (`history.go`). Up to 200 conversations across 12 months, as
   metadata: dates, subjects, thread keys. Gated the way the contact timeline is:
   the discover clause decides whether a row is visible, the audience arm
   decides whether its content comes back. A row the caller may not read still
   counts: it keeps its date, contributes no subject, and feeds the
   `activity_history` omission.
2. **Excerpt** (`excerpts.go`). The chosen threads' bodies, bounded to six
   threads, four messages each and 1200 characters per message, gated on the
   stronger content clause.

Between them, threads are folded (`threads.go`: a reply chain is one
conversation) and clustered into moments by silences longer than 21 days
(`arc.go`). Moments are ranked by kind of signal, not volume: an agreement
outranks every other signal, and each signal counts once per moment. Five
ordinary threads never outscore the one conversation where a promise was made.

## One brief, two surfaces

The `prep_for_meeting` MCP tool serves this assembly instead of composing its
own, so an agent and the user it acts for read the same brief. The binding is
`internal/compose/meetingbriefseam.go`. It takes the server's service instance
because the model lane is bound to that instance; a second service would serve
agents the deterministic floor while the contact page got model prose.

Every record the plan cites is charged against the agent's read budget, for the
same reason: the arc reaches a year of history the eight sections never
mention, and charging the sections alone would make the richest read the
cheapest.

## Degrading

With no model lane configured the whole brief is a deterministic composition
in the same shape. `generated_by`, on the brief and separately on the plan,
says which wrote it. The two can differ: the plan can fall to its
floor while the sections were written, and the reverse.
