// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

// The floor, tested as an attacker reads it.
//
// The interesting assertion is not "the small group was hidden" — that is the
// easy half and a naive implementation passes it. It is "the small group cannot
// be RECOVERED", which a naive implementation fails while looking correct.

import "testing"

func rows(counts ...int) []Row {
	out := make([]Row, len(counts))
	for i, c := range counts {
		out[i] = Row{Keys: []any{i}, Count: c, Values: []any{c * 1000}}
	}
	return out
}

func TestAGroupBelowTheFloorIsWithheld(t *testing.T) {
	t.Parallel()
	got, withheld := DefaultFloor.Apply(rows(20, 2, 30))
	if !withheld {
		t.Fatal("a group of two was reported alongside the rest")
	}
	if !got[1].Withheld || got[1].Values != nil {
		t.Errorf("the group of two still carries %v", got[1].Values)
	}
	// The row is still THERE, keys and all. Dropping it would make the answer's
	// row count a signal of its own, and a reader comparing two runs would
	// watch a group appear and vanish as it crossed the floor.
	if len(got) != 3 {
		t.Errorf("the answer has %d rows and the question had 3 groups", len(got))
	}
}

func TestAWithheldGroupCannotBeRecoveredBySubtraction(t *testing.T) {
	t.Parallel()
	// One group under the floor. Withholding only that one leaves it equal to
	// the total minus the other two, which a reader works out on a napkin.
	got, withheld := DefaultFloor.Apply(rows(20, 2, 30))
	if !withheld {
		t.Fatal("nothing was withheld")
	}

	shown := 0
	for _, row := range got {
		if !row.Withheld {
			shown++
		}
	}
	// At least two groups withheld, so the equation has two unknowns and the
	// reader has a sum rather than a value.
	if len(got)-shown < 2 {
		t.Errorf("%d of %d groups were withheld; with one unknown the reader subtracts and has it exactly",
			len(got)-shown, len(got))
	}
	// And the total goes with them, because total-minus-shown is the same
	// subtraction by another route.
	if TotalIsSafe(withheld) {
		t.Error("the total may still be reported, which is the subtraction with extra steps")
	}
}

func TestTheComplementTakenIsTheSmallestSoTheLeastIsLost(t *testing.T) {
	t.Parallel()
	// 2 is under the floor. 7 is the smallest that is not, so 7 is the one
	// that goes with it — losing the 40 would cost the reader far more.
	got, _ := DefaultFloor.Apply(rows(40, 2, 7))
	if !got[1].Withheld {
		t.Fatal("the group under the floor was reported")
	}
	if !got[2].Withheld {
		t.Error("the smallest remaining group was kept, so the withheld one is the total minus the rest")
	}
	if got[0].Withheld {
		t.Error("the largest group was withheld; the complement should cost the least information")
	}
}

func TestAnAnswerEntirelyAboveTheFloorIsReportedWhole(t *testing.T) {
	t.Parallel()
	// The mutation guard for the two tests above: a floor that withheld
	// something here would pass them while destroying every ordinary answer.
	got, withheld := DefaultFloor.Apply(rows(20, 30, 40))
	if withheld {
		t.Fatal("nothing was below the floor and something was withheld anyway")
	}
	for i, row := range got {
		if row.Withheld || row.Values == nil {
			t.Errorf("group %d was withheld from an answer entirely above the floor", i)
		}
	}
	if !TotalIsSafe(withheld) {
		t.Error("the total was withheld from an answer with nothing to protect")
	}
}

func TestBelowTheFloorNoMeasureButTheCountIsServed(t *testing.T) {
	t.Parallel()
	// A median over four rows is close to one row's value, and a sum over two
	// names both amounts to anybody who knows one.
	for _, fn := range []AggFn{Sum, Avg, Min, Max, CountDistinct} {
		if DefaultFloor.AllowsMeasure(fn, 2) {
			t.Errorf("%s is served over a group of two", fn)
		}
	}
	if !DefaultFloor.AllowsMeasure(CountAll, 2) {
		t.Error("the count is refused below the floor; it is the one measure the floor judges on")
	}
	// And above it, everything.
	for _, fn := range []AggFn{Sum, Avg, Min, Max, CountDistinct, CountAll} {
		if !DefaultFloor.AllowsMeasure(fn, 50) {
			t.Errorf("%s is refused over a group of fifty", fn)
		}
	}
}
