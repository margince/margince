// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Work an agent queued, and a run that authenticated once, both act under the
// passport's areas as they stand now, as every other call does.

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

var salesWorkOnly = principal.WorkAreas{Limited: true, Chosen: []string{"business"}}

func limitSeededPassport(t *testing.T, e *integration.Env, areas principal.WorkAreas) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	owner := identity.Identity{UserID: ids.From[ids.UserKind](e.Rep1), WorkspaceID: ids.From[ids.WorkspaceKind](e.WS)}
	human := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1})
	if err := identity.NewService(e.Pool).SetPassportAreas(human, owner, ids.From[ids.PassportKind](e.AgentPassport), areas); err != nil {
		t.Fatalf("limiting the seeded passport: %v", err)
	}
}

func TestQueuedWorkActsUnderThePassportsAreas(t *testing.T) {
	e := integration.Setup(t)
	limitSeededPassport(t, e, salesWorkOnly)
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	stored := migration.StoredActor{Type: principal.PrincipalAgent, ID: "agent:queued", UserID: e.Rep1, PassportID: e.AgentPassport}

	actor, err := storedActor{authority: identity.NewService(e.Pool)}.rebuild(ctx, stored, principal.ScopeWrite)
	if err != nil {
		t.Fatalf("rebuilding the queued agent: %v", err)
	}
	if !actor.Areas.Limited || !slices.Equal(actor.Areas.Chosen, salesWorkOnly.Chosen) {
		t.Fatalf("the queued agent acts under areas %+v, want its passport's sales work only", actor.Areas)
	}
}

func TestARunningSessionIsHeldToAreasNarrowedAfterItAuthenticated(t *testing.T) {
	e := integration.Setup(t)
	stamped := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:run", SeatType: principal.SeatFull,
		PassportID: e.AgentPassport, OnBehalfOf: e.Rep1, Scopes: principal.NewScopeSet(principal.ScopeRead),
	})
	gate := auth.NewGate(identity.NewService(e.Pool))
	spec := mcp.ToolSpec{Name: "list_roles", RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute, Areas: []string{"admin_settings"}}
	if _, err := gate.Admit(stamped, spec, func() (mcp.TierResolverInput, error) { return mcp.TierResolverInput{}, nil }); err != nil {
		t.Fatalf("before the change, at all areas → %v, want admitted", err)
	}

	limitSeededPassport(t, e, salesWorkOnly)
	_, err := gate.Admit(stamped, spec, func() (mcp.TierResolverInput, error) { return mcp.TierResolverInput{}, nil })
	var off *auth.AreaOffError
	if !errors.As(err, &off) || off.Area != "admin_settings" {
		t.Fatalf("the next call after its passport was narrowed → %v, want refused in admin settings", err)
	}
}

func TestWorkDoneForAMemberReadsInTheirLanguage(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	human := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1})
	if _, err := identity.NewService(e.Pool).SaveMyLocale(principal.WithCorrelationID(human, ids.NewV7()), "vi"); err != nil {
		t.Fatalf("saving the member's language: %v", err)
	}
	for _, stored := range []migration.StoredActor{
		{Type: principal.PrincipalHuman, UserID: e.Rep1},
		{Type: principal.PrincipalAgent, ID: "agent:queued", UserID: e.Rep1, PassportID: e.AgentPassport},
		{Type: principal.PrincipalAgent, ID: "agent:passportless", UserID: e.Rep1},
	} {
		actor, err := storedActor{authority: identity.NewService(e.Pool)}.rebuild(ctx, stored, principal.ScopeWrite)
		if got := systemtext.Locale(principal.WithActor(ctx, actor)); err != nil || got != textlang.Vietnamese {
			t.Errorf("work resumed as %+v reads system text in %q (%v), want vi", stored, got, err)
		}
	}
	owned, err := ownersAgent(ctx, identity.NewService(e.Pool), e.Rep1, marginceActorID)
	if err != nil || systemtext.Locale(owned) != textlang.Vietnamese {
		t.Errorf("the agent acting for the owner does not read the owner's language vi: %v", err)
	}
}
