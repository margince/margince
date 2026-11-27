// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migration

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ownImporter is ctx signed in as a seat that imports only its own records,
// as the human or as an agent acting for it.
func ownImporter(ctx context.Context, seat ids.UUID, kind principal.PrincipalType) context.Context {
	id := "human:" + seat.String()
	if kind == principal.PrincipalAgent {
		id = "agent:" + ids.NewV7().String()
	}
	return principal.WithActor(ctx, principal.Principal{
		Type: kind, ID: id, UserID: seat, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
			Objects: map[string]principal.ObjectGrant{ImportRunObject: {Create: true, Read: true, Update: true}},
		},
	})
}

// An agent's stamp names its passport, not its seat, so a run it opens is
// kept to the seat it acts for: the agent and its human read it, another
// seat does not, and a workspace importer reads every run.
func TestAnOwnImportRunIsReadByItsSeatWhoeverOpenedIt(t *testing.T) {
	admin, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	seat := ids.NewV7()
	agent := ownImporter(admin, seat, principal.PrincipalAgent)

	run := newDraft(agent, t, s, "")
	for name, ctx := range map[string]context.Context{
		"the agent": agent, "its human": ownImporter(admin, seat, principal.PrincipalHuman), "a workspace importer": admin,
	} {
		if _, err := s.GetStaged(ctx, run.ID); err != nil {
			t.Errorf("%s reading the agent's run: %v", name, err)
		}
	}
	if _, err := s.GetStaged(ownImporter(admin, ids.NewV7(), principal.PrincipalHuman), run.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("another seat reading the run: %v, want not found", err)
	}
}

// A source name an admin's run holds is free for a rep's own import, which
// neither waits on that run nor replaces it.
func TestAnOwnImportsSourceNameNeverMeetsAWorkspaceRun(t *testing.T) {
	admin, db := testWorkspaceCtx(t, adminImportRunGrant())
	s := NewRunStore(db)
	adminRun := toReport(admin, t, s, newDraft(admin, t, s, fairName))

	rep := ownImporter(admin, ids.NewV7(), principal.PrincipalHuman)
	if got := toReport(rep, t, s, newDraft(rep, t, s, fairName)); got.Status != StatusAwaitingApproval {
		t.Fatalf("the rep's run of the admin's name reads %s, want awaiting approval", got.Status)
	}
	if got := mustGetStaged(admin, t, s, adminRun.ID); got.Status != StatusAwaitingApproval {
		t.Errorf("the admin's run reads %s after the rep's test run, want it left alone", got.Status)
	}
}
