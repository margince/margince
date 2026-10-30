// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/forecasting"
)

// preregistration is the file a maintainer fills before a candidate is
// scored, as docs/how-to/pre-register-a-forecast-evaluation.md describes.
type preregistration struct {
	DatasetDigest           string `json:"dataset_digest"`
	TrainEnd                string `json:"train_end"`
	ValidationEnd           string `json:"validation_end"`
	HorizonEdges            *[]int `json:"horizon_edges"`
	SettleDays              *int   `json:"settle_days"`
	EligiblePopulation      string `json:"eligible_population"`
	MinimumOutcomes         *int   `json:"minimum_outcomes"`
	MinimumCalibrationDeals *int   `json:"minimum_calibration_deals"`
	ImprovementThreshold    string `json:"improvement_threshold"`
	CalibrationThreshold    string `json:"calibration_threshold"`
	MaxExcludedShare        string `json:"max_excluded_share"`
	EvaluationCost          string `json:"evaluation_cost"`
	ServingProposal         string `json:"serving_proposal"`
	FilledBy                string `json:"filled_by"`
	ReviewedBy              string `json:"reviewed_by"`
}

// checkPreregistration refuses a candidate comparison unless every value was
// fixed beforehand, for this dataset and for the plan the flags give.
func checkPreregistration(path string, dataset forecasting.EvaluationDataset, plan forecasting.EvaluationPlan) error {
	body, err := os.ReadFile(path) // #nosec G304 -- the operator's own -preregistration path
	if err != nil {
		return fmt.Errorf("reading the pre-registration: %w", err)
	}
	var pre preregistration
	if err := json.Unmarshal(body, &pre); err != nil {
		return fmt.Errorf("the pre-registration file is not a pre-registration: %w", err)
	}
	var missing []string
	for name, filled := range map[string]bool{
		"dataset_digest": pre.DatasetDigest != "", "train_end": pre.TrainEnd != "", "validation_end": pre.ValidationEnd != "",
		"horizon_edges": pre.HorizonEdges != nil, "settle_days": pre.SettleDays != nil,
		"eligible_population": pre.EligiblePopulation != "", "minimum_outcomes": pre.MinimumOutcomes != nil,
		"minimum_calibration_deals": pre.MinimumCalibrationDeals != nil, "improvement_threshold": pre.ImprovementThreshold != "",
		"calibration_threshold": pre.CalibrationThreshold != "", "max_excluded_share": pre.MaxExcludedShare != "",
		"evaluation_cost": pre.EvaluationCost != "", "serving_proposal": pre.ServingProposal != "",
		"filled_by": pre.FilledBy != "", "reviewed_by": pre.ReviewedBy != "",
	} {
		if !filled {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return fmt.Errorf("the pre-registration leaves %s unset; fill every value before a candidate is scored", strings.Join(missing, ", "))
	}
	if pre.FilledBy == pre.ReviewedBy {
		return errors.New("the pre-registration names one maintainer as filler and reviewer; a second maintainer reviews it")
	}
	digest, err := forecasting.DatasetDigest(dataset)
	if err != nil {
		return err
	}
	if pre.DatasetDigest != digest {
		return fmt.Errorf("the pre-registration was fixed for dataset %s, and this dataset is %s", pre.DatasetDigest, digest)
	}
	if pre.TrainEnd != plan.TrainEnd.Format(time.DateOnly) || pre.ValidationEnd != plan.ValidationEnd.Format(time.DateOnly) ||
		!slices.Equal(*pre.HorizonEdges, plan.HorizonEdges) || *pre.SettleDays != plan.SettleDays {
		return errors.New("the flags differ from the windows, horizon edges or settle days the pre-registration fixed")
	}
	return nil
}
