// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An agent's case merge runs through the composed tool registry, so it takes
// the carriers the HTTP door takes.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/authz/authztest"
)

// caseRepSeat resolves every seat to the own-scope rep the case suites use.
type caseRepSeat struct{}

func (caseRepSeat) EffectiveRBAC(context.Context, ids.UUID, ids.UUID) (authz.RBAC, error) {
	return authz.RBAC{Permissions: caseRepPerms}, nil
}

func (caseRepSeat) SeatType(context.Context, ids.UUID, ids.UUID) (principal.SeatType, error) {
	return principal.SeatFull, nil
}

func (s caseRepSeat) AdmittedAuthority(ctx context.Context, ws, human, _ ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return authztest.AdmittedFromPair(ctx, ws, human, s.EffectiveRBAC, s.SeatType)
}

func TestAnAgentsCaseMergeKeepsEveryOtherOwnersAccess(t *testing.T) {
	c := setupCases(t)
	members := c.seedCase(t, c.e.Rep1, c.e.Rep2, c.e.Rep3)
	c.setPolicy(t, true)
	read := c.read(c.rep(c.e.Rep1), t, members[0])
	registry := registryWithGate(c.e.DB(), auth.NewGate(caseRepSeat{}), nil,
		SendPath{}, companyEnricher{}, nil, nil, nil, nil, nil, registryFeatures{})

	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), c.e.WS), ids.NewV7())
	agent := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:case-merge", SeatType: principal.SeatFull,
		OnBehalfOf: c.e.Rep1, UserID: c.e.Rep1,
		Scopes:      principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite),
		Permissions: caseRepPerms,
	})
	args, err := json.Marshal(map[string]any{
		"record_type": "contact", "contact_id": members[0], "member_ids": members,
		"survivor_id": members[0], "owner_id": c.e.Rep1, "revision": read.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Invoke(agent, "merge_records", args); err != nil {
		t.Fatalf("the agent's case merge: %v", err)
	}
	for _, owner := range []ids.UUID{c.e.Rep2, c.e.Rep3} {
		if got := grantAccess(t, c.e, members[0], owner); got != "write" {
			t.Errorf("after the agent's merge owner %s holds %q on the survivor, want write", owner, got)
		}
	}
}
