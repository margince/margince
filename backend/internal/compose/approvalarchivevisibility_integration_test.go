// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestDealArchiveScopeRefusesAnUnreadableStakeholder(t *testing.T) {
	e := integration.Setup(t)
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["relationship"] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	ctx := e.As(e.AdminUser, nil, perms)
	store := deals.NewStore(e.DB(), DealsInstallation())
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal, err := store.CreateDeal(ctx, deals.CreateDealInput{Name: "Archive scope", PipelineID: pipeline, StageID: stage, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.DealKind](ids.UUID(deal.Id))
	contact, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: "Private stakeholder", Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.Rep3))})
	if err != nil {
		t.Fatal(err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	edge, err := e.Contacts.CreateRelationship(ctx, contacts.CreateRelationshipInput{Kind: "deal_stakeholder", ContactID: &contactID, DealID: &id, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	reader := e.As(e.Rep1, nil, perms)
	scope, err := store.PreviewDealArchive(reader, id, nil)
	if err != nil || len(scope.Relationships) != 1 || scope.Relationships[0] != edge.ID {
		t.Fatalf("readable scope=%+v err=%v", scope, err)
	}
	if _, err := e.Contacts.UpdateContact(ctx, contactID, contacts.UpdateContactInput{Visibility: new("owner")}); err != nil {
		t.Fatal(err)
	}
	scope, err = store.PreviewDealArchive(reader, id, nil)
	if !errors.Is(err, apperrors.ErrNotFound) || len(scope.Relationships) != 0 {
		t.Fatalf("hidden scope=%+v err=%v", scope, err)
	}
	delete(perms.Objects, "relationship")
	scope, err = store.PreviewDealArchive(e.As(e.Rep1, nil, perms), id, nil)
	if !errors.Is(err, apperrors.ErrPermissionDenied) || len(scope.Relationships) != 0 {
		t.Fatalf("denied scope=%+v err=%v", scope, err)
	}
}
