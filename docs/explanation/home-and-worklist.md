<!-- prose:plain -->
<!-- describes: backend/internal/compose/dealstatus backend/internal/platform/database/storekit/leadengagement.go backend/internal/compose/leadworktasks.go backend/internal/compose/attention backend/internal/compose/commitmentdispatch.go backend/internal/compose/commitmentsettle.go backend/internal/compose/weekly backend/internal/compose/weeklyjobs.go backend/internal/modules/deals/closedate.go backend/internal/modules/deals/changeacceptance.go frontend/src/features/brief/brief.queue.tsx frontend/src/features/worklist/worklist.next.ts frontend/src/features/worklist/worklist.nexttask.tsx backend/internal/compose/slipping.go backend/internal/compose/attentionriskseam.go backend/internal/modules/deals/deal_read.go -->
# Home and the Worklist: how the day is ranked

Home has one ranking engine behind three views: the Worklist, the **First** section on Morning, and the
weekly review. The server ranks every row once, by one fixed order of ranking steps. Focus is a
cut of that same order, taken before the queue's filter and page limit, so no client can rank work on its
own. Promises read from a conversation reach the queue through one dispatch rule.

The weekly job freezes
each member's week and then each team's. A frozen week never changes, except that a team week with no
measured member is replaced by its first real measure. The user-facing rules are in the
handbook pages `your-day.md` and `your-week.md`; this page holds the reasons and the code.

The code:

- `backend/internal/compose/attention/`: the ranking (`rank.go`, `ranksteps.go`, `bands.go`), the classifiers (`classify*.go`) and Focus (`focus.go`).
- `backend/internal/compose/commitmentdispatch.go` and `backend/internal/compose/commitmentsettle.go`: promises and the tasks they become.
- `backend/internal/compose/weeklyjobs.go`, `weeklyteamjobs.go`, `weeklyenrichjobs.go` and `backend/internal/compose/weekly/`: the weekly review.
- `backend/internal/modules/deals/closedate.go` and `closedatesweep.go`: the nightly close date repair; `changeacceptance.go`: **Accept**.
- `backend/internal/modules/notices/store.go` and `backend/internal/modules/automation/handlers_event.go`: stage change notices.
- `frontend/src/lib/router.tsx` and `frontend/src/features/brief/brief.queue.tsx`: old addresses.

## How a row is ranked

A row gets a hard level that says what kind of work it is (`rank.go`). From the top: `levelPinned`,
`levelWaiting` (a customer, a meeting or a legal clock), `levelPromise`, `levelMaterialRisk`, `levelAgreed`,
`levelBlocking` and `levelRoutine`. A level is never a score. With a score, a big enough pile of cheap work
outranks one customer who waits; with levels, sixty duplicate merges never reach the top.

`rankSteps` in `ranksteps.go` is the order of ranking steps, and `less` stops at the first step that decides
a pair. `pin`, `band` and `crowded` come before `level`, so a crowded row can sort below a row of a lower level. The steps, in order,
are `pin`, `band`, `crowded`, `level`, `deadline`, `expected_revenue`, `opportunity`,
`waiting_days`, `relationship`, `occurrence`. The comparison data follows the same steps.
The screen shows the facts that admit work, without a sentence for each comparison.
Deadline leads the steps after `level` because an agreed date runs out.

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

## Deal suggestions need current facts

`admittedBriefItems` in `attention/briefadmission.go` checks each brief suggestion before counts, filters and pages.
It uses current deal facts and the risk facts already read for this caller.
A stored signal or finding cannot create current work alone. Suggestions without current facts stay in the stored brief.

Home and Worklist use the same typed reasons. Deal values and dates keep their own fields.

Current deal actions use the deal page's rules over one batch of owning reads, before the page is cut.
They share the Worklist's database snapshot. A failed advice read keeps the work and names the gap.

Saved model words are an earlier view, with their written date. They never admit work or change its rank.
A reader must still have access to the source records used to write those words.

The action has no separate stored copy for an event handler to refresh.
Owner writes keep their normal audit and outbox path. Late or repeated jobs cannot make an old card supply a current action.
The client keeps its existing live reads and refreshes after writes.

A task from this action carries its rule and source basis into the normal task writer.
That writer checks the same rules again inside a `Serializable` transaction.
A changed basis returns a version conflict. Only one task can commit from the same basis.

The other prepared actions check that basis before opening. Tasks you create keep their existing write path.

Stored findings follow the current audience of their cited activities on both run reads and item read-backs.

