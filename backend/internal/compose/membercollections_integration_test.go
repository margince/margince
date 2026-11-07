// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestRemovingAMemberMovesListsAndArchivesViews(t *testing.T) {
	e := integration.Setup(t)
	manager := memberManagerFor(t, e)
	store := collections.NewStore(e.DB())
	permissions := integration.AdminWithSignals
	permissions.Objects = maps.Clone(permissions.Objects)
	permissions.Objects["saved_view"] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	owner := e.As(e.Rep1, nil, permissions)
	list, err := store.CreateList(owner, collections.CreateListInput{Name: "Private shortlist", EntityType: "contact", ListType: "static", Sharing: "private"})
	if err != nil {
		t.Fatal(err)
	}
	view, err := store.CreateSavedView(owner, collections.CreateSavedViewInput{Name: "My filter", Resource: "contacts", Query: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := e.As(e.AdminUser, nil, integration.AdminWithSignals)
	actor := identity.Identity{UserID: ids.From[ids.UserKind](e.AdminUser), Roles: []string{"admin"}, SeatType: "full", Permissions: integration.AdminWithSignals}
	source := ids.From[ids.UserKind](e.Rep1)
	if err := manager.service.DeactivateUser(ctx, actor, identity.DeactivateUserInput{UserID: source}); err != nil {
		t.Fatal(err)
	}
	preview, err := manager.Preview(ctx, e.Rep1, &e.Rep2, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Blockers) != 0 || preview.Owned.List != 1 {
		t.Fatalf("private collection preview = %+v", preview)
	}
	if err := manager.service.RetireMember(ctx, actor, source, &e.Rep2, preview.Token, false); err != nil {
		t.Fatal(err)
	}
	if got := e.WsScalar(t, `SELECT owner_id::text FROM list WHERE id=@id`, pgx.StrictNamedArgs{"id": list.ID}); got != e.Rep2.String() {
		t.Fatalf("list owner = %s", got)
	}
	if got := e.WsScalar(t, `SELECT steward_id::text FROM list WHERE id=@id`, pgx.StrictNamedArgs{"id": list.ID}); got != e.Rep2.String() {
		t.Fatalf("list steward = %s", got)
	}
	if got := e.WsScalar(t, `SELECT (archived_at IS NOT NULL)::text FROM saved_view WHERE id=@id`, pgx.StrictNamedArgs{"id": view.ID}); got != "true" {
		t.Fatal("personal saved view was not archived")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='list' AND entity_id=@id AND action='update'`, pgx.StrictNamedArgs{"id": list.ID}); got != 1 {
		t.Fatalf("list handover audits = %d", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type'='list.updated' AND envelope->'entity'->>'id'=@id::text`, pgx.StrictNamedArgs{"id": list.ID}); got != 1 {
		t.Fatalf("list handover events = %d", got)
	}
}

func TestRemovingAMemberWithOnlyPersonalViewsNeedsNoReceiver(t *testing.T) {
	e := integration.Setup(t)
	manager := memberManagerFor(t, e)
	permissions := integration.AdminWithSignals
	permissions.Objects = maps.Clone(permissions.Objects)
	permissions.Objects["saved_view"] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	view, err := collections.NewStore(e.DB()).CreateSavedView(e.As(e.Rep1, nil, permissions), collections.CreateSavedViewInput{Name: "My contacts", Resource: "contacts", Query: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := e.As(e.AdminUser, nil, integration.AdminWithSignals)
	actor := identity.Identity{UserID: ids.From[ids.UserKind](e.AdminUser), Roles: []string{"admin"}, SeatType: "full", Permissions: integration.AdminWithSignals}
	source := ids.From[ids.UserKind](e.Rep1)
	if err := manager.service.DeactivateUser(ctx, actor, identity.DeactivateUserInput{UserID: source}); err != nil {
		t.Fatal(err)
	}
	preview, err := manager.Preview(ctx, e.Rep1, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Blockers) != 0 {
		t.Fatalf("personal view blocks removal: %+v", preview.Blockers)
	}
	if err := manager.service.RetireMember(ctx, actor, source, nil, preview.Token, false); err != nil {
		t.Fatal(err)
	}
	if got := e.WsScalar(t, `SELECT (archived_at IS NOT NULL)::text FROM saved_view WHERE id=@id`, pgx.StrictNamedArgs{"id": view.ID}); got != "true" {
		t.Fatal("personal view remains live")
	}
}
