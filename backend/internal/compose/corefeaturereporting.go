// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/corefeatures"
)

func (metricEvaluator) FeaturesAvailable(ctx context.Context, selection crmcontracts.ReportingSelection) bool {
	availability := corefeatures.FromContext(ctx)
	if !availability.Enabled(crmcontracts.CoreFeatureAnalytics) {
		return false
	}
	if (selection.PipelineId != nil || selection.ProductId != nil) && !availability.RecordEnabled("deal") {
		return false
	}
	if (selection.Country != nil || selection.Industry != nil) && !availability.RecordEnabled("company") {
		return false
	}
	for _, spec := range reportingMetrics {
		if slices.Contains(selection.Metrics, spec.definition.Id) && !availability.RecordEnabled(spec.object) {
			return false
		}
	}
	for _, card := range reporting.QuestionCards(selection) {
		if !availability.RecordEnabled(card.Question.Entity) {
			return false
		}
	}
	return true
}
