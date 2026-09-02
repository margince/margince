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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// valueNone is the comparator value for a side that has nothing to compare —
// no date, or no amount. A named constant rather than the literal at each site,
// because "nothing to compare" is a decision about the ordering and reads as one
// here, where a bare "none" in three places reads as a coincidence.
const valueNone = "none"

// valueMoney is the comparator value kind for an amount, named for the reason
// valueNone is: three sites spell it, and a typo in one would hand the client
// a value it draws as nothing.
const valueMoney = "money"

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
	// expectedBase is expected revenue in the currency expectedCurrency names:
	// the installation's base currency once the FX seam priced the day, which
	// is the ONLY figure by which two deals may be compared — raw minor units
	// compare a yen to a euro and get it wrong. In an assembly without the
	// seam it is the deal's own raw amount and expectedCurrency stays empty.
	expectedBase int64
	hasExpected  bool
	// expectedCurrency names expectedBase's units, empty when the figure is a
	// raw amount not genuinely in any one currency — the comparison's money
	// values claim a currency only when one is true.
	expectedCurrency string
	// waitingDays is the TRUE age of a wait, in days, and it is what a reader is
	// ever shown — both in the row's own reasons and in the comparison naming
	// why it sits above the next one.
	//
	// waitingRank is the same age bounded by the ordering ceiling, and it is
	// what decides the order. The two are separate fields because they are read
	// by different callers for different purposes, and a single field serving
	// both published the bounded number as though it were the real one: a row
	// saying "waiting 180 days" while its own explanation compared 30 against
	// 20, which is a reason nobody can check.
	waitingDays int
	waitingRank int
	strength    int
	occurredAt  time.Time
	// owner is who this row answers to, for the sources that carry an owner but
	// no deal on the wire.
	//
	// The scope filters judge a deal-bearing row by its deal's owner, which a
	// waiting message does not have: the message names a person, not a deal, and
	// its ownership is the ownership of the record it is filed under. The lane
	// resolves that walk, and this is where the answer rides so the SAME filters
	// can judge it. Zero means the row names nobody, which for a wait is an
	// unowned customer rather than a missing answer.
	owner ids.UUID
	// foldedFrom names the sources of the rows this one stands for, once per
	// member. A folded group is shown INSTEAD of its members, so a count of
	// what the reader can see has to attribute it back to them — otherwise
	// every folded source reports zero shown, which reads as "nothing from
	// this source" rather than "folded into one row".
	foldedFrom []crmcontracts.WorklistItemSource
	// What a routine contact decision is ABOUT, for the group it joins. Read
	// from the staged payload rather than re-derived here: the verdict engine
	// already decided who the address belongs to, and a second opinion would
	// put the same decision in two groups on two reads.
	machineSender bool
	knownCompany  bool
	// crowded marks a row that is one of many of its kind. It sorts below its
	// unmarked siblings so one kind of work cannot fill the page, and it
	// changes nothing else: the row is still what it is, and every count that
	// reads its level still reads the truth.
	crowded bool
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
		// What this row offers a reader who is NOT going to answer it now, and
		// which of its verbs is the step. Stamped here rather than by each
		// classifier, so a source added later carries a primary action by
		// arriving in this loop instead of by its author remembering to.
		if item.Source == sourceWaiting {
			offered := waitingDispositions()
			item.Dispositions = &offered
		}
		item.PrimaryAction = primaryActionFor(item)
		// The heading this row sits under. Stamped after the sort, so it
		// describes a row's place on the page it is actually on.
		band := crmcontracts.WorklistItemBand(bandOfRow(row))
		item.Band = &band
		out = append(out, item)
	}
	return out
}

// less orders one pair by walking rankSteps until one of them decides.
//
// The steps live in ranksteps.go beside their own explanations, so the order a
// reader is shown and the reason they are given come from one list rather than
// from two functions kept in step by hand. They had already come apart there:
// crowding was the first thing this compared and the one thing the explanation
// never mentioned.
func less(a, b ranked) bool {
	for _, step := range rankSteps {
		if decided, aFirst := step.decides(a, b); decided {
			return aFirst
		}
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

// compare names the FIRST step that decided a pair, with both sides' values.
//
// Walks the SAME slice less() walks, in the same order, and asks the step that
// decided to describe its own decision — so a row cannot claim a reason that
// did not decide it, and a step cannot join the ordering without being given
// words to explain itself.
func compare(a, b ranked) crmcontracts.WorklistComparison {
	for _, step := range rankSteps {
		if decided, _ := step.decides(a, b); decided {
			return step.explain(a, b)
		}
	}
	// Everything tied and the ids broke it. Saying so honestly is better than
	// naming a comparator that decided nothing; the client draws no reason line
	// for this case.
	return crmcontracts.WorklistComparison{Comparator: crmcontracts.WorklistComparisonComparatorOrder}
}

// sameMinute reports whether two instants read alike on screen. The card shows
// a date and a time to the minute, so anything finer is a difference the reader
// cannot see and must not be offered as an explanation.
func sameMinute(a, b time.Time) bool {
	return a.Truncate(time.Minute).Equal(b.Truncate(time.Minute))
}

func levelValue(level int) *crmcontracts.WorklistValue {
	value := level
	return &crmcontracts.WorklistValue{Kind: "level", Level: &value}
}

func dateValue(r ranked) *crmcontracts.WorklistValue {
	if r.deadlineAt.IsZero() {
		return &crmcontracts.WorklistValue{Kind: valueNone}
	}
	at := r.deadlineAt
	return &crmcontracts.WorklistValue{Kind: "date", Date: &at}
}

func moneyValue(r ranked) *crmcontracts.WorklistValue {
	if !r.hasExpected {
		return &crmcontracts.WorklistValue{Kind: valueNone}
	}
	minor := r.expectedBase
	value := &crmcontracts.WorklistValue{Kind: valueMoney, Minor: &minor}
	if r.expectedCurrency != "" {
		currency := r.expectedCurrency
		value.Currency = &currency
	}
	return value
}

func occurredValue(r ranked) *crmcontracts.WorklistValue {
	at := r.occurredAt
	return &crmcontracts.WorklistValue{Kind: "date", Date: &at}
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
