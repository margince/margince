// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package automation

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A rule whose owner left is dead until someone else owns it: the owner change
// has to move the owner, count as a change of the rule, and refuse a seat the
// caller could not make an owner.
func TestAnAutomationCanBeHandedToAnotherMember(t *testing.T) {
	fx := setupAutomationDB(t)
	db := database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws))
	ctx := fx.humanCtx(fx.rep1, principal.RowScopeAll)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("fixture has no actor")
	}
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Create: true}
	ctx = principal.WithActor(ctx, actor)
	store := NewAutomationStore(db)
	created, err := store.Create(ctx, CreateAutomationInput{Key: noActivityReminderName, Name: "Nudge", Params: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}

	next := ids.From[ids.UserKind](fx.rep2)
	handed, err := store.Update(ctx, created.ID, UpdateAutomationInput{OwnerID: &next})
	if err != nil {
		t.Fatal(err)
	}
	if handed.OwnerID == nil || *handed.OwnerID != next {
		t.Fatalf("owner after the hand-over = %v, want %v", handed.OwnerID, next)
	}
	if handed.Version != created.Version+1 {
		t.Fatalf("version = %d, want %d: an owner change is a change of the rule", handed.Version, created.Version+1)
	}

	again, err := store.Update(ctx, created.ID, UpdateAutomationInput{OwnerID: &next})
	if err != nil || again.Version != handed.Version {
		t.Fatalf("handing to the current owner gave version %d, err %v; want no change", again.Version, err)
	}

	stranger := ids.From[ids.UserKind](ids.NewV7())
	var refused *auth.AssigneeNotAllowedError
	if _, err := store.Update(ctx, created.ID, UpdateAutomationInput{OwnerID: &stranger}); !errors.As(err, &refused) {
		t.Fatalf("handing to a seat that does not exist gave %v, want owner_not_assignable", err)
	}
}
