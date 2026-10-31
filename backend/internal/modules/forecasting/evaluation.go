// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
)

// EvaluationVersion names the rules Evaluate scores by. Bump it when a metric,
// an exclusion or a window rule changes. Two reports with one version and one
// dataset digest are then the same report.
const EvaluationVersion = "forecast_evaluation_v2"

// The labels a report row carries. An observed row scores a figure that was
// frozen when its snapshot was taken. A backtest row scores a figure computed
// afterwards from frozen inputs, which nobody saw at the time.
const (
	LabelObserved = "observed"
	LabelBacktest = "backtest"
)

// The methods compared. Stage weighting weights each open deal by its typed
// chance; self-correcting weighting by its chance in use with the risk effect.
// The range is the central 80% interval each snapshot froze. The candidate is
// the trivial one, fitted on the training window only.
const (
	MethodStageWeighting = "stage_weighting"
	MethodSelfCorrecting = "self_correcting_weighting"
	MethodCommitEvidence = "commit_evidence"
	MethodRange          = "range_central_80"
	MethodOpenClosure    = "candidate_open_closure"
)

// How much recorded evidence a vintage's open deals stood on. Sufficient means
// at least half of them stood in stages with PriorStrength or more resolved
// outcomes; thin means some evidence but less; none means no evidence at all.
const (
	EvidenceSufficient = "sufficient"
	EvidenceThin       = "thin"
	EvidenceNone       = "none"
)

// The chronological windows, by the local day a snapshot was taken.
const (
	WindowTrain      = "train"
	WindowValidation = "validation"
	WindowTest       = "test"
)

// The reasons a vintage is left out, in the order they are tested.
const (
	ExcludedAfterOutcome      = "after_outcome"
	ExcludedDefinitionChanged = "definition_changed"
	ExcludedOutcomePending    = "outcome_pending"
	ExcludedOutcomeMissing    = "outcome_missing"
	ExcludedEvidenceMissing   = "evidence_missing"
	ExcludedOutcomeIncomplete = "outcome_incomplete"
	ExcludedOtherScope        = "scope_not_evaluated"
	ExcludedBelowFloor        = "below_cohort_floor"
	ExcludedIndividualOff     = "individual_performance_off"
)

// EvaluationPlan is what a maintainer fixes before reading a report.
type EvaluationPlan struct {
	// TrainEnd and ValidationEnd are local days. A vintage taken before
	// TrainEnd is training, before ValidationEnd validation, and test after.
	TrainEnd      time.Time `json:"train_end"`
	ValidationEnd time.Time `json:"validation_end"`
	// HorizonEdges split the horizon in days: 30,60 gives 0-29, 30-59, 60+.
	HorizonEdges []int `json:"horizon_edges"`
	// SettleDays is how long after a period ends its outcome may still be
	// corrected. An outcome inside it is pending.
	SettleDays int `json:"settle_days"`
	// CohortFloor is the fewest deals behind a reported figure. The command
	// sets it from the reporting floor; it is no flag.
	CohortFloor int `json:"cohort_floor"`
	// ScoreCandidate is set only once a reviewed pre-registration for this
	// dataset was checked. Without it only the configured readings are scored.
	ScoreCandidate bool `json:"score_candidate"`
}

func (p EvaluationPlan) check() error {
	if !p.TrainEnd.Before(p.ValidationEnd) {
		return errors.New("forecasting: the training window must end before the validation window")
	}
	if p.CohortFloor < 1 {
		return errors.New("forecasting: an evaluation needs a cohort floor of at least one deal")
	}
	if p.SettleDays < 0 {
		return errors.New("forecasting: the settle days cannot be negative")
	}
	for i, edge := range p.HorizonEdges {
		if edge <= 0 || (i > 0 && edge <= p.HorizonEdges[i-1]) {
			return errors.New("forecasting: the horizon edges must be positive and rising")
		}
	}
	return nil
}

