// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// The bounce-lane read's refusal, proven above the query: the store here has
// no database at all, so a refusal that did not precede the read would panic
// rather than pass.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestHardBouncesForRefusesACallerWithNoPersonBehindIt(t *testing.T) {
	store := NewStore(nil, nil, nil)
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	if _, err := store.HardBouncesFor(ctx, time.Time{}, 8); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("HardBouncesFor with no person = %v, want the permission sentinel the lane withholds on", err)
	}
}
