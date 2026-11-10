// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// mayWriteTheCard is the lane rule every read shares, on a card kept per user.
// A user's agent keeps its user's card, and automatic work has no user's card.
func mayWriteTheCard(ctx context.Context) bool {
	actor, ok := principal.Actor(ctx)
	if !ok || !actor.HumanOrItsAgent() {
		return false
	}
	_, withheld := ai.ReadLane(ctx, struct{}{})
	return !withheld
}
