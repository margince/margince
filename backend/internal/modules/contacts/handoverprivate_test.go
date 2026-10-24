// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A caller without member administration neither counts nor moves a member's
// private records; the refusal comes before any row is read.
func TestOnlyAMemberAdministratorReachesAMembersPrivateRecords(t *testing.T) {
	rep := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:rep", UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"contact": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	member, receiver := ids.New[ids.UserKind](), ids.New[ids.UserKind]()

	if _, err := OwnerPrivateCountsTx(rep, nil, member); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("counting without the grant → %v, want permission denied", err)
	}
	if _, err := HandOverOwnerPrivateTx(rep, nil, member, receiver); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("moving without the grant → %v, want permission denied", err)
	}
}
