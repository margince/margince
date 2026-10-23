// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Only a seated human has former teammates: a background pass has no team.
func TestOnlyASeatedHumanHasFormerTeammates(t *testing.T) {
	svc := &Service{}
	if _, err := svc.FormerTeammatesOfCaller(context.Background()); err == nil {
		t.Error("a caller with no principal was answered")
	}
	system := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"})
	if _, err := svc.FormerTeammatesOfCaller(system); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a system pass → %v, want permission denied", err)
	}
}

// Asking about nobody is answered without a read, and a caller who is not a
// member is refused before one.
func TestTheDepartedReadAsksNothingForNobody(t *testing.T) {
	svc := &Service{}
	member := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:rep", UserID: ids.NewV7(), SeatType: principal.SeatFull,
	})
	departed, err := svc.DepartedAmong(member, nil)
	if err != nil || len(departed) != 0 {
		t.Errorf("asking about nobody → %v, %v; want an empty answer", departed, err)
	}
	if _, err := svc.DepartedAmong(context.Background(), []ids.UserID{ids.New[ids.UserKind]()}); err == nil {
		t.Error("a caller with no principal was answered")
	}
}
