// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestDisqualificationPreviewRequiresTheWritersRowAuthority(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	lead, inserted, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Shared lead"), Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.otherRep))})
	if err != nil || !inserted || lead.Version == nil {
		t.Fatalf("lead=%+v inserted=%t err=%v", lead, inserted, err)
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	actor.Permissions.RowScope = principal.RowScopeOwn
	own := principal.WithActor(ctx, actor)
	id := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	if _, err := e.store.GetLead(own, id, storekit.LiveOnly); err != nil {
		t.Fatalf("fixture is not readable: %v", err)
	}
	if preview, err := e.store.PreviewDisqualifyLead(own, id, DisqualifyLeadInput{}, *lead.Version); !errors.Is(err, apperrors.ErrPermissionDenied) || preview != nil {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	if _, err := e.store.DisqualifyLead(own, id, DisqualifyLeadInput{}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("writer=%v", err)
	}
	current, err := e.store.GetLead(ctx, id, storekit.LiveOnly)
	if err != nil || current.ArchivedAt != nil || current.Status != lead.Status {
		t.Fatalf("refusal changed lead: %+v err=%v", current, err)
	}
}
