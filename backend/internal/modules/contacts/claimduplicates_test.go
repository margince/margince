// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A refusal happens before any statement, so these cases need no database.
func TestTheArchivedClaimsTaskRefusesBeforeItReadsAnything(t *testing.T) {
	t.Parallel()
	human := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test", UserID: ids.NewV7(),
	})
	if _, err := deleteArchivedClaimsBatch(human, nil, datatask.BatchIn{Limit: 1}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a human ran the delete batch: %v, want a permission refusal", err)
	}
	system := principal.SystemActing(context.Background(), "system:test")
	var malformed *storekit.MalformedCursorError
	if _, err := deleteArchivedClaimsBatch(system, nil, datatask.BatchIn{Cursor: []byte(`"invalid"`), Limit: 1}); !errors.As(err, &malformed) {
		t.Errorf("a malformed cursor answered %v, want a malformed cursor", err)
	}
	if _, _, err := countArchivedClaims(human, nil, 10); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a reader without the data task grant counted the claims: %v", err)
	}
}