// EvaluationReport is one evaluation's result. It holds no generic accuracy
// figure: money error and calibration answer different questions.
type EvaluationReport struct {
	EvaluationVersion string           `json:"evaluation_version"`
	DatasetVersion    string           `json:"dataset_version"`
	DatasetDigest     string           `json:"dataset_digest"`
	OutcomesAsOf      time.Time        `json:"outcomes_as_of"`
	Truncated         bool             `json:"truncated"`
	Plan              EvaluationPlan   `json:"plan"`
	Census            PopulationCensus `json:"census"`
	Candidate         []CandidateFit   `json:"candidate"`
	// CandidateNotScored says why the candidate has no fit and no rows.
	CandidateNotScored string          `json:"candidate_not_scored,omitempty"`
	Rows               []EvaluationRow `json:"rows"`
}

// PopulationCensus says which vintages were scored and why the rest were not.
type PopulationCensus struct {
	Vintages int              `json:"vintages"`
	Included int              `json:"included"`
	Excluded []ExclusionCount `json:"excluded"`
	// The horizon range of the included vintages, in days.
	HorizonMin int `json:"horizon_min_days"`
	HorizonMax int `json:"horizon_max_days"`
}

// ExclusionCount is how many vintages one reason left out.
type ExclusionCount struct {
	Reason   string `json:"reason"`
	Vintages int    `json:"vintages"`
}

// EvaluationRow scores one method on one group of vintages. Every method in a
// window, currency and horizon bucket is scored on the same vintages.
type EvaluationRow struct {
	Label    string `json:"label"`
	Method   string `json:"method"`
	Window   string `json:"window"`
	Currency string `json:"currency"`
	Horizon  string `json:"horizon"`
	Evidence string `json:"evidence_sufficiency"`
	// NotScored says why the row has no scores, such as a candidate that had
	// no training data for this bucket.
	NotScored string `json:"not_scored,omitempty"`
	Vintages  int    `json:"vintages"`
	// Outcomes counts distinct periods per context: vintages of one period
	// share one outcome, so this is the effective sample.
	Outcomes int `json:"outcomes"`
	// Vintages whose readings left priced or converted money out.
	UnpricedVintages int          `json:"unpriced_vintages"`
	FxGapVintages    int          `json:"fx_gap_vintages"`
	Money            *MoneyError    `json:"money,omitempty"`
	Calibration      *Calibration   `json:"calibration,omitempty"`
	Interval         *IntervalScore `json:"interval,omitempty"`
	bucket           int
}

// scored is one included vintage and its place in the report.
type scored struct {
	vintage EvaluationVintage
	window  string
	bucket  int
}

// Evaluate scores a dataset against a plan. It is pure: the same dataset and
// plan give the same report byte for byte, whatever order the vintages came in.
func Evaluate(dataset EvaluationDataset, plan EvaluationPlan) (EvaluationReport, error) {
	if dataset.Version != EvaluationDatasetVersion {
		return EvaluationReport{}, fmt.Errorf("forecasting: dataset version %q is not %q",
			dataset.Version, EvaluationDatasetVersion)
	}
	if err := plan.check(); err != nil {
		return EvaluationReport{}, err
	}
	if !sameCalendarDay(dataset.TrainEnd, plan.TrainEnd) {
		return EvaluationReport{}, fmt.Errorf("forecasting: the dataset's training labels were read at %s, and the plan trains to %s",
			dataset.TrainEnd.Format(time.DateOnly), plan.TrainEnd.Format(time.DateOnly))
	}
	dataset.Vintages = oldestFirst(dataset.Vintages)
	digest, err := DatasetDigest(dataset)
	if err != nil {
		return EvaluationReport{}, err
	}
	included, census := admit(dataset, plan)
	report := EvaluationReport{
		EvaluationVersion: EvaluationVersion, DatasetVersion: dataset.Version, DatasetDigest: digest,
		OutcomesAsOf: dataset.OutcomesAsOf, Truncated: dataset.Truncated, Plan: plan, Census: census,
	}
	var fits map[fitKey]closureRatio
	if plan.ScoreCandidate {
		report.Candidate, fits = fitOpenClosure(included, plan)
	} else {
		report.CandidateNotScored = "no reviewed pre-registration for this dataset was given"
	}
	report.Rows, err = scoreRows(included, plan, fits)
	if err != nil {
		return EvaluationReport{}, err
	}
	return report, nil
}

