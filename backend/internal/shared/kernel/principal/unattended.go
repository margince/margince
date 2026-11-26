// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package principal

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type unattendedKey struct{}

// WithUnattended marks work with no human present to authorise its sends.
func WithUnattended(ctx context.Context) context.Context {
	return context.WithValue(ctx, unattendedKey{}, struct{}{})
}

// Unattended reports work nobody is watching. A run ID marks a scheduled run;
// the flag marks the runner even when no run row was written.
func Unattended(ctx context.Context) bool {
	if _, scheduled := AgentRunID(ctx); scheduled {
		return true
	}
	return ctx.Value(unattendedKey{}) != nil
}

// ConversationUser is the colleague whose live conversation this call comes
// from, or zero. That is an AI client connected on their own passport, outside
// an unattended run. A passport minted without a connection is unattended.
func ConversationUser(ctx context.Context, p Principal) ids.UUID {
	connected := p.Type == PrincipalAgent && !p.PassportID.IsZero() && !p.ConnectionID.IsZero()
	if Unattended(ctx) || !connected || p.OnBehalfOf.IsZero() {
		return ids.Nil
	}
	return p.OnBehalfOf
}
