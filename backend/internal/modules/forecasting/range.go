// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

// The range a period may finish in, given the chances in use.
//
// It changes no other figure. It answers how far the finish could fall from
// the weighted figure if every deal is decided by the chance the forecast
// weights it by. It is no claim that the chances are right.

import (
	"cmp"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// RangeVersion names the range rules. It is frozen with every snapshot, and a
// change to how a range is drawn bumps it.
const RangeVersion = "forecast_range_v2"

// RangeQuantiles names how the low and high ends are read from the outcomes.
// Each end is the smallest finish whose share of outcomes at or below it
// reaches the quantile.
const RangeQuantiles = "nearest_rank"

const (
	rangeLowQuantile  = 0.1
	rangeHighQuantile = 0.9
	// Up to this many undecided deals with no posterior, every outcome is enumerated.
	rangeExactMaxDeals = 12
	rangeBatchRuns     = 1000
	rangeMaxRuns       = 20000
	// A simulation stops once both ends moved less than this share between
	// two batches.
	rangeSettledShare = 0.005
)

// Range is the central 80% of outcomes: at most one finish in ten falls below
// LowMinor, and at most one in ten above HighMinor. Runs counts the outcomes
// enumerated or simulated.
type Range struct {
	LowMinor  int64
	HighMinor int64
	Runs      int
}

// rangeDeal is one open deal the range decides.
type rangeDeal struct {
	baseMinor int64
	// percent is the chance in use, effect its risk effect (1 when none).
	percent int
	effect  float64
}

// rangeStage is the open deals that share one stage's chance. strength is the
// posterior's total weight, zero when the chance has no posterior.
type rangeStage struct {
	chance   float64
	strength float64
	deals    []rangeDeal
}

// ForecastRange draws the range from a reading's contributions. Won money is
// always in; each open deal with a base amount is won or lost by its chance.
// A stage's chance is drawn once per run from its beta posterior, so deals in
// one stage share the draw. A row with no evidence count has no posterior.
// False when the money could pass values.MaxExactInteger: then no range is drawn.
func ForecastRange(rows []Contribution) (Range, bool) {
	won, stages, ok := rangeInputs(rows)
	if !ok {
		return Range{}, false
	}
	open, spread := 0, false
	for _, stage := range stages {
		open += len(stage.deals)
		spread = spread || stage.strength > 0
	}
	// Enumeration knows no posterior, so it only stands in for a simulation
	// that would draw none.
	var out Range
	if !spread && open <= rangeExactMaxDeals {
		out = exactRange(stages)
	} else {
		out = simulatedRange(stages, rangeRand(won, stages))
	}
	out.LowMinor += won
	out.HighMinor += won
	return out, true
}

// rangeInputs sums the won money and groups the open deals by the stage chance
// they share, both in deal order. A deal at 0% or 100% is decided in every run,
// so it is left out or counted as won. False when won plus every open amount
// passes values.MaxExactInteger; below it no partial sum can overflow.
func rangeInputs(rows []Contribution) (int64, []rangeStage, bool) {
	ordered := slices.SortedFunc(slices.Values(rows), func(a, b Contribution) int {
		return strings.Compare(a.DealID, b.DealID)
	})
	var won, ceiling int64
	index := map[string]int{}
	var stages []rangeStage
	for _, row := range ordered {
		if row.BaseMinor == nil || (!row.InWon && !row.InOpen) {
			continue
		}
		var err error
		if ceiling, err = addMinor(ceiling, *row.BaseMinor); err != nil || ceiling > values.MaxExactInteger {
			return 0, nil, false
		}
		if row.InWon || row.StageProbability >= 100 {
			won += *row.BaseMinor
			continue
		}
		if row.StageProbability <= 0 {
			continue
		}
		key, strength := rangeStageKey(row)
		at, ok := index[key]
		if !ok {
			at = len(stages)
			index[key] = at
			stages = append(stages, rangeStage{chance: float64(row.StageProbability) / 100, strength: strength})
		}
		effect := 1.0
		if row.Effect != nil && *row.Effect > 0 {
			effect = *row.Effect
		}
		stages[at].deals = append(stages[at].deals, rangeDeal{
			baseMinor: *row.BaseMinor, percent: row.StageProbability, effect: effect,
		})
	}
	return won, stages, true
}

// rangeStageKey is the group a deal's chance is drawn in, and the posterior's
// strength. The key carries the chance and evidence count, so two rows that
// disagree on them never share a draw. A deal with no stage stands alone.
func rangeStageKey(row Contribution) (string, float64) {
	strength := 0.0
	evidence := "-"
	if row.EvidenceN != nil {
		strength = float64(PriorStrength + *row.EvidenceN)
		evidence = optionalInt(row.EvidenceN)
	}
	stage := row.StageID
	if stage == "" {
		stage = "deal:" + row.DealID
	}
	return stage + "|" + optionalInt(&row.StageProbability) + "|" + evidence, strength
}

// withEffect applies a risk effect to a chance on the odds, as
// AppliedProbability does, without rounding to a percent.
func withEffect(chance, effect float64) float64 {
	if effect == 1 || chance <= 0 || chance >= 1 {
		return chance
	}
	return effect * chance / (1 - chance + effect*chance)
}

// exactRange enumerates every outcome of the open deals, each decided by its
// chance in use with its effect. The chance is rounded to a percent as the
// weighted figure rounds it, and no posterior is drawn on this path.
func exactRange(stages []rangeStage) Range {
	var deals []rangeDeal
	var chances []float64
	for _, stage := range stages {
		for _, deal := range stage.deals {
			deals = append(deals, deal)
			chances = append(chances, float64(AppliedProbability(deal.percent, &deal.effect))/100)
		}
	}
	type outcome struct {
		totalMinor  int64
		probability float64
	}
	outcomes := make([]outcome, 0, 1<<len(deals))
	for mask := range 1 << len(deals) {
		next := outcome{probability: 1}
		for i, deal := range deals {
			if mask&(1<<i) != 0 {
				next.totalMinor += deal.baseMinor
				next.probability *= chances[i]
			} else {
				next.probability *= 1 - chances[i]
			}
		}
		outcomes = append(outcomes, next)
	}
	slices.SortFunc(outcomes, func(a, b outcome) int { return cmp.Compare(a.totalMinor, b.totalMinor) })
	quantile := func(q float64) int64 {
		// The tolerance keeps a share that sums to 0.9 in floating point from
		// reading as just below it.
		cumulative := 0.0
		for _, o := range outcomes {
			cumulative += o.probability
			if cumulative >= q-1e-9 {
				return o.totalMinor
			}
		}
		return outcomes[len(outcomes)-1].totalMinor
	}
	return Range{LowMinor: quantile(rangeLowQuantile), HighMinor: quantile(rangeHighQuantile), Runs: len(outcomes)}
}

// simulatedRange runs batches until both ends settle or the cap is reached.
func simulatedRange(stages []rangeStage, rng *rand.Rand) Range {
	totals := make([]int64, 0, rangeMaxRuns)
	var low, high int64
	for batch := 0; len(totals) < rangeMaxRuns; batch++ {
		for range rangeBatchRuns {
			totals = append(totals, simulateRun(stages, rng))
		}
		sorted := slices.Sorted(slices.Values(totals))
		nextLow, nextHigh := nearestRank(sorted, rangeLowQuantile), nearestRank(sorted, rangeHighQuantile)
		settled := batch > 0 && settledEnd(low, nextLow) && settledEnd(high, nextHigh)
		low, high = nextLow, nextHigh
		if settled {
			break
		}
	}
	return Range{LowMinor: low, HighMinor: high, Runs: len(totals)}
}

// simulateRun is one possible finish of the open deals.
func simulateRun(stages []rangeStage, rng *rand.Rand) int64 {
	var total int64
	for _, stage := range stages {
		chance := stage.chance
		if stage.strength > 0 && chance > 0 && chance < 1 {
			chance = betaDraw(rng, stage.strength*chance, stage.strength*(1-chance))
		}
		for _, deal := range stage.deals {
			if rng.Float64() < withEffect(chance, deal.effect) {
				total += deal.baseMinor
			}
		}
	}
	return total
}

// nearestRank is the value at rank ceil(q·n) of an ascending list.
func nearestRank(sorted []int64, q float64) int64 {
	rank := int(math.Ceil(q * float64(len(sorted))))
	return sorted[max(rank, 1)-1]
}

// settledEnd answers whether an end moved less than rangeSettledShare of its
// previous value, counted from one minor unit so a zero end can settle.
func settledEnd(previous, next int64) bool {
	scale := math.Max(math.Abs(float64(previous)), 1)
	return math.Abs(float64(next-previous)) < rangeSettledShare*scale
}

// rangeRand seeds the simulation from what it decides and the range rules. So
// one forecast always draws the same range, and a decided deal moves nothing.
func rangeRand(won int64, stages []rangeStage) *rand.Rand {
	material := fmt.Appendf(nil, "%s|%d\n", RangeVersion, won)
	for _, stage := range stages {
		material = fmt.Appendf(material, "%g|%g", stage.chance, stage.strength)
		for _, deal := range stage.deals {
			material = fmt.Appendf(material, "|%d:%d:%g", deal.baseMinor, deal.percent, deal.effect)
		}
		material = append(material, '\n')
	}
	digest := sha256.Sum256(material)
	return rand.New(rand.NewPCG(binary.BigEndian.Uint64(digest[:8]), binary.BigEndian.Uint64(digest[8:16]))) // #nosec G404 -- a reproducible simulation, not a secret
}

// betaDraw draws from Beta(a, b) as the share of two gamma draws.
func betaDraw(rng *rand.Rand, a, b float64) float64 {
	x := gammaDraw(rng, a)
	y := gammaDraw(rng, b)
	if x+y == 0 {
		return a / (a + b)
	}
	return x / (x + y)
}

// gammaDraw draws from Gamma(shape, 1) by Marsaglia and Tsang's method. A shape
// below one is drawn at shape+1 and scaled by U^(1/shape).
func gammaDraw(rng *rand.Rand, shape float64) float64 {
	if shape < 1 {
		return gammaDraw(rng, shape+1) * math.Pow(rng.Float64(), 1/shape)
	}
	d := shape - 1.0/3
	c := 1 / math.Sqrt(9*d)
	for {
		x := rng.NormFloat64()
		v := 1 + c*x
		if v <= 0 {
			continue
		}
		v = v * v * v
		u := rng.Float64()
		if math.Log(u) < 0.5*x*x+d-d*v+d*math.Log(v) {
			return d * v
		}
	}
}
