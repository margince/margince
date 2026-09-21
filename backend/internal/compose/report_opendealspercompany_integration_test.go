// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// open-deals-per-company keeps the caller's own/team lens: TestATypedQueryAnswersTheAskersOwnPopulation
// and TestADealOutsideThePopulationIsNotAPermissionExclusion (both in this
// package) require it to narrow to the caller's own population, the same
// door prebuilt reports and typed queries share — so an unrouted deal has to
// reach a team manager's own company rollup through the same unowned-row
// arm every other own-lens report in this package takes, not through a wider
// population declaration.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestOpenDealsPerCompanyCountsAnUnownedDealForATeamManager(t *testing.T) {
	e := setupForecast(t)
	companyID := e.seedID(t, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Unowned Co', 'manual', 'human:x')`)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, company_id, amount_minor, currency, source, captured_by)
		VALUES ($1, 'Unowned', $2, $3, $4, 10000, 'EUR', 'manual', 'human:x')`,
		e.pipeline, e.stages[60], companyID)

	manager := e.dealReadCtx(ids.NewV7(), []ids.UUID{e.Team1}, principal.RowScopeTeam)
	result := e.runReport(manager, t, "open-deals-per-company",
		`{"group_by":["company_id"],"aggregates":[{"fn":"count","as":"open_deals"}]}`)
	if len(result.Rows) == 0 {
		t.Fatal("a Team1 manager read no open-deals-per-company rows, want the " +
			"unowned deal to still count toward their own managed-teams population")
	}
}
