# Morning and weekly brief priorities

The personal morning brief draws up to six server-selected Focus cards, including actionable proposals and commitments due today. Informational notices appear in Updates; pinned notices retain their chosen position. Work summary stays open in the overview context column, which selected contact context replaces. Brief creation time comes from the stored run; agenda refresh time comes from the live queue. The headline names the same focus cards. The full queue opens on demand in Brief and owns filtering and pagination. Future tasks and routine privacy work remain available there with their deadlines and actions.

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

Brief is the daily navigation entry. Its work queue opens in a right drawer,
with the existing scope, owner, filters, paging, actions, coaching and reviews.
The queue has its own `queue_scope` dial so it cannot change the Morning/Weekly
view behind it. Old `#/worklist` and owner/unassigned links redirect to the same
Brief queue state; the API contract and domain writers remain available.
Contact context replaces Brief's overview rail while selected. On a narrow
screen context has a back control and the queue stays mounted to retain place.
