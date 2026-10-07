// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

func reportingMetricMetadata(spec metricSpec) crmcontracts.ReportingMetricDefinition {
	out := spec.definition
	filters := []string{reportingScope, "period", "target_basis"}
	if out.Id != reportingMeetingsHeld && out.Id != reportingAcceptedOpportunities {
		filters = append(filters, "pipeline", "close_window")
	}
	out.AllowedFilters = &filters
	fields := reportingDealFields(out.Id)
	if out.Id == reportingMeetingsHeld {
		fields = []string{"meeting_status", "scheduled_start_at", "host_user_id", reportingSubject}
	}
	if out.Id == reportingAcceptedOpportunities {
		fields = []string{"accepted_credit"}
	}
	out.RequiredFields = &fields
	out.ReadObject = &spec.object
	minimum := 0
	if out.Id == reportingStageAge || out.Id == reportingClosedWinRate {
		minimum = 5
	}
	out.MinimumCohort = &minimum
	attribution := "Current owner; current-state capture time"
	switch out.Id {
	case reportingBookingsWon, reportingClosedWinRate:
		attribution = "Current deal owner and pipeline; current valid closing date"
	case reportingQualifiedPipelineCreated:
		attribution = "Owner, pipeline and valuation at the first valid qualifying stage transition"
	case reportingMeetingsHeld:
		attribution = "Recorded host at the held transition; current host and customer links for older meetings without attribution history"
	case reportingAcceptedOpportunities:
		attribution = "Originating SDR on the earliest accepted handoff per opportunity"
	}
	out.Attribution = &attribution
	policy := "Additive readings include permitted known contributions with coverage; ratios, percentiles, targets and movement withhold unsupported geometry. Missing attribution history is partial, never a measured zero."
	out.IncompletePolicy = &policy
	frozen := true
	out.FrozenSupport = &frozen
	limit := reportingFactLimit
	out.MaxContributions = &limit
	return out
}
