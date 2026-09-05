// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package automation

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestEnablingADraftStarterClaimsItsOwnerOnce(t *testing.T) {
	fx := setupAutomationDB(t)
	db := database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws))
	ctx := fx.humanCtx(fx.rep1, principal.RowScopeAll)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("fixture has no actor")
	}
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Create: true}
	ctx = principal.WithActor(ctx, actor)
	if err := db.Tx(ctx, func(tx pgx.Tx) error { return SeedStarterAutomationsTx(ctx, tx) }); err != nil {
		t.Fatal(err)
	}
	var id ids.AutomationID
	var enabled bool
	var owner *ids.UUID
	if err := fx.owner.QueryRow(ctx, "SELECT id, enabled, owner_id FROM automation WHERE key = 'post_meeting_recap'").Scan(&id, &enabled, &owner); err != nil {
		t.Fatal(err)
	}
	if enabled || owner != nil {
		t.Fatal("draft starter starts enabled or invents an owner")
	}
	store := NewAutomationStore(db)
	enable := true
	denied := fx.humanCtx(fx.rep2, principal.RowScopeAll)
	if _, err := store.Update(denied, id, UpdateAutomationInput{Enabled: &enable}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("enabling without draft authority: %v", err)
	}
	if _, err := store.Update(ctx, id, UpdateAutomationInput{Enabled: &enable}); err != nil {
		t.Fatal(err)
	}
	if err := fx.owner.QueryRow(ctx, "SELECT owner_id FROM automation WHERE id = $1", id).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner == nil || *owner != fx.rep1 {
		t.Fatal("enabler did not become draft owner")
	}
	actor.UserID = fx.rep2
	if _, err := store.Update(principal.WithActor(ctx, actor), id, UpdateAutomationInput{Enabled: &enable}); err != nil {
		t.Fatal(err)
	}
	if err := fx.owner.QueryRow(context.Background(), "SELECT owner_id FROM automation WHERE id = $1", id).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner == nil || *owner != fx.rep1 {
		t.Fatal("a later toggle reassigned the draft's owner")
	}
	if count := fx.count(t, "SELECT count(*) FROM audit_log WHERE entity_id = $1 AND after ? 'owner_id'", id); count != 1 {
		t.Fatalf("ownership was audited %d times, want once", count)
	}
}
