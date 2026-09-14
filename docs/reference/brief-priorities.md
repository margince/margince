# Home: morning and weekly priorities

The personal morning brief draws up to six server-selected Focus cards, including actionable proposals and commitments due today. Informational notices appear in Updates; pinned notices retain their chosen position. The six priorities lead the page; supporting readings follow them. Task evidence opens in the shared task dialog, and contact context opens in a separate drawer, preserving the overview. Brief creation time comes from the stored run; agenda refresh time comes from the live queue. The headline names the same focus cards. The full queue opens on demand in Home and owns filtering and pagination. Future tasks and routine privacy work remain available there with their deadlines and actions.

The team morning is a named-team board with routes to each member’s work and current plan. Morning and Weekly retain the same team selection in the URL. The board roster is resolved through live membership and reader authority; workspace-wide unassigned work is not attributed to a named team.

## What leads the day

Fresh waiting customers and approaching response deadlines retain precedence. Waiting threads older than fourteen days are recovery work: linked open deals retain material-risk priority, other old threads become routine. Privacy obligations enter the preparation window seven days before the actual deadline; the legal deadline itself is unchanged. Overdue or due-today tasks outside generic lead prospecting are urgent obligations. Deal recovery enters the urgent-work band when the deal is material relative to the priced risk population, or when its expected close is overdue or within fourteen days. The dated rule applies to equal-value, single-deal and unpriced portfolios too.

Existing customer work precedes routine prospecting. Actual response deadlines and urgent obligations still lead both. Within each band, deadlines, value and the remaining server tie-breaks decide the order. Clients preserve that order.

A provisional close is a reason to confirm or revise the forecast, not evidence of a customer commitment. Cards retain the provisional flag and forecast category, including an omitted forecast. The deal-work reading reports known deal value in the selected queue with an explicit incompleteness qualification; it includes opportunities as well as recovery, and is not a risk-adjusted forecast or expected revenue.

Only a task written by the lead SLA escalation duplicates a dated first-response row. Other lead-linked tasks keep their own identity, due date and actions. A lead without a configured response target still appears as prospecting, with company, status, administered source label and last activity where available, and no invented deadline.

Focus eligibility is evaluated before the six-card cap. The risk engine applies its overdue-close predicate before its bounded scan, and publishes truncation when a candidate source is incomplete.

## What the daily brief can claim

Dates, reply direction, deal standing and its reason stay on the row. Details contain supporting evidence, not comparisons of raw ranking timestamps. A task never inherits a deal-level recommendation. Pinning changes personal order without changing the server’s urgent classification.

Upcoming meetings count calendar entries, excluding past meetings awaiting outcomes. Preparation is unknown unless the source explicitly establishes it. A source failure is not a zero; failed sources are named with a retry. Permission exclusions and bounded scans do not produce a generic alarm; counts retain their bounds and pagination remains explicit. Deal values exclude unpriced deals and state their count.

Open weekly commitments with an intentional due date enter Today on that date in the installation timezone. Done uses the existing plan writer and invalidates the agenda. Undated, future and completed commitments stay out of the due lane. Creating a commitment can link an existing CRM record through the shared record picker. Team leads can read a member’s current plan and answer help requests through the existing permission-checked writer.

## What a weekly review can claim

A team snapshot covering zero reps reports unavailable measurement and suppresses performance cards and the agenda. Partial coverage precedes the figures and suppresses a whole-team performance verdict. Historic snapshots remain frozen. The selected period drives both headline and detail. Losses and recorded SDR activity contribute to the headline; missing snapshots do not masquerade as quiet weeks. Deal outcomes and the manager conversation agenda precede expandable supporting metrics, forecasts and observations. Forecasts retain their stored period and generation date; observations do not imply causation.

Recorded lead responses are not presented as an SLA success rate. A missing breach stamp does not prove a response target was configured. Both weekly counters and the lead scorecard use the same arrival cohort and closing instant; a response recorded after that instant cannot improve the closed week.

New team snapshots include deal recovery in their agenda, after requests for help, breached responses and missed commitments, and before meeting hygiene or celebration. The evidence comes from each member's frozen deal scorecard: forecast downgrades, stage regressions, unsound close dates or missing next steps. Missing scorecard coverage supplies no finding.

## Validation

