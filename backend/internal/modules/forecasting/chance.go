// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

// The chance a forecast weights an open deal by, and the risk effect on it.
//
// Both start at today's behaviour and move only on this installation's own
// evidence. With none, the chance in use is the typed stage chance. Every risk
// effect is then 1, so no number moves before the company's outcomes do.

import (
	"cmp"
	"math"
	"slices"
)

// PriorStrength is how many resolved deals the typed chance counts as. Twenty
// recorded outcomes weigh as much as the typed value; two hundred mostly
// replace it. It is part of the definition, so changing it bumps
// DefinitionVersion.
const PriorStrength = 20

// EntryOutcome is what became of a deal after it entered a stage.
type EntryOutcome int

// The outcomes. EntryOpen is censored: the deal had not closed by the evidence
// cutoff, or left the pipeline first. It says how long the deal stayed open.
const (
	EntryOpen EntryOutcome = iota
	EntryWon
	EntryLost
)

// StageEntry is one deal's first recorded entry into one stage. Days runs from
// the entry to the close, or to the cutoff for an open entry.
type StageEntry struct {
	Days    float64
	Outcome EntryOutcome
}

// StageEvidence is what one stage's recorded entries say. Resolved counts the
// entries that closed won or lost; WonShare is the estimated share of them that
// end won, in [0, 1].
type StageEvidence struct {
	Resolved int
	WonShare float64
}

// EstimateStageEvidence is the Aalen–Johansen estimate of the eventual won
// share, with won and lost as competing outcomes and open entries censored.
// Counting only closed deals would favour the quick ones, which are mostly the
// quick losses.
func EstimateStageEvidence(entries []StageEntry) StageEvidence {
	ordered := slices.SortedFunc(slices.Values(entries), func(a, b StageEntry) int {
		return cmp.Compare(a.Days, b.Days)
	})
	atRisk := len(ordered)
	survival, won, lost := 1.0, 0.0, 0.0
	out := StageEvidence{}
	for i := 0; i < len(ordered); {
		var wins, losses, censored int
		j := i
		for ; j < len(ordered) && ordered[j].Days == ordered[i].Days; j++ {
			switch ordered[j].Outcome {
			case EntryWon:
				wins++
			case EntryLost:
				losses++
			default:
				censored++
			}
		}
		// Ties put the closes before the censoring, the usual convention: an
		// entry censored at a day was still at risk on that day.
		if closes := wins + losses; closes > 0 {
			won += survival * float64(wins) / float64(atRisk)
			lost += survival * float64(losses) / float64(atRisk)
			survival *= 1 - float64(closes)/float64(atRisk)
		}
		out.Resolved += wins + losses
		atRisk -= j - i
		i = j
	}
	// The share among the closes the estimate reaches. With every entry closed
	// it equals the won incidence; the rest is still open past the last close.
	if won+lost > 0 {
		out.WonShare = won / (won + lost)
	}
	return out
}

// ChanceInUse blends the typed chance with the stage's evidence, as a beta
// prior of strength PriorStrength centred on the typed value. Both are integer
// percents. No evidence returns the typed value unchanged.
func ChanceInUse(typed int, evidence StageEvidence) int {
	if evidence.Resolved <= 0 {
		return typed
	}
	n := float64(evidence.Resolved)
	blended := (PriorStrength*float64(typed)/100 + n*evidence.WonShare) / (PriorStrength + n)
	return clampPercent(int(math.Round(blended * 100)))
}

// RiskTally counts settled frozen deals by whether a signal was present when
// the forecast was frozen, and how the deal ended.
type RiskTally struct {
	PresentWon, PresentLost int
	AbsentWon, AbsentLost   int
}

// RiskEffect is the odds ratio of winning with the signal against without it,
// shrunk on the log scale toward 1 by PriorStrength. No deal with the signal
// gives 1. Half a deal is added to each cell so an empty cell gives a
// finite ratio.
func RiskEffect(t RiskTally) float64 {
	present := t.PresentWon + t.PresentLost
	if present == 0 || t.AbsentWon+t.AbsentLost == 0 {
		return 1
	}
	logOdds := math.Log((float64(t.PresentWon) + 0.5) * (float64(t.AbsentLost) + 0.5) /
		((float64(t.PresentLost) + 0.5) * (float64(t.AbsentWon) + 0.5)))
	weight := float64(present) / float64(present+PriorStrength)
	return math.Exp(weight * logOdds)
}

// StrongestEffect picks the one effect a deal's signals apply: the one
// furthest from 1. Signals overlap, so multiplying them would count one
// problem several times. Nil when no signal moves the odds.
func StrongestEffect(signals []string, effects map[string]float64) *float64 {
	var strongest *float64
	for _, signal := range signals {
		effect, ok := effects[signal]
		if !ok || effect == 1 || effect <= 0 {
			continue
		}
		if strongest == nil || math.Abs(math.Log(effect)) > math.Abs(math.Log(*strongest)) {
			strongest = &effect
		}
	}
	return strongest
}

// AppliedProbability is the integer percent a deal is weighted by: its chance
// in use with the risk effect applied to the odds. A nil or neutral effect
// returns the chance unchanged.
func AppliedProbability(chance int, effect *float64) int {
	if effect == nil || *effect == 1 || chance <= 0 || chance >= 100 {
		return chance
	}
	p := float64(chance) / 100
	adjusted := *effect * p / (1 - p + *effect*p)
	return clampPercent(int(math.Round(adjusted * 100)))
}

func clampPercent(v int) int {
	return min(max(v, 0), 100)
}
