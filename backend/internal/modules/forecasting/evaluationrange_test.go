// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"testing"
)

// selfCorrectingCorpus is the evaluation corpus with every vintage frozen with
// self-correction on, a typed figure apart from the frozen weighted one, and a
// range.
func selfCorrectingCorpus(t *testing.T) evaluationCorpus {
	t.Helper()
	c := newEvaluationCorpus(t)
	on := true
	for i := range c.dataset.Vintages {
		v := &c.dataset.Vintages[i]
		v.SelfCorrecting = &on
		v.TypedWeighted = v.Weighted + 50_00
		low, high := v.WonMinor+100_00, v.WonMinor+300_00
		v.RangeLowMinor, v.RangeHighMinor = &low, &high
	}
	return c
}

func TestTypedAndSelfCorrectingWeightingAreScoredFromTheirOwnFigures(t *testing.T) {
	c := selfCorrectingCorpus(t)
	report := scoreCorpus(t, c.dataset, c.plan)
	// b2: won 200, frozen weighted 200, typed 250; 500 won.
	typed := findRow(t, report, MethodStageWeighting, WindowValidation, "0-29")
	if typed.Money.PredictedMinor != 450_00 || typed.Label != LabelBacktest {
		t.Fatalf("typed weighting of b2 is won 200 plus typed 250, recomputed: %+v %+v", typed, typed.Money)
	}
	learned := findRow(t, report, MethodSelfCorrecting, WindowValidation, "0-29")
	if learned.NotScored != "" || learned.Money.PredictedMinor != 400_00 || learned.Label != LabelObserved {
		t.Fatalf("self-correcting weighting of b2 is the frozen 200 plus won 200, as shown: %+v %+v", learned, learned.Money)
	}
}

func TestSelfCorrectingWeightingIsNotScoredOnAVintageFrozenWithItOff(t *testing.T) {
	c := newEvaluationCorpus(t)
	row := findRow(t, scoreCorpus(t, c.dataset, c.plan), MethodSelfCorrecting, WindowValidation, "0-29")
	if row.NotScored == "" || row.Money != nil {
		t.Fatalf("a vintage frozen with self-correction off was scored for it: %+v", row)
	}
	if typed := findRow(t, scoreCorpus(t, c.dataset, c.plan), MethodStageWeighting, WindowValidation, "0-29"); typed.Label != LabelObserved {
		t.Fatalf("typed weighting is what a snapshot showed with self-correction off, labelled %q", typed.Label)
	}
}

func TestTheRangeIsScoredByCoverageAndIntervalScore(t *testing.T) {
	c := selfCorrectingCorpus(t)
	report := scoreCorpus(t, c.dataset, c.plan)
	// b2 won 200, so its range is 300..500, and it closed on 500.
	inside := findRow(t, report, MethodRange, WindowValidation, "0-29").Interval
	if inside == nil || inside.Covered != 1 || inside.Coverage != "1.0000" || inside.MeanScoreMinor != 200_00 || inside.Nominal != "0.8000" {
		t.Fatalf("b2's range 300..500 holds its 500 and scores its width: %+v", inside)
	}
	// a1: 200..400 against 900, width 200 plus ten times 500. b1: 100..300
	// against 500, width 200 plus ten times 200. The mean is 3700.
	above := findRow(t, report, MethodRange, WindowTrain, "60+").Interval
	if above == nil || above.Covered != 0 || above.Coverage != "0.0000" || above.MeanScoreMinor != 3700_00 || above.MeanWidthMinor != 200_00 {
		t.Fatalf("both ranges fall short of what was won: %+v", above)
	}
	if row := findRow(t, report, MethodRange, WindowValidation, "0-29"); row.Money != nil || row.Calibration != nil {
		t.Fatalf("a range states no landing and no per-deal chance: %+v", row)
	}
}

func TestARangeIsNotScoredOnAVintageFrozenWithoutOne(t *testing.T) {
	c := selfCorrectingCorpus(t)
	for i := range c.dataset.Vintages {
		c.dataset.Vintages[i].RangeLowMinor = nil
	}
	row := findRow(t, scoreCorpus(t, c.dataset, c.plan), MethodRange, WindowValidation, "0-29")
	if row.NotScored == "" || row.Interval != nil {
		t.Fatalf("a vintage with no frozen range was scored: %+v", row)
	}
}

func TestSelfCorrectingCalibrationReadsTheChanceAfterTheRiskEffect(t *testing.T) {
	won, lost := true, false
	typed, inUse := 50, 50
	effect := 0.25
	tallies := TallyOpenDeals([]OpenDeal{
		{TypedProbability: &typed, StageProbability: &inUse, Effect: &effect, WonInHorizon: &lost},
		{TypedProbability: &typed, StageProbability: &inUse, WonInHorizon: &won},
	})
	// The effect turns 50% into 20%: 0.25·0.5 / (0.5 + 0.25·0.5).
	if tallies.Applied.Bins[2].Deals != 1 || tallies.Applied.Bins[5].Deals != 1 || tallies.Typed.Bins[5].Deals != 2 {
		t.Fatalf("typed bins %+v, applied bins %+v", tallies.Typed.Bins, tallies.Applied.Bins)
	}
	if tallies.Applied.BrierSum != 20*20+50*50 || tallies.Typed.BrierSum != 50*50+50*50 {
		t.Fatalf("Brier sums typed %d, applied %d", tallies.Typed.BrierSum, tallies.Applied.BrierSum)
	}
}

func TestAVintageIsGroupedByTheEvidenceItsDealsStoodOn(t *testing.T) {
	base := int64(1000_00)
	chance := 40
	deal := func(evidence *int) OpenDeal {
		return OpenDeal{BaseMinor: &base, StageProbability: &chance, EvidenceN: evidence}
	}
	none, few, many := 0, 5, PriorStrength
	cases := []struct {
		name  string
		deals []OpenDeal
		want  string
	}{
		{"no deals", nil, EvidenceNone},
		{"self-correction off", []OpenDeal{deal(nil), deal(nil)}, EvidenceNone},
		{"no recorded outcome", []OpenDeal{deal(&none)}, EvidenceNone},
		{"some outcomes", []OpenDeal{deal(&few), deal(&many), deal(&none)}, EvidenceThin},
		{"half at the prior strength", []OpenDeal{deal(&many), deal(&few)}, EvidenceSufficient},
	}
	for _, tc := range cases {
		weighted, got, err := summarizeOpenDeals(tc.deals)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("%s: %s, want %s", tc.name, got, tc.want)
		}
		if want := int64(400_00 * len(tc.deals)); weighted != want {
			t.Errorf("%s: typed weighted %d, want %d", tc.name, weighted, want)
		}
	}
}

func TestEveryMethodIsScoredPerEvidenceGroup(t *testing.T) {
	c := selfCorrectingCorpus(t)
	c.dataset.Vintages[3].Sufficiency = EvidenceSufficient
	report := scoreCorpus(t, c.dataset, c.plan)
	groups := map[string]int{}
	for _, row := range report.Rows {
		if row.Window == WindowValidation && row.Horizon == "0-29" {
			groups[row.Evidence]++
		}
	}
	if groups[EvidenceSufficient] != 5 || groups[EvidenceNone] != 0 {
		t.Fatalf("b2 alone stood on sufficient evidence; rows per group %v", groups)
	}
}
