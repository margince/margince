// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RequireSuggestionDecision gives a covered review the same authority as its owning action.
func RequireSuggestionDecision(ctx context.Context, linkExisting bool) error {
	if linkExisting {
		return auth.Require(ctx, "deal", principal.ActionUpdate)
	}
	return auth.Require(ctx, "deal", principal.ActionCreate)
}
