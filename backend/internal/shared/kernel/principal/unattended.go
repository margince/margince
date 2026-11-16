// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package principal

import "context"

type unattendedKey struct{}

// WithUnattended marks work with no person present to authorise its sends.
func WithUnattended(ctx context.Context) context.Context {
	return context.WithValue(ctx, unattendedKey{}, true)
}

func Unattended(ctx context.Context) bool {
	value, ok := ctx.Value(unattendedKey{}).(bool)
	_, running := AgentRunID(ctx)
	return running || ok && value
}
