// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// dailyAISettingsFlow is Settings → AI as an admin opens it. It reads the
// month's usage and cost, the week's figures by provider and by tier, and the
// first page of the call history. These are diagnostics read on demand, so
// PERF-12 judges them and the shell's PERF-2 does not.
var dailyAISettingsFlow = dailyFlow{
	Name: "ai_settings", ID: "PERF-12", Budget: Perf12Budget, PerCall: true,
	Calls: func(in dailyInput) []dailyCall {
		return []dailyCall{
			{Name: "usage", Path: "/v1/ai/usage?" + in.AIMonth},
			{Name: "call_stats_provider", Path: "/v1/ai/call-stats?window=7d&group=provider"},
			{Name: "call_stats_tier", Path: "/v1/ai/call-stats?window=7d&group=tier"},
			{Name: "calls", Path: "/v1/ai/calls"},
		}
	},
}

// dailyAIMonth is the usage card's query for the month the seeded calls fall
// in. The database stamped those calls, and the meter keys its days in UTC, so
// the month is read off the database's clock in UTC.
func dailyAIMonth(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	var month string
	if err := e.Owner.QueryRow(t.Context(), `
		SELECT 'from=' || to_char(m, 'YYYY-MM-DD') || '&to=' || to_char(m + interval '1 month - 1 day', 'YYYY-MM-DD')
		FROM date_trunc('month', now() AT TIME ZONE 'UTC') AS m`).Scan(&month); err != nil {
		t.Fatalf("reading the usage month: %v", err)
	}
	return month
}
