// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

func assertOwnersAgentKeepsTheSeat(t *testing.T, agent principal.Principal, owner ids.UUID) {
	t.Helper()
	if agent.SeatType != principal.SeatRead {
		t.Errorf("seat = %q, want read: an agent acting for a read seat stays a read seat", agent.SeatType)
	}
	if agent.Type != principal.PrincipalAgent || agent.ID != marginceActorID {
		t.Errorf("actor = %s %q, want agent %q", agent.Type, agent.ID, marginceActorID)
	}
	if agent.OnBehalfOf != owner || agent.UserID != owner {
		t.Errorf("on behalf of %s as user %s, want both %s", agent.OnBehalfOf, agent.UserID, owner)
	}
	if _, writes := agent.Scopes[principal.ScopeWrite]; !writes || len(agent.Scopes) != 1 {
		t.Errorf("scopes = %v, want write only", agent.Scopes)
	}
}

func TestOwnersAgentKeepsAReadSeatOwnersSeatAndNamesThem(t *testing.T) {
	t.Parallel()
	owner := ids.NewV7()
	member, err := authz.MemberPrincipal(context.Background(), readSeatOwner{}, ids.NewV7(), owner)
	if err != nil {
		t.Fatalf("MemberPrincipal: %v", err)
	}
	assertOwnersAgentKeepsTheSeat(t, ownersAgentPrincipal(member, marginceActorID), owner)
}

func TestOwnersAgentInATransactionKeepsTheSeatItReadThere(t *testing.T) {
	t.Parallel()
	owner := ids.NewV7()
	member := authz.HumanPrincipal(owner, authz.RBAC{}, principal.SeatRead)
	assertOwnersAgentKeepsTheSeat(t, ownersAgentPrincipal(member, marginceActorID), owner)
}
