// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/forecasting"
)

// The review stores its bars as one jsonb value with no CHECK on the
// elements, so this writer is what holds the horizon and bar vocabularies.
func TestEveryBridgeBarOfEveryHorizonIsFrozen(t *testing.T) {
	var moved []Movement
	for _, kind := range []forecasting.PeriodKind{forecasting.PeriodWeek, forecasting.PeriodMonth, forecasting.PeriodQuarter} {
		for _, bar := range BridgeBars() {
			moved = append(moved, Movement{PeriodKind: string(kind), Bar: bar, DeltaMinor: -wireExactMinor})
		}
	}
	bars, err := frozenBars(moved)
	if err != nil {
		t.Fatalf("the bridge's own bars were refused: %v", err)
	}
	if len(bars) != len(moved) {
		t.Fatalf("froze %d bars of %d", len(bars), len(moved))
	}
	for i, bar := range bars {
		want := frozenBar{PeriodKind: moved[i].PeriodKind, Bar: moved[i].Bar, DeltaMinor: moved[i].DeltaMinor}
		if bar != want {
			t.Errorf("bar %d froze as %+v, want %+v", i, bar, want)
		}
	}
}

func TestABarOutsideTheVocabulariesIsNotFrozen(t *testing.T) {
	refused := map[string]Movement{
		"a horizon nobody forecasts":     {PeriodKind: "year", Bar: BarWon, DeltaMinor: 1},
		"no horizon":                     {Bar: BarWon, DeltaMinor: 1},
		"a bar the bridge does not draw": {PeriodKind: "week", Bar: "renewed", DeltaMinor: 1},
		"an amount a reader cannot hold": {PeriodKind: "week", Bar: BarWon, DeltaMinor: wireExactMinor + 1},
		"a loss a reader cannot hold":    {PeriodKind: "week", Bar: BarLost, DeltaMinor: -wireExactMinor - 1},
		"no bar":                         {PeriodKind: "week", Bar: "", DeltaMinor: 1},
	}
	for name, move := range refused {
		if bars, err := frozenBars([]Movement{{PeriodKind: "week", Bar: BarCreated}, move}); err == nil {
			t.Errorf("%s was frozen as %+v, want it refused", name, bars)
		}
	}
	// A retired bar is read from old weeks and folded, and never written again.
	for _, bar := range retiredBars {
		if slices.Contains(BridgeBars(), bar) {
			t.Errorf("the retired bar %q is still one the writer freezes", bar)
		}
		if _, err := frozenBars([]Movement{{PeriodKind: "quarter", Bar: bar, DeltaMinor: 1}}); err == nil {
			t.Errorf("the retired bar %q was frozen, want it refused", bar)
		}
	}
}

func TestASecondBarForOneHorizonIsDropped(t *testing.T) {
	bars, err := frozenBars([]Movement{
		{PeriodKind: "week", Bar: BarCreated, DeltaMinor: 500},
		{PeriodKind: "month", Bar: BarCreated, DeltaMinor: 700},
		{PeriodKind: "week", Bar: BarCreated, DeltaMinor: 900},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []frozenBar{
		{PeriodKind: "week", Bar: BarCreated, DeltaMinor: 500},
		{PeriodKind: "month", Bar: BarCreated, DeltaMinor: 700},
	}
	if !slices.Equal(bars, want) {
		t.Fatalf("froze %+v, want the first bar of each horizon: %+v", bars, want)
	}
}
