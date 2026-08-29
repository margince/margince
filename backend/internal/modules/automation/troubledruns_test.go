// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

// The troubled-runs refusal, proven above the query: the store here has no
// database, so a refusal that did not precede the read would panic rather
// than pass. The admit arm lives in the integration lane, where the join's
// SQL is the subject.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTroubledRunsRefusesAReaderWithoutTheAutomationGrant(t *testing.T) {
	store := NewAutomationStore(nil)
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:x", UserID: ids.NewV7(),
	})
	if _, err := store.TroubledRuns(ctx, time.Time{}, 8); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("TroubledRuns without the grant = %v, want the permission sentinel", err)
	}
}