## Ownership comes before scan limits

`backend/internal/compose/attention/recordowners.go` resolves the queue's owners before the deal risk read.
The deal store applies these owners and the caller's visibility rules before limiting either the quiet or overdue scan.
An empty set reads no owners. No set means the visible workspace view.
Team reads keep work from former team members and work with no owner.

Company and lead duplicate pairs and deal and tag suggestions have no requested decider.
**Mine** and chosen member views skip those sources before building the decision page and counts.
**Unassigned** and wider views keep them. Approvals keep their requested-decider query.

Contact pairs reach the queue as contact cases (source `contact_case`, `attention/contactcaselane.go`), never one pair at a time.
A case is a group of contacts joined by open pairs, and its row id is the lowest member id.
The case read binds the page's owner before it ranks.
So **Mine** and a chosen member view show every case with a member that owner owns.
The row names that owner as its requested decider, so each owner of a member sees the case on their own page.

**Unassigned** shows the cases where no member has a live owner.
A wider view names the reader if they own a member, then an owner in the team the page shows, then the first owner.
**Done** hides a case for that owner alone, and the other owners keep it.
Cases count into the open duplicates total, and they come first in each round of the decision lane.

The final scope cut still checks the owner. It cannot add candidates a source has already left out.
Home, Focus, source reach and summary counts use the same scoped day.

## Totals and the rows below the ranked top

Only the first rows are ranked. Ranking needs every candidate in hand, so each lane reads to its cap.
The ranked rows are those candidates, frozen as a walk (`walk.go`).

A category states an exact `total` only when every source behind it was read whole or counted by its store
(`withTotals` in `attention/totals.go`). The count runs the source's own admission rules in SQL, so it
matches what the list holds. `attention/backlog.go` holds the counts and runs them only for a source its lane cut.
A count that fails leaves the category a floor and does not fail the page.

Once the ranked rows run out, the next pages load a tail (`attention/backlogtail.go`). Each cut source goes on in
its store's own order from its own keyset, skipping the rows the ranked top already holds. The tail rows go through
the same admission, scope, Done, view and filter steps as the top. The cursor carries where each source stopped, and
binds the same scope, owner, view and filter as before. The list ends when every source in the tail is read to its
end. A source that fails while the tail loads is named in `sources_unavailable`, and the list stops short of it.

The first page decides which sources the tail reads: those its lanes cut. After that, a source leaves the tail only
when its own read finds nothing more, so completing tasks above never ends it. The cursor also keeps the first page's
time, day end and time zone. Every later page reads that same window, even once the day has changed. Its fingerprint
names the tail's version, so a binary without the tail refuses the cursor instead of ending the walk early.

Today only the task source is counted and read on in the tail (`WithTaskBacklog`, `compose/attentiontaskseam.go`).
Its today window and its later window each have a count, taken on the first page only. A team page counts no task
window, because that count ran past the lane budget at 10,000 tasks a seat. Its task categories stay a floor.

Every other cut source leaves its category a floor, and the list ends at the ranked rows for it.
The reasons each one is not counted yet are in [the source census](../reference/worklist-source-census.md).

## Personal work needs its own owner

The mail request writer and waiting read share the direct-recipient rule in
`activities/mailresponsibility.go`. Owning a contact or being able to read shared mail does not assign its reply.
A personal or chosen team member view applies that rule before the source limit, counts and Focus.
A chosen member view uses that member’s choices, while access checks still use the viewer.
Team keeps its shared view.

The capture writer keeps the mailbox owner's parsed To and Cc roles without creating another contact.
A delivery record alone is not a To role.

Record task lists, project counts and deal next-step checks share
`activities.CurrentTaskClause`. Old automatic work needs current source evidence or a real human change.
Import audits and record filing do not count as human changes.
Raw history keeps the original records and their status.

An automatic mail task also needs a current request addressed to the user who holds it.
Editing the displayed body cannot change that evidence.
The request writer and current task readers share the recipient rule and mail horizon.
Reads hide old automatic work even before a background job runs.
A later email to a contact does not show that a task or promise was done.

Advice to review a lead uses the source event's time and the same horizon.
An answered inbound stops being a new reply; that does not settle a separate request.
A booked meeting keeps its state, including a future meeting or one called off.
Filing old mail keeps the source without making the advice new.

The writer and open-task readers share this admission, so stale tasks cannot block a new reply.
A task to review a lead or report a new arrival has no generated deadline.
The date of an old task that no human changed is not an overdue date.
Human changes keep their chosen deadlines.

