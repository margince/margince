// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package metriccopy

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestEveryReportingLabelIsWrittenInEveryLanguage(t *testing.T) {
	for _, table := range []any{Labels, BreakdownLabels, LeadStatusLabels, Coverage, ComponentKindLabels, ForecastWords, ForecastCaveats, CohortWords} {
		langcopytest.Census(t, table)
	}
}
