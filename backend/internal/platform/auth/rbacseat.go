// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AllowsWrite is Allows under the seat ceiling, for a control a read offers: a
// read seat writes nothing, whatever its role grants. It records nothing.
func AllowsWrite(ctx context.Context, object string, action principal.Action) bool {
	p, err := rbacActor(ctx)
	if err != nil {
		return false
	}
	return seatAdmitsWrites(p) && Allows(ctx, object, action)
}

// seatAdmitsWrites is the seat ceiling every offered write sits under. The
// system principal holds no seat and is trusted by construction.
func seatAdmitsWrites(p principal.Principal) bool {
	return p.Type == principal.PrincipalSystem || p.SeatType.CanMutate()
}
