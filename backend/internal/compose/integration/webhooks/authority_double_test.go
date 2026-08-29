// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webhooks

// The authority double every case in this package hands the delivery path.

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// admittedFromPair answers the seam's admission read from a fixture's own two
// reads.
//
// One helper rather than the same body on each fixture. Every one of these
// doubles stands for the AUTHORITY seam and none for a passport's liveness —
// that is the gate suite's subject — so the passport half answers as live and
// whether the call is admitted is left to the double's own two reads. A double
// that refuses (deadAuthority) still refuses: its EffectiveRBAC answers
// not-found and that is the first read. Written out per type, the identical body
// appeared three times in this package alone, and a double that had to be
// corrected would have been corrected in one of them.
func admittedFromPair(
	ctx context.Context, ws, human ids.UUID,
	rbacOf func(context.Context, ids.UUID, ids.UUID) (authz.RBAC, error),
	seatOf func(context.Context, ids.UUID, ids.UUID) (principal.SeatType, error),
) (authz.RBAC, principal.SeatType, error) {
	rbac, err := rbacOf(ctx, ws, human)
	if err != nil {
		return authz.RBAC{}, "", err
	}
	seat, err := seatOf(ctx, ws, human)
	return rbac, seat, err
}