## Focus is a cut of the same ranking

`focusOf` in `focus.go` sorts the whole assembled day, keeps the rows `focusEligible` accepts, and cuts at
`focusLimit` (6). It runs before the queue filter and the page limit, so a filter or a short page can never
hide a card. It is a projection of the one ranking, not a second scoring system.

`focusEligible` keeps rows under `levelRoutine`, with four rules by source. A notice must be urgent work. A
task with a future deadline counts only when it is due today. A meeting counts only when it is unprepared
and starts within 24 hours. An admitted deal suggestion counts when current facts call for action.

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

The nightly sweep in `closedatesweep.go` replaces a missing or past expected close only when
observed history supports an estimate, and keeps any future date. `proposedCloseDate` in `closedate.go` takes today plus the median days per stage of won deals in the
pipeline, times `StagesToGo`, rounded up to whole weeks. The floor is 7 days. Below `CloseDateMinHistory`
(20 won deals) it keeps the date as it is. You still need to confirm estimates. They record the observed
won-deal count, median days per stage and stages left.

A stalled deal keeps its future date while its forecast category drops one step. A missing or past
date still needs enough observed history before it can be replaced. Only a replaced date is
provisional; a future date a human set stays theirs.

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
- If no colleague can take the promise as a task, it stays an open claim on the contact. No task or proposal is made. This happens when nobody is named, when the named seat is an agent or has left, or when they may not read the private mail.
- In private mail, `PrivateTo` keeps the promise with its owner, and the task is readable by its holder alone.

`commitmentmeaning.go` checks what the words ask someone to do before the dispatcher creates work.
Completed work, an offer, a request without agreement and work that depends on something that has not happened yet create no task or proposal.
The reading keeps that reason with the source. Missing or invalid meaning stops the reading so it can be retried.
A deadline such as "next week" keeps its quoted range and sets no due day. A day needs its own source words.

Before writing new work, `CommitmentAdmissionEvidenceTx` checks the source and any later mail. The dispatcher uses the source date saved by capture, or reads the original mail date for older rows.
Its rule version is `CommitmentAdmissionVersion`, in `backend/internal/modules/activities/commitmentadmission.go`.
The commitment window uses `automaticCommitmentHorizonDays` (30). A promise or agreed deadline within that window stays current, as does a deadline still ahead.

Unknown times and unread later context stop new work. This does not mark the promise done.
Human changes keep tasks current. A machine claim without a current admission stays with its source for review. It does not become personal work because the reader owns the contact. Assigned promises use the task lane; claims without tasks name no personal owner.

Canceled meetings create no new work after their scheduled start. A future booked meeting waits until it has taken place.
Promises that need review do not create fresh open account signals.

A history read adds the newest bounded thread window, without moving the history cursor past unread older messages.
`extractContextMessages` bounds both windows together. A gap or unread new text stops admission. A cut counts as read only when every line left out is quoted history; an inline answer below the cut stays unread.

`activity_commitment_admission` keeps the reason, source quote, rule version and read time under the source's access rules.
The activity audit stores decision metadata without copying source text. Later read times stop stale results from replacing newer ones.
Contact and company timelines show commitment review counts on sources and their threads.

Open **Commitment review** in the source drawer to see why no work was created. Readers can check later replies and add a follow-up from a linked record.
This is the bounded review route for threads with more later context than the read window holds; the scan still moves forward.
A transcript reading also reports why a commitment needs more evidence.

Partial delivery leaves the original promise open. A repeated promise keeps the original source and quoted clause, so it cannot create a second task for the same work.
A stated renewal records its message and source time. It can update the deadline on an open automatic task and its claim; human changes and completed or archived tasks keep their state.
The dispatcher locks the original source before applying a renewal. An older renewing message cannot replace a newer one, even when a later pass reads it.

Completion follows the stored source and the quoted clause's key. Pending task proposals end only after every recipient claim for that promise is done or dismissed.
A completed commitment task can end its proposal when no contact claim exists. A blank key proves no link.

One promise to several known recipients creates one task with a claim for each recipient. Home and Worklist show each visible recipient and their status. Different asks keep separate tasks.
A later reading that leaves out a recipient does not remove an existing claim. A human can dismiss a claim that is no longer owed.

A later recipient joins automatic work directly when no human has changed it. A task a human accepted or changed needs approval to attach that recipient.
That approval names the existing task and changes no task fields.

Automatic completion counts all claims for the promise, including recipients whose attachment needs approval. Completing one recipient's claim cannot complete work still owed to another recipient.

