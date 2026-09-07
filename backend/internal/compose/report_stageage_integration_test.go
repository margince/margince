// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// stage-age is reachable only through the generic run_report/Analytics door,
// with no personal-work screen framing "how long has MY stage been sitting"
// — an aging-analysis question about the installation, not about the caller.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A deal owned by a seat on a different team must still count toward a team
// manager's stage-age reading: this report answers for the whole
// installation and must not narrow to the caller's own teams.
func TestStageAgeIsNotNarrowedToATeamManagersOwnTeams(t *testing.T) {
	e := setupForecast(t)
	e.seedOpenDeal(t, "Cross-team", 60, &e.Rep3, int64p(10000), stringp("commit"))

	manager := e.dealReadCtx(ids.NewV7(), []ids.UUID{e.Team1}, principal.RowScopeTeam)
	result := e.runReport(manager, t, "stage-age",
		`{"group_by":["stage_id"],"aggregates":[{"fn":"count","as":"deals"}]}`)
	if len(result.Rows) == 0 {
		t.Fatal("a Team1 manager read no stage-age rows, want Rep3's Team2 " +
			"deal to still count — stage-age answers for the whole installation")
	}
}
