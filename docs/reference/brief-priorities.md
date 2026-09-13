# Morning and weekly brief priorities

The personal morning brief draws all loaded rows of the server-ranked worklist, including proposals to approve and dated weekly commitments. Show more appends the next page in place; the headline and work summary use the same answer. Tasks and flagged deals have one agenda placement. Routine privacy work can be expanded together without losing its deadlines or actions.

The team morning is a named-team board with routes to each member’s work and current plan. Morning and Weekly retain the same team selection in the URL. The board roster is resolved through live membership and reader authority; workspace-wide unassigned work is not attributed to a named team.

## What leads the day

Fresh waiting customers and approaching response deadlines retain precedence. Waiting threads older than fourteen days are recovery work: linked open deals retain material-risk priority, other old threads become routine. Privacy obligations enter the preparation window seven days before the actual deadline; the legal deadline itself is unchanged. Overdue or due-today tasks outside generic lead prospecting are urgent obligations. Deal recovery enters the urgent-work band when the deal is material relative to the priced risk population, or when its expected close is overdue or within fourteen days. The dated rule applies to equal-value, single-deal and unpriced portfolios too.

The prospecting and existing-work labels share ranking precedence. Deadlines, value and the remaining server tie-breaks decide between them. Clients preserve the order and may repeat section labels; they must not regroup the queue by label.

A provisional close is a reason to confirm or revise the forecast, not evidence of a customer commitment. Cards retain the provisional flag and forecast category, including an omitted forecast. The deal-work reading reports known deal value in the selected queue with an explicit incompleteness qualification; it includes opportunities as well as recovery, and is not a risk-adjusted forecast or expected revenue.

Only a task written by the lead SLA escalation duplicates a dated first-response row. Other lead-linked tasks keep their own identity, due date and actions. A lead without a configured response target still appears as prospecting, with company, status, administered source label and last activity where available, and no invented deadline.

The agenda has no separate remaining-risk panel or fixed five-row prefix. The risk engine applies its overdue-close predicate before its bounded scan, and publishes truncation when a candidate source is incomplete.

## What the daily brief can claim

Dates and record context stay on the row. Details contain supporting evidence, not comparisons of raw ranking timestamps. Pinning changes personal order without changing the server’s urgent classification.

Upcoming meetings count calendar entries, excluding past meetings awaiting outcomes. Preparation is unknown unless the source explicitly establishes it. A source failure is not a zero; incomplete sources are named separately from pagination. Deal values exclude unpriced deals and state their count.

Open weekly commitments with an intentional due date enter Today on that date in the installation timezone. Done uses the existing plan writer and invalidates the agenda. Undated, future and completed commitments stay out of the due lane. Creating a commitment can link an existing CRM record through the shared record picker. Team leads can read a member’s current plan and answer help requests through the existing permission-checked writer.

## What a weekly review can claim

A team snapshot covering zero reps reports unavailable measurement and suppresses performance cards and the agenda. Partial coverage precedes the figures and suppresses a whole-team performance verdict. Historic snapshots remain frozen. The selected period drives both headline and detail. Losses and recorded SDR activity contribute to the headline; missing snapshots do not masquerade as quiet weeks. Deal outcomes and the manager conversation agenda precede expandable supporting metrics, forecasts and observations. Forecasts retain their stored period and generation date; observations do not imply causation.

Recorded lead responses are not presented as an SLA success rate. A missing breach stamp does not prove a response target was configured. Both weekly counters and the lead scorecard use the same arrival cohort and closing instant; a response recorded after that instant cannot improve the closed week.

New team snapshots include deal recovery in their agenda, after requests for help, breached responses and missed commitments, and before meeting hygiene or celebration. The evidence comes from each member's frozen deal scorecard: forecast downgrades, stage regressions, unsound close dates or missing next steps. Missing scorecard coverage supplies no finding.

## Validation

Regression tests cover dated risk without a materiality advantage, retained lead follow-ups, response deadlines outranking recovery, personal/team scope, zero and partial weekly coverage, and response timestamps on the week boundary. Real-Postgres tests cover an overdue deal behind more than one hundred current deals, and the shared response cutoff in both weekly projections.
