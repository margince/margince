// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

// Where the period lands.
//
// The claim that costs most if it is wrong is the SHAPE of the sum: commit and
// weighted are remaining measures and a manager call is a total. Adding Won to
// a call would report money twice, and forgetting to add it to the other two
// would report a quarter as though nothing had closed yet.

import (
	"math"
	"testing"
)

func measured(won, evidence, weighted int64) Readings {
	return Readings{WonMinor: won, EvidenceMinor: evidence, WeightedMinor: weighted}
}

func TestCommitLandingIsWonPlusRemainingCommit(t *testing.T) {
	t.Parallel()
	got, err := ProjectLanding(measured(400_00, 250_00, 180_00), MeasureCommitEvidence, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.AmountMinor != 650_00 {
		t.Fatalf("landing = %d, want 65000 — Won plus the commit still to come", got.AmountMinor)
	}
	// The two halves travel, because the reconciliation line prints them and a
	// reader who cannot see the split cannot check the sum.
	if got.WonMinor != 400_00 || got.RemainingMinor != 250_00 {
		t.Fatalf("the split reads %d + %d, want 40000 + 25000", got.WonMinor, got.RemainingMinor)
	}
	if got.Caveat != "" {
		t.Fatalf("a plain projection carried the caveat %q", got.Caveat)
	}
}

func TestWeightedLandingIsWonPlusRemainingWeighted(t *testing.T) {
	t.Parallel()
	got, err := ProjectLanding(measured(400_00, 250_00, 180_00), MeasureWeighted, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.AmountMinor != 580_00 {
		t.Fatalf("landing = %d, want 58000 — Won plus the weighted pipeline", got.AmountMinor)
	}
	// The measure a reader is shown is the one that was used, so a label can
	// never describe a different arithmetic.
	if got.Measure != MeasureWeighted {
		t.Fatalf("the projection reports measure %q", got.Measure)
	}
}

// A manager call is a TOTAL, and adding Won to it would report the money
// already banked twice — a quarter with 400 won and a 900 call would read 1300,
// which is not a number anybody said.
func TestAManagerCallIsATotalLandingAndNeverAddsWonAgain(t *testing.T) {
	t.Parallel()
	call := int64(900_00)
	got, err := ProjectLanding(measured(400_00, 250_00, 180_00), MeasureManagerCall, &call)
	if err != nil {
		t.Fatal(err)
	}
	if got.AmountMinor != 900_00 {
		t.Fatalf("landing = %d, want exactly the call of 90000", got.AmountMinor)
	}
	if got.RemainingMinor != 0 {
		t.Fatalf("a call carried a remaining half of %d; it is one authored total, "+
			"and a split invites a reader to add it back", got.RemainingMinor)
	}
}

// A call below the money already won is reported, not corrected. The call is
// somebody's stated belief and this code does not overrule it — but a landing
// under what is already banked is a number nobody should read past.
func TestAManagerCallBelowWonIsFlagged(t *testing.T) {
	t.Parallel()
	call := int64(300_00)
	got, err := ProjectLanding(measured(400_00, 250_00, 180_00), MeasureManagerCall, &call)
	if err != nil {
		t.Fatal(err)
	}
	if got.AmountMinor != 300_00 {
		t.Fatalf("the call was rewritten to %d; it is the author's number", got.AmountMinor)
	}
	if got.Caveat != CaveatCallBelowActual {
		t.Fatalf("caveat = %q, want %q", got.Caveat, CaveatCallBelowActual)
	}
}

// A manager-call installation with no current call falls back to commit AND
// says so. Falling back silently shows a manager a number they did not ask for
// under the label of the one they did.
func TestAManagerCallMeasureWithNoCallFallsBackToCommitAndSaysSo(t *testing.T) {
	t.Parallel()
	got, err := ProjectLanding(measured(400_00, 250_00, 180_00), MeasureManagerCall, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.AmountMinor != 650_00 {
		t.Fatalf("landing = %d, want the commit projection of 65000", got.AmountMinor)
	}
	if got.Measure != MeasureCommitEvidence {
		t.Fatalf("measure = %q, want the one actually used", got.Measure)
	}
	if got.Caveat != CaveatCallAbsent {
		t.Fatalf("caveat = %q, want %q", got.Caveat, CaveatCallAbsent)
	}
}

// A call is consulted ONLY where the setting asks for one. Preferring an
// authored number under a commit or weighted setting would make the setting a
// lie: an installation that chose the weighted pipeline gets the weighted
// pipeline, whatever anybody has written down.
func TestACallIsIgnoredUnderTheOtherTwoMeasures(t *testing.T) {
	t.Parallel()
	call := int64(999_00)
	for _, measure := range []ForwardMeasure{MeasureCommitEvidence, MeasureWeighted} {
		got, err := ProjectLanding(measured(400_00, 250_00, 180_00), measure, &call)
		if err != nil {
			t.Fatal(err)
		}
		if got.AmountMinor == 999_00 {
			t.Fatalf("%s took the manager's call; the setting names a different question", measure)
		}
	}
}

// An unknown measure is refused rather than defaulted. A default here would be
// one arm of a switch silently answering for another, which is the shape that
// reported a week as a quarter one file over.
func TestAnUnknownForwardMeasureIsRefused(t *testing.T) {
	t.Parallel()
	if _, err := ProjectLanding(measured(1, 1, 1), ForwardMeasure("best_case"), nil); err == nil {
		t.Fatal("an unrecognised measure produced a landing, which is some other measure's number")
	}
}

// A landing that wrapped would still look like money, and would disagree with
// the readings it claims to be the sum of. Refused instead.
func TestALandingThatCannotBeRepresentedIsRefused(t *testing.T) {
	t.Parallel()
	readings := measured(math.MaxInt64-10, 1_000, 0)

	if _, err := ProjectLanding(readings, MeasureCommitEvidence, nil); err == nil {
		t.Fatal("a landing past the representable range answered instead of refusing — " +
			"a wrapped total reads as a negative quarter and still looks like money")
	}
}
