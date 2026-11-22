// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// readSeatResolver answers every member as a read seat holding the same grants.
type readSeatResolver struct{ authz.Resolver }

func (readSeatResolver) AdmittedAuthority(context.Context, ids.UUID, ids.UUID, ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return authz.RBAC{TeamIDs: []ids.UUID{ids.NewV7()}}, principal.SeatRead, nil
}

func TestConnectorActingKeepsTheGrantingMembersSeatAndNamesThem(t *testing.T) {
	t.Parallel()
	member := ids.From[ids.UserKind](ids.NewV7())
	ctx, err := connectorActing(principal.WithWorkspaceID(context.Background(), ids.NewV7()), readSeatResolver{}, "gmail", member)
	if err != nil {
		t.Fatalf("connectorActing: %v", err)
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("connectorActing bound no actor")
	}
	if actor.SeatType != principal.SeatRead {
		t.Errorf("seat = %q, want read: a connector acting for a read seat is a read seat", actor.SeatType)
	}
	if actor.Type != principal.PrincipalConnector || actor.ID != "connector:gmail" {
		t.Errorf("actor = %s %q, want connector connector:gmail", actor.Type, actor.ID)
	}
	if actor.OnBehalfOf != member.UUID || actor.UserID != member.UUID {
		t.Errorf("on behalf of %s as user %s, want both %s", actor.OnBehalfOf, actor.UserID, member.UUID)
	}
	if len(actor.TeamIDs) != 1 {
		t.Errorf("teams = %v, want the member's one team", actor.TeamIDs)
	}
}
