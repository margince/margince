// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/authz/authztest"
)

// readSeatOwner resolves a member whose role writes contacts on a read seat —
// the case where a missing seat would let a standing write share act for them.
type readSeatOwner struct{}

func (readSeatOwner) EffectiveRBAC(context.Context, ids.UUID, ids.UUID) (authz.RBAC, error) {
	return authz.RBAC{Permissions: principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"contact": {Read: true, Update: true}},
		RowScope: principal.RowScopeOwn,
	}}, nil
}

func (readSeatOwner) SeatType(context.Context, ids.UUID, ids.UUID) (principal.SeatType, error) {
	return principal.SeatRead, nil
}

func (r readSeatOwner) AdmittedAuthority(ctx context.Context, ws, human, _ ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return authztest.AdmittedFromPair(ctx, ws, human, r.EffectiveRBAC, r.SeatType)
}

func TestTheGhostOwnerPassRunsUnderTheOwnersSeat(t *testing.T) {
	t.Parallel()
	owner := ids.NewV7()
	ctx, err := asGhostOwner(context.Background(), readSeatOwner{}, ids.NewV7(), owner)
	if err != nil {
		t.Fatalf("asGhostOwner: %v", err)
	}
	p, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("asGhostOwner bound no actor")
	}
	if p.SeatType != principal.SeatRead {
		t.Errorf("seat = %q, want read: the write-share arm drops only for a principal that says it is a read seat", p.SeatType)
	}
	if p.ID != "user:"+owner.String() || p.OnBehalfOf != owner || p.UserID != owner {
		t.Errorf("principal = %+v, want the owner's user id form acting on their own behalf", p)
	}
}
