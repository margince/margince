// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// How the day is ORDERED.
//
// The lane feed answers "which producers have rows?". This answers "what should
// I do next?", which is a different question and needs a rule rather than a
// layout: fourteen lanes leave a reader comparing the position of one panel with
// another to work out that an item several screens down matters more.
//
// The rule is HARD LEVELS first, tie-breaks only inside a level. That ordering
// is a product decision and not a score, because a score lets a large enough
// pile of cheap work outrank a customer waiting — which is exactly the failure
// the ranked queue exists to end. Sixty duplicate merges never reach the top;
// one unanswered buyer always does.
//
// Inside a level the tie-breaks run deadline → expected revenue → waiting days →
// relationship → occurrence. Deadline leads because a date somebody agreed to is
// the one fact on the page that expires; a bigger deal closing in nine months
// can wait a day, and a smaller one closing tomorrow cannot.

import (
	"sort"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
)

// The hard priority bands. A level is a claim about WHAT KIND of work an item
// is, never about how much of it there is.
const (
	// levelPinned is the reader's own override, and the only one they have.
	levelPinned = 0
	// levelWaiting is somebody else's clock running: a customer waiting on a
	// reply, a meeting about to start, a legal deadline.
	levelWaiting = 1
	// levelPromise is a commitment the rep made, or an action they approved
	// and believe happened. Both are promises the product is breaking.
	levelPromise = 2
	// levelMaterialRisk is revenue at risk worth the interruption — measured
	// against the pipeline's own median, so "material" tracks the business
	// rather than a number somebody typed once.
	levelMaterialRisk = 3
	// levelAgreed is work already agreed: an assigned task, or a deal drifting
	// below the material bar.
	levelAgreed = 4
	// levelBlocking is a decision that holds up customer work.
	levelBlocking = 5
	// levelRoutine is data hygiene. It never outranks anything above it,
	// however large the pile.
	levelRoutine = 6
)

// deadlineHorizon is how near a date has to be before it counts as a deadline
// worth leading on. A month out is a plan; a week out is a deadline.
const deadlineHorizon = 7 * 24 * time.Hour

// meetingHorizon is how soon a meeting must start to become the reader's most
// urgent item. Two hours is the window in which preparing is still possible and
// no longer optional.
const meetingHorizon = 2 * time.Hour

// ranked is one item plus the facts the comparator needs, kept beside the wire
// item rather than on it: the client renders an order, and the arithmetic that
// produced it is the server's business.
type ranked struct {
	item crmcontracts.WorklistItem
	// deadlineAt is the moment this item's clock runs out, zero when it has
	// none. Overdue items carry a past instant and sort first by that fact.
	deadlineAt time.Time
	overdue    bool
	// expectedBase is expected revenue in the installation's base currency,
	// which is the ONLY figure by which two deals may be compared. Raw minor
	// units compare a yen to a euro and get it wrong.
	expectedBase int64
	hasExpected  bool
	waitingDays  int
	strength     int
	occurredAt   time.Time
}

// rankAll orders the day and explains itself.
//
// The explanation is produced HERE, during the sort, rather than by re-comparing
// on the client: the tie-breaks depend on the base-currency conversion and the
// material threshold, both of which the server holds and the browser does not.
func rankAll(rows []ranked) []crmcontracts.WorklistItem {
	sort.SliceStable(rows, func(i, j int) bool {
		return less(rows[i], rows[j])
	})
	out := make([]crmcontracts.WorklistItem, 0, len(rows))
	for i, row := range rows {
		item := row.item
		// Every row but the last says what it beat, and how. The last has
		// nothing below it, so it says nothing rather than inventing a
		// comparison against a row that is not there.
		if i+1 < len(rows) {
			comparison := compare(row, rows[i+1])
			item.AboveNext = &comparison
		}
		out = append(out, item)
	}
	return out
}

// less is the whole ordering, in one place so the comparator and the
// explanation cannot come to disagree — compare() below walks the same steps in
// the same order and names the first one that decided.
func less(a, b ranked) bool {
	if a.item.Level != b.item.Level {
		return a.item.Level < b.item.Level
	}
	if decided, order := byDeadline(a, b); decided {
		return order
	}
	if decided, order := byExpected(a, b); decided {
		return order
	}
	if a.waitingDays != b.waitingDays {
		return a.waitingDays > b.waitingDays
	}
	if a.strength != b.strength {
		return a.strength > b.strength
	}
	if !a.occurredAt.Equal(b.occurredAt) {
		return a.occurredAt.Before(b.occurredAt)
	}
	// The ids break a complete tie, so one read's order is the next read's
	// order. An unstable queue teaches a reader that the ranking means nothing.
	return a.item.Id < b.item.Id
}

