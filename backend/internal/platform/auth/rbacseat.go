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
	return SeatAdmitsWrites(ctx) && Allows(ctx, object, action)
}

// SeatAdmitsWrites is the seat ceiling alone, for a refusal that names the seat
// apart from the role.
func SeatAdmitsWrites(ctx context.Context) bool {
	p, err := rbacActor(ctx)
	return err == nil && seatAdmitsWrites(p)
}

// seatAdmitsWrites is the seat ceiling every offered write sits under. The
// system principal holds no seat and is trusted by construction.
func seatAdmitsWrites(p principal.Principal) bool {
	return p.Type == principal.PrincipalSystem || p.SeatType.CanMutate()
}
