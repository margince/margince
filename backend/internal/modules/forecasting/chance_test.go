// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"math"
	"testing"
	"time"
)

func entries(won, lost, open int, days float64) []StageEntry {
	var out []StageEntry
	for range won {
		out = append(out, StageEntry{Days: days, Outcome: EntryWon})
	}
	for range lost {
		out = append(out, StageEntry{Days: days, Outcome: EntryLost})
	}
	for range open {
		out = append(out, StageEntry{Days: days, Outcome: EntryOpen})
	}
	return out
}

func TestTheChanceInUseMovesFromTheTypedValueByTheResolvedCount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		typed    int
		evidence StageEvidence
		want     int
	}{
		{"no evidence keeps the typed chance", 90, StageEvidence{}, 90},
		{"no evidence keeps a typed zero", 0, StageEvidence{}, 0},
		{"a won share with no resolved deal is ignored", 90, StageEvidence{WonShare: 0.1}, 90},
		{"five outcomes move it a fifth of the way", 90, StageEvidence{Resolved: 5, WonShare: 0.4}, 80},
		{"twenty outcomes weigh as much as the typed value", 90, StageEvidence{Resolved: 20, WonShare: 0.5}, 70},
		{"two hundred outcomes mostly replace it", 90, StageEvidence{Resolved: 200, WonShare: 0.55}, 58},
		{"evidence can raise a chance", 10, StageEvidence{Resolved: 20, WonShare: 0.9}, 50},
	}
	for _, c := range cases {
		if got := ChanceInUse(c.typed, c.evidence); got != c.want {
			t.Errorf("%s: ChanceInUse(%d, %+v) = %d, want %d", c.name, c.typed, c.evidence, got, c.want)
		}
	}
}

func TestStageEvidenceCountsOnlyClosedEntriesAndCensorsOpenOnes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		in       []StageEntry
		resolved int
		share    float64
	}{
		{"no entries", nil, 0, 0},
		{"only open entries resolve nothing", entries(0, 0, 4, 30), 0, 0},
		{"all closed is the plain share", entries(3, 1, 0, 10), 4, 0.75},
		{"open entries at the end leave the share of closes", append(entries(1, 1, 0, 10), entries(0, 0, 6, 40)...), 2, 0.5},
		// Two quick losses, then two open entries censored before the late
		// win. The plain share is 1/3; the estimate weighs the win by the
		// deals still open when it came, so it reads higher.
		{"censoring reweighs a late close", []StageEntry{
			{Days: 5, Outcome: EntryLost}, {Days: 6, Outcome: EntryLost},
			{Days: 10, Outcome: EntryOpen}, {Days: 11, Outcome: EntryOpen},
			{Days: 50, Outcome: EntryWon}, {Days: 60, Outcome: EntryOpen},
		}, 3, 0.5},
	}
	for _, c := range cases {
		got := EstimateStageEvidence(c.in)
		if got.Resolved != c.resolved || math.Abs(got.WonShare-c.share) > 1e-9 {
			t.Errorf("%s: got %+v, want resolved %d share %v", c.name, got, c.resolved, c.share)
		}
	}
}

func TestARiskEffectIsNeutralWithoutEvidenceAndShrunkWithIt(t *testing.T) {
	t.Parallel()
	if got := RiskEffect(RiskTally{}); got != 1 {
		t.Fatalf("no settled deal gave effect %v, want exactly 1", got)
	}
	if got := RiskEffect(RiskTally{AbsentWon: 10, AbsentLost: 10}); got != 1 {
		t.Fatalf("no deal with the signal gave effect %v, want exactly 1", got)
	}
	if got := RiskEffect(RiskTally{PresentWon: 1, PresentLost: 9}); got != 1 {
		t.Fatalf("nothing to compare against gave effect %v, want exactly 1", got)
	}
	few := RiskEffect(RiskTally{PresentWon: 1, PresentLost: 4, AbsentWon: 10, AbsentLost: 10})
	many := RiskEffect(RiskTally{PresentWon: 20, PresentLost: 80, AbsentWon: 100, AbsentLost: 100})
	if few >= 1 || many >= few {
		t.Fatalf("a signal that loses deals gave %v on 5 deals and %v on 100; want both below 1, the larger sample further", few, many)
	}
	if got := RiskEffect(RiskTally{PresentWon: 30, PresentLost: 10, AbsentWon: 10, AbsentLost: 30}); got <= 1 {
		t.Fatalf("a signal on deals that win gave %v, want above 1", got)
	}
}

func TestOnlyTheStrongestEffectApplies(t *testing.T) {
	t.Parallel()
	effects := map[string]float64{"quiet": 0.5, "stuck": 0.8, "no_next_step": 1}
	got := StrongestEffect([]string{"stuck", "quiet", "no_next_step"}, effects)
	if got == nil || *got != 0.5 {
		t.Fatalf("strongest = %v, want 0.5", got)
	}
	if got := StrongestEffect([]string{"no_next_step", "unknown"}, effects); got != nil {
		t.Fatalf("neutral and unknown signals applied %v, want none", *got)
	}
	up := map[string]float64{"quiet": 0.8, "stuck": 1.5}
	if got := StrongestEffect([]string{"quiet", "stuck"}, up); got == nil || *got != 1.5 {
		t.Fatalf("strongest = %v, want 1.5, which is further from 1 on the log scale", got)
	}
}

func TestTheAppliedProbabilityMovesTheOdds(t *testing.T) {
	t.Parallel()
	half, double := 0.5, 2.0
	cases := []struct {
		chance int
		effect *float64
		want   int
	}{
		{60, nil, 60},
		{60, new(1.0), 60},
		{50, &half, 33},
		{50, &double, 67},
		{0, &double, 0},
		{100, &half, 100},
	}
	for _, c := range cases {
		if got := AppliedProbability(c.chance, c.effect); got != c.want {
			t.Errorf("AppliedProbability(%d, %v) = %d, want %d", c.chance, c.effect, got, c.want)
		}
	}
}

func TestAWeightedReadingUsesTheChanceInUseWithTheRiskEffect(t *testing.T) {
	t.Parallel()
	half := 0.5
	deal := healthyDeal(t)
	deal.BaseMinor = new(int64(10_000))
	deal.StageProbability = 40
	deal.Effect = &half
	got, err := Compute(testPeriod(t), *day(t, time.May, 14), []Deal{deal})
	if err != nil {
		t.Fatal(err)
	}
	// 40% at odds halved is 25%.
	if got.WeightedMinor != 2_500 {
		t.Fatalf("weighted = %d, want 2500", got.WeightedMinor)
	}
}
