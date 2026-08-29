// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package authztest is what a test double for the authority seam needs so it
// does not have to be written again in each package that has one.
package authztest

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// AdmittedFromPair answers the seam's admission read from a double's own two
// reads.
//
// It exists because the seam grew a third method and every double in the tree
// needed one — the same ten lines in nine packages, each of which would be
// corrected separately. A double cannot simply inherit it either: Go promotes an
// embedded method with the BASE receiver, so a base copy dispatches to the
// base's EffectiveRBAC rather than to the fixture's override, which is why the
// copies existed in the first place.
//
// The passport half answers as LIVE. Every one of these doubles stands for the
// authority seam and none for a credential's liveness — that is the gate suite's
// own subject — so whether the call is admitted is left to the double's two
// reads. A double that refuses still refuses: its EffectiveRBAC answers
// not-found and that is the first read.
func AdmittedFromPair(
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
