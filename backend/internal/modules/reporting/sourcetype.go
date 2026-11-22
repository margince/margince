// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// SourceType is what a metric's fact says it was counted from, and what
// report_edition_contribution.source_type stores. Each metric names its own
// source; this is the list of them. The schema CHECK on the column is held equal
// to it, and a fact naming anything else is refused before it is kept.
//
// Held by: TestEveryDomainEnumMatchesItsSchemaCheck (backend/gates/enumsync_test.go)
// Held by: TestEveryMetricSourceNamesAReportingSourceType (backend/gates/approvaltargettypes_test.go)
type SourceType string

// The sources a metric counts from. The record kinds are rows a reader may open.
// The hand-off and the introduction are credit rows with their own grant.
// A forecast projection has no source row at all.
const (
	SourceDeal               SourceType = "deal"
	SourceActivity           SourceType = "activity"
	SourceLead               SourceType = "lead"
	SourceCompany            SourceType = "company"
	SourceSDRHandoff         SourceType = "sdr_handoff"
	SourceIntroRequest       SourceType = "intro_request"
	SourceForecastProjection SourceType = "forecast_projection"
)

var knownSourceTypes = map[string]struct{}{
	string(SourceDeal): {}, string(SourceActivity): {}, string(SourceLead): {}, string(SourceCompany): {},
	string(SourceSDRHandoff): {}, string(SourceIntroRequest): {}, string(SourceForecastProjection): {},
}

// requireKnownSourceType refuses a fact whose source the CHECK would refuse, so
// the whole edition is not lost to a constraint error halfway through its rows.
func requireKnownSourceType(sourceType string) error {
	if _, known := knownSourceTypes[sourceType]; known {
		return nil
	}
	return fmt.Errorf("reporting: source type %q is not one a metric counts from: %w", sourceType, apperrors.ErrInvalidArgument)
}
