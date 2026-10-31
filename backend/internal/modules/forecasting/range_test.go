// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"fmt"
	"slices"
	"testing"
)

// openRow is an open in-period deal worth base at chance, in stage. A non-nil
// evidence gives its chance a posterior.
func openRow(id, stage string, base int64, chance int, evidence *int) Contribution {
	return Contribution{
		DealID: id, StageID: stage, BaseMinor: &base, InOpen: true,
		StageProbability: chance, EvidenceN: evidence,
	}
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
	first, second := ForecastRange(rows), ForecastRange(slices.Clone(rows))
	if first != second {
		t.Fatalf("one forecast drew two ranges: %+v and %+v", first, second)
	}
	// Pinned: a change to these numbers is a change to the range rules, which
	// bumps RangeVersion.
	want := Range{LowMinor: 22700, HighMinor: 46400, Runs: 4000}
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
	if got, want := ForecastRange(reversed), ForecastRange(rows); got != want {
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
		got := ForecastRange(rows)
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
		got := ForecastRange(rows)
		if got.LowMinor >= 1_000_000 {
			t.Fatalf("%d small deals: the low end %d counts the large deal", small, got.LowMinor)
		}
		if got.HighMinor < 1_000_000 {
			t.Fatalf("%d small deals: the high end %d leaves the large deal out", small, got.HighMinor)
		}
	}
}

func TestEnumeratingAndSimulatingAgreeAroundTheCutover(t *testing.T) {
	t.Parallel()
	for _, n := range []int{12, 13} {
		rows := make([]Contribution, 0, n)
		for i := range n {
			rows = append(rows, openRow(fmt.Sprintf("deal-%02d", i), fmt.Sprintf("stage-%d", i%3),
				int64(2000+500*i), 20+5*i, nil))
		}
		_, stages := rangeInputs(rows)
		exact, simulated := exactRange(stages), simulatedRange(stages, rangeRand(rows))
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

func TestOneStageDrawSharedByItsDealsWidensTheRange(t *testing.T) {
	t.Parallel()
	evidence := 0
	shared := pipelineRows(40, 50, &evidence)
	independent := slices.Clone(shared)
	for i := range independent {
		independent[i].StageID = fmt.Sprintf("stage-%02d", i)
	}
	wide, narrow := ForecastRange(shared), ForecastRange(independent)
	if wide.HighMinor-wide.LowMinor <= narrow.HighMinor-narrow.LowMinor {
		t.Fatalf("a shared stage draw gave %+v, independent draws %+v; the shared one must be wider", wide, narrow)
	}
}

func TestAChanceWithNoEvidenceCountHasNoPosteriorSpread(t *testing.T) {
	t.Parallel()
	evidence := 0
	withPosterior, typedOnly := ForecastRange(pipelineRows(40, 50, &evidence)), ForecastRange(pipelineRows(40, 50, nil))
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
	if got, base := ForecastRange(risky), ForecastRange(plain); got.HighMinor >= base.HighMinor {
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
	if got := ForecastRange(rows); got.LowMinor != 400 || got.HighMinor != 400 {
		t.Fatalf("range = %+v, want only the 400 won", got)
	}
}
