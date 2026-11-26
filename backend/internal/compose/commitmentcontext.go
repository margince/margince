// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// derefSeat is the seat or the zero id, which no private conversation names.
func derefSeat(seat *ids.UUID) ids.UUID {
	if seat == nil {
		return ids.UUID{}
	}
	return *seat
}

// Only system extraction changes its audit identity; human callers keep theirs.
func extractorContext(ctx context.Context, extractor string) context.Context {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalSystem {
		return ctx
	}
	actor.ID = extractor
	return principal.WithActor(ctx, actor)
}
