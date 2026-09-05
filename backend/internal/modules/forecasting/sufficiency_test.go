// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

// Whether there is enough pipeline.
//
// Every "coverage ratio" in this industry gets this wrong the same way: divide
// the pipeline by a target derived from the pipeline and the answer is always
// fine. The tests below hold the two things that stop that — the basis comes
// from OUTSIDE the current projection, and where no basis exists the answer is
// that there is none rather than a figure with a shrug attached.

import (
	"math"
	"testing"
)

func fourPeriods(won ...int64) ConversionHistory {
	return ConversionHistory{
		ClosedDeals: 50, BlendedWonPerReached: 0.25, ComparableWon: won,
	}
}

func TestTheReferenceIsTheManagerCallWhenPresent(t *testing.T) {
	t.Parallel()
	call := int64(1_000_00)
	got, err := AssessSufficiency(
		Readings{WonMinor: 200_00, OpenMinor: 500_00},
		fourPeriods(900_00, 800_00, 700_00, 600_00), &call)
	if err != nil {
		t.Fatal(err)
	}
	if got.Basis != BasisManagerCall {
		t.Fatalf("basis = %q, want the call somebody wrote down for THIS period", got.Basis)
	}
	if got.ReferenceLandingMinor != 1_000_00 {
		t.Fatalf("reference = %d, want the call of 100000", got.ReferenceLandingMinor)
	}
}

