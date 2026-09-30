// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

// The refusal the overtaking pass makes ABOVE any query — the store here holds
// no database, so a guard that did not precede the read would panic rather
// than pass.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The seat is never a parameter on a personal entry point, so the door that
// writes another seat's read-state is a system pass and nothing else.
func TestOvertakingRefusesAnyPrincipalThatIsNotTheSystem(t *testing.T) {
	store := NewStore(nil)
	human := ids.NewV7()
	taking := []Overtaking{{Approval: ids.New[ids.ApprovalKind]()}}
	for _, tc := range []struct {
		name string
		bind func(context.Context) context.Context
	}{
		{"a human, even one who could decide it", func(ctx context.Context) context.Context {
			return principal.WithActor(ctx, principal.Principal{
				Type: principal.PrincipalHuman, ID: "human:" + human.String(), UserID: human,
			})
		}},
		{"an agent acting for one", func(ctx context.Context) context.Context {
			return principal.WithActor(ctx, principal.Principal{
				Type: principal.PrincipalAgent, ID: "agent:copilot", UserID: human, OnBehalfOf: human,
			})
		}},
		{"nobody bound at all", func(ctx context.Context) context.Context { return ctx }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.bind(context.Background())
			if _, err := store.OvertakeApprovalNotices(ctx, taking); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("OvertakeApprovalNotices = %v, want the permission sentinel", err)
			}
			if _, err := store.StandingApprovalReferences(ctx); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("StandingApprovalReferences = %v, want the permission sentinel", err)
			}
		})
	}
}
