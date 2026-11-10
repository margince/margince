// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Who may write or read a seat's picture is decided before any query, so these hold on a
// service with no database behind it.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func actingAs(kind principal.PrincipalType, id string, user ids.UUID) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{Type: kind, ID: id, UserID: user})
}

func TestOnlyTheSeatItselfSetsItsPicture(t *testing.T) {
	t.Parallel()
	svc := NewServiceFor(nil)
	seat := ids.NewV7()
	agent := actingAs(principal.PrincipalAgent, "agent:test", seat)
	if _, _, err := svc.SetMyAvatar(agent, "ws/avatar/agent.png"); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an agent uploading its user's picture: %v, want permission denied", err)
	}
	if _, _, err := svc.RemoveMyAvatar(agent); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an agent deleting its user's picture: %v, want permission denied", err)
	}
	member := actingAs(principal.PrincipalHuman, "human:"+seat.String(), seat)
	user := ids.From[ids.UserKind](seat)
	if _, _, err := svc.FillAvatarFromProvider(member, user, "ws/avatar/x.png"); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a member calling the provider fill: %v, want permission denied", err)
	}
	if _, _, err := svc.ProviderMayFill(member, user); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a member asking whether a provider may fill: %v, want permission denied", err)
	}
}

func TestADealRoomBuyerReadsNoPicture(t *testing.T) {
	t.Parallel()
	buyer := actingAs(principal.PrincipalBuyer, "buyer:room-guest", ids.Nil)
	if _, err := NewServiceFor(nil).SeatAvatarKey(buyer, ids.New[ids.UserKind]()); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a Deal Room buyer reading a picture: %v, want permission denied", err)
	}
}
