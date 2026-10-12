// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// stage-age measures every deal the reader may see: how long deals sit is a
// question about the pipeline. Unowned deals and deals owned outside the
// caller's teams both count.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestStageAgeCountsEveryReadableDealForATeamManager(t *testing.T) {
	e := setupForecast(t)
	e.seedOpenDeal(t, "Unowned", 60, nil, new(int64(10000)), new("commit"))
	e.seedOpenDeal(t, "Team2's", 60, &e.Rep3, new(int64(10000)), new("commit"))

	manager := e.dealReadCtx(ids.NewV7(), []ids.UUID{e.Team1}, principal.RowScopeTeam)
	result := e.runReport(manager, t, "stage-age",
		`{"group_by":["stage_id"],"aggregates":[{"fn":"count","as":"deals"}]}`)
	if len(result.Rows) != 1 || wireInt(t, result.Rows[0], "deals") != 2 {
		t.Fatalf("a Team1 manager read %+v, want one stage counting 2 — the unowned "+
			"deal and Rep3's, whose team is not theirs", result.Rows)
	}
}
