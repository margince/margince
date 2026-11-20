// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
)

// RequireOneOf admits when the actor holds any one of checks, for a read that
// several unrelated grants each open, such as the system health page.
func RequireOneOf(ctx context.Context, checks ...Check) error {
	p, err := rbacActor(ctx)
	if err != nil {
		return err
	}
	c := Check{OneOf: checks}
	noteCheck(ctx, c)
	if err := refuseBuyer(p); err != nil {
		return refusedOn(ctx, c, err)
	}
	if c.HeldBy(p) {
		return nil
	}
	return refusedOn(ctx, c, refuseGrant(c))
}
