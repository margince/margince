// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

// The refusals both personal reads make ABOVE any query — the store here
// holds no database, so a refusal that did not precede the read would panic
// rather than pass. A notice is one PERSON's: a system or agent principal
// carrying that person's id is still not them.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestThePersonalReadsRefuseAnyPrincipalThatIsNotThePersonThemselves(t *testing.T) {
	store := NewStore(nil)
	human := ids.NewV7()
	for _, tc := range []struct {
		name string
		p    principal.Principal
	}{
		{"system principal carrying a user id", principal.Principal{Type: principal.PrincipalSystem, ID: "system:x", UserID: human}},
		{"agent principal carrying a user id", principal.Principal{Type: principal.PrincipalAgent, ID: "agent:x", UserID: human, OnBehalfOf: human}},
		{"human with no user id", principal.Principal{Type: principal.PrincipalHuman, ID: "human:x"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := principal.WithActor(context.Background(), tc.p)
			if _, err := store.UnreadFor(ctx, 8); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("UnreadFor = %v, want the permission sentinel", err)
			}
			if err := store.MarkRead(ctx, ids.NewV7()); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("MarkRead = %v, want the permission sentinel", err)
			}
		})
	}
}
