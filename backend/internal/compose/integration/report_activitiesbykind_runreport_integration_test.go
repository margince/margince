// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// activities-by-kind's population opt-out, over the governed run_report
// tool — a real migrated Postgres, a real team-scoped reader.
//
// `activity` carries no owner_id column, so the report engine's population
// default (measureCallersOwn, which appends a hardcoded `owner_id = caller`
// clause) rendered invalid SQL for any caller whose row scope was not
// RowScopeAll — a 500, not a wrong answer. Row scope alone already narrows
// this report correctly (activityWalk routes it through
// auth.ActivityContentClause, real per-row narrowing, not the TRUE an
// identity table's row scope renders), so opting the spec out of the
// population default is a complete fix on its own.
//
// The saved-analytics door onto the same spec (RunAnalyticsQuery /
// specNarrowings) is covered separately, in
// backend/internal/compose/report_activitypopulation_integration_test.go,
// since it lives in a different package with a different harness.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// teamReaderWith mints a RowScopeTeam reader on Team1 holding exactly the
// named read grants — the tier that actually reaches the population
// resolver (an all-scope reader's lens never does), as against the
// RowScopeAll readers the rest of this package's report tests use.
func (e *Env) teamReaderWith(objects ...string) context.Context {
	grants := map[string]principal.ObjectGrant{"installation_settings": {Read: true}}
	for _, object := range objects {
		grants[object] = principal.ObjectGrant{Read: true}
	}
	return e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{Objects: grants, RowScope: principal.RowScopeTeam})
}

func TestActivitiesByKindDoesNotCrashForATeamScopedReader(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	fileActivity(admin, t, e, "call", time.Now().UTC().Add(-time.Hour), nil)

	reader := e.teamReaderWith("activity")
	rows, err := tryPrebuiltReport(reader, e, "activities-by-kind", "")
	if err != nil {
		t.Fatalf("activities-by-kind for a team-scoped reader errored (want a real answer, not the owner_id crash): %v", err)
	}
	if len(rows) != 1 || cell(rows[0], "activities") != "1" {
		t.Errorf("activities-by-kind for a team-scoped reader = %v, want the one call counted", rows)
	}
}
