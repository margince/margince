// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReviewReadsRejectNonHumanPrincipalsWithExceptionGrants(t *testing.T) {
	for _, actor := range []principal.Principal{
		{Type: principal.PrincipalConnector, UserID: ids.NewV7()},
		{Type: principal.PrincipalSystem},
		{Type: principal.PrincipalAgent, UserID: ids.NewV7()},
		{Type: principal.PrincipalBuyer, UserID: ids.NewV7()},
	} {
		t.Run(string(actor.Type), func(t *testing.T) {
			actor.Permissions = principal.Permissions{
				Objects: map[string]principal.ObjectGrant{
					entityCommunicationException: {Create: true, Read: true},
				},
			}
			ctx := principal.WithActor(context.Background(), actor)
			// Admission must refuse before attempting any database access.
			store := NewStore(nil)
			if _, err := store.ReviewForReader(ctx, ids.NewV7()); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("reading a review: %v, want permission denied", err)
			}
			if _, _, err := store.AwaitingDecision(ctx, 100); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("reading the queue: %v, want permission denied", err)
			}
		})
	}
}
