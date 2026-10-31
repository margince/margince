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
// share of open money that closed. It is measured on training vintages whose
// outcome had settled before the training window ended.
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

// fitOpenClosure fits the candidate per currency and horizon bucket, on the
// outcomes as the history stood at TrainEnd. A training vintage whose period
// had not settled by then is skipped.
func fitOpenClosure(included []scored, plan EvaluationPlan) ([]CandidateFit, map[fitKey]closureRatio) {
	sums := map[fitKey]closureRatio{}
	counts := map[fitKey]int{}
	for _, s := range included {
		key := fitKey{s.vintage.Currency, s.bucket}
		if _, ok := sums[key]; !ok {
			sums[key] = closureRatio{num: new(big.Int), den: new(big.Int)}
		}
		known := s.vintage.TrainOutcome
		if s.window != WindowTrain || known == nil || known.FxMissing > 0 ||
			!outcomeSettled(s.vintage.PeriodEnd, plan.TrainEnd, plan.SettleDays) {
			continue
		}
		counts[key]++
		sums[key].num.Add(sums[key].num, big.NewInt(known.WonMinor-s.vintage.WonMinor))
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
	evidence string
}

var windowOrder = []string{WindowTrain, WindowValidation, WindowTest}

// scoreRows scores every method on every group, and all three see the same
// vintages. A candidate that cannot predict a group says so instead of being
// scored on a smaller set.
func scoreRows(included []scored, plan EvaluationPlan, fits map[fitKey]closureRatio) ([]EvaluationRow, error) {
	groups := map[groupKey][]EvaluationVintage{}
	for _, s := range included {
		key := groupKey{s.window, s.vintage.Currency, s.bucket, s.vintage.Sufficiency}
		groups[key] = append(groups[key], s.vintage)
	}
	var rows []EvaluationRow
	methods := []string{MethodStageWeighting, MethodSelfCorrecting, MethodCommitEvidence, MethodRange}
	if plan.ScoreCandidate {
		methods = append(methods, MethodOpenClosure)
	}
	for key, vintages := range groups {
		for _, method := range methods {
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
			strings.Compare(a.Currency, b.Currency), a.bucket-b.bucket, strings.Compare(a.Evidence, b.Evidence))
	})
	return rows, nil
}

func scoreRow(method string, key groupKey, vintages []EvaluationVintage, plan EvaluationPlan, fits map[fitKey]closureRatio) (EvaluationRow, error) {
	row := EvaluationRow{
		Label: methodLabel(method, vintages), Method: method, Window: key.window, Currency: key.currency,
		Horizon: bucketName(key.bucket, plan.HorizonEdges), Evidence: key.evidence, Vintages: len(vintages), bucket: key.bucket,
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
	if row.NotScored = unscorable(method, vintages); row.NotScored != "" {
		return row, nil
	}
	if method == MethodRange {
		row.Interval = scoreIntervals(vintages)
		return row, nil
	}
	predict := configuredPrediction(method)
	if method == MethodOpenClosure {
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
	row.Calibration = calibrate(method, vintages, plan.CohortFloor)
	return row, nil
}

// methodLabel is observed when every vintage showed the method's figure when
// it was frozen. Typed weighting is what a snapshot showed with self-correction
// off; with it on, the typed figure is recomputed from frozen chances.
func methodLabel(method string, vintages []EvaluationVintage) string {
	switch method {
	case MethodOpenClosure:
		return LabelBacktest
	case MethodStageWeighting:
		for _, v := range vintages {
			if v.SelfCorrecting != nil && *v.SelfCorrecting {
				return LabelBacktest
			}
		}
	}
	return LabelObserved
}

// unscorable says why a method cannot score every vintage of a group, so no
// method is scored on a smaller set than the others.
func unscorable(method string, vintages []EvaluationVintage) string {
	for _, v := range vintages {
		switch {
		case method == MethodSelfCorrecting && (v.SelfCorrecting == nil || !*v.SelfCorrecting):
			return "a vintage in this group was frozen with self-correction off, so no chance in use was kept"
		case method == MethodRange && (v.RangeLowMinor == nil || v.RangeHighMinor == nil):
			return "a vintage in this group was frozen before ranges were drawn"
		}
	}
	return ""
}

type prediction func(EvaluationVintage) (int64, error)

// configuredPrediction is won money plus the method's remaining figure:
// open deals at typed chances, the frozen weighted figure under the chances in
// use, or committed evidence.
func configuredPrediction(method string) prediction {
	return func(v EvaluationVintage) (int64, error) {
		readings := Readings{WonMinor: v.WonMinor, EvidenceMinor: v.Evidence, WeightedMinor: v.TypedWeighted}
		measure := MeasureWeighted
		switch method {
		case MethodSelfCorrecting:
			readings.WeightedMinor = v.Weighted
		case MethodCommitEvidence:
			measure = MeasureCommitEvidence
		}
		landing, err := ProjectLanding(readings, measure, nil)
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
