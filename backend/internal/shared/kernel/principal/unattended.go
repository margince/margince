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

// ConversationUser is the user of an OAuth-connected credential, or zero.
// Chat clients and automatic clients on that credential have the same answer.
// A minted passport has no conversation; the built-in runner is also excluded.
func ConversationUser(ctx context.Context, p Principal) ids.UUID {
	connected := p.Type == PrincipalAgent && !p.PassportID.IsZero() && !p.ConnectionID.IsZero()
	if Unattended(ctx) || !connected || p.OnBehalfOf.IsZero() {
		return ids.Nil
	}
	return p.OnBehalfOf
}
