// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"strings"
	"testing"
)

const testQuiet = "quiet"

func quietSignal(days int) []Signal {
	return []Signal{{Code: testQuiet, Days: &days}}
}

func learnedModel(effect float64, sample int) ChanceModel {
	return ChanceModel{
		SelfCorrecting: true, QuietSignal: testQuiet,
		Stages:  map[string]StageEvidence{"s1": {Resolved: 40, WonShare: 0.45}},
		Effects: map[string]float64{testQuiet: effect},
		Samples: map[string]int{testQuiet: sample},
	}
}

func TestTheHalvingEffectTakesAnyChanceToHalfOfItself(t *testing.T) {
	t.Parallel()
	for chance := 2; chance < 100; chance++ {
		e := halvingEffect(chance)
		if e == nil {
			t.Fatalf("no halving effect for %d", chance)
		}
		if got, want := AppliedProbability(chance, e), chance/2; got != want {
			t.Fatalf("chance %d halves to %d, want %d", chance, got, want)
		}
	}
	for _, chance := range []int{0, 1, 100} {
		if halvingEffect(chance) != nil {
			t.Errorf("halving moves nothing at %d, want no effect", chance)
		}
	}
}

func TestAQuietDealFallsBackToHalvingBelowTheSampleFloor(t *testing.T) {
	t.Parallel()
	model := learnedModel(0.2, QuietSampleFloor-1)
	got := ResolveWinChance(model, ChanceInput{StageID: "s1", Typed: 60, Source: SourceStage, Signals: quietSignal(12)})
	// 40 closed deals, 45% won, typed 60: the chance in use is 50; halved it is 25.
	if got.InUse != 50 || got.Percent != 25 {
		t.Fatalf("in use %d, chance %d; want 50 halved to 25", got.InUse, got.Percent)
	}
	if got.EffectLearned || got.EffectSignal != testQuiet {
		t.Fatalf("effect learned = %v from %q; want the halving fallback for quiet", got.EffectLearned, got.EffectSignal)
	}
	if line := got.Line(); !strings.Contains(line, "quiet 12 days halves it (too few closed deals to learn from)") {
		t.Fatalf("explanation %q does not say the halving stands in", line)
	}
}

func TestAQuietDealUsesTheLearnedEffectOnceTheSampleFloorIsMet(t *testing.T) {
	t.Parallel()
	model := learnedModel(0.5, QuietSampleFloor)
	got := ResolveWinChance(model, ChanceInput{StageID: "s1", Typed: 60, Source: SourceStage, Signals: quietSignal(12)})
	if want := AppliedProbability(50, new(0.5)); got.Percent != want {
		t.Fatalf("chance %d, want %d from the learned odds ratio 0.5", got.Percent, want)
	}
	if !got.EffectLearned || got.EffectSample != QuietSampleFloor {
		t.Fatalf("learned = %v from %d deals, want learned from %d", got.EffectLearned, got.EffectSample, QuietSampleFloor)
	}
	want := "Stage typed 60% · 40 deals from here closed, 45% won · quiet 12 days lowers it (learned from 20 closed deals) · 33%"
	if line := got.Line(); line != want {
		t.Fatalf("explanation %q, want %q", line, want)
	}
}

func TestADealWithoutASignalKeepsTheChanceInUse(t *testing.T) {
	t.Parallel()
	got := ResolveWinChance(learnedModel(0.5, 50), ChanceInput{StageID: "s1", Typed: 60, Source: SourceStage, Signals: []Signal{}})
	if got.Percent != 50 || got.Effect != nil {
		t.Fatalf("chance %d with effect %v, want 50 and none", got.Percent, got.Effect)
	}
}

func TestAProbabilitySetOnTheDealStandsAsSet(t *testing.T) {
	t.Parallel()
	got := ResolveWinChance(learnedModel(0.5, 50), ChanceInput{
		StageID: "s1", Typed: 60, Own: 80, Source: SourceHuman, Signals: quietSignal(30),
	})
	if got.Percent != 80 || got.Effect != nil || got.EvidenceN != nil {
		t.Fatalf("chance %d, effect %v, evidence %v; want 80 as set, untouched", got.Percent, got.Effect, got.EvidenceN)
	}
	if got.Line() != "Set by a colleague at 80%" {
		t.Fatalf("explanation %q", got.Line())
	}
}

func TestSwitchingLearningOffKeepsTheTypedChanceAndSaysSo(t *testing.T) {
	t.Parallel()
	model := learnedModel(0.5, 50)
	model.SelfCorrecting = false
	got := ResolveWinChance(model, ChanceInput{StageID: "s1", Typed: 60, Source: SourceStage, Signals: quietSignal(30)})
	if got.Percent != 60 || got.Effect != nil {
		t.Fatalf("chance %d, effect %v; want the typed 60 and no effect", got.Percent, got.Effect)
	}
	if got.Line() != "Stage typed 60% · learning from closed deals is off" {
		t.Fatalf("explanation %q", got.Line())
	}
}

func TestBinAccuracyStatesAShareOnlyAboveTheFloor(t *testing.T) {
	t.Parallel()
	var settled []SettledChance
	for i := range 10 {
		settled = append(settled, SettledChance{Percent: 64, Won: i < 5})
	}
	settled = append(settled, SettledChance{Percent: 100, Won: true}, SettledChance{Percent: 3})
	got := BinAccuracy(settled, 10)
	if got.Deals != 12 || len(got.Bands) != 3 {
		t.Fatalf("deals %d in %d bands, want 12 in 3", got.Deals, len(got.Bands))
	}
	low, mid, top := got.Bands[0], got.Bands[1], got.Bands[2]
	if low.From != 0 || low.To != 9 || low.WonPercent != nil || low.Deals != 1 {
		t.Errorf("low band %+v, want 0 to 9 with one deal and no share", low)
	}
	if mid.From != 60 || mid.To != 69 || mid.MeanPercent == nil || *mid.MeanPercent != 64 || *mid.WonPercent != 50 {
		t.Errorf("mid band %+v, want 60 to 69, mean 64, 50 won", mid)
	}
	if top.To != 100 || top.WonPercent != nil {
		t.Errorf("top band %+v, want 90 to 100 with no share under the floor", top)
	}
}
