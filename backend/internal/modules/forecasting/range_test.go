// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// openRow is an open in-period deal worth base at chance, in stage. A non-nil
// evidence gives its chance a posterior.
func openRow(id, stage string, base int64, chance int, evidence *int) Contribution {
	return Contribution{
		DealID: id, StageID: stage, BaseMinor: &base, InOpen: true,
		StageProbability: chance, EvidenceN: evidence,
	}
}

// drawn is the range of rows, failing the test when none was drawn.
func drawn(t *testing.T, rows []Contribution) Range {
	t.Helper()
	out, ok := ForecastRange(rows)
	if !ok {
		t.Fatalf("no range was drawn over %d rows", len(rows))
	}
	return out
}

func wonRow(id string, base int64) Contribution {
	return Contribution{DealID: id, BaseMinor: &base, InWon: true}
}

// pipelineRows is n open deals of rising amounts in one stage.
func pipelineRows(n, chance int, evidence *int) []Contribution {
	rows := make([]Contribution, 0, n)
	for i := range n {
		rows = append(rows, openRow(fmt.Sprintf("deal-%03d", i), "stage-a", int64(1000+100*i), chance, evidence))
	}
	return rows
}

func TestARangeIsTheSameForTheSameForecast(t *testing.T) {
	t.Parallel()
	evidence := 12
	rows := append(pipelineRows(30, 40, &evidence), wonRow("won-1", 5000))
	first, second := drawn(t, rows), drawn(t, slices.Clone(rows))
	if first != second {
		t.Fatalf("one forecast drew two ranges: %+v and %+v", first, second)
	}
	// Pinned: a change to these numbers is a change to the range rules, which
	// bumps RangeVersion.
	want := Range{LowMinor: 22800, HighMinor: 46400, Runs: 4000}
	if first != want {
		t.Fatalf("range = %+v, want %+v", first, want)
	}
}

func TestTheOrderOfTheDealsDoesNotMoveTheRange(t *testing.T) {
	t.Parallel()
	evidence := 3
	rows := pipelineRows(25, 35, &evidence)
	reversed := slices.Clone(rows)
	slices.Reverse(reversed)
	if got, want := drawn(t, reversed), drawn(t, rows); got != want {
		t.Fatalf("reversed deals drew %+v, in order %+v", got, want)
	}
}

func TestCertainDealsGiveARangeOfOnePoint(t *testing.T) {
	t.Parallel()
	evidence := 40
	for _, n := range []int{3, 20} {
		rows := append(pipelineRows(n, 100, &evidence), wonRow("won-1", 700))
		sum := int64(700)
		for _, row := range rows[:n] {
			sum += *row.BaseMinor
		}
		got := drawn(t, rows)
		if got.LowMinor != sum || got.HighMinor != sum {
			t.Fatalf("%d deals at 100%%: range %+v, want both ends %d", n, got, sum)
		}
	}
}

func TestOneLargeDealAtEvenOddsSitsBetweenTheEnds(t *testing.T) {
	t.Parallel()
	for _, small := range []int{3, 20} {
		rows := []Contribution{openRow("big", "stage-b", 1_000_000, 50, nil)}
		for i := range small {
			rows = append(rows, openRow(fmt.Sprintf("small-%02d", i), "stage-c", 100, 50, nil))
		}
		got := drawn(t, rows)
		if got.LowMinor >= 1_000_000 {
			t.Fatalf("%d small deals: the low end %d counts the large deal", small, got.LowMinor)
		}
		if got.HighMinor < 1_000_000 {
			t.Fatalf("%d small deals: the high end %d leaves the large deal out", small, got.HighMinor)
		}
	}
}

// cutoverRows is n open deals spread over three stages at rising chances.
func cutoverRows(n int, evidence *int) []Contribution {
	rows := make([]Contribution, 0, n)
	for i := range n {
		rows = append(rows, openRow(fmt.Sprintf("deal-%02d", i), fmt.Sprintf("stage-%d", i%3),
			int64(2000+500*i), 20+5*i, evidence))
	}
	return rows
}

func TestEnumeratingAndSimulatingAgreeAroundTheCutover(t *testing.T) {
	t.Parallel()
	for _, n := range []int{12, 13} {
		rows := cutoverRows(n, nil)
		_, stages, _ := rangeInputs(rows)
		won := int64(0)
		exact, simulated := exactRange(stages), simulatedRange(stages, rangeRand(won, stages))
		// One deal's worth: the finishes are a step function of whole deals.
		const tolerance = 4500
		if diff := exact.LowMinor - simulated.LowMinor; diff > tolerance || diff < -tolerance {
			t.Errorf("%d deals: exact low %d, simulated low %d", n, exact.LowMinor, simulated.LowMinor)
		}
		if diff := exact.HighMinor - simulated.HighMinor; diff > tolerance || diff < -tolerance {
			t.Errorf("%d deals: exact high %d, simulated high %d", n, exact.HighMinor, simulated.HighMinor)
		}
		if exact.Runs != 1<<n {
			t.Errorf("%d deals: the exact path counted %d outcomes, want %d", n, exact.Runs, 1<<n)
		}
	}
}

