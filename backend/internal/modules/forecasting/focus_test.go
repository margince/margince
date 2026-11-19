// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

var focusDay = time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)

// openDeal is an open priced deal weighted at weighted, expected to close
// closesIn days after focusDay.
func openDeal(id string, weighted int64, closesIn int) Contribution {
	return Contribution{
		DealID: id, InOpen: true, BaseMinor: new(weighted * 2), WeightedMinor: weighted,
		EffectiveClose: new(focusDay.AddDate(0, 0, closesIn)),
	}
}

func ranks(list FocusList) map[string]int {
	out := map[string]int{}
	for id, focus := range list.Deals {
		out[id] = focus.Rank
	}
	return out
}

func TestBehindTheGoalRanksByWeightedAmountRaisedForCloseDateRisk(t *testing.T) {
	t.Parallel()
	pushed := openDeal("b-pushed", 300, 10)
	one := 3
	pushed.Signals = []Signal{{Code: "close_pushed", Count: &one}}
	readings := Readings{Contributions: []Contribution{
		openDeal("a-big", 500, 10),
		pushed,
		openDeal("c-small", 100, 10),
	}}
	got := RankFocus(readings, 1_000, focusDay)
	if got.OnTarget {
		t.Fatal("a positive gap ranked as on target")
	}
	// One close-date risk doubles 300 to 600, past the 500 of the safe deal.
	want := map[string]int{"b-pushed": 1, "a-big": 2, "c-small": 3}
	if !reflect.DeepEqual(ranks(got), want) {
		t.Fatalf("ranks = %v, want %v", ranks(got), want)
	}
}

func TestOnTargetListsOnlyDealsWithACloseDateRiskMostRisksFirst(t *testing.T) {
	t.Parallel()
	passed := openDeal("passed-and-guessed", 100, -3)
	passed.CloseProvisional = true
	guessed := openDeal("guessed", 900, 20)
	guessed.CloseProvisional = true
	soon := openDeal("soon-no-step", 50, 2)
	soon.Signals = []Signal{{Code: "no_next_step", ClosesInDays: new(2)}}
	quiet := openDeal("quiet-only", 800, 20)
	quiet.Signals = []Signal{{Code: "quiet", Days: new(30)}}
	readings := Readings{Contributions: []Contribution{openDeal("safe", 5_000, 20), passed, guessed, soon, quiet}}

	got := RankFocus(readings, 0, focusDay)
	if !got.OnTarget {
		t.Fatal("a gap of zero is the goal met, so the list ranks risk")
	}
	// Two risks lead; the two single-risk deals go by weighted amount. A safe
	// deal, and one whose only signal says nothing about its date, are not risks
	// to the target.
	want := map[string]int{"passed-and-guessed": 1, "guessed": 2, "soon-no-step": 3}
	if !reflect.DeepEqual(ranks(got), want) {
		t.Fatalf("ranks = %v, want %v", ranks(got), want)
	}
}

func TestAFocusListNamesAtMostTwentyDeals(t *testing.T) {
	t.Parallel()
	var readings Readings
	for i := range FocusCap + 5 {
		readings.Contributions = append(readings.Contributions, openDeal(fmt.Sprintf("deal-%02d", i), int64(100+i), 10))
	}
	got := RankFocus(readings, 1, focusDay)
	if len(got.Deals) != FocusCap {
		t.Fatalf("listed %d deals, want the cap of %d", len(got.Deals), FocusCap)
	}
	// The cap keeps the top of the list: the five smallest deals fall off.
	for i := range 5 {
		if _, listed := got.Deals[fmt.Sprintf("deal-%02d", i)]; listed {
			t.Fatalf("deal-%02d is among the five smallest and was listed", i)
		}
	}
	if got.Deals["deal-24"].Rank != 1 {
		t.Fatalf("the largest deal ranks %d, want 1", got.Deals["deal-24"].Rank)
	}
}

func TestOnlyOpenPricedDealsWithWeightAreRanked(t *testing.T) {
	t.Parallel()
	won := openDeal("won", 900, 1)
	won.InWon = true
	closed := openDeal("not-open", 900, 1)
	closed.InOpen = false
	unpriced := openDeal("unpriced", 0, 1)
	unpriced.BaseMinor = nil
	weightless := openDeal("weightless", 0, 1)
	readings := Readings{Contributions: []Contribution{won, closed, unpriced, weightless, openDeal("open", 10, 1)}}
	if got := ranks(RankFocus(readings, 500, focusDay)); !reflect.DeepEqual(got, map[string]int{"open": 1}) {
		t.Fatalf("ranks = %v, want only the open priced deal", got)
	}
}

func TestEqualScoresRankByDealIDSoOneReadingGivesOneList(t *testing.T) {
	t.Parallel()
	readings := Readings{Contributions: []Contribution{openDeal("b", 100, 5), openDeal("a", 100, 5)}}
	if got := ranks(RankFocus(readings, 1, focusDay)); !reflect.DeepEqual(got, map[string]int{"a": 1, "b": 2}) {
		t.Fatalf("ranks = %v, want a before b", got)
	}
}

func TestAReasonHoldsTheStoredFactsAndNotTheScore(t *testing.T) {
	t.Parallel()
	deal := openDeal("late", 400, -4)
	deal.CloseProvisional = true
	deal.Signals = []Signal{{Code: "stuck", Days: new(40)}, {Code: "close_pushed", Count: new(3), ClosePassed: true}}
	got := RankFocus(Readings{Contributions: []Contribution{deal}}, 1, focusDay).Deals["late"].Reason
	want := FocusReason{ClosesInDays: new(-4), CloseProvisional: true, Signals: deal.Signals}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reason = %+v, want the deal's own facts %+v", got, want)
	}
}

func TestASnapshotRanksAgainstTheLandingItFreezes(t *testing.T) {
	t.Parallel()
	zone := time.FixedZone("UTC+7", 7*3600)
	in := NewSnapshot{
		Readings: Readings{Contributions: []Contribution{openDeal("risky", 100, -1)}},
		// 23:00 UTC is already the next local day, so the close is two days back.
		TakenAt: focusDay.Add(23 * time.Hour), Period: Period{Zone: zone},
	}
	if focusOf(in, Landing{Measure: MeasureWeighted, AmountMinor: 900}) != nil {
		t.Fatal("a snapshot with no goal ranked deals")
	}
	in.FocusGoal = new(int64(900))
	if focusOf(in, Landing{}) != nil {
		t.Fatal("a snapshot with no landing ranked deals")
	}
	list := focusOf(in, Landing{Measure: MeasureWeighted, AmountMinor: 900})
	if list == nil || !list.OnTarget {
		t.Fatalf("a landing equal to the goal reads %+v, want on target", list)
	}
	if days := *list.Deals["risky"].Reason.ClosesInDays; days != -2 {
		t.Fatalf("closes in %d days, want -2 counted from the local day", days)
	}
	if list = focusOf(in, Landing{Measure: MeasureWeighted, AmountMinor: 899}); list.OnTarget {
		t.Fatal("a landing one short of the goal read as on target")
	}
}