Regression tests cover dated risk without a materiality advantage, retained lead follow-ups, response deadlines outranking recovery, personal/team scope, zero and partial weekly coverage, and response timestamps on the week boundary. Real-Postgres tests cover an overdue deal behind more than one hundred current deals, and the shared response cutoff in both weekly projections.

## Reviewing automatic changes

Changes made for you shows recorded work, its subject, reason and occurrence time.
Close-date corrections name the old and new date when the date changed; a change
in confidence alone is not described as a date change. Automatic stage moves use
the progression ledger's guarded reversal; date corrections use record history's
correction restore. Accept records a durable, audited review of that exact change
and retains its applied values. It does not replay the change or turn an unconfirmed
forecast into a customer commitment. Reversed and superseded changes cannot be
accepted. A generic stage notification is information, not evidence that an agent
changed the deal. New notifications retain the actual stage names at occurrence.

## Focus and the work queue

Morning's Focus is an additive `/worklist` projection, selected on the server
before the queue's filter and page limit. It carries up to six cards in the
existing rank order. The headline reads exactly those cards. A quiet day has
zero cards; future tasks and routine maintenance remain in the full queue.
Pins are explicit exceptions, without changing a row's semantic urgency.
Meeting preparation is eligible within 24 hours when preparation is missing;
prepared meetings remain in Schedule. Privacy preparation retains its existing
seven-day classifier window. Unpriced commercial work remains eligible.

`focus.total` counts eligible cards, with a group counting once.
`focus.urgent_remaining` counts urgent underlying work not individually named by the cards in the
same units as `summary.urgent`; it remains visible even without another queue
page. Source bounds and failed or withheld reads retain their separate coverage
meaning. Neither an exhausted page nor a complete focus projection certifies
that every source was fully read.

Home is the daily navigation entry. Its work queue opens in a right drawer,
with the existing scope, owner, filters, paging, actions, coaching and reviews.
The queue has its own `queue_scope` dial so it cannot change the Morning/Weekly
view behind it. Old `#/worklist` and owner/unassigned links redirect to the same
Home queue state; the API contract and domain writers remain available.
Context opens independently of the overview. Closing it restores keyboard focus to its opener. The queue keeps its own filtering and scroll state.

## Attribution and commitments

New stage-change notifications carry the original event actor and occurrence time through live and retry dispatch. Delivery still writes as the automation, and a recipient/event key prevents duplicate notifications. Stage changes made by the recipient themselves are skipped before notification delivery and excluded from existing unread feeds before the page limit. Deal history still records those changes. Other human changes name the member; machine changes remain visible even when they ran on the recipient’s behalf. Unknown authorship is never treated as a self-made change. The reader retains its “You” formatting for compatible older servers and cached responses; the current server does not deliver self-made stage updates. Stage updates use the deal name once, then the recorded from/to stages and who changed them. Historical notices recover these facts through the exact notice-created event’s causation link to the stage-change event. Recorded stage names take precedence; without a snapshot, only a never-edited stage configuration supplies a name. Edited configurations remain unknown, without comparing clocks. Missing history is stated plainly; ownership is never substituted for authorship.

Task responsibility comes from the assigned user ID. Recognized reader-prefixed task wording is presented as “You need to …”, without rewriting the stored promise. Details retain the original wording and evidence. Similar tasks from distinct transcripts or deadlines remain separate obligations; text similarity alone cannot establish supersession.

## Stable close dates

The nightly repair replaces missing or overdue dates. It retains a valid future date, including a provisional estimate from an earlier sweep. Quietness may lower forecast confidence without moving the date. A replacement is today plus observed median stage days multiplied by remaining open stages, rounded up to whole weeks, with a minimum of seven days. Without sufficient history the fallback is fourteen days per stage. All generated replacement dates remain provisional; existing opt-outs, reversal memory and review controls still apply.

## Regional presentation and links

Installation settings place date and time notation beside base currency. Date options are interface-language default, DD.MM.YYYY, MM/DD/YYYY and YYYY-MM-DD. Time options are interface-language default, 24-hour and 12-hour. Shared formatters apply the preference throughout the authenticated interface, including typed close-date receipts, without changing stored values or their reporting/record timezone. Native date-input editing remains governed by the browser.

The canonical destination is `#/home` in every language. Visible navigation is localized (Home, Startseite, Trang chủ). Older `#/brief` links preserve their query parameters while redirecting to Home. Existing Worklist links still open the queue with their owner and filters.

