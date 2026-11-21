// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/compose/metriccopy"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

// The words reporting says live in metriccopy; these names keep the
// compose package's spelling.
var (
	reportingMetricNames         = metriccopy.Names
	reportingDefinitionWords     = metriccopy.Definitions
	reportingAttributionWords    = metriccopy.Attribution
	reportingIncompleteWords     = metriccopy.Incomplete
	reportingLabelWords          = metriccopy.Labels
	reportingBreakdownLabels     = metriccopy.BreakdownLabels
	reportingLeadStatusLabels    = metriccopy.LeadStatusLabels
	reportingCoverageWords       = metriccopy.Coverage
	reportingForecastWords       = metriccopy.ForecastWords
	reportingForecastCaveats     = metriccopy.ForecastCaveats
	cohortWords                  = metriccopy.CohortWords
	reportingComponentKindLabels = metriccopy.ComponentKindLabels
)

// reportingMetricName names a metric in said's language. An edition keeps no
// name, so a metric retired since it was captured says so.
func reportingMetricName(said langcopy.Spoken, metric crmcontracts.ReportingMetricID) string {
	name, ok := reportingMetricNames[metric]
	if !ok {
		name = reportingLabelWords.MetricRetired
	}
	return said.Say(name)
}
