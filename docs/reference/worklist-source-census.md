<!-- describes: backend/internal/compose/attention backend/internal/compose/worklistsnap backend/internal/modules/activities/waiting.go backend/internal/modules/activities/awaitingreply.go -->
# Worklist source census

Every source `GET /v1/worklist` admits, with the store that reads it, the bound that cuts it, how it narrows to
a scope and what runs in Go after its SQL. `Service.Worklist` in `backend/internal/compose/attention/worklist.go`
assembles the day (`assembleDay` in `feed.go`), reads the side sources, then projects one ranked queue in
`worklistFrom`. The two bounds that `attention` holds as copies of a seam's own constant are checked in both
directions by `backend/gates/worklistbounds_test.go`; read their values there.

## Steps after SQL

`worklistFrom` runs these steps in this order over every candidate row. The source table below names the steps
that can remove or merge a source's rows.

| Step | Function | What it does |
|---|---|---|
| A | `admitBySource` (`admission.go`) | Drops a row whose owner kind its source does not allow |
| B | `dropDealsAlreadyWaiting` (`classify.go`) | Drops a `deal_at_risk` row when a `customer_waiting` row names the same deal |
| C | `narrowToScope` (`scope.go`) | Owner admission: keeps the reader's, a named owner's, the unowned or the roster's rows. `team` filters by roster here, after the lane caps |
| D | `relationshipChoices` (`relationshipchoices.go`) | Moves `relationship_decay` rows out of the queue unless pinned; drops one whose contact is already on another row |
| E | `foldBriefIntoRisk` (`briefdedupe.go`) | Merges a `brief_item` onto the row about the same deal |
| F | `dropDone` (`done.go`) | Drops rows the reader marked done that have not come back |
| G | `foldAndRepin` and `foldRoutineDecisionsBounded` (`batches.go`, `pins.go`) | Folds routine decisions and system incidents into `batch` rows at `batchFloor` = 3 members; skipped for `filter=decisions` and `filter=system` |
| H | `keepFiltered` (`page.go`) | Applies the `filter` value |
| I | `markCrowding` (`crowding.go`) | Marks rows past the lead of one source; removes none |

The page counts two populations. `considered` is the rows left after step F, before folding, filtering and
crowding; `summary`, `readings` and the considered half of `counts` and `reach` read it. `shown` is what is left
after steps G and H and the page cut. No source count query feeds them, with two exceptions. `needs_you` carries
the stores' own totals for approvals, duplicate pairs and suggestions (`decisionsToDepth` in `feed.go`). A
category's `total` uses the exact approvals count (`exactSources` in `totals.go`).

## Sources: store, bound and scope

Paths are under `backend/internal/`. "SQL" means the bound is a `LIMIT` in the store's query; "Go" means the
seam or lane cuts after reading.

