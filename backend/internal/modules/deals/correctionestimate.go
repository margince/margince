// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"encoding/json"
	"math"
)

// CorrectionEstimateBasis preserves the observations recorded when the date changed.
type CorrectionEstimateBasis struct {
	WonDeals           int
	MedianDaysPerStage float64
	RemainingStages    int
}

func correctionEstimateBasis(evidence []byte) *CorrectionEstimateBasis {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(evidence, &fields); err != nil {
		return nil
	}
	var basis CorrectionEstimateBasis
	if err := json.Unmarshal(fields[CloseDateSampleCountKey], &basis.WonDeals); err != nil {
		return nil
	}
	if err := json.Unmarshal(fields[CloseDateMedianDaysKey], &basis.MedianDaysPerStage); err != nil {
		return nil
	}
	if err := json.Unmarshal(fields[CloseDateRemainingStagesKey], &basis.RemainingStages); err != nil {
		return nil
	}
	if basis.WonDeals < CloseDateMinHistory || basis.MedianDaysPerStage <= 0 || math.IsInf(basis.MedianDaysPerStage, 0) || basis.RemainingStages <= 0 {
		return nil
	}
	return &basis
}
