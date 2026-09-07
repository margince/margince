// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A Team Lead's inbox carries what their TEAM is waiting on.
//
// The `team` arm of the row scope was written and unreached for months — no
// seeded role used it — so this half of the inbox's behaviour was never
// exercised. The negative half is: TestApprovalAuthorityHonorsTargetRowScope
// proves another team's staging stays absent. That test passes for a scope that
// admits nothing at all, so it is not evidence that team scope WORKS; this is.
//
// The seeded role that reaches it is `manager`, and identity's own
// TestTheSeededRolesReachTheRowScopeTheyClaim holds that fact against the
// seeded row rather than against the compiled defaults.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestATeamLeadsInboxCarriesTheirTeammatesStagedDecision(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	svc := approvals.NewService(e.DB())

	// Rep2 shares Team1 with Rep1; Rep3 sits in Team2.
	teammateDeal := e.SeedDeal(t, "A teammate's", pipeline, open, &e.Rep2)
	strangerDeal := e.SeedDeal(t, "Another team's", pipeline, open, &e.Rep3)
	teammates := stageFor(t, svc, e, "advance_deal", "deal", teammateDeal)
	strangers := stageFor(t, svc, e, "advance_deal", "deal", strangerDeal)

	lead := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	pending := listIDs(lead, t, svc, approvals.ListInput{Status: strPtr("pending"), Limit: 50})

	if !pending[teammates] {
		t.Error("a lead under team scope cannot see their own teammate's staged decision — " +
			"which is the whole of what team scope is for, and the arm nothing else exercises")
	}
	if pending[strangers] {
		t.Error("the inbox reached past the team into another one")
	}
}

func TestASeatUnderOwnScopeSeesOnlyItsOwn(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	svc := approvals.NewService(e.DB())

	mine := e.SeedDeal(t, "Mine", pipeline, open, &e.Rep1)
	teammates := e.SeedDeal(t, "A teammate's", pipeline, open, &e.Rep2)
	ownStaging := stageFor(t, svc, e, "advance_deal", "deal", mine)
	teammateStaging := stageFor(t, svc, e, "advance_deal", "deal", teammates)

	// The same grid, narrowed to own — which is what the seeded `rep` carries,
	// and the difference this pair of tests is about.
	ownScope := RepPerms
	ownScope.RowScope = principal.RowScopeOwn
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, ownScope)

	pending := listIDs(rep, t, svc, approvals.ListInput{Status: strPtr("pending"), Limit: 50})
	if !pending[ownStaging] {
		t.Error("a seat cannot see a staged decision against its own record")
	}
	if pending[teammateStaging] {
		t.Error("own scope reached a teammate's record — team membership is not by itself " +
			"permission to decide about a colleague's deal")
	}
}
