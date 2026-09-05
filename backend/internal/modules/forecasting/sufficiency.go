// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

// Whether there is enough pipeline to land where the period is expected to.
//
// The question a manager asks in week two, and the one every "coverage ratio"
// in this industry gets wrong by answering it circularly: pipeline divided by
// a target derived from the pipeline is a number that is always fine.
//
// So the basis is NAMED and comes from outside the current projection. Either a
// manager wrote a call down, or four comparable periods actually finished and
// their median is what this business does. If neither is available the answer is
// that there is no basis — never a figure with a shrug attached.
//
// This is not a target. Margince has no target model and this must not become
// one by implication: the words below are "reference" and "basis", and the
// surface drawing them says the same. A number a rep is measured against is a
// product decision nobody has made.

import (
	"fmt"
	"math"
	"sort"
)

// The bars below which an answer is not worth giving.
const (
	// comparablePeriods is how many completed periods a median needs. Four,
	// because it is a year of quarters and long enough that one exceptional
	// period cannot carry the median on its own.
	comparablePeriods = 4
	// minimumClosedDeals is how many closed deals a conversion rate needs
	// before it is a rate rather than an anecdote. Twenty is not a statistical
	// threshold; it is the point below which a single deal moves the answer
	// by more than the answer is worth.
	minimumClosedDeals = 20
	// minimumStageArrivals is how many deals must have reached a stage before
	// its own rate is used instead of the blended one. Five, for the same
	// reason and at a smaller scale: a stage two deals have ever entered has
	// no rate, it has two stories.
	minimumStageArrivals = 5
)

// SufficiencyBasis names where the reference came from, because a coverage
// figure without its basis is a number a reader cannot argue with.
type SufficiencyBasis string

const (
	// BasisManagerCall is the current authored call for this period and scope.
	BasisManagerCall SufficiencyBasis = "manager_call"
	// BasisHistoricalMedian is the median actual Won of the last four
	// comparable completed periods.
	BasisHistoricalMedian SufficiencyBasis = "historical_median"
)

// SufficiencyAbsence says why there is no answer, when there is none.
type SufficiencyAbsence string

const (
	// AbsenceInsufficientBasis means neither a call nor four completed periods.
	// There is nothing honest to compare the pipeline against.
	AbsenceInsufficientBasis SufficiencyAbsence = "insufficient_basis"
	// AbsenceInsufficientHistory means fewer than twenty closed deals, so no
	// conversion rate is worth dividing by.
	AbsenceInsufficientHistory SufficiencyAbsence = "insufficient_history"
)

// Sufficiency is how much open pipeline the reference needs, and how much there
// is.
type Sufficiency struct {
	// Absent is set when no answer is supportable; every other field is then
	// zero and the surface says why rather than drawing a figure.
	Absent SufficiencyAbsence
	// Basis and ReferenceLandingMinor are what the pipeline is measured
	// against, and where that number came from.
	Basis                 SufficiencyBasis
	ReferenceLandingMinor int64
	// RemainingToSupportMinor is the reference minus what is already won,
	// floored at zero: a period already past its reference needs no more
	// pipeline, and a negative requirement would print as a coverage figure
	// nobody can read.
	RemainingToSupportMinor int64
	// NeededOpenMinor is the open pipeline that remainder implies at the
	// conversion rate, and CurrentOpenMinor is what there is.
	NeededOpenMinor  int64
	CurrentOpenMinor int64
	// CoverageBp is current over needed in basis points, so a caller renders a
	// ratio without this package choosing a rounding for them. Zero needed is
	// full coverage rather than a division by zero.
	CoverageBp int64
}

// ConversionHistory is what the sufficiency read needs from outside itself.
//
// Taken as data rather than read here, because this package does no I/O and a
// pure function is what makes every case below testable without a database.
type ConversionHistory struct {
	// ClosedDeals is how many deals closed in the window the rates were read
	// over, whatever they closed as. The bar the whole answer rests on.
	ClosedDeals int
	// BlendedWonPerReached is the fraction of deals reaching ANY stage that
	// were eventually won, used where a stage has too few arrivals of its own.
	BlendedWonPerReached float64
	// ComparableWon is the actual Won of completed comparable periods, newest
	// first. Four are needed for a median.
	ComparableWon []int64
}

