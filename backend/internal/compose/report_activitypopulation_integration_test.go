// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// activities-by-kind's population opt-out, over the saved-analytics door
// (RunAnalyticsQuery / specNarrowings) as well as the run_report one —
// report_projectgrant_integration_test.go already covers this key's project
// grant over this same door, with a RowScopeAll reader that never reaches
// the population resolver at all (an all-scope lens skips it). This is the
// other half: a non-all-scope reader, the shape that used to render the
// engine's hardcoded `owner_id = caller` clause against a table that has no
// such column.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// teamScopedActivityReader is activityReader's RowScopeTeam twin: the tier
// that actually reaches AnalyticsPopulationClause's owner_id resolution,
// which an all-scope reader's lens never does.
func (e *forecastEnv) teamScopedActivityReader() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		TeamIDs: []ids.UUID{e.Team1},
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true}, "forecast": {Read: true}, "installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeTeam,
		},
	})
}

// A team-scoped reader asking activities-by-kind through the saved-analytics
// door must get a real count, not the 500 the population default's
// hardcoded owner_id clause produced against a table with no such column.
func TestActivitiesByKindAnalyticsQueryDoesNotCrashForATeamScopedReader(t *testing.T) {
	e := setupForecast(t)
	// Above analyticsquery.DefaultFloor (5): a count at or below the floor is
	// WITHHELD by design (a separate mechanism from the population crash this
	// test is about), and a withheld answer would read like this one too.
	for i := 0; i < 6; i++ {
		e.seedID(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
			VALUES ($1, 'call', 'checking in', now() - interval '1 hour', 'manual', 'human:x')`)
	}

	answer, err := e.askAnalytics(e.teamScopedActivityReader(), t, analyticsquery.Query{
		Entity:   "activities-by-kind",
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "activities"}},
	})
	if err != nil {
		t.Fatalf("activities-by-kind for a team-scoped reader errored (want a real answer, not the owner_id crash): %v", err)
	}
	if answer.Withheld {
		t.Fatalf("answer withheld: %+v, want it above the floor", answer)
	}
	if len(answer.Rows) != 1 || answer.Rows[0]["activities"] != int64(6) && answer.Rows[0]["activities"] != float64(6) {
		t.Errorf("activities-by-kind for a team-scoped reader = %+v, want the six calls counted", answer.Rows)
	}
}
