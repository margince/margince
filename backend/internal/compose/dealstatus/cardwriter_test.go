// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTheCardIsWrittenUnderTheLaneRuleEveryReadShares(t *testing.T) {
	user, passport := ids.NewV7(), ids.NewV7()
	enrich := principal.NewScopeSet(principal.ScopeRead, principal.ScopeEnrich)
	read := principal.NewScopeSet(principal.ScopeRead)
	for name, tc := range map[string]struct {
		actor principal.Principal
		want  bool
	}{
		"a user": {principal.Principal{Type: principal.PrincipalHuman, UserID: user}, true},
		"their agent holding enrich": {principal.Principal{
			Type: principal.PrincipalAgent, UserID: user, PassportID: passport, OnBehalfOf: user, Scopes: enrich,
		}, true},
		"their agent without enrich": {principal.Principal{
			Type: principal.PrincipalAgent, UserID: user, PassportID: passport, OnBehalfOf: user, Scopes: read,
		}, false},
		"automatic work holding enrich": {principal.Principal{
			Type: principal.PrincipalAgent, ID: "agent:enrich", UserID: user, Scopes: enrich,
		}, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := mayWriteTheCard(principal.WithActor(context.Background(), tc.actor)); got != tc.want {
				t.Fatalf("mayWriteTheCard = %t, want %t", got, tc.want)
			}
		})
	}
}