// AssessSufficiency answers whether the open pipeline supports the reference.
//
// call is the current manager call, or nil. It OUTRANKS history: a manager who
// has written a number down has said what this period is for, and a median of
// what happened before is the fallback for when nobody has.
func AssessSufficiency(
	readings Readings, history ConversionHistory, call *int64,
) (Sufficiency, error) {
	basis, reference, ok := referenceLanding(history, call)
	if !ok {
		return Sufficiency{Absent: AbsenceInsufficientBasis}, nil
	}
	if history.ClosedDeals < minimumClosedDeals {
		return Sufficiency{Absent: AbsenceInsufficientHistory}, nil
	}
	rate := history.BlendedWonPerReached
	if rate <= 0 {
		// No rate is not a rate of zero: dividing by it would demand infinite
		// pipeline, and a surface drawing that would tell a manager to give up.
		return Sufficiency{Absent: AbsenceInsufficientHistory}, nil
	}
	if rate > 1 {
		return Sufficiency{}, fmt.Errorf(
			"forecasting: a conversion rate of %v is more deals won than reached the stage", rate)
	}

	remaining := reference - readings.WonMinor
	if remaining < 0 {
		// Already past the reference. No more pipeline is needed, and a
		// negative requirement would print as a coverage figure nobody can read.
		remaining = 0
	}
	// Bounded before the conversion. A float64 above the int64 ceiling converts
	// to an IMPLEMENTATION-DEFINED value in Go — in practice MinInt64 — so an
	// unchecked cast turns "more pipeline than exists" into a negative
	// requirement, which then reads as a fully covered book.
	wanted := math.Ceil(float64(remaining) / rate)
	if wanted > float64(math.MaxInt64) {
		return Sufficiency{}, fmt.Errorf(
			"forecasting: a reference of %d at a conversion rate of %v needs more pipeline "+
				"than money can represent: %w", reference, rate, ErrReadingOutOfRange)
	}
	needed := int64(wanted)
	out := Sufficiency{
		Basis: basis, ReferenceLandingMinor: reference,
		RemainingToSupportMinor: remaining,
		NeededOpenMinor:         needed,
		CurrentOpenMinor:        readings.OpenMinor,
	}
	if needed == 0 {
		// Nothing needed is covered, whatever is open. Reported as full rather
		// than as a division by zero, and not as an unbounded ratio: a manager
		// reading "∞% covered" learns nothing they can act on.
		out.CoverageBp = fullCoverageBp
	} else {
		// Basis points, so a caller renders the ratio and this package chooses
		// no rounding for them. Both operands are already minor units in one
		// base currency, so nothing here converts between scales.
		//
		// Scaled before dividing, so the multiply is what can wrap: a book of
		// open pipeline above roughly 9.2e14 minor units overflows, and a
		// wrapped coverage reads NEGATIVE — which draws as a book with no
		// pipeline at all, the opposite of what such a number means.
		coverage, err := coverageBp(readings.OpenMinor, needed)
		if err != nil {
			return Sufficiency{}, err
		}
		out.CoverageBp = coverage
	}
	return out, nil
}

// fullCoverageBp is a pipeline that meets its requirement exactly: 100%.
const fullCoverageBp int64 = 10_000 // money-scale-exempt: basis points, not a currency scale

// coverageBp is open over needed, in basis points, refusing an overflow.
//
// The scaling multiply is the operation that can wrap, so it is checked rather
// than reordered: dividing first would throw away the precision the basis
// points exist to keep.
func coverageBp(open, needed int64) (int64, error) {
	if open > math.MaxInt64/fullCoverageBp {
		return 0, fmt.Errorf(
			"forecasting: an open pipeline of %d is too large to express as a coverage "+
				"ratio: %w", open, ErrReadingOutOfRange)
	}
	return open * fullCoverageBp / needed, nil
}

// referenceLanding picks what the pipeline is measured against.
//
// The call first, because it is somebody's stated intention for THIS period,
// and the median is what the business did when nobody stated one. Neither is a
// target: both are named on the surface so a reader can disagree with the
// basis rather than with the arithmetic.
func referenceLanding(history ConversionHistory, call *int64) (SufficiencyBasis, int64, bool) {
	if call != nil {
		return BasisManagerCall, *call, true
	}
	if len(history.ComparableWon) < comparablePeriods {
		return "", 0, false
	}
	// The most recent four, because a business four years ago is a different
	// business. Sorted on a copy: the caller's slice is theirs, and reordering
	// it would change what a second reader sees.
	recent := append([]int64{}, history.ComparableWon[:comparablePeriods]...)
	sort.Slice(recent, func(i, j int) bool { return recent[i] < recent[j] })
	// The mean of the middle two, which is what a median of an even count is.
	// Integer division truncates toward zero and both values are non-negative
	// money, so the result is at most one minor unit low — and biasing the
	// reference DOWN understates what is needed by less than a cent.
	//
	// Halved BEFORE adding, so two large historical totals cannot wrap into a
	// negative reference — which would report a period as needing no pipeline
	// at all. The two halves' remainders are added back separately, so the
	// answer is the same one the direct sum gives.
	low, high := recent[1], recent[2]
	median := low/2 + high/2 + (low%2+high%2)/2
	return BasisHistoricalMedian, median, true
}

// StageConversion is one stage's own win rate, or the blended one where too few
// deals have ever reached it.
//
// Its own function because the fallback is the part worth naming: a stage two
// deals have entered has no rate, and using it would let one deal's outcome set
// the pipeline a whole team is told to build.
func StageConversion(arrivals, won int, blended float64) float64 {
	if arrivals < minimumStageArrivals {
		return blended
	}
	if arrivals == 0 {
		return blended
	}
	return float64(won) / float64(arrivals)
}
