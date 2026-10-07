// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RequireActingForAHuman answers whether a human is behind THIS call, not which
// transport it arrived on: a human, or an agent whose passport names the human
// it acts for.
//
// A passport carries the human it was minted by (UserID, OnBehalfOf, the seat,
// the teams and the permissions all come from that human), so an agent call is
// already bounded by everything the same act is bounded by for a human: the
// RBAC the write needs, row-scope visibility, and the licensing ceiling. What
// is refused here is a call with NO human behind it: the system principal, a
// connector, and an agent carrying no on_behalf_of, which is a credential nobody
// lent.
//
// `what` completes the sentence "<what> by a contact", so a refusal names the
// act the caller attempted.
func RequireActingForAHuman(ctx context.Context, what string) error {
	p, ok := principal.Actor(ctx)
	if !ok {
		return fmt.Errorf("no actor is bound to this call: %w", apperrors.ErrPermissionDenied)
	}
	switch p.Type {
	case principal.PrincipalHuman:
		return nil
	case principal.PrincipalAgent:
		if p.OnBehalfOf.IsZero() {
			return fmt.Errorf("this credential names no human it acts for, so it does not do this: %w",
				apperrors.ErrPermissionDenied)
		}
		return nil
	default:
		return fmt.Errorf("%s by a contact, not by a %s principal: %w", what, p.Type, apperrors.ErrPermissionDenied)
	}
}
