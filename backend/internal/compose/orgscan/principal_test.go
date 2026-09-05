// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package orgscan

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// grants is an authority that answers one snapshot, or refuses.
type grants struct {
	rbac authz.RBAC
	seat principal.SeatType
	err  error
}

func (g grants) EffectiveAuthority(context.Context, ids.UUID, ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return g.rbac, g.seat, g.err
}

// The worker runs as the reader themselves, not as a system with their name
// on it: the principal it binds carries the reader's own grants and teams,
// and the scan row is the correlation every trace joins on.
func TestTheWorkerRunsAsTheReaderWithTheScanAsItsTrace(t *testing.T) {
	viewer := ids.New[ids.UserKind]()
	team := ids.NewV7()
	scan := ids.NewV7()
	authority := grants{
		rbac: authz.RBAC{Permissions: principal.Permissions{RoleKeys: []string{"rep"}}, TeamIDs: []ids.UUID{team}},
		seat: principal.SeatFull,
	}
	ctx, err := WorkerContext(principal.WithWorkspaceID(t.Context(), ids.NewV7()), authority, viewer, scan)
	if err != nil {
		t.Fatalf("worker context: %v", err)
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID != viewer.UUID || actor.SeatType != principal.SeatFull {
		t.Errorf("actor = %+v, want the viewer's own human principal", actor)
	}
	if len(actor.TeamIDs) != 1 || actor.TeamIDs[0] != team || len(actor.Permissions.RoleKeys) != 1 {
		t.Errorf("actor carries teams %v and roles %v, want the authority's snapshot", actor.TeamIDs, actor.Permissions.RoleKeys)
	}
	if got, ok := principal.CorrelationID(ctx); !ok || got != scan {
		t.Errorf("correlation = %v, want the scan row %v", got, scan)
	}
}

func TestTheWorkerRefusesToRunWithoutAWorkspaceOrAnAuthority(t *testing.T) {
	viewer := ids.New[ids.UserKind]()
	if _, err := WorkerContext(t.Context(), grants{}, viewer, ids.NewV7()); err == nil {
		t.Error("a job context with no workspace was bound to a principal")
	}
	refused := errors.New("no such human")
	_, err := WorkerContext(principal.WithWorkspaceID(t.Context(), ids.NewV7()), grants{err: refused}, viewer, ids.NewV7())
	if !errors.Is(err, refused) {
		t.Errorf("err = %v, want the authority's refusal carried", err)
	}
}
