// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"math/big"
	"strconv"
)

// decimalPlaces is how many places a ratio in a report is written to.
const decimalPlaces = 4

// MoneyError is how far a method's landings were from what was won, summed
// over a group of vintages in minor units.
type MoneyError struct {
	ActualMinor    int64 `json:"actual_minor"`
	PredictedMinor int64 `json:"predicted_minor"`
	AbsErrorMinor  int64 `json:"abs_error_minor"`
	// SignedErrorMinor is predicted minus actual: above zero over-forecast.
	SignedErrorMinor  int64 `json:"signed_error_minor"`
	MeanAbsErrorMinor int64 `json:"mean_abs_error_minor"`
	// AbsErrorPerActual is the absolute error over the money won. Empty when
	// nothing was won, because a share of zero is not a number.
	AbsErrorPerActual string `json:"abs_error_per_actual,omitempty"`
	// Vintages whose period won nothing, scored apart by absolute error.
	ZeroOutcomeVintages      int   `json:"zero_outcome_vintages"`
	ZeroOutcomeAbsErrorMinor int64 `json:"zero_outcome_abs_error_minor"`
}

func moneyError(vintages []EvaluationVintage, predict prediction) (*MoneyError, error) {
	out := &MoneyError{}
	for _, v := range vintages {
		predicted, err := predict(v)
		if err != nil {
			return nil, err
		}
		actual := v.Outcome.WonMinor
		miss, err := addMinor(predicted, -actual)
		if err != nil {
			return nil, err
		}
		abs := max(miss, -miss)
		for _, add := range []struct {
			into *int64
			by   int64
		}{
			{&out.ActualMinor, actual},
			{&out.PredictedMinor, predicted},
			{&out.AbsErrorMinor, abs},
			{&out.SignedErrorMinor, miss},
		} {
			if *add.into, err = addMinor(*add.into, add.by); err != nil {
				return nil, err
			}
		}
		if actual == 0 {
			out.ZeroOutcomeVintages++
			if out.ZeroOutcomeAbsErrorMinor, err = addMinor(out.ZeroOutcomeAbsErrorMinor, abs); err != nil {
				return nil, err
			}
		}
	}
	if len(vintages) > 0 {
		out.MeanAbsErrorMinor = roundDiv(big.NewInt(out.AbsErrorMinor), big.NewInt(int64(len(vintages)))).Int64()
	}
	if out.ActualMinor != 0 {
		out.AbsErrorPerActual = ratio(big.NewInt(out.AbsErrorMinor), big.NewInt(out.ActualMinor))
	}
	return out, nil
}

// Calibration compares what a method claimed about each open deal with
// whether the deal was won inside the period.
type Calibration struct {
	Deals int `json:"deals"`
	// Deals with no stage probability, which no bin can hold.
	NoProbability int              `json:"no_probability"`
	Bins          []CalibrationBin `json:"bins"`
	// BrierScore is the mean squared gap between probability and result,
	// for the one method that states a probability per deal.
	BrierScore string `json:"brier_score,omitempty"`
}

// CalibrationBin is one group of deals and the share of them that was won.
// Below the cohort floor it says only how many deals it holds.
type CalibrationBin struct {
	Bin           string `json:"bin"`
	Deals         int    `json:"deals"`
	MeanPredicted string `json:"mean_predicted,omitempty"`
	WonShare      string `json:"won_share,omitempty"`
}

