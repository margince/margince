// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestMarkAssertableFollowsTheWriteRoutesRule(t *testing.T) {
	mine, foreign := ids.NewV7(), ids.NewV7()
	scope := func(kind crmcontracts.AnalyticsScopeKind, id *ids.UUID) crmcontracts.AnalyticsScope {
		out := crmcontracts.AnalyticsScope{Kind: kind}
		if id != nil {
			out.Id = ptrUUID(*id)
		}
		return out
	}
	cases := []struct {
		name  string
		actor principal.Principal
		scope crmcontracts.AnalyticsScope
		want  bool
	}{
		{"all-scope seat, workspace", actorWithScope(principal.RowScopeAll), scope(crmcontracts.AnalyticsScopeKindWorkspace, nil), true},
		{"all-scope seat, any team", actorWithScope(principal.RowScopeAll), scope(crmcontracts.AnalyticsScopeKindTeam, &foreign), true},
		{"team seat, workspace", actorWithScope(principal.RowScopeTeam, mine), scope(crmcontracts.AnalyticsScopeKindWorkspace, nil), false},
		{"team seat, own team", actorWithScope(principal.RowScopeTeam, mine), scope(crmcontracts.AnalyticsScopeKindTeam, &mine), true},
		{"team seat, unrelated team", actorWithScope(principal.RowScopeTeam, mine), scope(crmcontracts.AnalyticsScopeKindTeam, &foreign), false},
		{"team seat, managed teams", actorWithScope(principal.RowScopeTeam, mine), scope(crmcontracts.AnalyticsScopeKindManagedTeams, nil), false},
		{"own seat, a team", actorWithScope(principal.RowScopeOwn), scope(crmcontracts.AnalyticsScopeKindTeam, &mine), false},
		{"own seat, themselves", actorWithScope(principal.RowScopeOwn), scope(crmcontracts.AnalyticsScopeKindOwner, &mine), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			markAssertable(tc.actor, &tc.scope)
			if tc.scope.Assertable == nil || *tc.scope.Assertable != tc.want {
				t.Fatalf("assertable = %v, want %v", tc.scope.Assertable, tc.want)
			}
		})
	}
}
