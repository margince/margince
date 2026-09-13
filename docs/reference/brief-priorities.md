# Morning and weekly brief priorities

The morning brief draws the first five non-approval rows of the server-ranked worklist. Approvals retain their decision panel. Personal and team views read their respective worklist scope; the greeting, readings, Today feed and remaining-risk panel use that same answer. Links into the worklist retain the selected scope.

## What leads the day

Actual waiting customers and approaching response deadlines retain precedence. Overdue or due-today tasks outside generic lead prospecting are urgent obligations. Deal recovery enters the urgent-work band when the deal is material relative to the priced risk population, or when its expected close is overdue or within fourteen days. The dated rule applies to equal-value, single-deal and unpriced portfolios too.

The prospecting and existing-work labels share ranking precedence. Deadlines, value and the remaining server tie-breaks decide between them. Clients preserve the order and may repeat section labels; they must not regroup the queue by label.

A provisional close is a reason to confirm or revise the forecast, not evidence of a customer commitment. Cards retain the provisional flag and forecast category, including an omitted forecast. The deal-work reading reports known deal value in the selected queue with an explicit incompleteness qualification; it includes opportunities as well as recovery, and is not a risk-adjusted forecast or expected revenue.

Only a task written by the lead SLA escalation duplicates a dated first-response row. Other lead-linked tasks keep their own identity, due date and actions. A lead without a configured response target still appears as prospecting, with company, status, administered source label and last activity where available, and no invented deadline.

The remaining-risk panel takes rows after Today's prefix from the same queue. It does not query an unrelated first page of deals. The risk engine applies its overdue-close predicate before its bounded scan, and publishes truncation when a candidate source is incomplete.

## What a weekly review can claim

A team snapshot covering zero reps reports unavailable measurement and suppresses performance cards and the agenda. Partial coverage precedes the figures and suppresses a whole-team performance verdict. Historic snapshots remain frozen.

Recorded lead responses are not presented as an SLA success rate. A missing breach stamp does not prove a response target was configured. Both weekly counters and the lead scorecard use the same arrival cohort and closing instant; a response recorded after that instant cannot improve the closed week.

New team snapshots include deal recovery in their agenda, after requests for help, breached responses and missed commitments, and before meeting hygiene or celebration. The evidence comes from each member's frozen deal scorecard: forecast downgrades, stage regressions, unsound close dates or missing next steps. Missing scorecard coverage supplies no finding.

## Validation

Regression tests cover dated risk without a materiality advantage, retained lead follow-ups, response deadlines outranking recovery, personal/team scope, zero and partial weekly coverage, and response timestamps on the week boundary. Real-Postgres tests cover an overdue deal behind more than one hundred current deals, and the shared response cutoff in both weekly projections.
