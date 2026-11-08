// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ReadLane is the lane a read that writes on a cache miss may use. AI work an
// agent starts spends the enrich scope, so an agent without it gets no lane and
// reads the deterministic floor. withheld tells the caller not to cache that
// floor, because a later reader who may spend would then never get the model.
func ReadLane[L any](ctx context.Context, lane L) (usable L, withheld bool) {
	if p, ok := principal.Actor(ctx); ok && p.Type == principal.PrincipalAgent && !p.Scopes.Has(principal.ScopeEnrich) {
		var none L
		return none, true
	}
	return lane, false
}
