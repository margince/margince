// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
)

func reportingDealFields(metric crmcontracts.ReportingMetricID) []string {
	fields := []string{reportingName, fieldOwnerID, "pipeline_id", "stage_id", reportingStatusField}
	switch metric {
	case reportingBookingsWon:
		fields = append(fields, "closed_at", "amount_minor", reportingCurrency)
	case reportingClosedWinRate:
		fields = append(fields, "closed_at")
	case reportingQualifiedPipelineCreated:
		fields = append(fields, "amount_minor", reportingCurrency)
	case reportingStageAge:
		fields = append(fields, "created_at", "expected_close_date")
	default:
		fields = append(fields, "amount_minor", reportingCurrency, "expected_close_date", "forecast_category", "win_probability")
	}
	return fields
}

func reportingFieldScope(ctx context.Context, object, alias string, fields []string, b *reportingBindings) (string, bool, error) {
	clause := sqlUnnarrowed
	withheld := false
	for _, field := range fields {
		predicate, masked, err := auth.MaskExcludedClause(ctx, object, field, alias, b.arg)
		if err != nil {
			return "", false, err
		}
		if masked && predicate != "" {
			clause += " AND " + predicate
			withheld = true
		}
	}
	return clause, withheld, nil
}

func reportingMeetingFields() []string {
	return []string{"meeting_status", "scheduled_start_at", "host_user_id", reportingSubject}
}
