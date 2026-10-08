<!-- prose:plain -->
# Home and the Worklist: how the day is ranked

Home has one ranking engine behind three views: the Worklist, the Focus cards on Morning, and the weekly
review. The server ranks every row once, by hard levels and then by a fixed order of tie-breaks. Focus is a
cut of that same order, taken before the queue's filter and page limit, so no client can rank work on its
own. Promises read from a conversation reach the queue through one dispatch rule. The weekly job freezes
each member's week and then each team's, and never changes a frozen week. The user-facing rules are in the
handbook pages `your-day.md` and `your-week.md`; this page holds the reasons and the code.

The code:

- `backend/internal/compose/attention/`: the ranking (`rank.go`, `ranksteps.go`, `bands.go`), the classifiers (`classify*.go`) and Focus (`focus.go`).
- `backend/internal/compose/commitmentdispatch.go` and `backend/internal/compose/commitmentsettle.go`: promises and the tasks they become.
- `backend/internal/compose/weeklyjobs.go`, `weeklyteamjobs.go`, `weeklyenrichjobs.go` and `backend/internal/compose/weekly/`: the weekly review.
- `backend/internal/modules/deals/closedate.go` and `closedatesweep.go`: the nightly close date repair; `changeacceptance.go`: **Accept**.
- `backend/internal/modules/notices/store.go` and `backend/internal/modules/automation/handlers_event.go`: stage change notices.
- `frontend/src/app/router.tsx` and `frontend/src/screens/brief.queue.tsx`: old addresses.

## How a row is ranked

A row gets a hard level that says what kind of work it is (`rank.go`). From the top: `levelPinned`,
`levelWaiting` (a customer, a meeting or a legal clock), `levelPromise`, `levelMaterialRisk`, `levelAgreed`,
`levelBlocking` and `levelRoutine`. A level is never a score. With a score, a big enough pile of cheap work
outranks one customer who waits; with levels, sixty duplicate merges never reach the top.

`rankSteps` in `ranksteps.go` is the order of tie-breaks. Both the sort and the "why it ranks here" sentence walk it in this order: `pin`, `band`, `crowded`, `level`, `deadline`, `expected_revenue`, `opportunity`,
`waiting_days`, `relationship`, `occurrence`. One slice serves both, so a step that can decide a pair always
explains it too. Deadline leads the tie-breaks because a date somebody agreed to is the one fact on the page
that runs out.

Bands are headings drawn from the level and the source, never stored (`bands.go`). `bandOrder` is `now`,
`keep_momentum`, `build_pipeline`, `review`. Crowding moves a row down and never up. Past the first group of waiting customers, the rest leave the `now` band but stay ahead of upkeep work. Without that rule a hundred
replies would fill the page with one kind of work.

The classifiers hold the windows. `waitingStaleDays` (14, `classifywaiting.go`) moves an old wait out of the
top band, since two weeks of silence has already cost what it will cost. An old wait on an open deal stays in recovery.

`classifylegal.go` lifts a privacy notice duty 7 days before its deadline. Then `openedOverdue`
ranks a duty recorded more than 24 hours after its deadline as routine, with the reason `opened_overdue`.
That is imported history, not today's breach; a duty that fell due while on record stays urgent.
`recoveryHorizonDays` (14, `classifyrisk.go`) makes deal recovery urgent when the expected close is past or
inside it.

## Focus is a cut of the same ranking

`focusOf` in `focus.go` sorts the whole assembled day, keeps the rows `focusEligible` accepts, and cuts at
`focusLimit` (6). It runs before the queue filter and the page limit, so a filter or a short page can never
hide a card. It is a projection of the one ranking, not a second scoring system.

`focusEligible` keeps rows under `levelRoutine`, with four rules by source. A notice must be urgent work. A
task with a future deadline counts only when it is due today. A meeting counts only when it is unprepared
and starts within 24 hours. A `brief_item` of kind `moved` never counts.

The counts on `WorklistFocus` in `backend/api/crm.yaml`:

| Field | Meaning |
|---|---|
| `focus.total` | Eligible cards before the cut; a group counts once |
| `focus.urgent_remaining` | Urgent rows no card names one by one, in the units of `summary.urgent` |

A group card does not prove each member is urgent, so `urgent_remaining` takes off only the rows a card
names. Source limits, failed reads and held-back reads keep their own meaning on the enclosing worklist.
Neither the last page nor a full Focus proves that every source was read in full.

## What the brief may claim

These rules hold the morning brief to what it read:

- A failed source is named with a retry, never shown as zero. A source the reader may not open is `withheld`, and one that did not answer is `failed`.
- Deal values leave out deals with no price and say how many (`attention/readings.go`). The value is not a forecast.
- A close date the repair set is a reason to confirm the forecast, not evidence of a customer promise. Cards keep the provisional flag and the forecast group.
- Meetings count calendar entries still to come. The brief does not claim a user is prepared unless the source says so.
- A pin changes the reader's order only, never how urgent a row is.
- A task never takes on advice made for its deal.
- A weekly commitment with a due date enters the day on that date, in the installation's time zone (`attention/planlane.go`).

## Close dates and Accept

