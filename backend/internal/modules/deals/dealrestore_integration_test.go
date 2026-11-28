// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asEditor holds edit rights on deals and no delete, over userID's own deals
// or over every deal.
func (e *configEnv) asEditor(userID ids.UUID, scope principal.RowScope) context.Context {
	ctx := principal.WithCorrelationID(context.Background(), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + userID.String(), UserID: userID,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"}, RowScope: scope,
			Objects: map[string]principal.ObjectGrant{"deal": {Read: true, Update: true}},
		},
	})
}

func (e *configEnv) restoreAs(ctx context.Context, deal ids.DealID) error {
	return e.store.Tx(ctx, func(tx pgx.Tx) error {
		_, err := e.store.RestoreDealTx(ctx, tx, deal, nil, noLinksRestore)
		return err
	})
}

// Restore asks what the archive asks. With edit rights alone, the owner
// restores only an archive they made, so a manager's archive stays until a
// delete holder restores it. An editor of every deal who owns none restores
// no archive.
func TestADealRestoreAsksTheArchivesOwnAuthority(t *testing.T) {
	e := setupConfigEnv(t)
	deal, _, _ := seedOwnedDeal(t, e)
	owner := e.asEditor(e.admin, principal.RowScopeOwn)
	manager := e.asDealArchiver()
	actor, ok := principal.Actor(manager)
	if !ok {
		t.Fatal("the manager's context carries no actor")
	}
	actor.UserID = ids.NewV7()
	actor.ID = "human:" + actor.UserID.String()
	manager = principal.WithActor(manager, actor)

	if _, err := e.store.ArchiveDeal(manager, deal, nil); err != nil {
		t.Fatalf("the manager's archive: %v", err)
	}
	if _, refused := errors.AsType[*auth.RestoreAuthorityError](e.restoreAs(owner, deal)); !refused {
		t.Error("the owner restored the manager's archive, want restore_archiver_required")
	}
	if err := e.restoreAs(manager, deal); err != nil {
		t.Fatalf("the manager's restore: %v", err)
	}

	if _, err := e.store.ArchiveDeal(owner, deal, nil); err != nil {
		t.Fatalf("the owner's archive with edit rights: %v", err)
	}
	if _, refused := errors.AsType[*auth.RestoreAuthorityError](e.restoreAs(e.asEditor(ids.NewV7(), principal.RowScopeAll), deal)); !refused {
		t.Error("an editor of every deal who owns none restored it, want restore_archiver_required")
	}
	if err := e.restoreAs(owner, deal); err != nil {
		t.Fatalf("the owner's restore of their own archive: %v", err)
	}
}