func TestWithoutACallTheReferenceIsTheMedianOfFourComparablePeriods(t *testing.T) {
	t.Parallel()
	got, err := AssessSufficiency(
		Readings{WonMinor: 200_00, OpenMinor: 500_00},
		fourPeriods(900_00, 800_00, 700_00, 600_00), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Basis != BasisHistoricalMedian {
		t.Fatalf("basis = %q, want the median of what actually happened", got.Basis)
	}
	// The middle two of 600, 700, 800, 900.
	if got.ReferenceLandingMinor != 750_00 {
		t.Fatalf("reference = %d, want the median 75000", got.ReferenceLandingMinor)
	}
}

// Neither a call nor four completed periods is NO ANSWER, not a guess. A figure
// here would be the circular one this whole file exists to avoid.
func TestWithoutCallOrFourPeriodsSufficiencyIsAbsent(t *testing.T) {
	t.Parallel()
	got, err := AssessSufficiency(
		Readings{WonMinor: 200_00, OpenMinor: 500_00},
		fourPeriods(900_00, 800_00, 700_00), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Absent != AbsenceInsufficientBasis {
		t.Fatalf("absent = %q, want %q", got.Absent, AbsenceInsufficientBasis)
	}
	if got.NeededOpenMinor != 0 || got.CoverageBp != 0 {
		t.Fatal("an unsupportable answer still carried figures, which a surface would draw")
	}
}

func TestFewerThanTwentyClosedDealsIsInsufficientHistory(t *testing.T) {
	t.Parallel()
	history := fourPeriods(900_00, 800_00, 700_00, 600_00)
	history.ClosedDeals = 19
	got, err := AssessSufficiency(Readings{WonMinor: 200_00, OpenMinor: 500_00}, history, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Absent != AbsenceInsufficientHistory {
		t.Fatalf("absent = %q, want %q", got.Absent, AbsenceInsufficientHistory)
	}
}

// A stage two deals have ever entered has no rate — it has two stories, and
// using them would let one outcome set the pipeline a whole team is told to
// build.
func TestAStageWithUnderFiveArrivalsUsesTheBlendedRate(t *testing.T) {
	t.Parallel()
	if got := StageConversion(2, 2, 0.25); got != 0.25 {
		t.Fatalf("a stage with two arrivals reported its own rate %v; two deals is not a rate", got)
	}
	// And a stage with enough arrivals uses its own, or the fallback would be
	// every stage's answer and the per-stage rate would be dead code.
	if got := StageConversion(10, 5, 0.25); got != 0.5 {
		t.Fatalf("a stage with ten arrivals reported %v, want its own 0.5", got)
	}
}

// The needed pipeline is measured against the REFERENCE after Won, never
// against the projection. Deriving it from the current landing is the circular
// answer: a pipeline is then always exactly sufficient for the pipeline.
func TestNeededPipelineUsesRemainingReferenceAfterWonNotProjectedLanding(t *testing.T) {
	t.Parallel()
	call := int64(1_000_00)
	got, err := AssessSufficiency(
		Readings{
			WonMinor: 200_00, OpenMinor: 500_00,
			// A big weighted number, which must not enter the requirement.
			WeightedMinor: 5_000_00, EvidenceMinor: 4_000_00,
		},
		fourPeriods(900_00, 800_00, 700_00, 600_00), &call)
	if err != nil {
		t.Fatal(err)
	}
	if got.RemainingToSupportMinor != 800_00 {
		t.Fatalf("remaining = %d, want the reference 100000 minus won 20000", got.RemainingToSupportMinor)
	}
	// 80000 at a quarter conversion needs 320000 of open pipeline.
	if got.NeededOpenMinor != 3_200_00 {
		t.Fatalf("needed = %d, want 320000", got.NeededOpenMinor)
	}
}

func TestCoverageIsOpenPipelineOverNeeded(t *testing.T) {
	t.Parallel()
	call := int64(1_000_00)
	got, err := AssessSufficiency(
		Readings{WonMinor: 200_00, OpenMinor: 1_600_00},
		fourPeriods(900_00, 800_00, 700_00, 600_00), &call)
	if err != nil {
		t.Fatal(err)
	}
	// 160000 open against 320000 needed is half.
	if got.CoverageBp != 5_000 {
		t.Fatalf("coverage = %d bp, want 5000 (half)", got.CoverageBp)
	}
}

// A period already past its reference needs no more pipeline. The subtraction
// floors at zero, because a negative requirement divides into a coverage figure
// nobody can read.
func TestAPeriodAlreadyPastItsReferenceNeedsNothingMore(t *testing.T) {
	t.Parallel()
	call := int64(100_00)
	got, err := AssessSufficiency(
		Readings{WonMinor: 500_00, OpenMinor: 0},
		fourPeriods(900_00, 800_00, 700_00, 600_00), &call)
	if err != nil {
		t.Fatal(err)
	}
	if got.RemainingToSupportMinor != 0 {
		t.Fatalf("remaining = %d over a reference already passed", got.RemainingToSupportMinor)
	}
	if got.CoverageBp != 10_000 {
		t.Fatalf("coverage = %d bp, want full — nothing is needed", got.CoverageBp)
	}
}

func TestAZeroConversionRateNeverDividesByZero(t *testing.T) {
	t.Parallel()
	history := fourPeriods(900_00, 800_00, 700_00, 600_00)
	history.BlendedWonPerReached = 0
	got, err := AssessSufficiency(Readings{WonMinor: 200_00, OpenMinor: 500_00}, history, nil)
	if err != nil {
		t.Fatal(err)
	}
	// No rate is not a rate of zero: dividing by it demands infinite pipeline,
	// and a surface drawing that tells a manager to give up.
	if got.Absent != AbsenceInsufficientHistory {
		t.Fatalf("absent = %q, want %q", got.Absent, AbsenceInsufficientHistory)
	}
}

// The caller's history slice is theirs. Sorting it in place would change what a
// second reader of the same slice sees — and the four periods arrive newest
// first, which is an order somebody else may rely on.
func TestTheReferenceReadDoesNotReorderTheCallersHistory(t *testing.T) {
	t.Parallel()
	history := fourPeriods(900_00, 600_00, 800_00, 700_00)
	before := append([]int64{}, history.ComparableWon...)
	if _, err := AssessSufficiency(Readings{OpenMinor: 1}, history, nil); err != nil {
		t.Fatal(err)
	}
	for i := range before {
		if history.ComparableWon[i] != before[i] {
			t.Fatalf("the history was reordered: %v became %v", before, history.ComparableWon)
		}
	}
}

// Each guard below is a wrap that reports the OPPOSITE of the truth. That is
// what makes them worth refusing rather than clamping: a negative requirement
// and a negative coverage both draw as a book that needs nothing.

// A median taken by summing the middle two wraps on large historical totals,
// and a negative reference reports a period as needing no pipeline at all.
func TestAMedianOfTwoVeryLargePeriodsDoesNotWrapNegative(t *testing.T) {
	t.Parallel()
	big := int64(math.MaxInt64 - 3)
	got, err := AssessSufficiency(
		Readings{WonMinor: 0, OpenMinor: 100},
		// Four periods, the middle two of which are the large pair: summing
		// them directly overflows before the halving.
		fourPeriods(big, big, big, big), nil)
	if err != nil {
		// Refusing is a correct answer here too; wrapping is not.
		return
	}
	if got.ReferenceLandingMinor < 0 {
		t.Fatalf("the median of four large periods is %d, a NEGATIVE reference — "+
			"which reports the period as needing no pipeline at all",
			got.ReferenceLandingMinor)
	}
}

// The scaling multiply for basis points wraps on a large open pipeline. The
// wrap does not go negative — it divides down to ZERO, which draws as a book
// holding no pipeline at all while the book is in fact enormous. Asserting on
// a negative would have missed this entirely.
func TestAnOpenPipelineTooLargeToScaleIsRefusedRatherThanWrapped(t *testing.T) {
	t.Parallel()
	call := int64(1_000_00)
	open := int64(math.MaxInt64 / 2)
	got, err := AssessSufficiency(
		Readings{WonMinor: 0, OpenMinor: open},
		fourPeriods(900_00, 800_00, 700_00, 600_00), &call)
	if err != nil {
		// Refusing is the answer this guard gives. Anything but a wrapped
		// figure is acceptable; a wrapped one is not.
		return
	}
	if got.CoverageBp <= 0 {
		t.Fatalf("an open pipeline of %d reports %d basis points of coverage — a "+
			"multiply that wrapped divides down to nothing, so the largest possible "+
			"book draws as an empty one", open, got.CoverageBp)
	}
}

// A requirement above the representable range is refused rather than converted.
//
// The unchecked conversion is IMPLEMENTATION-DEFINED in Go: it saturates to
// MaxInt64 on this platform rather than wrapping to MinInt64. So the test
// asserts the refusal itself — a figure that silently means "the largest
// number there is" is not an answer whichever end it lands on, and asserting
// on one platform's saturation value would pass on that platform for the wrong
// reason and fail on another.
func TestARequirementBeyondTheMoneyRangeIsRefused(t *testing.T) {
	t.Parallel()
	call := int64(math.MaxInt64)
	got, err := AssessSufficiency(
		Readings{WonMinor: 0, OpenMinor: 1_000},
		ConversionHistory{
			ClosedDeals: 50, BlendedWonPerReached: 0.0001,
			ComparableWon: []int64{1, 1, 1, 1},
		}, &call)
	if err == nil {
		t.Fatalf("a requirement of about 9.2e22 minor units answered %d instead of "+
			"refusing — the conversion cannot represent it, so whatever came back is "+
			"a number nobody can act on", got.NeededOpenMinor)
	}
}
