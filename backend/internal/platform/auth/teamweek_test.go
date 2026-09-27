// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// Which teams' coaching weeks each seat reaches, stated per seeded seat and
// per departure from one.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// holds names which of the two team grants a test seat carries.
type holds struct{ leads, oversees bool }

// seatReading is a seat that reads deals, as every seeded role does. It holds
// team_lead.read when it leads and team_oversight.read when it oversees.
func seatReading(kind principal.PrincipalType, scope principal.RowScope, grants holds) context.Context {
	return seatHolding(kind, scope, map[string]principal.ObjectGrant{
		objTeamLead:      {Read: grants.leads},
		objTeamOversight: {Read: grants.oversees},
		"deal":           {Read: true},
	})
}

// seatWithoutDeals is seatReading with its deal read revoked.
func seatWithoutDeals(scope principal.RowScope, grants holds) context.Context {
	return seatHolding(principal.PrincipalHuman, scope, map[string]principal.ObjectGrant{
		objTeamLead:      {Read: grants.leads},
		objTeamOversight: {Read: grants.oversees},
	})
}

// seatHolding carries no role key at all: a key decides nothing here.
func seatHolding(kind principal.PrincipalType, scope principal.RowScope, objects map[string]principal.ObjectGrant) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: kind,
		ID:   "seat:test",
		Permissions: principal.Permissions{
			Objects:  objects,
			RowScope: scope,
		},
	})
}

func TestEachSeatReachesTheTeamWeeksItLeadsOrOversees(t *testing.T) {
	human := principal.PrincipalHuman
	cases := []struct {
		name string
		ctx  context.Context
		want TeamWeekReach
	}{
		{"admin", seatReading(human, principal.RowScopeAll, holds{leads: true, oversees: true}), ReachesEveryTeam},
		{"management", seatReading(human, principal.RowScopeAll, holds{leads: true, oversees: true}), ReachesEveryTeam},
		{"manager", seatReading(human, principal.RowScopeTeam, holds{leads: true}), ReachesTeamsLed},
		{"rep", seatReading(human, principal.RowScopeOwn, holds{}), ReachesNoTeam},
		// Row scope all is not a claim to read a colleague's verdict.
		{"read_only", seatReading(human, principal.RowScopeAll, holds{}), ReachesNoTeam},
		{"ops", seatReading(human, principal.RowScopeAll, holds{}), ReachesNoTeam},
		// The grant, not the row scope, is what widens a seat that leads nobody.
		{"read_only granted oversight", seatReading(human, principal.RowScopeAll, holds{oversees: true}), ReachesEveryTeam},
		// Management with its grant removed by an operator leads only its teams.
		{"management without the grant", seatReading(human, principal.RowScopeAll, holds{leads: true}), ReachesTeamsLed},
		// A custom role granted team_lead leads the teams it is on.
		{"custom role granted team_lead", seatReading(human, principal.RowScopeTeam, holds{leads: true}), ReachesTeamsLed},
		// A seeded lead whose team_lead grant was taken away leads nobody.
		{"manager without team_lead", seatReading(human, principal.RowScopeTeam, holds{}), ReachesNoTeam},
		// The week carries deal totals, so a revoked deal read closes both arms.
		{"manager without deal read", seatWithoutDeals(principal.RowScopeTeam, holds{leads: true}), ReachesNoTeam},
		{"management without deal read", seatWithoutDeals(principal.RowScopeAll, holds{leads: true, oversees: true}), ReachesNoTeam},
		// An own-scoped seat would read about colleagues whose rows it cannot.
		{"own-scoped with the grant", seatReading(human, principal.RowScopeOwn, holds{leads: true, oversees: true}), ReachesNoTeam},
		// Coaching is a human's act; an agent holding a lead's grant leads nobody.
		{"agent with a lead's grant", seatReading(principal.PrincipalAgent, principal.RowScopeTeam, holds{leads: true}), ReachesNoTeam},
		{"buyer", seatReading(principal.PrincipalBuyer, principal.RowScopeAll, holds{leads: true, oversees: true}), ReachesNoTeam},
		{"system", principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"}), ReachesEveryTeam},
		{"no principal", context.Background(), ReachesNoTeam},
	}
	for _, tc := range cases {
		if got := TeamWeekReachOf(tc.ctx); got != tc.want {
			t.Errorf("%s reaches %d, want %d", tc.name, got, tc.want)
		}
	}
}
