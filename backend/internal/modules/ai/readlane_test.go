// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type readLaneStub interface{ name() string }

type namedLane struct{}

func (namedLane) name() string { return "lane" }

func TestAReadStartsAModelCallOnlyForACallerWhoMaySpend(t *testing.T) {
	read := principal.NewScopeSet(principal.ScopeRead)
	enrich := principal.NewScopeSet(principal.ScopeRead, principal.ScopeEnrich)
	for name, tc := range map[string]struct {
		ctx      context.Context
		withheld bool
	}{
		"nobody signed in":        {context.Background(), false},
		"a human":                 {principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman}), false},
		"a system pass":           {principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem}), false},
		"an agent with enrich":    {principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalAgent, Scopes: enrich}), false},
		"an agent without enrich": {principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalAgent, Scopes: read}), true},
	} {
		var lane readLaneStub = namedLane{}
		got, withheld := ReadLane(tc.ctx, lane)
		if withheld != tc.withheld || (got == nil) != tc.withheld {
			t.Errorf("%s: lane = %v, withheld = %t, want withheld = %t", name, got, withheld, tc.withheld)
		}
	}
}