// byDeadline answers whether a date decided this pair.
//
// Overdue beats merely due, and sooner beats later. An item with no date loses
// to one that has a near date and beats nothing — a task with no due date is
// not urgent, but it is not ahead of a deal closing tomorrow either.
func byDeadline(a, b ranked) (decided, order bool) {
	if a.overdue != b.overdue {
		return true, a.overdue
	}
	aNear, bNear := nearDeadline(a), nearDeadline(b)
	if aNear != bNear {
		return true, aNear
	}
	if aNear && bNear && !a.deadlineAt.Equal(b.deadlineAt) {
		return true, a.deadlineAt.Before(b.deadlineAt)
	}
	return false, false
}

// byExpected answers whether money decided this pair. A deal whose value is
// unknown sorts BELOW one whose value is known, never above: absence of a figure
// is not a large figure.
func byExpected(a, b ranked) (decided, order bool) {
	if a.hasExpected != b.hasExpected {
		return true, a.hasExpected
	}
	if a.hasExpected && a.expectedBase != b.expectedBase {
		return true, a.expectedBase > b.expectedBase
	}
	return false, false
}

func nearDeadline(r ranked) bool {
	return !r.deadlineAt.IsZero()
}

// compare names the FIRST tie-break that decided a pair, with both sides'
// values. It walks the same steps as less(), in the same order, so a row can
// never claim a reason that did not decide it.
func compare(a, b ranked) crmcontracts.WorklistComparison {
	if a.item.Level == levelPinned && b.item.Level != levelPinned {
		return crmcontracts.WorklistComparison{Comparator: crmcontracts.WorklistComparisonComparatorPin}
	}
	if a.item.Level != b.item.Level {
		return crmcontracts.WorklistComparison{
			Comparator: crmcontracts.WorklistComparisonComparatorLevel,
			Mine:       levelValue(a.item.Level),
			Theirs:     levelValue(b.item.Level),
		}
	}
	if decided, _ := byDeadline(a, b); decided {
		return crmcontracts.WorklistComparison{
			Comparator: crmcontracts.WorklistComparisonComparatorDeadline,
			Mine:       dateValue(a),
			Theirs:     dateValue(b),
		}
	}
	if decided, _ := byExpected(a, b); decided {
		return crmcontracts.WorklistComparison{
			Comparator: crmcontracts.WorklistComparisonComparatorExpectedRevenue,
			Mine:       moneyValue(a),
			Theirs:     moneyValue(b),
		}
	}
	if a.waitingDays != b.waitingDays {
		return crmcontracts.WorklistComparison{
			Comparator: crmcontracts.WorklistComparisonComparatorWaitingDays,
			Mine:       daysValue(a.waitingDays),
			Theirs:     daysValue(b.waitingDays),
		}
	}
	if a.strength != b.strength {
		return crmcontracts.WorklistComparison{
			Comparator: crmcontracts.WorklistComparisonComparatorRelationship,
		}
	}
	// Everything tied and the ids broke it. Saying so honestly is better than
	// naming a comparator that decided nothing; the client draws no reason line
	// for this case.
	return crmcontracts.WorklistComparison{Comparator: crmcontracts.WorklistComparisonComparatorOrder}
}

func levelValue(level int) *crmcontracts.WorklistValue {
	value := level
	return &crmcontracts.WorklistValue{Kind: "level", Level: &value}
}

func dateValue(r ranked) *crmcontracts.WorklistValue {
	if r.deadlineAt.IsZero() {
		return &crmcontracts.WorklistValue{Kind: "none"}
	}
	at := r.deadlineAt
	return &crmcontracts.WorklistValue{Kind: "date", Date: &at}
}

func moneyValue(r ranked) *crmcontracts.WorklistValue {
	if !r.hasExpected {
		return &crmcontracts.WorklistValue{Kind: "none"}
	}
	minor := r.expectedBase
	return &crmcontracts.WorklistValue{Kind: "money", Minor: &minor}
}

func daysValue(days int) *crmcontracts.WorklistValue {
	value := days
	return &crmcontracts.WorklistValue{Kind: "days", Days: &value}
}

// overdueAt reports whether a due moment is behind the read instant, through the
// one authority that decides lateness product-wide.
//
// Held by: TestOnlyOnePlaceDecidesWhetherSomethingIsLate
// (backend/gates/overdueboundary_test.go).
func overdueAt(due *time.Time, asOf time.Time) bool {
	return deadline.Passed(due, asOf)
}
