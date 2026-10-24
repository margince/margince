// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func reportingMetricMetadata(spec metricSpec, language textlang.Lang) crmcontracts.ReportingMetricDefinition {
	out := spec.definition
	filters := []string{reportingScope, "period", "target_basis"}
	switch out.Id {
	case reportingMeetingsHeld, reportingAcceptedOpportunities, reportingLeadsCreated, reportingLeadStatusMoves,
		reportingLeadsPromoted, reportingLeadFirstResponse, reportingPartnerCommission, reportingPartnerActivity:
	case reportingPartnerPipeline, reportingPartnerWon:
		filters = append(filters, "pipeline")
	default:
		filters = append(filters, "pipeline", "close_window")
	}
	out.AllowedFilters = &filters
	fields := reportingRequiredFields(out.Id)
	out.RequiredFields = &fields
	out.ReadObject = &spec.object
	minimum := 0
	if out.Id == reportingStageAge || out.Id == reportingClosedWinRate || out.Id == reportingLeadFirstResponse {
		minimum = analyticsquery.PercentileSampleFloor
	}
	out.MinimumCohort = &minimum
	out.Definition = reportingDefinitionWords[out.Id].In(language)
	attribution := reportingAttributionWords[out.Id].In(language)
	out.Attribution = &attribution
	policy := reportingIncompleteWords[out.Id].In(language)
	out.IncompletePolicy = &policy
	frozen := true
	out.FrozenSupport = &frozen
	limit := reportingFactLimit
	out.MaxContributions = &limit
	return out
}

// reportingRequiredFields names the columns a metric reads, which a field mask
// on any of them withholds.
func reportingRequiredFields(metric crmcontracts.ReportingMetricID) []string {
	switch metric {
	case reportingMeetingsHeld:
		return reportingMeetingFields()
	case reportingAcceptedOpportunities:
		return []string{"accepted_credit"}
	case reportingLeadsCreated, reportingLeadStatusMoves, reportingLeadsPromoted, reportingLeadFirstResponse:
		return reportingLeadFields(metric)
	case reportingPartnerActivity:
		return []string{companyDisplayName, fieldOwnerID, partnerLastActivity, partnerCertStatus}
	case reportingPartnerCommission:
		return []string{reportingAmountField, reportingCurrency, reportingPartnerField}
	}
	return reportingDealFields(metric)
}