// Enumeration draws no posterior, so a forecast whose chances carry evidence is
// simulated at any size, and the range does not jump where enumeration stops.
func TestChancesWithEvidenceAreSimulatedOnBothSidesOfTheCutover(t *testing.T) {
	t.Parallel()
	evidence := 6
	for _, n := range []int{3, 12, 13} {
		got := drawn(t, cutoverRows(n, &evidence))
		if got.Runs%rangeBatchRuns != 0 {
			t.Errorf("%d deals with evidence: %d runs, want whole simulated batches", n, got.Runs)
		}
	}
	if got := drawn(t, cutoverRows(12, nil)); got.Runs != 1<<12 {
		t.Errorf("12 deals with no evidence: %d runs, want every one of the %d outcomes", got.Runs, 1<<12)
	}
}

// A deal with no chance is lost in every run, so a thirteenth one moves
// neither the method nor the seed.
func TestADealWithNoChanceDoesNotMoveTheRange(t *testing.T) {
	t.Parallel()
	evidence := 6
	for _, posterior := range []*int{nil, &evidence} {
		rows := cutoverRows(12, posterior)
		withLoser := append(slices.Clone(rows), openRow("deal-zz", "stage-0", 50_000, 0, posterior))
		if got, want := drawn(t, withLoser), drawn(t, rows); got != want {
			t.Errorf("evidence %v: a 13th deal at 0%% drew %+v, without it %+v", posterior != nil, got, want)
		}
	}
}

// Past values.MaxExactInteger a JSON number loses precision, so no range is drawn.
func TestARangeIsDrawnUpToTheLargestExactAmountAndNoFurther(t *testing.T) {
	t.Parallel()
	limit := int64(values.MaxExactInteger)
	atLimit := []Contribution{wonRow("won", limit-1000), openRow("open", "stage-a", 1000, 50, nil)}
	if got := drawn(t, atLimit); got.HighMinor != limit {
		t.Fatalf("at the limit the high end is %d, want %d", got.HighMinor, limit)
	}
	for name, rows := range map[string][]Contribution{
		"won":  {wonRow("won", limit-1000), wonRow("more", 1001)},
		"open": {openRow("a", "stage-a", limit-1000, 50, nil), openRow("b", "stage-a", 1001, 50, nil)},
		"wrap": {wonRow("won", math.MaxInt64), openRow("b", "stage-a", 1, 50, nil)},
	} {
		if got, ok := ForecastRange(rows); ok {
			t.Errorf("%s money one past the limit drew %+v, want no range", name, got)
		}
	}
}

func TestOneStageDrawSharedByItsDealsWidensTheRange(t *testing.T) {
	t.Parallel()
	evidence := 0
	shared := pipelineRows(40, 50, &evidence)
	independent := slices.Clone(shared)
	for i := range independent {
		independent[i].StageID = fmt.Sprintf("stage-%02d", i)
	}
	wide, narrow := drawn(t, shared), drawn(t, independent)
	if wide.HighMinor-wide.LowMinor <= narrow.HighMinor-narrow.LowMinor {
		t.Fatalf("a shared stage draw gave %+v, independent draws %+v; the shared one must be wider", wide, narrow)
	}
}

func TestAChanceWithNoEvidenceCountHasNoPosteriorSpread(t *testing.T) {
	t.Parallel()
	evidence := 0
	withPosterior, typedOnly := drawn(t, pipelineRows(40, 50, &evidence)), drawn(t, pipelineRows(40, 50, nil))
	if typedOnly.HighMinor-typedOnly.LowMinor >= withPosterior.HighMinor-withPosterior.LowMinor {
		t.Fatalf("typed chances drew %+v, chances with a posterior %+v; typed must be narrower", typedOnly, withPosterior)
	}
}

func TestARiskEffectLowersTheRange(t *testing.T) {
	t.Parallel()
	plain := pipelineRows(20, 60, nil)
	risky := slices.Clone(plain)
	effect := 0.2
	for i := range risky {
		risky[i].Effect = &effect
	}
	if got, base := drawn(t, risky), drawn(t, plain); got.HighMinor >= base.HighMinor {
		t.Fatalf("an effect of %.1f left the high end at %d, against %d without it", effect, got.HighMinor, base.HighMinor)
	}
}

func TestOnlyOpenAndWonMoneyEntersTheRange(t *testing.T) {
	t.Parallel()
	lost := int64(9000)
	rows := []Contribution{
		wonRow("won", 400),
		{DealID: "lost", BaseMinor: &lost, InLost: true},
		{DealID: "unpriced", InOpen: true, StageProbability: 100},
	}
	if got := drawn(t, rows); got.LowMinor != 400 || got.HighMinor != 400 {
		t.Fatalf("range = %+v, want only the 400 won", got)
	}
}
