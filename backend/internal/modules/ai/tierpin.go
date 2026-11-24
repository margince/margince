// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "context"

type tierPinKey struct{}

// WithTierPin makes Router.Complete serve the call from that one rung of the
// task's ladder. It rides the context, not model.Request, so a unit's tier
// choice is not part of the shape every site's check is certified against.
func WithTierPin(ctx context.Context, tier string) context.Context {
	if tier == "" {
		return ctx
	}
	return context.WithValue(ctx, tierPinKey{}, tier)
}

func tierPinOf(ctx context.Context) string {
	tier, _ := ctx.Value(tierPinKey{}).(string)
	return tier
}