## Personal relevance and email requests

Mine means the reader's responsibilities for managers and individual contributors alike.
Access to a record does not assign its work. Team oversight is selected explicitly;
team exceptions follow live team membership, and broad backlog diagnostics appear only
under All. Manager authority does not grant access to colleagues' private correspondence.

| Surface | Personal scope | Wider scope |
| --- | --- | --- |
| Focus and deal drill-down | Owned deals, assigned tasks and confirmed incoming requests | Select the queue's team or all scope explicitly |
| Team board and exceptions | Absent | Team members and visible unassigned work, under existing grants |
| Overnight | Own imported mail and owned projects, revalidated when read | Does not become a team feed merely because the reader manages colleagues |
| Contact conversations | Same stored thread grouped together | Each original retains its content gate |

Unanswered does not mean actionable. Focus requires an `asks_us` verdict and a
`commitment` capture label. Missing, conflicting, informational or meeting-only
classification remains reviewable without claiming urgency, even on an open deal.
A confirmed first request needs no previous outbound message to deserve attention.
The classifier reads the sender's new words rather than quoted earlier requests.

The hourly request pass creates one undated personal task for a confirmed unanswered
request with exactly one directly addressed importing seat. It preserves the source
message and record links, honors the recipient's set-aside state, and never invents a
deadline. Existing verdicts can be processed without a configured model. Private and
restricted mail remains outside this automatic classification flow. Ambiguous assignment
is left for review rather than guessed.

The reminder replaces its source email in the assignee’s queue. Completion settles
the request for every reader. Archiving an incomplete reminder makes the original
request reviewable again, without creating another task; reopening remains explicit. A reply
settles only its own conversation and does not prove a promised deliverable was completed.
The task remains until handled. Its source action opens the original email reader, and
removing access to the source also withholds the derived task text.

When no classifier is available, uncertain mail stays in the conversation review
queue. It does not claim Focus priority or a confirmed team obligation. Outbound
intent is not classified by this pass: a sent acknowledgement does not prove that
the recipient owes an answer. Email move therefore remains unknown (`none`) unless
there is positive request evidence. The wire retains older move values for client
compatibility; the current producer never infers them from direction alone.

## Weekly measurement and recovery

Personal reports summarize the member's recorded responsibilities even when that
member is a manager. Team reports are an explicit management view of member
summaries, gated by live team authority; they do not expose private message text.
Ownership is resolved when the report is generated. Once measured, the report
retains that attribution and roster rather than following later reassignments.

The reporting window is the complete local Monday-to-Sunday week. A completion
at the following Monday's midnight belongs to the next week. Completed tasks
include undated work and earlier overdue work, separately from the ratio of tasks
due that week. Older snapshots have no completed-task measurement, rather than
an invented zero. Carryover includes tasks due during or before the reviewed week
that were unfinished at its closing boundary.

A recorded lead response includes a late response; breach counts remain separate.
A breach therefore does not mean a lead remains unanswered. Meeting follow-up
counts require an explicit source link to a readable task created before week end.
A task sharing a contact or account is insufficient evidence. Missing linkage is
a recording gap, not proof that the member failed to follow up. The same content
permission checks apply to task and contact evidence in the deal scorecard.

Personal and team measurements wait until Monday's configured review hour;
catch-up also runs on Sunday. Both complete before optional model calls and mail.
Configured narration, observations and unattempted delivery remain independently
reachable on retries. Unconfigured services do not keep measured members due.
A team waits when one of its members failed measurement on the current pass.
The job selects a member with team read authority rather than assuming any
member can read a team report. A team report with zero measured members can be replaced by its first
measurement, retaining its ID and recording the correction in the audit trail.
Its roster is captured at that first measurement. A nonempty snapshot, including
explicitly partial coverage, remains frozen.

A team total is unavailable if a member snapshot is missing, cannot be converted, or uses a
different currency. Comparisons name the actual earlier report's week, which may
not be the immediately preceding week. An absence of coaching signals says no
priority was identified; it makes no claim about productivity or inactivity.
Current-plan and work-queue links explicitly refer to today's responsibilities.

Older frozen reports retain the measurement rules used when they were created.
Missing meeting documentation ranks below recorded wins or kept commitments.

The scheduled writer and web reader share the composed weekly engine, including
plan settlement and forecast snapshots. A refused forecast does not discard the
member's recorded-work report.
