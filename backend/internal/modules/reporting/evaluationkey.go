// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
)

// Captions may change without changing figures, scope or source identity.
// Clone slices before clearing prose so the response retains its explanations.
func evaluationKey(evaluation Evaluation) (string, error) {
	evaluation.Result.EvaluationKey = ""
	evaluation.Result.Context.ProseLanguage = nil
	evaluation.Result.Metrics = slices.Clone(evaluation.Result.Metrics)
	for i := range evaluation.Result.Metrics {
		evaluation.Result.Metrics[i].Coverage.Reason = nil
	}
	evaluation.Result.Charts = slices.Clone(evaluation.Result.Charts)
	for i := range evaluation.Result.Charts {
		chart := &evaluation.Result.Charts[i]
		chart.Coverage.Reason = nil
		chart.Points = slices.Clone(chart.Points)
		for j := range chart.Points {
			chart.Points[j].Label = ""
		}
		if chart.CaptureStatus != nil {
			status := *chart.CaptureStatus
			if status.Failure != nil {
				blank := ""
				status.Failure = &blank
			}
			chart.CaptureStatus = &status
		}
	}
	evaluation.Facts = slices.Clone(evaluation.Facts)
	for i := range evaluation.Facts {
		evaluation.Facts[i].Row.Label = ""
		evaluation.Facts[i].StageLabel = ""
		evaluation.Facts[i].OwnerLabel = ""
	}
	raw, err := json.Marshal(evaluation)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}
