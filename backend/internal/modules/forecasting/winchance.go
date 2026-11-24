// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

// The one win chance of a deal. Every surface that shows or weights it asks
// ResolveWinChance. These are the forecast, the deal page, lists, reports,
// briefs and tools. So a deal has one number and one explanation.

import (
	"maps"
	"math"

	"github.com/margince/margince/backend/internal/shared/kernel/winprobability"
)

// QuietSampleFloor is how many settled deals, both with and without the quiet
// signal, the installation needs before its own quiet effect replaces the fixed
// halving.
const QuietSampleFloor = PriorStrength

// ChanceModel is what an installation's own closed deals say about a stage's
// chance and about each risk signal, read once per forecast.
type ChanceModel struct {
	// SelfCorrecting is false while the installation switched learning off:
	// every chance then stays the typed one.
	SelfCorrecting bool
	// Stages holds each open stage's recorded evidence, keyed by stage id.
	Stages map[string]StageEvidence
	// Effects holds each risk signal's odds ratio; a signal missing is 1.
	Effects map[string]float64
	// Samples counts the settled frozen deals behind each signal's effect.
	Samples map[string]RiskSample
	// QuietSignal is the code of the signal whose learned effect replaces the
	// fixed halving.
	QuietSignal string
}

// RiskSample counts the settled frozen deals that carried a signal and those
// that did not. An effect is a contrast between the two, so it needs both.
type RiskSample struct {
	Present, Absent int
}

// ChanceInput is one deal as the resolver reads it.
type ChanceInput struct {
	StageID string
	// Typed is the stage's typed chance; Own is the deal's own probability and
	// is read only where Source is not SourceStage.
	Typed  int
	Own    int
	Source ProbabilitySource
	// Signals are the deal's assessed risk signals; nil means not assessed.
	Signals []Signal
}

// ResolveWinChance is the single decision behind a deal's win chance. A
// probability set on the deal stands as it is. Otherwise the stage's typed
// chance is moved by the stage's recorded outcomes and by the strongest risk
// signal's effect. The quiet effect is the installation's own, once
// QuietSampleFloor settled deals back it, and the halving below that.
func ResolveWinChance(model ChanceModel, in ChanceInput) winprobability.Chance {
	out := winprobability.Chance{Source: string(in.Source), Typed: in.Typed, SelfCorrecting: model.SelfCorrecting}
	if in.Source != SourceStage {
		out.Percent, out.InUse = in.Own, in.Own
		return out
	}
	out.InUse = in.Typed
	if model.SelfCorrecting {
		evidence := model.Stages[in.StageID]
		out.InUse = ChanceInUse(in.Typed, evidence)
		out.EvidenceN = new(evidence.Resolved)
		out.WonShare = int(math.Round(evidence.WonShare * 100))
		out.Effect, out.EffectSignal, out.EffectLearned, out.EffectSample = pickEffect(model, in.Signals, out.InUse)
		out.EffectDays = signalDays(in.Signals, out.EffectSignal)
	}
	out.Percent = AppliedProbability(out.InUse, out.Effect)
	return out
}

// pickEffect picks the effect a deal's signals apply. The quiet signal's
// effect is the learned one only with QuietSampleFloor settled deals behind
// it. With fewer, it is the halving of the chance in use.
func pickEffect(model ChanceModel, signals []Signal, inUse int) (effect *float64, signal string, learned bool, sample int) {
	effects := make(map[string]float64, len(model.Effects)+1)
	maps.Copy(effects, model.Effects)
	codes := SignalCodes(signals)
	quietFallback := false
	for _, code := range codes {
		if code != model.QuietSignal || model.Samples[code].Present >= QuietSampleFloor && model.Samples[code].Absent >= QuietSampleFloor {
			continue
		}
		if half := halvingEffect(inUse); half != nil {
			effects[code], quietFallback = *half, true
		} else {
			delete(effects, code)
		}
	}
	effect = StrongestEffect(codes, effects)
	if effect == nil {
		return nil, "", false, 0
	}
	for _, code := range codes {
		if e, ok := effects[code]; ok && e == *effect {
			signal = code
			break
		}
	}
	return effect, signal, !quietFallback || signal != model.QuietSignal, model.Samples[signal].Present
}

// halvingEffect is the odds ratio that takes a chance to half of itself. A
// quiet deal falls back to it while too few closed deals have shown what quiet
// does. Nil where halving moves nothing.
func halvingEffect(chance int) *float64 {
	if chance < 2 || chance >= 100 {
		return nil
	}
	half := float64(chance / 2)
	c := float64(chance)
	e := (half / (100 - half)) / (c / (100 - c))
	return &e
}

// signalDays is how long the named signal has held on a deal, nil where it
// has no length or the deal does not carry it.
func signalDays(signals []Signal, code string) *int {
	for _, s := range signals {
		if s.Code == code {
			return s.Days
		}
	}
	return nil
}