| Source | Owning store | Bound | Scope narrowing |
|---|---|---|---|
| `task` | `modules/activities` `Store.ListActivities`, through `compose/attentiontaskseam.go` | Due by end of today: `plannedCap` = 12 (SQL). Due in the next `upcomingHorizonDays` = 14: `upcomingCap` = 6 (SQL). Due reminders: every page of `reminderPage` = 200. Overdue tasks age out after `worklistAgeOutDays` = 30 (`modules/activities/worklistageout.go`), unless a human reopened the task, the reader pinned it, or its reminder is due | `mine` and a named owner: `assignee_id` in SQL. `unassigned`: `assignee_id IS NULL`. `team` and `all`: row scope only |
| `meeting` | `activities` `ListActivities` through `attentionMeetings.Today` (`compose/attentionmeetingseam.go`) Two reads, timed and all-day, each of `plannedCap` + 1 = 13 rows (SQL) and cut to 12 in Go, so at most 24, from now to end of today. Meetings still to confirm are kept past the cut | `mine`: meeting responsibility. Named owner: host. `unassigned`: no host. `team` and `all`: row scope only |
| `meeting_outcome` | `activities` `ListActivities` through `attentionMeetingsAwaitingOutcome` | `plannedCap` (SQL), oldest first, back `unansweredLookbackDays` = 14 | As `meeting` |
| `brief_item` | `compose/briefs` latest run, through `compose/attentionbriefingseam.go` | `briefQueueTarget` = 7 when the run is built (Go) | The reader's own run only; dropped on a named owner's page |
| `deal_at_risk` | `modules/deals` `Store.ListDeals`, two sweeps in `compose/slipping.go` | `slippingScanLimit` = 50 per sweep (SQL) | `storekit.OwnerSetClause` in SQL: reader, named owner, no owner, or roster plus former teammates |
| `customer_waiting` | `activities` `WaitingReplies*Before` (`modules/activities/waiting.go`) | `WaitingScanCap` = 200 per page (SQL), up to `waitingRefillRounds` = 3 pages (`compose/attentionwaitingseam.go`) | Mail responsibility in SQL: the reader's or named owner's seat, the roster's seats, or any seat |
| `awaiting_reply` | `activities` `Store.AwaitingReplies` (`modules/activities/awaitingreply.go`) | `AwaitingReplyScanCap` = 200 (SQL), sent at least the follow-up setting's days ago and within `AwaitingReplyLookbackDays` = 14 after that | Sent by the reader; not read for a named owner or `unassigned` |
| `meeting_follow_up` | `activities` `MeetingsAwaitingFollowUp` | The same cap and window, from the meeting's end | Held meetings the reader answers for; not read for a named owner or `unassigned` |
| `weekly_commitment` | `modules/weeklyplan` store, through `compose/attentionplanseam.go` | None; Go keeps open commitments due by end of today | Reader's plan; named owner's plan; each roster member's plan for `team`; withheld for `all` |
| `approval` | `modules/approvals` `Service.ListWire` and `CountPending` | `batchScanDepth` = 200 (Go, over 200-row SQL batches) | Requested-for arms judged in Go per batch, then the decidable check |
| `dedupe_candidate` | `modules/contacts` `ListDedupeCandidates` | `batchScanDepth` asked, clamped to `dedupeQueueMaxLimit` = 100 (SQL) | Both sides visible; no owner predicate; not read for `mine` or a named owner |
| `contact_case` | `contacts` `ListContactCases` | `contactCaseListMax` = 50 per call; the seam pages to `batchScanDepth` | Case scope mine, unassigned or all; filtered again in Go |
| `deal_suggestion` | `deals` `ListSuggestions` | `suggestionMaxLimit` = 200 (SQL) | Company row scope; not read for `mine` or a named owner |
| `tag_suggestion` | `modules/collections` `OpenTagSuggestions` | `tagSuggestionMaxLimit` = 200 (SQL) | Row scope and `tag` read; not read for `mine` or a named owner |
| `relationship_decay` | `modules/search` quiet contacts, through `compose/attentionlanesseam.go` | `decayCandidateCap` = 40 times `decayCandidatePages` = 3 candidates (SQL), then `decayLaneCap` = 5 (Go) | Contact `owner_id` for `mine` and a named owner; no live owner for `unassigned`; none for `team` and `all` |
| `domain_question` | `contacts` `OpenDomainQuestionsForOwner` | `DomainQuestionLimit` = 100 (SQL) | The reader decides the question |
| `introduction_request` | `modules/introductions` `AwaitingMyAnswer` | `doneCap` = 8 (SQL) | The reader is the introducer |
| `failed_approval`, `dsr`, `notice_case`, `capture_health`, `ai_work_health`, `automation_run`, `bounce`, `undelivered`, `notice` | See [System sources](#system-sources) | `doneCap` = 8 (SQL), except `capture_health`, which has none | See [System sources](#system-sources) |

The decision lane cuts its producers together: `interleave` in `suggestionlane.go` takes `batchScanDepth` rows across
cases, pairs, approvals and both suggestion kinds.

`conversation_claim` and `lead_response` have no producer. The `Commitments` reader is bound to `nil` in
`backend/internal/compose/attentionseam.go`, and `lead.go` keeps `lead_response` only for the contract.

## Sources: Go steps and SQL capability

"Order", "continue" and "count" ask whether the owning store can, in SQL today, order the admitted population,
resume it after a position, and count it.

Step F applies only to the sources in `doneInPlace` (`done.go`) and to `customer_waiting`. The others leave through
a verb of their own: a task is completed, a decision is decided.

| Source | Steps that remove or merge its rows | Order | Continue | Count |
|---|---|---|---|---|
| `task` | A, C, H | Yes, by due date | Yes, list cursor | `CountActivities`, today's window only |
| `meeting` | A, C, F, H | Yes | Cursor exists, unused | `CountActivities` exists, uncalled |
| `meeting_outcome` | A, C, H | Yes | Cursor exists, unused | `CountActivities` exists, uncalled |
| `brief_item` | A, C, E, H | Rank in Go | No | No |
| `deal_at_risk` | A, B, C, E (absorbs), F, H | Yes | Cursor exists, unused | No |
| `customer_waiting` | Seam drops machine senders and folds threads; then A, B, C (twice), F, H | Yes, newest first | Yes, `WaitingPosition` | Clipped at the scan cap |
| `awaiting_reply`, `meeting_follow_up` | A, C, F, H | Yes | No | No |
| `weekly_commitment` | A, C, H | Go | No | No |
| `approval` | A, C, G, H | Yes | Yes | Yes, `CountPending` |
| `dedupe_candidate` | A, C, G, H | Yes | Yes | Yes |
| `contact_case` | A, C, F, G, H | Ranked in Go | Opaque cursor | Yes |
| `deal_suggestion` | A, C, H | Yes | Yes | Yes |
| `tag_suggestion` | A, C, H | Yes | No | Yes |
| `relationship_decay` | A, C, D, H | Candidates only | No | No |
| `introduction_request` | A, C, F, H | Yes | No | No |
| `domain_question` | A, C, F, G, H | Yes | Yes, `DomainQuestionsForOwner` | No |
| System sources | A, C, F, G (incidents), H | Yes | No | No |

No source's admitted population can be counted in SQL alone today: steps A to G run in Go, and the team roster is
applied after the caps of every source that reads `team` unnarrowed.

## System sources

`categoryOfSource` in `reach.go` files every source it does not name under `system` (`categorySystem` in
`batches.go`). Ownership depends on the classifier. Among the rows `classifySystem` draws, `systemRowOwner` in
`classifydelivery.go` makes `notice` and `capture_health` the reader's and the rest unowned, so step C drops
those from `mine`. `bounce`, `undelivered` and `failed_approval` are the reader's. `dsr` and `notice_case` belong
to their assignee (`classifylegal.go`).

| Source | Store and predicate | Receives it today | Can fix it |
|---|---|---|---|
| `failed_approval` | `approvals` `OwnFailedDecisions`: `decided_by` is the reader | The human who approved it; dropped from `unassigned` and named-owner pages | That human, if the decision is still decidable |
| `dsr` | `modules/consent` `OpenDSRsDueSoonest`: `privacy_request` read | Every `privacy_request` reader, on `unassigned`, `team` and `all` only | A `privacy_request` holder |
| `notice_case` | `consent` `OpenNoticeCasesDueSoonest`: `privacy_request` and `contact` read, owner from the case or contact | Holders whose scope reaches the case owner | A holder who can open the contact |
| `capture_health` | `modules/capture` `Registry.HealthConcerns`: the reader's own connections | The mailbox owner | The mailbox owner, by reconnecting |
| `ai_work_health` | `modules/aiactivity` `Store.Troubled`: `actor_user_id` is the reader | The actor, but only on `unassigned`, `team` and `all`, because the row is unowned | The actor |
| `automation_run` | `modules/automation` `TroubledRuns`: `automation` read, every run in the workspace | Every `automation` reader, on `unassigned`, `team` and `all` | An `automation` editor |
| `bounce`, `undelivered` | `modules/comms` send lanes: the reader sent it | The sender | The sender |
| `notice` | `modules/notices` `UnreadWorkFor`: the reader is the recipient | The recipient | The recipient |

`categoryOfSource` also files `domain_question`, `awaiting_reply`, `meeting_follow_up` and `batch` under `system`,
while their classifiers give the rows `decisions`, `tasks`, `meetings`, and `decisions` or `system`. `counts` and
`sources_unavailable` for those sources therefore name the wrong category.

## Filter values

`keepFiltered` and `keepsRow` in `page.go` decide each value.

| Value | Keeps | Natural view and category |
|---|---|---|
| `all` or absent | Every row | Today |
| A category name | Rows of that category | That category; `decisions` and `system` also skip step G |
| `leads` | Tasks that build pipeline | Today, `tasks` |
| `urgent` | Rows whose own level is at most `levelPromise` (`worklistsummary.go`); a pin does not change it | Today, across categories |
| `except_decisions` | Every row whose source is not `approval` | Today without the Decisions view |
| `changed_since_brief` | Rows that are not approvals and arrived after the brief's data cutoff | Today; empty when no brief ran |
| `upcoming` | Tasks due after today, which carry the `upcoming` category. No other value keeps them | Today, its Upcoming and backlog section |

The `view` parameter (`today`, `decisions`, `system`) is applied by `keepInView` in `views.go`, beside
step H. `resolveView` reads `filter=decisions` or `filter=system` without a view as that view, and
answers 422 for a filter from another view. The cursor binds the view through `askedFor`.

## Future-dated and undated planned work

| Work | Read | Bound | Owner narrowing |
|---|---|---|---|
| Tasks due after today | `UpcomingForViewer` in `compose/attentiontaskseam.go` | `upcomingCap` = 6, within `upcomingHorizonDays` = 14 | The `task` arms |
| Tasks due later than that | Not read | None | None |
| Undated tasks | Only in the due-now read, and only for `storekit.UndatedTaskSources()`; every other undated task is never read (`modules/activities/taskwindow.go`) | `plannedCap` = 12 | The `task` arms |
| Weekly plan commitments | `compose/attentionplanseam.go` keeps only those due by end of today; undated and later ones are skipped | None | As `weekly_commitment` |
| Meetings after today | `meetingHorizon` (`meetinghorizon.go`) reads one, the next customer meeting within `nextMeetingHorizon` = 30 days, into `next_meeting`, for `mine` only and only when today has no meeting left | One row | Meeting responsibility |

## Other bounds

| Bound | Constant and file | Effect |
|---|---|---|
| Frozen walk length | `maxRows` = 1000 in `backend/internal/compose/worklistsnap/store.go` | A walk keeps this many identities and drops the rest |
| Walk lifetime and count | `Life` = 4 hours and `KeptPerReader` = 3, same file | A walk expires, and a reader keeps this many |
| Team roster | `teamRosterCap` = 100 in `backend/internal/modules/identity/teamroster.go` | A cut roster sets `scope_truncated`, and every roster-narrowed source misses the teammates past it |
| Waiting horizon | `DefaultWaitingHorizonDays` = 30 and `WaitingHorizonStaleAfter` = 24 hours in `backend/internal/platform/database/storekit/waitinghorizon.go`; the learned value in `backend/internal/modules/activities/waitinghorizon.go` | A request older than the horizon leaves the waiting source, unless it is an open request a human took |
| Per-statement time | `laneBudget` = 4 seconds in `snapshot.go` | Each optional lane's statement times out and the lane is named as failed; it does not bound the whole lane |
| Page size | `worklistPage` = 25 and `worklistMaxPage` = 100 in `worklist.go` | Rows per page |
