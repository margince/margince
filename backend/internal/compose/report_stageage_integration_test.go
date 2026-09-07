// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// stage-age keeps the caller's own/team lens (owner_id is both a dimension
// and a filter here, over an identity table with no other narrowing — a
// wider population would let a rep pull a named colleague's exact
// stage-aging figures by filtering to their id). An unowned deal — one
// nobody has claimed — must still count toward a team manager's own
// managed-teams population.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestStageAgeCountsAnUnownedDealForATeamManager(t *testing.T) {
	e := setupForecast(t)
	e.seedOpenDeal(t, "Unowned", 60, nil, int64p(10000), stringp("commit"))

	manager := e.dealReadCtx(ids.NewV7(), []ids.UUID{e.Team1}, principal.RowScopeTeam)
	result := e.runReport(manager, t, "stage-age",
		`{"group_by":["stage_id"],"aggregates":[{"fn":"count","as":"deals"}]}`)
	if len(result.Rows) == 0 {
		t.Fatal("a Team1 manager read no stage-age rows, want the unowned " +
			"deal to still count toward their own managed-teams population")
	}
}