// DatasetDigest is the SHA-256 of a dataset as Evaluate reads it, with its
// vintages oldest first. A pre-registration names the dataset by it.
func DatasetDigest(dataset EvaluationDataset) (string, error) {
	dataset.Vintages = oldestFirst(dataset.Vintages)
	body, err := json.Marshal(dataset)
	if err != nil {
		return "", fmt.Errorf("forecasting: encoding the dataset for its digest: %w", err)
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func oldestFirst(vintages []EvaluationVintage) []EvaluationVintage {
	out := slices.Clone(vintages)
	slices.SortFunc(out, func(a, b EvaluationVintage) int {
		return cmp.Or(a.TakenAt.Compare(b.TakenAt), strings.Compare(a.ID, b.ID))
	})
	return out
}

// admit applies the exclusions and places every included vintage in its
// window and horizon bucket.
func admit(dataset EvaluationDataset, plan EvaluationPlan) ([]scored, PopulationCensus) {
	census := PopulationCensus{Vintages: len(dataset.Vintages)}
	excluded := map[string]int{}
	for _, left := range dataset.LeftOut {
		census.Vintages += left.Vintages
		excluded[left.Reason] += left.Vintages
	}
	var out []scored
	for _, v := range dataset.Vintages {
		if reason := exclusion(v, dataset, plan); reason != "" {
			excluded[reason]++
			continue
		}
		horizon := HorizonDays(v.LocalDay, v.PeriodEnd)
		if census.Included == 0 || horizon < census.HorizonMin {
			census.HorizonMin = horizon
		}
		census.HorizonMax = max(census.HorizonMax, horizon)
		census.Included++
		out = append(out, scored{vintage: v, window: windowOf(v.LocalDay, plan), bucket: horizonBucket(horizon, plan.HorizonEdges)})
	}
	for _, reason := range slices.Sorted(maps.Keys(excluded)) {
		census.Excluded = append(census.Excluded, ExclusionCount{Reason: reason, Vintages: excluded[reason]})
	}
	return out, census
}

// exclusion is the first reason a vintage cannot be scored, or empty.
func exclusion(v EvaluationVintage, dataset EvaluationDataset, plan EvaluationPlan) string {
	switch {
	case PredictionKindOf(v.Trigger, v.LocalDay, v.PeriodEnd) != PredictionObserved:
		return ExcludedAfterOutcome
	case v.Eligible < plan.CohortFloor:
		return ExcludedBelowFloor
	case v.Definition != dataset.DefinitionVersion:
		return ExcludedDefinitionChanged
	case !outcomeSettled(v.PeriodEnd, dataset.OutcomesDay, plan.SettleDays):
		return ExcludedOutcomePending
	case v.Outcome == nil:
		return ExcludedOutcomeMissing
	case v.Eligible > 0 && v.Rows == 0:
		return ExcludedEvidenceMissing
	case v.Outcome.FxMissing > 0:
		return ExcludedOutcomeIncomplete
	}
	return ""
}

// outcomeSettled answers whether a period's settle days ended before day.
func outcomeSettled(periodEnd, day time.Time, settleDays int) bool {
	return dayBefore(periodEnd.AddDate(0, 0, settleDays), day)
}

func windowOf(localDay time.Time, plan EvaluationPlan) string {
	switch {
	case dayBefore(localDay, plan.TrainEnd):
		return WindowTrain
	case dayBefore(localDay, plan.ValidationEnd):
		return WindowValidation
	default:
		return WindowTest
	}
}

func horizonBucket(horizon int, edges []int) int {
	for i, edge := range edges {
		if horizon < edge {
			return i
		}
	}
	return len(edges)
}

// bucketName spells a horizon bucket in days.
func bucketName(bucket int, edges []int) string {
	if len(edges) == 0 {
		return "all"
	}
	from := 0
	if bucket > 0 {
		from = edges[bucket-1]
	}
	if bucket == len(edges) {
		return strconv.Itoa(from) + "+"
	}
	return strconv.Itoa(from) + "-" + strconv.Itoa(edges[bucket]-1)
}
