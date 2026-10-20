// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCreationActorRequiresAnIdentifiableOwner(t *testing.T) {
	for _, approval := range []crmcontracts.Approval{
		{ProposedBy: "human:00000000-0000-0000-0000-000000000000"},
		{ProposedBy: "agent:assistant"},
		{ProposedBy: "agent:assistant", OnBehalfOf: new(crmcontracts.Id{})},
	} {
		if _, err := creationActor(approval); !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Errorf("unidentifiable creator %s: %v", approval.ProposedBy, err)
		}
	}
	user := ids.NewV7()
	for _, approval := range []crmcontracts.Approval{
		{ProposedBy: "human:" + user.String()},
		{ProposedBy: "agent:assistant", OnBehalfOf: new(crmcontracts.Id(user))},
	} {
		actor, err := creationActor(approval)
		if err != nil || actor.UserID != user {
			t.Errorf("creator owner: %+v, %v", actor, err)
		}
	}
}

func TestCreationActorPreservesTheAuthenticatedAgentsOwner(t *testing.T) {
	authenticated := (identity.AgentIdentity{
		PassportID: ids.New[ids.PassportKind](), OnBehalfOf: ids.New[ids.UserKind](),
	}).Principal()
	restored, err := creationActor(crmcontracts.Approval{
		ProposedBy: authenticated.ID, OnBehalfOf: new(crmcontracts.Id(authenticated.OnBehalfOf)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if restored.Type != authenticated.Type || restored.ID != authenticated.ID || restored.UserID != authenticated.UserID || restored.OnBehalfOf != authenticated.OnBehalfOf {
		t.Fatalf("restored creator changed ownership: %+v, want %+v", restored, authenticated)
	}
}

func TestSystemCreationHasNoBorrowedHumanAuthority(t *testing.T) {
	actor, err := (approvalPreviewReader{}).creationAuthority(context.Background(), crmcontracts.Approval{ProposedBy: "system:workflow", TargetEntityType: new("company")})
	if err != nil || actor.Type != principal.PrincipalSystem || !actor.UserID.IsZero() || !actor.OnBehalfOf.IsZero() {
		t.Fatalf("system creator: %+v,%v", actor, err)
	}
}
