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
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7())
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

// Restore asks what the archive asks. Edit rights alone admit the deal's
// owner, and refuse a seat that may write every deal but owns none of them.
func TestADealRestoreAsksTheArchivesOwnAuthority(t *testing.T) {
	e := setupConfigEnv(t)
	deal, _, _ := seedOwnedDeal(t, e)
	if _, err := e.store.ArchiveDeal(e.asDealArchiver(), deal, nil); err != nil {
		t.Fatalf("archiving the deal: %v", err)
	}

	err := e.restoreAs(e.asEditor(ids.NewV7(), principal.RowScopeAll), deal)
	if refusal, isArchiveRule := errors.AsType[*auth.ArchiveAuthorityError](err); !isArchiveRule || refusal.SelfAssigned {
		t.Fatalf("an editor of every deal who owns none restores → %v, want archive_owner_required", err)
	}

	if err := e.restoreAs(e.asEditor(e.admin, principal.RowScopeOwn), deal); err != nil {
		t.Fatalf("the owner's restore with edit rights alone: %v", err)
	}
}