// calibrate sums the vintages' tallies. Stage weighting bins by tenths of the
// typed chance, self-correcting weighting by tenths of the applied chance in
// use, and commit evidence by whether a deal was committed. The candidate and
// the range state nothing per deal and have no calibration.
func calibrate(method string, vintages []EvaluationVintage, floor int) *Calibration {
	if method == MethodOpenClosure || method == MethodRange {
		return nil
	}
	out := &Calibration{}
	var bins [10]Tally
	var brier int64
	for _, v := range vintages {
		out.Deals += v.Tallies.Labelled
		if method == MethodCommitEvidence {
			for i, t := range v.Tallies.Commit {
				bins[i] = addTally(bins[i], t)
			}
			continue
		}
		chances := v.Tallies.Typed
		if method == MethodSelfCorrecting {
			chances = v.Tallies.Applied
		}
		out.NoProbability += v.Tallies.NoProbability
		brier += chances.BrierSum
		for i, t := range chances.Bins {
			bins[i] = addTally(bins[i], t)
		}
	}
	perDeal := method != MethodCommitEvidence
	binned := int64(0)
	for bin, t := range bins {
		if t.Deals == 0 {
			continue
		}
		binned += t.Deals
		row := CalibrationBin{Bin: calibrationBinName(method, bin), Deals: int(t.Deals)}
		if t.Deals >= int64(floor) {
			row.WonShare = ratio(big.NewInt(t.Won), big.NewInt(t.Deals))
			if perDeal {
				row.MeanPredicted = ratio(big.NewInt(t.PercentSum), big.NewInt(100*t.Deals))
			}
		}
		out.Bins = append(out.Bins, row)
	}
	if perDeal && binned >= int64(floor) {
		out.BrierScore = ratio(big.NewInt(brier), big.NewInt(10000*binned))
	}
	return out
}

// rangeMissPenalty is 2/alpha for the central 80% interval, alpha = 0.2: the
// interval score charges ten times every unit an outcome falls outside it.
const rangeMissPenalty = 10

// IntervalScore is how the frozen ranges did against what was won: how many
// outcomes fell inside, and the Gneiting–Raftery interval score, the width
// plus 2/alpha times every unit outside. Lower is better.
type IntervalScore struct {
	Vintages int    `json:"vintages"`
	Covered  int    `json:"covered"`
	Nominal  string `json:"nominal_coverage"`
	Coverage string `json:"coverage,omitempty"`
	// Means over the vintages, in minor units.
	MeanWidthMinor int64 `json:"mean_width_minor"`
	MeanScoreMinor int64 `json:"mean_interval_score_minor"`
}

// scoreIntervals scores each vintage's frozen range against its outcome. Every
// vintage carries a range; the caller checks.
func scoreIntervals(vintages []EvaluationVintage) *IntervalScore {
	out := &IntervalScore{Vintages: len(vintages), Nominal: ratio(big.NewInt(8), big.NewInt(10))}
	width, score := new(big.Int), new(big.Int)
	for _, v := range vintages {
		low, high, won := big.NewInt(*v.RangeLowMinor), big.NewInt(*v.RangeHighMinor), big.NewInt(v.Outcome.WonMinor)
		span := new(big.Int).Sub(high, low)
		width.Add(width, span)
		score.Add(score, span)
		switch {
		case won.Cmp(low) < 0:
			score.Add(score, new(big.Int).Mul(big.NewInt(rangeMissPenalty), new(big.Int).Sub(low, won)))
		case won.Cmp(high) > 0:
			score.Add(score, new(big.Int).Mul(big.NewInt(rangeMissPenalty), new(big.Int).Sub(won, high)))
		default:
			out.Covered++
		}
	}
	if len(vintages) > 0 {
		n := big.NewInt(int64(len(vintages)))
		out.Coverage = ratio(big.NewInt(int64(out.Covered)), n)
		out.MeanWidthMinor = roundDiv(width, n).Int64()
		out.MeanScoreMinor = roundDiv(score, n).Int64()
	}
	return out
}

func addTally(a, b Tally) Tally {
	return Tally{Deals: a.Deals + b.Deals, Won: a.Won + b.Won, PercentSum: a.PercentSum + b.PercentSum}
}

func calibrationBinName(method string, bin int) string {
	if method == MethodCommitEvidence {
		return [2]string{"not_committed", "committed"}[bin]
	}
	if bin == 9 {
		return "90-100"
	}
	return strconv.Itoa(bin*10) + "-" + strconv.Itoa(bin*10+9)
}

// ratio writes num/den to decimalPlaces. den is never zero here.
func ratio(num, den *big.Int) string {
	return new(big.Rat).SetFrac(num, den).FloatString(decimalPlaces)
}