Completion events use the original member's current access. A task completion read later checks the task again and leaves reopened work alone.
The notice worker removes notices for proposals that ended; the source and audit stay available under their access rules.

The day folds an approval's notice into its decision before counting or paging. The approval engine checks ownership and access by identifier, even beyond the loaded decision page.
Titles and target records prove no link. A notice keeps its place when no pending decision is in scope.
`notices_more_available` keeps the original scan bound after folding; Worklist derives its notice coverage from that same field.

Signal and transcript readers use the `premium` tier by default. Their certification cases check meaning, ownership and recipient attribution.
Each reading uses tokens at that tier. Work waits in the queue when the budget runs out.

The task threshold applies only after the meaning of the promise admits work. A soft offer cannot become a task through confidence alone.
`TestAutomaticTaskConfidenceFitsCertifiedCommitments` checks the threshold against current certification records. The corpus also checks that soft offers create no work.

`commitmentLocator` keys a promise on its source, side, party and quoted words, built by the server and not
from the model's text. A new reading that words the summary another way still finds the old promise. The key
covers archived tasks, refused proposals and dismissed claims, so none of them comes back.

`commitmentsettle.go` keeps a promise and its task in step, as consumer group `cg:commitment-settle`. A task
marked done settles its readable open claims. A claim settled as done completes its task only when no recipient claim stays open. Each side does nothing
when the other is already done, so the pair cannot loop. Opening the task again leaves the claim kept, and
dismissing a claim leaves the task alone: a kept promise that was reopened was still kept. The write runs
as the human behind the event, so it cannot settle a claim or a task that human may not change.

A confirmed email request becomes a task through a separate hourly pass, `owedclassify.go`. The task is
undated unless the request states a deadline after a word such as "Deadline" or "by". Its
rules are on [customer-requests.md](customer-requests.md).

## The weekly job

`weekly_review_generate` ticks hourly (`backend/api/jobs.yaml`) so it can reach personal delivery
windows. Reviews keep the installation's last closed reporting week. Later working days allow
catch-up within that week; once the next reporting week opens, the candidate query asks about the
newly closed week. Existing review and mail claims keep repeated ticks from sending a second copy.

`measureWorkspace` in `weeklyjobs.go` measures every due member first, each under their own authority.
Model text and mail come after, because a stalled relay must not cost a member the review itself. One
member's failure does not stop the others; the job fails at the end with all of them joined, and retries.
`narrateBudget` (40 seconds) bounds the model call for one member.
The sentence is asked for in the member's saved language, else the installation's;
`recipientLanguage` in `maillanguage.go` answers that for the weekly mail too.

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

## What a row offers

The verbs on a row that change a record come from its `actions`, which the server sets. So the screen never
offers a write the server would refuse. A task row (`taskActions` in `taskactions.go`) offers `complete` and `snooze`, and `take`
when nobody holds the task. `take` shows on every scope, because nobody holding a task is a fact about the
task and not about the view.

A meeting owed an outcome (`meetingOutcomeActions` in `meeting.go`) offers `decide`, and `cancel` only when
no live calendar invite holds it. `PATCH /activities/{id}` refuses that cancel with `calendar_managed`,
because the invite owns the meeting's time and words. Record pages ask the same question through
`Activity.awaiting_outcome`, which uses the lane's rule without its look-back window.

A reader who may see a task or meeting and not change it gets no verbs. The reason `read_only` says why,
so an empty row does not look broken.

## Addresses

Home's address is `#/home` in every language. `parseHash` in `router.tsx` sends `#/brief` to Home with its
query kept. `WorklistRedirect` in `brief.queue.tsx` sends `#/worklist` and its owner or `unassigned` form
to Home with the queue open, its `queue_scope` dial set and its filters kept. The queue drawer has its own
`queue_scope`, so changing it never changes the Morning or Weekly view behind it.

**Next task** on a record page's next-step bar reads the Worklist again on each press, so an open page polls
nothing (`worklist.nexttask.tsx`). `nextTaskHref` in `worklist.next.ts` takes each record once, where its
first row stands, and leaves out rows that open no record page. It follows the list the record was opened
from, else the reader's own list, and goes back to the top after the last record.

Quiet relationships use the same scoped read and row renderer in a separate optional
list. They join the work queue and Focus only when the reader pins them. A pin
keeps the routine meaning and names the reader's choice; it adds no deadline or
deal dependency. Existing tasks and waiting replies remove duplicate suggestions.
