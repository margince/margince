// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"cmp"
	"errors"
	"maps"
	"math/big"
	"slices"
	"strings"
)

// CandidateFit is the trivial candidate's one parameter for one bucket: the
// share of open money that closed, measured on training vintages whose outcome
// had settled before the training window ended.
type CandidateFit struct {
	Currency      string `json:"currency"`
	Horizon       string `json:"horizon"`
	TrainVintages int    `json:"train_vintages"`
	Ratio         string `json:"ratio,omitempty"`
	NotFit        string `json:"not_fit,omitempty"`
}

type fitKey struct {
	currency string
	bucket   int
}

// closureRatio is a fitted ratio kept exact, so a prediction rounds once.
type closureRatio struct{ num, den *big.Int }

// fitOpenClosure fits the candidate per currency and horizon bucket. A
// training vintage whose period had not settled by TrainEnd is skipped: its
// outcome was not known when the training window closed.
func fitOpenClosure(included []scored, plan EvaluationPlan) ([]CandidateFit, map[fitKey]closureRatio) {
	sums := map[fitKey]closureRatio{}
	counts := map[fitKey]int{}
	for _, s := range included {
		key := fitKey{s.vintage.Currency, s.bucket}
		if _, ok := sums[key]; !ok {
			sums[key] = closureRatio{num: new(big.Int), den: new(big.Int)}
		}
		if s.window != WindowTrain || !outcomeSettled(s.vintage.PeriodEnd, plan.TrainEnd, plan.SettleDays) {
			continue
		}
		counts[key]++
		sums[key].num.Add(sums[key].num, big.NewInt(s.vintage.Outcome.WonMinor-s.vintage.WonMinor))
		sums[key].den.Add(sums[key].den, big.NewInt(s.vintage.OpenMinor))
	}
	keys := slices.SortedFunc(maps.Keys(sums), func(a, b fitKey) int {
		return cmp.Or(strings.Compare(a.currency, b.currency), a.bucket-b.bucket)
	})
	fits := map[fitKey]closureRatio{}
	out := make([]CandidateFit, 0, len(keys))
	for _, key := range keys {
		fit := CandidateFit{Currency: key.currency, Horizon: bucketName(key.bucket, plan.HorizonEdges), TrainVintages: counts[key]}
		switch sum := sums[key]; {
		case counts[key] == 0:
			fit.NotFit = "no training vintage had a settled outcome before the training window ended"
		case sum.den.Sign() == 0:
			fit.NotFit = "the training vintages held no open money"
		default:
			fit.Ratio = ratio(sum.num, sum.den)
			fits[key] = sum
		}
		out = append(out, fit)
	}
	return out, fits
}

type groupKey struct {
	window   string
	currency string
	bucket   int
}

var windowOrder = []string{WindowTrain, WindowValidation, WindowTest}

// scoreRows scores every method on every group. All three methods see the
// same vintages of a group; a candidate that cannot predict a group says so
// instead of being scored on a smaller set.
func scoreRows(included []scored, plan EvaluationPlan, fits map[fitKey]closureRatio) ([]EvaluationRow, error) {
	groups := map[groupKey][]EvaluationVintage{}
	for _, s := range included {
		key := groupKey{s.window, s.vintage.Currency, s.bucket}
		groups[key] = append(groups[key], s.vintage)
	}
	var rows []EvaluationRow
	for key, vintages := range groups {
		for _, method := range []string{MethodStageWeighting, MethodCommitEvidence, MethodOpenClosure} {
			row, err := scoreRow(method, key, vintages, plan, fits)
			if err != nil {
				return nil, err
			}
			rows = append(rows, row)
		}
	}
	slices.SortFunc(rows, func(a, b EvaluationRow) int {
		return cmp.Or(strings.Compare(a.Label, b.Label), strings.Compare(a.Method, b.Method),
			slices.Index(windowOrder, a.Window)-slices.Index(windowOrder, b.Window),
			strings.Compare(a.Currency, b.Currency), a.bucket-b.bucket)
	})
	return rows, nil
}

func scoreRow(method string, key groupKey, vintages []EvaluationVintage, plan EvaluationPlan, fits map[fitKey]closureRatio) (EvaluationRow, error) {
	row := EvaluationRow{
		Label: LabelObserved, Method: method, Window: key.window, Currency: key.currency,
		Horizon: bucketName(key.bucket, plan.HorizonEdges), Vintages: len(vintages), bucket: key.bucket,
	}
	outcomes := map[string]bool{}
	for _, v := range vintages {
		outcomes[v.Context+"|"+v.PeriodStart.Format("2006-01-02")+"|"+v.PeriodEnd.Format("2006-01-02")] = true
		if v.Priced < v.Eligible {
			row.UnpricedVintages++
		}
		if v.FxMissing > 0 {
			row.FxGapVintages++
		}
	}
	row.Outcomes = len(outcomes)
	predict := configuredPrediction(method)
	if method == MethodOpenClosure {
		row.Label = LabelBacktest
		fit, ok := fits[fitKey{key.currency, key.bucket}]
		switch {
		case key.window == WindowTrain:
			row.NotScored = "the candidate was fitted on this window"
			return row, nil
		case !ok:
			row.NotScored = "the candidate has no fit for this bucket"
			return row, nil
		}
		predict = openClosurePrediction(fit)
	}
	money, err := moneyError(vintages, predict)
	if err != nil {
		return EvaluationRow{}, err
	}
	row.Money = money
	row.Calibration = calibrate(method, vintages)
	return row, nil
}

type prediction func(EvaluationVintage) (int64, error)

func configuredPrediction(method string) prediction {
	measure := MeasureWeighted
	if method == MethodCommitEvidence {
		measure = MeasureCommitEvidence
	}
	return func(v EvaluationVintage) (int64, error) {
		landing, err := ProjectLanding(Readings{WonMinor: v.WonMinor, EvidenceMinor: v.Evidence, WeightedMinor: v.Weighted}, measure, nil)
		return landing.AmountMinor, err
	}
}

var errPredictionOverflow = errors.New("forecasting: a candidate prediction does not fit in minor units")

func openClosurePrediction(fit closureRatio) prediction {
	return func(v EvaluationVintage) (int64, error) {
		remaining := roundDiv(new(big.Int).Mul(big.NewInt(v.OpenMinor), fit.num), fit.den)
		total := remaining.Add(remaining, big.NewInt(v.WonMinor))
		if !total.IsInt64() {
			return 0, errPredictionOverflow
		}
		return total.Int64(), nil
	}
}

// roundDiv divides n by a positive d, rounding half away from zero.
func roundDiv(n, d *big.Int) *big.Int {
	q, r := new(big.Int).QuoRem(n, d, new(big.Int))
	twice := new(big.Int).Lsh(new(big.Int).Abs(r), 1)
	if twice.Cmp(d) >= 0 {
		q.Add(q, big.NewInt(int64(n.Sign())))
	}
	return q
}
