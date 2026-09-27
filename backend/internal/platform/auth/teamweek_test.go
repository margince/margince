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

// seatReading is a seat that reads deals, as every seeded role does.
func seatReading(kind principal.PrincipalType, scope principal.RowScope, role string, oversees bool) context.Context {
	return seatHolding(kind, scope, role, map[string]principal.ObjectGrant{
		objTeamOversight: {Read: oversees},
		"deal":           {Read: true},
	})
}

// seatWithoutDeals is seatReading with its deal read revoked.
func seatWithoutDeals(scope principal.RowScope, role string, oversees bool) context.Context {
	return seatHolding(principal.PrincipalHuman, scope, role, map[string]principal.ObjectGrant{
		objTeamOversight: {Read: oversees},
	})
}

func seatHolding(kind principal.PrincipalType, scope principal.RowScope, role string, objects map[string]principal.ObjectGrant) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: kind,
		ID:   "seat:test",
		Permissions: principal.Permissions{
			RoleKeys: []string{role},
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
		{"admin", seatReading(human, principal.RowScopeAll, "admin", true), ReachesEveryTeam},
		{"management", seatReading(human, principal.RowScopeAll, "management", true), ReachesEveryTeam},
		{"manager", seatReading(human, principal.RowScopeTeam, "manager", false), ReachesTeamsLed},
		{"rep", seatReading(human, principal.RowScopeOwn, "rep", false), ReachesNoTeam},
		// Row scope all is not a claim to read a colleague's verdict.
		{"read_only", seatReading(human, principal.RowScopeAll, "read_only", false), ReachesNoTeam},
		{"ops", seatReading(human, principal.RowScopeAll, "ops", false), ReachesNoTeam},
		// The grant, not the row scope, is what widens a seat that leads nobody.
		{"read_only granted oversight", seatReading(human, principal.RowScopeAll, "read_only", true), ReachesEveryTeam},
		// Management with its grant removed by an operator leads only its teams.
		{"management without the grant", seatReading(human, principal.RowScopeAll, "management", false), ReachesTeamsLed},
		// The week carries deal totals, so a revoked deal read closes both arms.
		{"manager without deal read", seatWithoutDeals(principal.RowScopeTeam, "manager", false), ReachesNoTeam},
		{"management without deal read", seatWithoutDeals(principal.RowScopeAll, "management", true), ReachesNoTeam},
		// An own-scoped seat would read about colleagues whose rows it cannot.
		{"own-scoped with the grant", seatReading(human, principal.RowScopeOwn, "admin", true), ReachesNoTeam},
		// Coaching is a human's act; an agent holding a lead's key leads nobody.
		{"agent with a lead's key", seatReading(principal.PrincipalAgent, principal.RowScopeTeam, "manager", false), ReachesNoTeam},
		{"buyer", seatReading(principal.PrincipalBuyer, principal.RowScopeAll, "admin", true), ReachesNoTeam},
		{"system", principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"}), ReachesEveryTeam},
		{"no principal", context.Background(), ReachesNoTeam},
	}
	for _, tc := range cases {
		if got := TeamWeekReachOf(tc.ctx); got != tc.want {
			t.Errorf("%s reaches %d, want %d", tc.name, got, tc.want)
		}
	}
}
