// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAReadSharedWithAnotherPrincipalRequiresBothGrants(t *testing.T) {
	reader := func(allowed bool) principal.Principal {
		id := ids.NewV7()
		return principal.Principal{
			Type: principal.PrincipalHuman, ID: principal.HumanIDPrefix + id.String(), UserID: id,
			Permissions: principal.Permissions{
				RowScope: principal.RowScopeOwn,
				Objects:  map[string]principal.ObjectGrant{"contact": {Read: allowed}},
			},
		}
	}
	for _, tc := range []struct {
		name          string
		caller, other bool
	}{
		{"both allow", true, true},
		{"caller refuses", false, true},
		{"other refuses", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := principal.WithActor(context.Background(), reader(tc.caller))
			err := RequireReadForReaders(ctx, "contact", reader(tc.other))
			if tc.caller && tc.other {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("shared read refusal = %v", err)
			}
		})
	}
}
