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
			{&out.ActualMinor, actual}, {&out.PredictedMinor, predicted},
			{&out.AbsErrorMinor, abs}, {&out.SignedErrorMinor, miss},
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
type CalibrationBin struct {
	Bin           string `json:"bin"`
	Deals         int    `json:"deals"`
	MeanPredicted string `json:"mean_predicted,omitempty"`
	WonShare      string `json:"won_share"`
}

type binTally struct{ deals, won, percentSum int64 }

// calibrate bins the labelled open deals of a group. Stage weighting bins by
// tenths of its probability; commit evidence by whether a deal was committed.
// The candidate states nothing per deal and has no calibration.
func calibrate(method string, vintages []EvaluationVintage) *Calibration {
	if method == MethodOpenClosure {
		return nil
	}
	out := &Calibration{}
	tallies := map[int]*binTally{}
	var brier, binned int64
	for _, v := range vintages {
		for _, deal := range v.OpenDeals {
			if deal.WonInHorizon == nil {
				continue
			}
			out.Deals++
			won := int64(0)
			if *deal.WonInHorizon {
				won = 1
			}
			bin := 0
			if deal.InEvidence {
				bin = 1
			}
			if method == MethodStageWeighting {
				if deal.StageProbability == nil {
					out.NoProbability++
					continue
				}
				p := int64(*deal.StageProbability)
				bin = int(min(p/10, 9))
				gap := p - 100*won
				brier += gap * gap
				binned++
				tally(tallies, bin).percentSum += p
			}
			tally(tallies, bin).deals++
			tally(tallies, bin).won += won
		}
	}
	for bin := range 10 {
		t, ok := tallies[bin]
		if !ok {
			continue
		}
		row := CalibrationBin{Bin: calibrationBinName(method, bin), Deals: int(t.deals), WonShare: ratio(big.NewInt(t.won), big.NewInt(t.deals))}
		if method == MethodStageWeighting {
			row.MeanPredicted = ratio(big.NewInt(t.percentSum), big.NewInt(100*t.deals))
		}
		out.Bins = append(out.Bins, row)
	}
	if binned > 0 {
		out.BrierScore = ratio(big.NewInt(brier), big.NewInt(10000*binned))
	}
	return out
}

func tally(tallies map[int]*binTally, bin int) *binTally {
	if tallies[bin] == nil {
		tallies[bin] = &binTally{}
	}
	return tallies[bin]
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
