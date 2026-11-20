// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/compose/metriccopy"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
)

// The words each metric carries live in metriccopy; these names keep the
// compose package's spelling.
var (
	reportingMetricNames      = metriccopy.Names
	reportingDefinitionWords  = metriccopy.Definitions
	reportingAttributionWords = metriccopy.Attribution
	reportingIncompleteWords  = metriccopy.Incomplete
)

// reportingMetricName names a metric in the reading's language. An edition
// keeps no name, so a metric retired since it was captured says so.
func reportingMetricName(frame crmcontracts.ReportingContext, metric crmcontracts.ReportingMetricID) string {
	name, ok := reportingMetricNames[metric]
	if !ok {
		name = reportingLabelWords.MetricRetired
	}
	return reporting.ExplanationCopy(frame).Say(name)
}