The nightly sweep in `closedatesweep.go` replaces a missing or past expected close, and keeps any future
date. `proposedCloseDate` in `closedate.go` takes today plus the median days per stage of won deals in the
pipeline, times `StagesToGo`, rounded up to whole weeks. The floor is 7 days. Below `CloseDateMinHistory`
(20 won deals) it uses `CloseDateStageDays` (14).

A stalled deal never has its date pushed forward. Its forecast category drops one step instead, since a fresh date is how a dead deal stays in the forecast. Only
a replaced date is provisional; a date a human set stays theirs.

`setCloseDate` and `setForecastCategory` in `closedatecorrect.go` write only a real change. The sweep sees a quiet deal again every night. Without that guard, the audit log would show a forecast that moves each night while it stands
still.

**Accept** (`acceptAppliedDealChange`, `changeacceptance.go`) records an audited review of a close date
fix or an automatic stage move, and keeps the values the change applied. It is not another write of the
change. It needs the deal version the reader saw, and refuses a change that was undone or replaced. Undo
goes through the guarded stage undo for a stage move, and through the record history for a date.

## Promises read from a conversation

`CommitmentDispatcher` in `commitmentdispatch.go` is the one rule every extractor calls, for meetings and
mail alike:

- A customer's promise is filed as a claim on their contact and watched. It never becomes a task, since nobody here can do it. `GET /deals/{id}/commitments` lists the open ones for the deal's account.
- A named colleague's promise becomes their task. At or above `CommitmentTaskConfidence` (0.85) the task is written at once, captured by the extractor's `agent:` principal. Below it the promise is staged as a `commitment_task` proposal.
- A promise nobody can be named for is proposed to the member the reading belongs to.
- In private mail, `PrivateTo` keeps the promise with its owner, and the task is readable by its holder alone.

The 0.85 sits in the gap the certification records show. A plain dated promise reads at 0.9 or more and a soft one at 0.3 or less, and the extractors drop anything under 0.7.
`TestCommitmentTaskConfidenceSitsInTheCertifiedGap` holds it there.

`commitmentLocator` keys a promise on its source, side, party and quoted words, built by the server and not
from the model's text. A new reading that words the summary another way still finds the old promise. The key
covers archived tasks, refused proposals and dismissed claims, so none of them comes back.

`commitmentsettle.go` keeps a promise and its task in step, as consumer group `cg:commitment-settle`. A task
marked done settles its open claims, and a claim settled as done completes its task. Each side does nothing
when the other is already done, so the pair cannot loop. Opening the task again leaves the claim kept, and
dismissing a claim leaves the task alone: a kept promise that was reopened was still kept. The write runs
as the human behind the event, so it cannot settle a claim or a task that human may not change.

A confirmed email request becomes an undated task through a separate hourly pass, `owedclassify.go`. Its
rules are on [customer-requests.md](customer-requests.md).

## The weekly job

`weekly_review_generate` ticks every 6 hours (`backend/api/jobs.yaml`). The Monday it aims at is the installation's local Monday. A tick that crosses `reviewHour` (6) in every time zone is what makes one
cadence correct everywhere. `reviewWindowOpen` in `weeklyenrichjobs.go` is closed only on Monday before that
hour. A later tick in the same week, up to Sunday, catches up a member the last tick missed. Once the next
Monday comes, the candidate query asks about the new week only.

`measureWorkspace` in `weeklyjobs.go` measures every due member first, each under their own authority.
Model text and mail come after, because a stalled relay must not cost a member the review itself. One
member's failure does not stop the others; the job fails at the end with all of them joined, and retries.
`narrateBudget` (40 seconds) bounds the model call for one member.

`snapshotTeams` in `weeklyteamjobs.go` skips a team while any of its members failed on this run, so a team
total never stands on a missing member. It reads the team as a member who has the rights to open it, trying each in turn. Not every member may read a team's week. A team with no live members gets no
snapshot, because an empty week would claim the team did nothing.

`team_weekly_review` is written once per team and week (`teamreviewstore.go`). The one exception is a
snapshot that counted zero members: its first real measure replaces it, keeps its id, and writes an audit
row. A snapshot that counted anyone never changes.

## Who a notice names

Stage change notices come from `stageChangeNotify` in `handlers_event.go`. A notice keeps the actor and time
of the stage move through live and retry delivery. A dedupe key per recipient and event stops a second
copy. A move the recipient made is dropped before delivery. `notTheReadersOwnStageMove` in `store.go` drops
it from every unread list too, before the page limit, and the deal history keeps it. An unknown author is
never read as the recipient.

Older notices got their facts back in migration
`1789335792_stage_notices_recover_the_change_they_report`, by following the event that created each notice
to the stage change that caused it. It never matched by deal name or nearby times, since neither names one
move. Recorded stage names come first; without them, only a stage setup nobody has edited gives a name.

## Addresses

Home's address is `#/home` in every language. `parseHash` in `router.tsx` sends `#/brief` to Home with its
query kept. `WorklistRedirect` in `brief.queue.tsx` sends `#/worklist` and its owner or `unassigned` form
to Home with the queue open, its `queue_scope` dial set and its filters kept. The queue drawer has its own
`queue_scope`, so changing it never changes the Morning or Weekly view behind it.
