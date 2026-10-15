// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What counts as a next step on a deal, spelled ONCE for the two sweeps that
// ask: the overnight follow-up reconciliation (reconcile.go) and the slipping
// lane's "nothing planned" reading (compose/slippingnextstep.go).
//
// The two used to carry their own copies of "an open task exists", and the copy
// is what let them disagree the moment a booked meeting became a next step:
// fixing one would have left the other proposing follow-ups for deals that
// visibly had one.

import "fmt"

// OpenNextStepSQL renders the predicate "this deal has a next step planned",
// true when either is on its timeline:
//
//   - an open task, or
//   - a meeting that is still standing and has not ENDED yet — which covers the
//     one in progress as well as the one on Thursday. A rep sitting in a
//     meeting has a next step; telling them they do not is the same defect as
//     telling them tomorrow's meeting already happened.
//
// The end test is written as a direct comparison rather than as `NOT
// MeetingIsOverSQL(...)`. Negating that expression looks equivalent and is not:
// it contains an `IN` over a nullable column, so an unmarked meeting made it
// NULL, `NOT NULL` is NULL, and every such row silently failed the filter.
//
// A meeting on the calendar IS a next step, which is the half that was missing.
// A deal whose only forward commitment is Thursday's review reads as planned to
// the rep looking at it, and a sweep that asked only about tasks proposed a
// follow-up for exactly that deal.
//
// dealExpr names the deal being asked about (an expression, not a value — the
// callers pass `d.id` and `l.deal_id`) and asOf is the placeholder holding the
// sweep's clock. Both are compile-time literals at every call site: nothing
// off a request body is ever formatted in here.
func OpenNextStepSQL(dealExpr, asOf string) string {
	return fmt.Sprintf(`(
		EXISTS (
			SELECT 1 FROM activity t
			JOIN activity_link tl ON tl.activity_id = t.id AND tl.deal_id = %[1]s
			WHERE t.kind = 'task' AND t.is_done = false AND t.archived_at IS NULL
		)
		OR EXISTS (
			SELECT 1 FROM activity m
			JOIN activity_link ml ON ml.activity_id = m.id AND ml.deal_id = %[1]s
			WHERE m.kind = 'meeting' AND m.archived_at IS NULL
			  AND %[3]s
			  AND %[2]s < m.occurred_at
			         + make_interval(secs => coalesce(m.duration_seconds, 0))
		)
	)`, dealExpr, asOf, meetingStillStandingSQL("m"))
}

// MeetingIsOverSQL renders "this meeting is finished", the reading the waiting
// queue and the brief's snooze-lift already share: over means ENDED — start
// plus duration, not start — and a meeting that will never happen counts as
// over.
//
// A third spelling of a rule that already lives in `activities` is a cost, and
// the layering is why it is paid: a module never imports a sibling, so `deals`
// cannot reach activities' copy. backend/gates/meetingoverspelling_test.go is
// what keeps the two honest — it derives the clause from the activities owner
// and fails when either side drifts, in both directions.
func MeetingIsOverSQL(alias, asOf string) string {
	return fmt.Sprintf(`(%[1]s.archived_at IS NOT NULL
		OR %[1]s.meeting_status IN ('canceled', 'no_show')
		OR %[1]s.occurred_at
		   + make_interval(secs => coalesce(%[1]s.duration_seconds, 0)) <= %[2]s)`,
		alias, asOf)
}

// meetingStillStandingSQL asks whether a meeting is still expected to happen:
// nobody has called it off, and nobody has already recorded it as done.
//
// `held` is excluded alongside the two cancellations, because everywhere else
// in this tree it is a statement about the PAST — the meeting brief's history
// and the weekly scorecard both read it as "this one happened". A future row
// carrying it is a meeting somebody has already settled, not a plan the deal
// can be told it has.
//
// NULL is admitted deliberately, and it is the whole reason this is not written
// as `meeting_status NOT IN (...)`. The column is nullable (the schema's own
// CHECK allows it) and the overwhelming majority of captured meetings carry no
// status at all — a calendar entry nobody has marked. `NOT IN` is NULL-false,
// so that spelling would call every unmarked meeting settled and quietly
// exclude nearly every meeting there is.
func meetingStillStandingSQL(alias string) string {
	return fmt.Sprintf(
		`(%[1]s.meeting_status IS NULL OR %[1]s.meeting_status NOT IN ('canceled', 'no_show', 'held'))`,
		alias)
}
