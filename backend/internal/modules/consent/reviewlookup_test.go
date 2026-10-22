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

func reader(grants map[string]principal.ObjectGrant) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, UserID: ids.NewV7(),
		Permissions: principal.Permissions{Objects: grants},
	})
}

// The scheduled-send list asks only with activity:read, and the answer is
// refused before any database access for a caller without it.
func TestLiveReviewsForIntentsNeedsActivityRead(t *testing.T) {
	store := NewStore(nil)
	if _, err := store.LiveReviewsForIntents(reader(nil), []ids.UUID{ids.NewV7()}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a caller without activity:read: %v, want permission denied", err)
	}
}

// Asking about no messages answers an empty map without a query.
func TestLiveReviewsForNoIntentsIsEmpty(t *testing.T) {
	store := NewStore(nil)
	got, err := store.LiveReviewsForIntents(reader(map[string]principal.ObjectGrant{"activity": {Read: true}}), nil)
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("no messages answered %v, %v; want an empty map and no error", got, err)
	}
}
