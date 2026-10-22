// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCreationPreviewResolvesTheProposersLiveAuthority(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	e.GrantRole(t, e.Rep1, "rep")
	ctx := e.Admin()
	reader := newApprovalPreviewReader(e.DB())
	payload := map[string]any{"record_type": "lead", "fields": map[string]any{"full_name": "Proposed Buyer", "source": "manual"}}
	proposals := []crmcontracts.Approval{
		{Status: "pending", Kind: "create_record", ProposedBy: "human:" + e.Rep1.String(), TargetEntityType: new("lead"), ProposedChange: &payload},
		{Status: "pending", Kind: "create_record", ProposedBy: "agent:proposal", OnBehalfOf: new(crmcontracts.Id(e.Rep1)), TargetEntityType: new("lead"), ProposedChange: &payload},
	}
	for _, proposal := range proposals {
		actor, err := reader.creationAuthority(ctx, proposal)
		if err != nil {
			t.Fatal(err)
		}
		if actor.UserID != e.Rep1 || !actor.Permissions.Objects["lead"].Create || !actor.SeatType.CanMutate() {
			t.Fatalf("proposer authority: %+v", actor)
		}
		if actor.Type == principal.PrincipalAgent && actor.OnBehalfOf != actor.UserID {
			t.Fatal("agent lost its granting human")
		}
	}
	service := identity.NewService(e.Pool)
	if err := service.ChangeUserRole(ctx, adminIdentity(e), ids.From[ids.UserKind](e.Rep1), "read_only"); err != nil {
		t.Fatal(err)
	}
	for _, proposal := range proposals {
		if _, err := reader.creationAuthority(ctx, proposal); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Fatalf("reviewer authority replaced revoked proposer grant: %v", err)
		}
	}
	if err := service.ChangeUserRole(ctx, adminIdentity(e), ids.From[ids.UserKind](e.Rep1), "rep"); err != nil {
		t.Fatal(err)
	}
	for _, proposal := range proposals {
		if _, err := reader.creationAuthority(ctx, proposal); err != nil {
			t.Fatalf("restored proposer grant: %v", err)
		}
	}
	if err := service.DeactivateUser(ctx, adminIdentity(e), identity.DeactivateUserInput{UserID: ids.From[ids.UserKind](e.Rep1)}); err != nil {
		t.Fatal(err)
	}
	for _, proposal := range proposals {
		if _, err := reader.creationAuthority(ctx, proposal); !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("deactivated proposer: %v", err)
		}
		preview, err := reader.read(ctx, proposal)
		if err != nil || preview == nil || preview.State != "incomplete" || preview.Details != nil {
			t.Fatalf("inactive proposer preview: %+v,%v", preview, err)
		}
	}
}
